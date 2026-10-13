"""The worker's heartbeat, and the check a container runs against it.

The worker has no HTTP port, so it says it is well by writing a small file. It writes
only after it has talked to the job queue successfully: while idle, on every poll, and
while it runs a job, on every lease heartbeat. A worker that is running but cannot reach
the database stops writing, and the check turns unhealthy. What it does NOT detect is a
job that hangs while the worker keeps heartbeating it.

    python health.py        exits 0 when the last heartbeat is recent enough, 1 otherwise
"""
import json
import os
import sys
import time

DEFAULT_FILE = "/tmp/codice-worker-health"


def health_file():
    return os.getenv("WORKER_HEALTH_FILE") or DEFAULT_FILE


def _number(name, default):
    """An environment number; unset or empty means the default (compose passes empty values)."""
    raw = os.getenv(name)
    if raw is None or raw.strip() == "":
        return default
    return float(raw)


def max_age():
    """How old a heartbeat may be. It must outlast the slowest thing the worker does between
    two heartbeats: a poll while idle, or a lease heartbeat while it runs a job."""
    explicit = _number("WORKER_HEALTH_MAX_AGE", 0)
    if explicit > 0:
        return explicit
    poll = _number("WORKER_POLL_SECONDS", 5)
    lease = _number("JOB_LEASE_SECONDS", 120)
    return max(90.0, 3 * max(poll, lease / 4))


class Heartbeat:
    """Writes the heartbeat file. Failing to write never stops the worker."""

    def __init__(self, path=None, clock=time.time, on_beat=None):
        self.path = path or health_file()
        self.clock = clock
        self.on_beat = on_beat

    def beat(self, state="idle", job=None):
        body = {"at": self.clock(), "state": state, "pid": os.getpid()}
        if job is not None:
            body["job"] = job
        tmp = f"{self.path}.{os.getpid()}.tmp"
        try:
            with open(tmp, "w") as f:
                json.dump(body, f)
            os.replace(tmp, self.path)  # a reader never sees a half-written file
        except OSError as err:
            print(f"   ⚠️ could not write the health file: {err}")
        if self.on_beat is not None:
            try:
                self.on_beat(state, job)
            except Exception as err:  # telling the administration is never a reason to stop working
                print(f"   ⚠️ could not tell the administration that the worker is here: {err}")


class Presence:
    """Tells the administration, in the database, that this worker is here (DEC-176): the file is for the container's check, this is for the
    "Serviços" of the system tab. It writes at most once in `every` seconds, so a worker that beats on every poll does not write on every poll."""

    def __init__(self, db, key, every=30.0, clock=time.time):
        self.db, self.key, self.every, self.clock = db, key, every, clock
        self.last = None

    def __call__(self, state="idle", job=None):
        now = self.clock()
        if self.last is not None and now - self.last < self.every:
            return
        self.last = now
        self.db.execute(
            """INSERT INTO settings (key, value) VALUES (%s, %s::jsonb)
               ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()""",
            (self.key, json.dumps({"state": state, "pid": os.getpid()})))


def check(path=None, limit=None, now=None):
    """Returns (healthy, message)."""
    path = path or health_file()
    limit = limit if limit is not None else max_age()
    now = time.time() if now is None else now
    try:
        with open(path) as f:
            body = json.load(f)
        at = float(body["at"])
    except FileNotFoundError:
        return False, "no heartbeat yet"
    except (ValueError, KeyError, TypeError, OSError):
        return False, "the heartbeat file is unreadable"
    age = now - at
    if age < -60:  # a timestamp from the future: do not trust it
        return False, "the heartbeat is from the future"
    if age > limit:
        return False, f"the last heartbeat was {int(age)}s ago (limit {int(limit)}s), state {body.get('state')}"
    return True, f"ok: {body.get('state')}, {int(max(age, 0))}s ago"


if __name__ == "__main__":
    healthy, message = check()
    print(message)
    sys.exit(0 if healthy else 1)
