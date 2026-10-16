"""The hash KOReader knows a book by (`util.partialMD5`), kept for every file so that the Códice can say which book a device is reading
(KOSync, DEC-190).

It is the MD5 of up to twelve samples of 1 KiB, taken at the offsets 1024 << (2 * i) for i from -1 to 10: the first sample is at the start
of the file (in LuaJIT `lshift(1024, -2)` is a shift by 30, which goes past the end of a small file and then wraps), then 1 KiB, 4 KiB, 16 KiB
... up to 1 GiB, stopping at the first offset past the end. It was measured against a Kindle: the hash the device sent is the one this gives
for the same file.
"""
import hashlib
import os

from textindex.store import resolve

SAMPLE = 1024
BATCH = 100


def partial_md5(path):
    """The hash of the file at `path`, or None when it cannot be read."""
    digest = hashlib.md5()
    try:
        with open(path, 'rb') as f:
            for i in range(-1, 11):
                f.seek(0 if i == -1 else SAMPLE << (2 * i))
                chunk = f.read(SAMPLE)
                if not chunk:
                    break
                digest.update(chunk)
    except OSError:
        return None
    return digest.hexdigest()


def fill(db, storage_root, limit=BATCH, log=print):
    """Gives a hash to the files that have none: up to `limit` of them, the newest first. Returns how many it did."""
    rows = db.fetchall("""
        SELECT f.id, l.mode, l.root, l.path
        FROM files f
        LEFT JOIN LATERAL (SELECT mode, root, path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1) l ON TRUE
        WHERE f.koreader_md5 IS NULL AND f.availability = 'available' AND l.path IS NOT NULL
        ORDER BY f.id DESC LIMIT %s""", (limit,))
    done = 0
    for file_id, mode, root, path in rows or []:
        full = resolve(storage_root, mode, root, path)
        if not full or not os.path.isfile(full):
            continue
        digest = partial_md5(full)
        if digest:
            db.execute('UPDATE files SET koreader_md5 = %s WHERE id = %s AND koreader_md5 IS NULL', (digest, file_id))
            done += 1
    if done:
        log(f'   🔑 koreader hash: {done} file(s)')
    return done
