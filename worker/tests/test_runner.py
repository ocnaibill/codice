"""Tests for the job runner, with a fake queue client. The queue's own rules
(retries, leases, priorities) are tested in Go against PostgreSQL; here we test
that the worker reports outcomes correctly and stops when it should."""
import threading
import time

import pytest

from runner import JobRunner, Cancelled, LeaseLost, classify, describe, new_owner_name


class FakeJobs:
    def __init__(self, job=None, fail_outcome='failed', complete_ok=True, heartbeats=None):
        self._job = job
        self.fail_outcome = fail_outcome
        self.complete_ok = complete_ok
        self.heartbeats = list(heartbeats or [])
        self.calls = []

    def claim(self):
        job, self._job = self._job, None
        return job

    def heartbeat(self, job_id):
        self.calls.append(('heartbeat', job_id))
        if self.heartbeats:
            value = self.heartbeats.pop(0)
            if isinstance(value, Exception):
                raise value
            return value
        return 'ok'

    def complete(self, job_id):
        self.calls.append(('complete', job_id))
        return self.complete_ok

    def fail(self, job_id, kind, message):
        self.calls.append(('fail', job_id, kind, message))
        if isinstance(self.fail_outcome, Exception):
            raise self.fail_outcome
        return self.fail_outcome

    def cancel_ack(self, job_id):
        self.calls.append(('cancel_ack', job_id))
        return True

    def names(self):
        return [c[0] for c in self.calls if c[0] != 'heartbeat']


JOB = {'id': 5, 'work_id': 9, 'payload': {'file_path': '/f/a.epub'}, 'attempts': 1, 'max_attempts': 3}


def runner_for(jobs, process, **kw):
    seen = {'start': [], 'success': [], 'failure': [], 'retry': []}
    runner = JobRunner(
        jobs, process,
        on_start=lambda j: seen['start'].append(j['id']),
        on_success=lambda j, r: seen['success'].append(r),
        on_failure=lambda j, kind, msg: seen['failure'].append((kind, msg)),
        on_retry=lambda j, msg: seen['retry'].append(msg),
        heartbeat_every=kw.get('heartbeat_every', 30.0))
    return runner, seen


class TestOutcomes:
    def test_no_job_means_nothing_to_do(self):
        runner, seen = runner_for(FakeJobs(job=None), lambda j, c: 1)
        assert runner.run_one() is False
        assert seen['start'] == []

    def test_success_completes_the_job_and_then_reports_it(self):
        jobs = FakeJobs(dict(JOB))
        runner, seen = runner_for(jobs, lambda j, c: 'metadata')
        assert runner.run_one() is True
        assert jobs.names() == ['complete']
        assert seen['start'] == [5] and seen['success'] == ['metadata'] and seen['failure'] == []

    def test_a_result_is_not_published_when_the_lease_was_lost_at_the_end(self):
        jobs = FakeJobs(dict(JOB), complete_ok=False)
        runner, seen = runner_for(jobs, lambda j, c: 'metadata')
        runner.run_one()
        assert seen['success'] == []  # someone else owns the job now

    def test_a_permanent_error_is_reported_as_permanent_and_final(self):
        jobs = FakeJobs(dict(JOB), fail_outcome='failed')

        def process(job, checkpoint):
            raise ValueError("not a valid EPUB")

        runner, seen = runner_for(jobs, process)
        runner.run_one()
        (call,) = [c for c in jobs.calls if c[0] == 'fail']
        assert call[2] == 'permanent' and call[3] == 'ValueError: not a valid EPUB'
        assert seen['failure'] == [('permanent', 'ValueError: not a valid EPUB')] and seen['retry'] == []

    def test_a_temporary_error_is_retried_not_announced_as_failure(self):
        jobs = FakeJobs(dict(JOB), fail_outcome='retry')

        def process(job, checkpoint):
            raise ConnectionError("database restarted")

        runner, seen = runner_for(jobs, process)
        runner.run_one()
        (call,) = [c for c in jobs.calls if c[0] == 'fail']
        assert call[2] == 'temporary'
        assert seen['retry'] == ['ConnectionError: database restarted']
        assert seen['failure'] == []  # the UI is not told "error" while a retry is coming

    def test_a_temporary_error_with_no_attempts_left_becomes_a_failure(self):
        jobs = FakeJobs(dict(JOB, attempts=3), fail_outcome='failed')

        def process(job, checkpoint):
            raise TimeoutError("provider timed out")

        runner, seen = runner_for(jobs, process)
        runner.run_one()
        assert seen['failure'] == [('temporary', 'TimeoutError: provider timed out')]

    def test_failing_to_report_does_not_crash_the_worker(self):
        jobs = FakeJobs(dict(JOB), fail_outcome=ConnectionError("db gone"))

        def process(job, checkpoint):
            raise RuntimeError("boom")

        runner, seen = runner_for(jobs, process)
        assert runner.run_one() is True  # the lease will expire and the job is taken over
        assert seen['failure'] == [] and seen['retry'] == []


class TestCooperativeStop:
    def test_a_cancel_request_stops_the_job_at_a_checkpoint_without_publishing(self):
        jobs = FakeJobs(dict(JOB), heartbeats=['cancel'])
        reached = []

        def process(job, checkpoint):
            deadline = time.time() + 3
            while time.time() < deadline:
                checkpoint()
                time.sleep(0.01)
            reached.append('finished')  # must not be reached
            return 'x'

        runner, seen = runner_for(jobs, process, heartbeat_every=0.02)
        runner.run_one()
        assert reached == []
        assert jobs.names() == ['cancel_ack']
        assert seen['success'] == [] and seen['failure'] == []

    def test_losing_the_lease_stops_the_job_and_writes_nothing(self):
        jobs = FakeJobs(dict(JOB), heartbeats=['lost'])

        def process(job, checkpoint):
            deadline = time.time() + 3
            while time.time() < deadline:
                checkpoint()
                time.sleep(0.01)
            return 'x'

        runner, seen = runner_for(jobs, process, heartbeat_every=0.02)
        runner.run_one()
        # No completion, no failure, no acknowledgement: the new owner decides the outcome.
        assert jobs.names() == []
        assert seen['success'] == [] and seen['failure'] == [] and seen['retry'] == []

    def test_a_failed_heartbeat_does_not_abort_the_work(self):
        jobs = FakeJobs(dict(JOB), heartbeats=[ConnectionError("blip"), 'ok'])

        def process(job, checkpoint):
            time.sleep(0.1)
            checkpoint()
            return 'done'

        runner, seen = runner_for(jobs, process, heartbeat_every=0.02)
        runner.run_one()
        assert seen['success'] == ['done']

    def test_heartbeats_stop_when_the_job_ends(self):
        jobs = FakeJobs(dict(JOB))
        runner, _ = runner_for(jobs, lambda j, c: 1, heartbeat_every=0.02)
        runner.run_one()
        before = len([c for c in jobs.calls if c[0] == 'heartbeat'])
        time.sleep(0.1)
        assert len([c for c in jobs.calls if c[0] == 'heartbeat']) == before


class TestClassification:
    def test_files_that_will_never_work_are_permanent(self):
        for err in (ValueError("x"), FileNotFoundError("x"), UnicodeDecodeError("utf-8", b"", 0, 1, "x"), NotImplementedError()):
            assert classify(err) == 'permanent'

    def test_infrastructure_hiccups_are_temporary(self):
        for err in (ConnectionError("x"), TimeoutError("x"), OSError("x"), RuntimeError("x")):
            assert classify(err) == 'temporary'

    def test_the_description_is_short_and_has_no_traceback(self):
        text = describe(ValueError("a" * 2000))
        assert text.startswith("ValueError: aaa") and len(text) == 500 and "Traceback" not in text

    def test_each_process_gets_its_own_name(self):
        assert new_owner_name() != new_owner_name()

    def test_control_flow_exceptions_are_not_ordinary_errors(self):
        assert not issubclass(Cancelled, ValueError) and not issubclass(LeaseLost, ValueError)


class TestJobsClientClaim:
    """The worker must ask the queue for its own job types only."""

    class RecordingDB:
        def __init__(self):
            self.calls = []

        def fetchone(self, query, params=()):
            self.calls.append((" ".join(query.split()), params))
            return None

    def test_it_asks_only_for_ingestion_jobs(self):
        from runner import JobsClient

        db = self.RecordingDB()
        JobsClient(db, "worker-1", lease_seconds=60, max_running=2).claim()
        query, params = db.calls[0]
        assert "jobs_claim(%s, %s, %s, %s::text[])" in query
        assert params == ("worker-1", 60, 2, ['ingest', 'extract_text'])

    def test_the_types_can_be_narrowed_or_widened_explicitly(self):
        from runner import JobsClient

        db = self.RecordingDB()
        JobsClient(db, "w", types=['ingest', 'ocr']).claim()
        assert db.calls[0][1][3] == ['ingest', 'ocr']
