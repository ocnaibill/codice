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

# 2. Configures the Redis connection
REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379/0")

r = redis.from_url(
    REDIS_URL, 
    decode_responses=True,
    socket_timeout=10.0,
    socket_connect_timeout=5.0,
    socket_keepalive=True,
    retry_on_timeout=True
)

STREAM_NAME = 'ingestion_tasks'
GROUP_NAME = 'python_workers'
CONSUMER_NAME = 'worker_1'


def setup_redis_stream():
    """Creates the Consumer Group in Redis, if it does not exist."""
    try:
        r.ping()
        print("✅ Successfully connected to Redis!")
        r.xgroup_create(STREAM_NAME, GROUP_NAME, id='0', mkstream=True)
        print(f"📦 Consumer group '{GROUP_NAME}' configured.")
    except redis.exceptions.ResponseError as e:
        if "BUSYGROUP" not in str(e):
            print(f"❌ Error creating group: {e}")
    except redis.exceptions.ConnectionError:
        print("❌ Could not connect to Redis. Is the container running?")
        exit(1)


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


def listen_for_tasks():
    setup_redis_stream()

    db = CodiceDatabase()
    extractors = register_extractors()
    provider_registry = ProviderRegistry()
    analyzer = Analyzer(db)

    print("⏳ Python Worker waiting for tasks in the queue...")

    while True:
        try:
            messages = r.xreadgroup(
                groupname=GROUP_NAME,
                consumername=CONSUMER_NAME,
                streams={STREAM_NAME: '>'},
                block=5000,
                count=1
            )

            if not messages:
                continue

            for stream, message_list in messages:
                for message_id, data in message_list:
                    file_path = data.get('file_path')
                    work_id = data.get('work_id')

                    print(f"\n📥 New task received! ID: {message_id}")
                    print(f"   Work ID: {work_id}")
                    print(f"   File: {file_path}")

                    try:
                        # 1. Set status to ANALYZING
                        analyzer.update_status(work_id, MediaStatus.ANALYZING)

                        # 2. Find the right extractor
                        extractor = find_extractor(extractors, file_path)
                        print(f"   🔍 Using extractor: {extractor.__class__.__name__}")

                        # 3. Extract local metadata
                        storage_path = os.getenv('CODICE_STORAGE_PATH', './uploads')
                        # If relative, resolve from project root (two levels up from worker/)
                        if not os.path.isabs(storage_path):
                            project_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
                            storage_path = os.path.join(project_root, storage_path)
                        covers_dir = os.path.join(storage_path, 'covers')
                        os.makedirs(covers_dir, exist_ok=True)

                        metadata = extractor.extract(file_path, covers_dir)
                        print(f"   📄 Local metadata: {metadata.title} ({metadata.page_count} pages)")

                        # 4. Enrich via external providers (search ALL providers, pick best)
                        original_filename = os.path.basename(file_path)
                        enriched = provider_registry.search_best(metadata.title, metadata.format)
                        if enriched:
                            if enriched.title:
                                metadata.title = enriched.title
                            if enriched.author:
                                metadata.author = enriched.author
                            if enriched.series:
                                metadata.series = enriched.series
                            if enriched.series_index:
                                metadata.series_index = enriched.series_index
                            if enriched.isbn:
                                metadata.isbn = enriched.isbn
                            if enriched.description:
                                metadata.description = enriched.description
                            if enriched.tags:
                                metadata.tags = enriched.tags

                            # Download cover from provider
                            if enriched.cover_url:
                                print(f"   🖼️ Provider cover URL found, attempting download...")
                                local_cover = provider_registry.download_cover(
                                    enriched.cover_url, file_path, covers_dir
                                )
                                if local_cover:
                                    metadata.cover_path = local_cover
                                    print(f"   🖼️ Cover downloaded to: {metadata.cover_path}")
                                else:
                                    print(f"   ⚠️ Cover download returned empty path")
                            else:
                                print(f"   ⚠️ No cover_url returned by provider")

                        # 5. Save to database (all extracted + enriched fields)
                        save_meta = {
                            'title': metadata.title,
                            'author': metadata.author,
                            'format': metadata.format,
                            'page_count': metadata.page_count,
                            'cover_path': metadata.cover_path,
                            'series': metadata.series,
                            'series_index': metadata.series_index,
                            'isbn': metadata.isbn,
                            'language': metadata.language,
                            'publisher': metadata.publisher,
                            'publication_date': metadata.publication_date,
                            'description': metadata.description,
                            'tags': metadata.tags,
                            'raw': metadata.raw,
                        }

                        # Track which provider enriched the metadata
                        if enriched:
                            save_meta['enriched_source'] = enriched.source if hasattr(enriched, 'source') else None
                            if enriched.raw:
                                save_meta['raw'].update(enriched.raw)

                        analyzer.save_metadata(work_id, save_meta)
                        analyzer.save_identifiers(work_id, save_meta)
                        analyzer.save_media_pages(work_id, save_meta)

                        # 6. Set status to READY
                        analyzer.update_status(work_id, MediaStatus.READY)

                        # 7. Broadcast WORK_READY event
                        event = {
                            "type": "WORK_READY",
                            "work_id": work_id,
                            "title": metadata.title
                        }
                        r.publish('codice_updates', json.dumps(event))
                        print(f"📡 Broadcast WORK_READY event for Work ID: {work_id}")

                        # 8. Acknowledge task
                        r.xack(STREAM_NAME, GROUP_NAME, message_id)
                        print(f"✅ Task {message_id} completed successfully.")

                    except (ValueError, FileNotFoundError) as sec_err:
                        print(f"⚠️ Validation/security error: {sec_err}")
                        analyzer.update_status(work_id, MediaStatus.ERROR, str(sec_err))
                        r.xack(STREAM_NAME, GROUP_NAME, message_id)
                    except Exception as err:
                        print(f"❌ Processing failure: {err}")
                        analyzer.update_status(work_id, MediaStatus.ERROR, str(err))

        except Exception as e:
            print(f"⚠️ Unexpected network error: {e}")
            time.sleep(2)


if __name__ == "__main__":
    listen_for_tasks()