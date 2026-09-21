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
from runner import JobRunner, JobsClient, new_owner_name, poll_once
from health import Heartbeat
from textindex.store import TextIndexer
from embeddings import EmbeddingIndexer, SentenceTransformersProvider

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
    """Keep a reconnectable client even when Redis starts after the worker."""
    client = None
    try:
        client = redis.from_url(
            REDIS_URL, decode_responses=True, socket_timeout=10.0, socket_connect_timeout=5.0,
            socket_keepalive=True, retry_on_timeout=True)
        client.ping()
        print("✅ Connected to Redis (used only to wake the worker up)")
        return client
    except Exception as err:
        print(f"⚠️ Redis unavailable ({err}); polling the database instead")
        return client


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


def build_runner(db, client, heartbeat=None):
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

    indexer = TextIndexer(db, storage_path)
    embeddings = None
    if os.getenv('EMBEDDINGS_PROVIDER', '').lower() in ('labse', 'sentence-transformers'):
        embeddings = EmbeddingIndexer(db, SentenceTransformersProvider())
        embeddings.enqueue_missing()

    def process(job, checkpoint):
        if job.get('type') == 'extract_text':
            # Reading the text is a job of its own: it never changes the status of the work, so a work
            # that can be read stays readable while its text is being extracted (or if that fails).
            print(f"\n📚 Text job {job['id']} (work {job['work_id']}, attempt {job['attempts']}/{job['max_attempts']})")
            outcome = indexer.run(job['work_id'], force=bool(job['payload'].get('force')), checkpoint=checkpoint)
            print(f"   📝 {outcome or 'nothing to read'}")
            return outcome
        if job.get('type') == 'embed_text':
            print(f"\n🧭 Embedding job {job['id']} (work {job['work_id']}, attempt {job['attempts']}/{job['max_attempts']})")
            outcome = embeddings.run(job['work_id'], checkpoint=checkpoint) if embeddings else {}
            print(f"   🧠 {outcome or 'nothing to embed'}")
            return outcome
        file_path = job['payload'].get('file_path')
        print(f"\n📥 Job {job['id']} (work {job['work_id']}, attempt {job['attempts']}/{job['max_attempts']})")
        print(f"   File: {file_path}")
        ensure_file(file_path)
        extractor = find_extractor(extractors, file_path)
        print(f"   🔍 Using extractor: {extractor.__class__.__name__}")
        checkpoint()
        return analyze_file(job['work_id'], file_path, extractor, analyzer, provider_registry, covers_dir, checkpoint)

    def is_ingest(job):
        return job.get('type', 'ingest') == 'ingest'

    def on_start(job):
        if not is_ingest(job):
            return
        analyzer.update_status(job['work_id'], MediaStatus.ANALYZING)
        publish(client, {"type": "WORK_ANALYZING", "work_id": job['work_id']})

    def on_success(job, metadata):
        if not is_ingest(job):
            print(f"✅ Text job {job['id']} completed.")
            return
        analyzer.update_status(job['work_id'], MediaStatus.READY)
        publish(client, {"type": "WORK_READY", "work_id": job['work_id'], "title": metadata.title})
        print(f"✅ Job {job['id']} completed.")

    def on_failure(job, kind, message):
        if not is_ingest(job):
            print(f"   ❌ Text job {job['id']} failed: {message}")
            return
        analyzer.update_status(job['work_id'], MediaStatus.ERROR, message)
        publish(client, {"type": "WORK_ERROR", "work_id": job['work_id'], "error": message})

    def on_retry(job, message):
        if not is_ingest(job):
            print(f"   🔁 Text job will retry: {message}")
            return
        # Not final: the job waits and runs again, so the work goes back to queued.
        analyzer.update_status(job['work_id'], MediaStatus.QUEUED)
        print(f"   🔁 Will retry: {message}")

    owner = new_owner_name()
    print(f"🆔 Worker {owner}")
    jobs = JobsClient(db, owner, lease_seconds=LEASE_SECONDS, max_running=MAX_RUNNING)
    runner = JobRunner(jobs, process, on_start=on_start, on_success=on_success, on_failure=on_failure,
                     on_retry=on_retry, heartbeat_every=max(1.0, LEASE_SECONDS / 4),
                     on_heartbeat=(lambda job: heartbeat.beat("working", job['id'])) if heartbeat else None)
    runner.embeddings = embeddings
    return runner


def listen_for_tasks():
    db = CodiceDatabase()
    client = connect_redis()
    heartbeat = Heartbeat()
    runner = build_runner(db, client, heartbeat)
    heartbeat.beat("starting")
    last_id = '$'

    print("⏳ Python Worker waiting for jobs...")
    while True:
        outcome = poll_once(runner, heartbeat)
        if outcome == "worked":
            continue  # more may be waiting: look again before sleeping
        if outcome == "error":
            # The database itself is unreachable: nothing to do but wait for it.
            time.sleep(POLL_SECONDS)
            continue
        if runner.embeddings is not None:
            runner.embeddings.enqueue_missing()
        last_id = wait_for_work(client, last_id)


if __name__ == "__main__":
    listen_for_tasks()
