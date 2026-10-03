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
from providers.gate import asks_providers, db_gate, report_keys
from authority import resolve_pending
from maintenance import repair_descriptions
from db import CodiceDatabase
from analyzer import Analyzer, MediaStatus
from pipeline import analyze_file, ensure_file
from runner import JobRunner, JobsClient, new_owner_name, poll_once
from health import Heartbeat
from textindex.store import TextIndexer
from embeddings import EmbeddingIndexer, SentenceTransformersProvider
from ocr import OcrIndexer
from dictionary import DictionaryImporter, connect_from_env

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
    # Only the providers the owner turned on are asked (#68): the title of a work goes to no one else.
    provider_registry = ProviderRegistry(enabled=db_gate(db))
    analyzer = Analyzer(db)

    storage_path = os.getenv('CODICE_STORAGE_PATH', './uploads')
    # If relative, resolve from project root (two levels up from worker/)
    if not os.path.isabs(storage_path):
        project_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        storage_path = os.path.join(project_root, storage_path)
    covers_dir = os.path.join(storage_path, 'covers')
    os.makedirs(covers_dir, exist_ok=True)

    indexer = TextIndexer(db, storage_path)
    # A dictionary the owner asked to install: downloaded from the address in the server's catalog, imported in one
    # transaction. The file is kept only while it is read.
    dictionaries = DictionaryImporter(connect_from_env(), workdir=os.path.join(storage_path, 'tmp'))
    os.makedirs(dictionaries.workdir, exist_ok=True)
    # Only the worker that is given the job type reads by OCR: it is the one the image with the engine is for.
    ocr = OcrIndexer(db, storage_path) if 'ocr' in [t.strip() for t in os.getenv('WORKER_JOB_TYPES', '').split(',')] else None
    embeddings = None
    if os.getenv('EMBEDDINGS_PROVIDER', '').lower() in ('labse', 'sentence-transformers'):
        embeddings = EmbeddingIndexer(db, SentenceTransformersProvider(), selectable=True)
        embeddings.enqueue_missing()

    def process(job, checkpoint):
        if job.get('type') == 'dictionary':
            print(f"\n📖 Dictionary job {job['id']} ({job['payload'].get('package')}, attempt {job['attempts']}/{job['max_attempts']})")
            outcome = dictionaries.run(job, checkpoint)
            print(f"   📖 {outcome}")
            return outcome
        if job.get('type') == 'ocr':
            # Reading the pages of a scan is a job of its own, and slow: what it reads is kept page by page, so a job
            # that is stopped (or a worker that dies) does not lose it. It never changes the status of the work.
            print(f"\n🔤 OCR job {job['id']} (work {job['work_id']}, attempt {job['attempts']}/{job['max_attempts']})")
            outcome = ocr.run(job['work_id'], retry_failed=bool(job['payload'].get('retry_failed')), checkpoint=checkpoint) if ocr else {}
            print(f"   🔤 {outcome or 'nothing to read'}")
            return outcome
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
        if job.get('type') == 'dictionary':
            dictionaries.fail(job, message)
            return
        if not is_ingest(job):
            print(f"   ❌ Text job {job['id']} failed: {message}")
            return
        analyzer.update_status(job['work_id'], MediaStatus.ERROR, message)
        publish(client, {"type": "WORK_ERROR", "work_id": job['work_id'], "error": message})

    def on_retry(job, message):
        if job.get('type') == 'dictionary':
            dictionaries.retrying(job, message)
            return
        if not is_ingest(job):
            print(f"   🔁 Text job will retry: {message}")
            return
        # Not final: the job waits and runs again, so the work goes back to queued.
        analyzer.update_status(job['work_id'], MediaStatus.QUEUED)
        print(f"   🔁 Will retry: {message}")

    owner = new_owner_name()
    print(f"🆔 Worker {owner}")
    jobs = JobsClient(db, owner, lease_seconds=LEASE_SECONDS, max_running=MAX_RUNNING)
    def on_job_heartbeat(job):
        if heartbeat:
            heartbeat.beat("working", job['id'])
        if embeddings is not None:
            embeddings.heartbeat()
        if ocr is not None:
            ocr.heartbeat()
    runner = JobRunner(jobs, process, on_start=on_start, on_success=on_success, on_failure=on_failure,
                     on_retry=on_retry, heartbeat_every=max(1.0, LEASE_SECONDS / 4),
                     on_heartbeat=on_job_heartbeat)
    runner.embeddings = embeddings
    runner.ocr = ocr
    return runner


def resolve_authors(db, allowed):
    """With nothing else to do, look up a few authors whose keys nobody has looked up (only if the owner turned
    Open Library on, and only by the worker that asks the providers). It never gets in the way of the work."""
    if not asks_providers():
        return
    try:
        resolve_pending(db, allowed)
    except Exception as err:
        print(f"   ⚠️ Author lookup failed ({type(err).__name__}: {err})")


def listen_for_tasks():
    db = CodiceDatabase()
    allowed = db_gate(db)
    report_keys(db)  # which API keys this worker has: the administration says so before a provider is turned on
    if asks_providers():
        repair_descriptions(db)  # once, for the descriptions an older version stored with their HTML (#58)
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
        if getattr(runner, 'ocr', None) is not None:
            runner.ocr.enqueue_missing()
        resolve_authors(db, allowed)
        last_id = wait_for_work(client, last_id)


if __name__ == "__main__":
    listen_for_tasks()
