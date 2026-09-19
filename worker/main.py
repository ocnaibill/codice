import os
import sys
import time
import json
import redis
from dotenv import load_dotenv

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

from extractors import EpubExtractor, PdfExtractor, CbzExtractor, CbrExtractor, TxtExtractor, AudiobookExtractor, MobiExtractor
from extractors.base import BaseExtractor
from providers import ProviderRegistry
from db import CodiceDatabase
from analyzer import Analyzer, MediaStatus
from pipeline import analyze_file, ensure_file
from runner import JobRunner, JobsClient, new_owner_name

# 1. Loads variables from .env, trying multiple locations
env_paths = ["../.env", ".env"]
loaded = False
for p in env_paths:
    if load_dotenv(dotenv_path=p):
        print(f"Config loaded from: {p}")
        loaded = True
        break
if not loaded:
    print("Warning: No .env file found. Using system environment variables.")

# 2. Redis is optional: it only wakes the worker up sooner than its next poll.
REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379/0")
WAKEUP_STREAM = "codice_jobs_wakeup"
EVENTS_CHANNEL = "codice_updates"

POLL_SECONDS = float(os.getenv("WORKER_POLL_SECONDS", "5"))
LEASE_SECONDS = int(os.getenv("JOB_LEASE_SECONDS", "120"))
# One heavy job at a time by default (DEC-069); the owner can raise it.
MAX_RUNNING = int(os.getenv("JOBS_MAX_CONCURRENT", "1"))


def connect_redis():
    """Returns a Redis client, or None: the worker works without it."""
    try:
        client = redis.from_url(
            REDIS_URL, decode_responses=True, socket_timeout=10.0, socket_connect_timeout=5.0,
            socket_keepalive=True, retry_on_timeout=True)
        client.ping()
        print("✅ Connected to Redis (used only to wake the worker up)")
        return client
    except Exception as err:
        print(f"⚠️ Redis unavailable ({err}); polling the database instead")
        return None


def publish(client, event: dict):
    """Broadcast a UI event. Failing to do so never affects a job."""
    if client is None:
        return
    try:
        client.publish(EVENTS_CHANNEL, json.dumps(event))
    except Exception as err:
        print(f"   ⚠️ could not publish event: {err}")


def register_extractors() -> list:
    """Register all available format extractors."""
    return [
        EpubExtractor(),
        PdfExtractor(),
        CbzExtractor(),
        CbrExtractor(),
        TxtExtractor(),
        AudiobookExtractor(),
        MobiExtractor(),
    ]


def find_extractor(extractors: list, file_path: str) -> BaseExtractor:
    """Find the first extractor that can handle the given file."""
    for ext in extractors:
        if ext.can_extract(file_path):
            return ext
    raise ValueError(f"Unsupported format: {file_path}")


def wait_for_work(client, last_id):
    """Sleep until Redis says there may be work, or until the next poll."""
    if client is None:
        time.sleep(POLL_SECONDS)
        return last_id
    try:
        messages = client.xread({WAKEUP_STREAM: last_id}, block=int(POLL_SECONDS * 1000), count=50)
        for _stream, entries in messages or []:
            last_id = entries[-1][0]
    except Exception as err:
        print(f"⚠️ Redis wake-up failed ({err}); falling back to polling")
        time.sleep(POLL_SECONDS)
    return last_id


def build_runner(db, client):
    extractors = register_extractors()
    provider_registry = ProviderRegistry()
    analyzer = Analyzer(db)

    storage_path = os.getenv('CODICE_STORAGE_PATH', './uploads')
    # If relative, resolve from project root (two levels up from worker/)
    if not os.path.isabs(storage_path):
        project_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        storage_path = os.path.join(project_root, storage_path)
    covers_dir = os.path.join(storage_path, 'covers')
    os.makedirs(covers_dir, exist_ok=True)

    def process(job, checkpoint):
        file_path = job['payload'].get('file_path')
        print(f"\n📥 Job {job['id']} (work {job['work_id']}, attempt {job['attempts']}/{job['max_attempts']})")
        print(f"   File: {file_path}")
        ensure_file(file_path)
        extractor = find_extractor(extractors, file_path)
        print(f"   🔍 Using extractor: {extractor.__class__.__name__}")
        checkpoint()
        return analyze_file(job['work_id'], file_path, extractor, analyzer, provider_registry, covers_dir, checkpoint)

    def on_start(job):
        analyzer.update_status(job['work_id'], MediaStatus.ANALYZING)
        publish(client, {"type": "WORK_ANALYZING", "work_id": job['work_id']})

    def on_success(job, metadata):
        analyzer.update_status(job['work_id'], MediaStatus.READY)
        publish(client, {"type": "WORK_READY", "work_id": job['work_id'], "title": metadata.title})
        print(f"✅ Job {job['id']} completed.")

    def on_failure(job, kind, message):
        analyzer.update_status(job['work_id'], MediaStatus.ERROR, message)
        publish(client, {"type": "WORK_ERROR", "work_id": job['work_id'], "error": message})

    def on_retry(job, message):
        # Not final: the job waits and runs again, so the work goes back to queued.
        analyzer.update_status(job['work_id'], MediaStatus.QUEUED)
        print(f"   🔁 Will retry: {message}")

    owner = new_owner_name()
    print(f"🆔 Worker {owner}")
    jobs = JobsClient(db, owner, lease_seconds=LEASE_SECONDS, max_running=MAX_RUNNING)
    return JobRunner(jobs, process, on_start=on_start, on_success=on_success, on_failure=on_failure,
                     on_retry=on_retry, heartbeat_every=max(1.0, LEASE_SECONDS / 4))


def listen_for_tasks():
    db = CodiceDatabase()
    client = connect_redis()
    runner = build_runner(db, client)
    last_id = '$'

    print("⏳ Python Worker waiting for jobs...")
    while True:
        try:
            if runner.run_one():
                continue  # more may be waiting: look again before sleeping
        except Exception as err:
            # The database itself is unreachable: nothing to do but wait for it.
            print(f"⚠️ Could not reach the job queue: {err}")
            time.sleep(POLL_SECONDS)
            continue
        last_id = wait_for_work(client, last_id)


if __name__ == "__main__":
    listen_for_tasks()
