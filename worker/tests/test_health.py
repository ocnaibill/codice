"""The worker's heartbeat and its health check."""
import json
import os
import subprocess
import sys

import pytest

import health
from health import Heartbeat, check
from runner import JobRunner, poll_once
from tests.test_runner import FakeJobs, JOB


class Clock:
    def __init__(self, now=1_000_000.0):
        self.now = now

    def __call__(self):
        return self.now


@pytest.fixture(autouse=True)
def clean_env(monkeypatch):
    for name in ("WORKER_HEALTH_FILE", "WORKER_HEALTH_MAX_AGE", "WORKER_POLL_SECONDS", "JOB_LEASE_SECONDS"):
        monkeypatch.delenv(name, raising=False)


class TestCheck:
    def test_a_fresh_heartbeat_is_healthy(self, tmp_path):
        clock = Clock()
        hb = Heartbeat(str(tmp_path / "hb"), clock)
        hb.beat("idle")
        ok, msg = check(str(tmp_path / "hb"), 90, now=clock.now + 10)
        assert ok and "idle" in msg

    def test_a_stale_heartbeat_is_not(self, tmp_path):
        clock = Clock()
        Heartbeat(str(tmp_path / "hb"), clock).beat("working", job=7)
        ok, msg = check(str(tmp_path / "hb"), 90, now=clock.now + 200)
        assert not ok and "200s ago" in msg and "working" in msg

    def test_no_heartbeat_yet_is_not_healthy(self, tmp_path):
        ok, msg = check(str(tmp_path / "none"), 90)
        assert not ok and "no heartbeat" in msg

    @pytest.mark.parametrize("content", ["", "not json", "[]", '{"at": "soon"}', '{"state": "idle"}', "null"])
    def test_an_unreadable_file_is_not_healthy(self, tmp_path, content):
        p = tmp_path / "hb"
        p.write_text(content)
        ok, msg = check(str(p), 90)
        assert not ok and "unreadable" in msg

    def test_a_timestamp_from_the_future_is_not_trusted(self, tmp_path):
        clock = Clock(now=5_000_000.0)
        Heartbeat(str(tmp_path / "hb"), clock).beat()
        ok, msg = check(str(tmp_path / "hb"), 90, now=1_000_000.0)
        assert not ok and "future" in msg

    def test_small_clock_skew_is_tolerated(self, tmp_path):
        clock = Clock(now=1_000_030.0)
        Heartbeat(str(tmp_path / "hb"), clock).beat()
        ok, _ = check(str(tmp_path / "hb"), 90, now=1_000_000.0)
        assert ok


class TestHeartbeat:
    def test_writes_atomically_and_leaves_nothing_behind(self, tmp_path):
        hb = Heartbeat(str(tmp_path / "hb"))
        hb.beat("idle")
        hb.beat("working", job=3)
        assert sorted(os.listdir(tmp_path)) == ["hb"]
        body = json.loads((tmp_path / "hb").read_text())
        assert body["state"] == "working" and body["job"] == 3 and body["pid"] == os.getpid()

    def test_failing_to_write_never_stops_the_worker(self, tmp_path):
        Heartbeat(str(tmp_path / "no" / "such" / "dir" / "hb")).beat("idle")  # must not raise

    def test_the_path_comes_from_the_environment(self, tmp_path, monkeypatch):
        monkeypatch.setenv("WORKER_HEALTH_FILE", str(tmp_path / "custom"))
        Heartbeat().beat("idle")
        assert (tmp_path / "custom").exists()


class TestMaxAge:
    def test_default_outlasts_the_slowest_beat(self):
        assert health.max_age() == 90.0  # poll 5 s, lease 120 s => beats every 30 s

    def test_it_grows_with_a_longer_lease(self, monkeypatch):
        monkeypatch.setenv("JOB_LEASE_SECONDS", "1200")  # beats every 300 s
        assert health.max_age() == 900.0

    def test_it_grows_with_a_longer_poll(self, monkeypatch):
        monkeypatch.setenv("WORKER_POLL_SECONDS", "60")
        assert health.max_age() == 180.0

    def test_it_can_be_set_and_empty_means_unset(self, monkeypatch):
        monkeypatch.setenv("WORKER_HEALTH_MAX_AGE", "15")
        assert health.max_age() == 15.0
        monkeypatch.setenv("WORKER_HEALTH_MAX_AGE", "")  # compose passes an empty value when unset
        assert health.max_age() == 90.0


class TestCommand:
    def run(self, tmp_path, env=None):
        e = dict(os.environ, WORKER_HEALTH_FILE=str(tmp_path / "hb"), **(env or {}))
        return subprocess.run([sys.executable, "health.py"], capture_output=True, text=True, env=e,
                              cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

    def test_exit_code_follows_the_heartbeat(self, tmp_path):
        assert self.run(tmp_path).returncode == 1  # nothing yet
        Heartbeat(str(tmp_path / "hb")).beat("idle")
        done = self.run(tmp_path)
        assert done.returncode == 0 and "ok" in done.stdout
        old = tmp_path / "hb"
        old.write_text(json.dumps({"at": 1.0, "state": "idle"}))
        assert self.run(tmp_path).returncode == 1


class TestLoop:
    def beats(self, tmp_path, jobs, process=lambda j, c: 1, **kw):
        runner = JobRunner(jobs, process, **kw)
        hb = Heartbeat(str(tmp_path / "hb"))
        return runner, hb

    def test_only_a_successful_turn_beats(self, tmp_path):
        class Broken(FakeJobs):
            def claim(self):
                raise RuntimeError("the database is down")

        runner, hb = self.beats(tmp_path, Broken())
        assert poll_once(runner, hb) == "error"
        assert not (tmp_path / "hb").exists(), "a worker that cannot reach the queue must not look alive"

    def test_an_idle_turn_beats(self, tmp_path):
        runner, hb = self.beats(tmp_path, FakeJobs(job=None))
        assert poll_once(runner, hb) == "idle"
        assert json.loads((tmp_path / "hb").read_text())["state"] == "idle"

    def test_a_turn_that_ran_a_job_beats(self, tmp_path):
        runner, hb = self.beats(tmp_path, FakeJobs(job=dict(JOB)))
        assert poll_once(runner, hb) == "worked"
        assert (tmp_path / "hb").exists()

    def test_a_running_job_keeps_beating_through_its_lease_heartbeats(self, tmp_path):
        import time as _time
        seen = []
        hb = Heartbeat(str(tmp_path / "hb"))

        def process(job, checkpoint):
            _time.sleep(0.15)  # long enough for several lease heartbeats
            return 1

        runner = JobRunner(FakeJobs(job=dict(JOB)), process, heartbeat_every=0.02,
                           on_heartbeat=lambda j: (seen.append(j['id']), hb.beat("working", j['id'])))
        runner.run_one()
        assert len(seen) >= 2 and set(seen) == {JOB['id']}
        assert json.loads((tmp_path / "hb").read_text())["state"] == "working"

    def test_a_failed_lease_heartbeat_does_not_count_as_alive(self, tmp_path):
        import time as _time
        beats = []

        def process(job, checkpoint):
            _time.sleep(0.12)
            return 1

        jobs = FakeJobs(job=dict(JOB), heartbeats=[RuntimeError("blip")] * 50)
        runner = JobRunner(jobs, process, heartbeat_every=0.02, on_heartbeat=lambda j: beats.append(1))
        runner.run_one()
        assert beats == [], "a heartbeat that failed must not be reported as proof of life"
