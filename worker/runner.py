"""Runs jobs taken from the PostgreSQL queue.

The queue's rules (who may take a job, leases, retries and their waiting times,
priorities, the limit on simultaneous jobs) live in SQL functions, migration
00005. This module only calls them: claim a job, keep its lease alive while it
runs, then report success or the kind of failure. Redis is not needed for
correctness; it only wakes the worker earlier than its next poll.
"""
import os
import socket
import threading
import uuid

# Errors that will fail the same way every time: the file is not what it says,
# is missing or unsupported. Retrying them only wastes time, so they are marked
# permanent (DEC-068). Anything else (a database or network hiccup, a timeout)
# is temporary and retried with a growing wait.
PERMANENT_ERRORS = (ValueError, FileNotFoundError, PermissionError, UnicodeError, NotImplementedError)


class Cancelled(Exception):
    """An admin asked to stop this job; it finishes without publishing anything."""


class LeaseLost(Exception):
    """Another worker took the job over; this one must stop and write nothing."""


def classify(err: Exception) -> str:
    return 'permanent' if isinstance(err, PERMANENT_ERRORS) else 'temporary'


def describe(err: Exception) -> str:
    """A short, readable reason: the error class and message, never a traceback."""
    return f"{type(err).__name__}: {err}"[:500]


def new_owner_name() -> str:
    """Unique per process, so a restarted worker is never mistaken for the old one."""
    return f"{socket.gethostname()}-{os.getpid()}-{uuid.uuid4().hex[:8]}"


class JobsClient:
    """Thin wrapper over the queue functions in PostgreSQL."""

    def __init__(self, db, owner: str, lease_seconds: int = 120, max_running: int = 1):
        self.db = db
        self.owner = owner
        self.lease_seconds = lease_seconds
        self.max_running = max_running

    def claim(self):
        row = self.db.fetchone(
            "SELECT id, work_id, payload, attempts, max_attempts FROM jobs_claim(%s, %s, %s)",
            (self.owner, self.lease_seconds, self.max_running))
        if not row or row[0] is None:
            return None
        return {'id': row[0], 'work_id': row[1], 'payload': row[2] or {}, 'attempts': row[3], 'max_attempts': row[4]}

    def heartbeat(self, job_id) -> str:
        return self.db.fetchone("SELECT jobs_heartbeat(%s, %s, %s)", (job_id, self.owner, self.lease_seconds))[0]

    def complete(self, job_id) -> bool:
        return bool(self.db.fetchone("SELECT jobs_complete(%s, %s)", (job_id, self.owner))[0])

    def fail(self, job_id, kind: str, message: str) -> str:
        return self.db.fetchone("SELECT jobs_fail(%s, %s, %s, %s)", (job_id, self.owner, kind, message))[0]

    def cancel_ack(self, job_id) -> bool:
        return bool(self.db.fetchone("SELECT jobs_cancel_ack(%s, %s)", (job_id, self.owner))[0])


class JobRunner:
    """Takes one job at a time and carries it to a definite outcome.

    process(job, checkpoint) does the work. It must call checkpoint() between
    steps: that raises Cancelled or LeaseLost when the job should stop, so the
    work ends at a safe point instead of being cut in the middle of a write.
    """

    def __init__(self, jobs: JobsClient, process, on_start=None, on_success=None, on_failure=None,
                 on_retry=None, heartbeat_every: float = 30.0):
        self.jobs = jobs
        self.process = process
        self.on_start = on_start or (lambda job: None)
        self.on_success = on_success or (lambda job, result: None)
        self.on_failure = on_failure or (lambda job, kind, message: None)
        self.on_retry = on_retry or (lambda job, message: None)
        self.heartbeat_every = heartbeat_every

    def run_one(self) -> bool:
        """Runs one job if there is one. Returns whether a job was taken."""
        job = self.jobs.claim()
        if job is None:
            return False

        stop_cancel = threading.Event()
        stop_lost = threading.Event()
        done = threading.Event()

        def beat():
            while not done.wait(self.heartbeat_every):
                try:
                    status = self.jobs.heartbeat(job['id'])
                except Exception as err:  # a database blip: the next beat tries again
                    print(f"   ⚠️ heartbeat failed: {err}")
                    continue
                if status == 'cancel':
                    stop_cancel.set()
                elif status == 'lost':
                    stop_lost.set()
                    return

        def checkpoint():
            if stop_lost.is_set():
                raise LeaseLost()
            if stop_cancel.is_set():
                raise Cancelled()

        heart = threading.Thread(target=beat, daemon=True)
        heart.start()
        try:
            self.on_start(job)
            result = self.process(job, checkpoint)
            checkpoint()
            if self.jobs.complete(job['id']):
                self.on_success(job, result)
            else:
                print(f"   ⚠️ job {job['id']}: the lease was lost before completion; the result is ignored")
        except Cancelled:
            self.jobs.cancel_ack(job['id'])
            print(f"   🛑 job {job['id']} cancelled")
        except LeaseLost:
            print(f"   ⚠️ job {job['id']} was taken over by another worker; stopping")
        except Exception as err:
            kind, message = classify(err), describe(err)
            print(f"   ❌ job {job['id']} failed ({kind}): {message}")
            try:
                outcome = self.jobs.fail(job['id'], kind, message)
            except Exception as report_err:
                # Cannot even report: the lease will expire and the job is taken over.
                print(f"   ⚠️ could not report the failure ({report_err}); the lease will expire")
                outcome = 'lost'
            if outcome == 'failed':
                self.on_failure(job, kind, message)
            elif outcome == 'retry':
                self.on_retry(job, message)
        finally:
            done.set()
        return True
