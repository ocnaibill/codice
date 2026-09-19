import os
import sys
import shutil
import time
import json
import hashlib
import psycopg2
from urllib.parse import urlparse

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

def sha256_of(path):
    digest = hashlib.sha256()
    with open(path, 'rb') as f:
        for block in iter(lambda: f.read(1 << 20), b''):
            digest.update(block)
    return digest.hexdigest()


def enqueue_file(cursor, src_path, dst_path):
    """Copy one file into storage and queue it, in the caller's transaction.

    Same contract as the API upload: identical bytes are never stored twice, and
    the work, the file hash and the job are written together. Returns
    ('duplicate', existing_work_id) or ('enqueued', work_id). The caller commits,
    and removes dst_path if anything raised."""
    digest = sha256_of(src_path)
    cursor.execute("SELECT e.work_id FROM files f JOIN editions e ON e.id = f.edition_id WHERE f.sha256 = %s", (digest,))
    existing = cursor.fetchone()
    if existing:
        return 'duplicate', existing[0]

    shutil.copy2(src_path, dst_path)
    cursor.execute("INSERT INTO works (original_title, file_path) VALUES (%s, %s) RETURNING id",
                   (os.path.basename(src_path), os.path.basename(dst_path)))
    work_id = cursor.fetchone()[0]
    # The database projects the work onto its primary file; record the bytes' facts there.
    cursor.execute(
        "UPDATE files SET sha256 = %s, size_bytes = %s "
        "WHERE id = (SELECT file_id FROM work_primary WHERE work_id = %s)",
        (digest, os.path.getsize(src_path), work_id))
    cursor.execute(
        "INSERT INTO jobs (type, work_id, payload, priority) VALUES ('ingest', %s, %s, 0) "
        "ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING",
        (work_id, json.dumps({"file_path": os.path.abspath(dst_path)})))
    cursor.execute("UPDATE works SET media_status = 'QUEUED' WHERE id = %s", (work_id,))
    return 'enqueued', work_id


def run_bulk_import(target_dir):
    """
    Recursively scans target_dir for .pdf, .epub, and .cbz files,
    copies them to storage, creates PostgreSQL work entries, and enqueues tasks in Redis Stream.
    """
    base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    storage_path = os.getenv("CODICE_STORAGE_PATH", os.path.abspath(os.path.join(base_dir, "backend", "uploads")))
    os.makedirs(storage_path, exist_ok=True)

    db_url = os.getenv("DATABASE_URL", "postgresql://codice_user:codice_secret@localhost:5432/codice_db")

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
    duplicate_count = 0
    error_count = 0

    for idx, src_path in enumerate(discovered_files, start=1):
        original_filename = os.path.basename(src_path)
        dst_path = os.path.join(storage_path, f"{int(time.time())}_{idx}_{original_filename}")

        try:
            outcome, work_id = enqueue_file(cursor, src_path, dst_path)
            conn.commit()
            if outcome == 'duplicate':
                duplicate_count += 1
                print(f"  [{idx}/{total_files}] ♻️ Already stored as Work #{work_id}: {original_filename}")
            else:
                enqueued_count += 1
                print(f"  [{idx}/{total_files}] 📥 Enqueued Work #{work_id}: {original_filename}")

        except Exception as e:
            conn.rollback()
            if os.path.exists(dst_path):
                os.remove(dst_path)
            error_count += 1
            print(f"  [{idx}/{total_files}] ⚠️ Error processing '{original_filename}': {e}")

    cursor.close()
    conn.close()

    print("\n🎉 Bulk Import Completed!")
    print(f"  • Total Discovered: {total_files}")
    print(f"  • Successfully Enqueued: {enqueued_count}")
    print(f"  • Already stored (duplicates): {duplicate_count}")
    print(f"  • Errors/Skipped: {error_count}")

if __name__ == "__main__":
    if len(sys.argv) > 1:
        target = sys.argv[1]
    else:
        target = os.getenv("BULK_IMPORT_DIR", "./import")

    run_bulk_import(target)
