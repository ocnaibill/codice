import os
import sys
import shutil
import time
import json
import redis
import psycopg2
from urllib.parse import urlparse

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

def run_bulk_import(target_dir):
    """
    Recursively scans target_dir for .pdf, .epub, and .cbz files,
    copies them to storage, creates PostgreSQL work entries, and enqueues tasks in Redis Stream.
    """
    base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    storage_path = os.getenv("CODICE_STORAGE_PATH", os.path.abspath(os.path.join(base_dir, "backend", "uploads")))
    os.makedirs(storage_path, exist_ok=True)

    db_url = os.getenv("DATABASE_URL", "postgresql://codice_user:codice_secret@localhost:5432/codice_db")
    redis_url = os.getenv("REDIS_URL", "redis://localhost:6379/0")

    print(f"🚀 Starting Bulk Import from: '{target_dir}'...")
    print(f"📁 Target Storage Directory: '{storage_path}'")

    if not os.path.exists(target_dir):
        print(f"❌ Error: Target directory does not exist: {target_dir}")
        sys.exit(1)

    # 1. Connect to PostgreSQL
    try:
        conn = psycopg2.connect(db_url)
        cursor = conn.cursor()
        print("✅ Connected to PostgreSQL")
    except Exception as e:
        print(f"❌ Failed to connect to PostgreSQL: {e}")
        sys.exit(1)

    # 2. Connect to Redis
    try:
        r = redis.Redis.from_url(redis_url)
        r.ping()
        print("✅ Connected to Redis")
    except Exception as e:
        print(f"❌ Failed to connect to Redis: {e}")
        sys.exit(1)

    valid_exts = ('.pdf', '.epub', '.cbz')
    discovered_files = []

    for root, _, files in os.walk(target_dir):
        for f in files:
            if f.lower().endswith(valid_exts):
                discovered_files.append(os.path.join(root, f))

    total_files = len(discovered_files)
    print(f"🔍 Found {total_files} supported documents (.pdf, .epub, .cbz)")

    if total_files == 0:
        print("✨ No new documents to import.")
        return

    enqueued_count = 0
    error_count = 0

    for idx, src_path in enumerate(discovered_files, start=1):
        original_filename = os.path.basename(src_path)
        safe_filename = f"{int(time.time())}_{idx}_{original_filename}"
        dst_path = os.path.join(storage_path, safe_filename)

        try:
            # Copy file to storage directory
            shutil.copy2(src_path, dst_path)

            # Insert into PostgreSQL
            query = "INSERT INTO works (original_title, file_path) VALUES (%s, %s) RETURNING id;"
            cursor.execute(query, (original_filename, safe_filename))
            work_id = cursor.fetchone()[0]
            conn.commit()

            abs_dst_path = os.path.abspath(dst_path)

            # Enqueue task in Redis Stream
            r.xadd("ingestion_tasks", {
                "file_path": abs_dst_path,
                "work_id": str(work_id)
            })

            enqueued_count += 1
            print(f"  [{idx}/{total_files}] 📥 Enqueued Work #{work_id}: {original_filename}")

        except Exception as e:
            conn.rollback()
            error_count += 1
            print(f"  [{idx}/{total_files}] ⚠️ Error processing '{original_filename}': {e}")

    cursor.close()
    conn.close()

    print("\n🎉 Bulk Import Completed!")
    print(f"  • Total Discovered: {total_files}")
    print(f"  • Successfully Enqueued: {enqueued_count}")
    print(f"  • Errors/Skipped: {error_count}")

if __name__ == "__main__":
    if len(sys.argv) > 1:
        target = sys.argv[1]
    else:
        target = os.getenv("BULK_IMPORT_DIR", "./import")

    run_bulk_import(target)
