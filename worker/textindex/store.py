"""Reads the text of the files of a work and publishes it (job `extract_text`).

Reading never waits for this and never depends on it: it runs after the file is analysed, as a job
of its own, and touches nothing about the work itself (its status stays what it was). The queue
decides when; the rules for publishing (a new generation of a file's text is invisible until it is
published, and publishing replaces the old one at once) are SQL functions, migration 00019.
"""
import json
import os
import zipfile

from . import EXTRACTOR_VERSION, LOCATOR_VERSION
from .epub import epub_segments
from .pdf import pdf_segments
from .plain import plain_segments

BATCH = 200
# The formats that have text to read. Comics and audio have none (until OCR, for comics); MOBI is not
# read here.
READERS = {
    'epub': lambda path, checkpoint: epub_segments(path, checkpoint),
    'pdf': lambda path, checkpoint: pdf_segments(path, checkpoint),
    'txt': lambda path, checkpoint: plain_segments(path, 'txt', checkpoint),
    'md': lambda path, checkpoint: plain_segments(path, 'md', checkpoint),
}

FILES_OF_WORK = """
    SELECT f.id, COALESCE(f.format, ''), f.sha256, COALESCE(e.language, ''), f.availability,
           l.mode, l.root, l.path, tx.extractor_version, tx.source_sha256, tx.status
    FROM files f
    JOIN editions e ON e.id = f.edition_id
    LEFT JOIN LATERAL (
        SELECT mode, root, path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1
    ) l ON TRUE
    LEFT JOIN text_extractions tx ON tx.file_id = f.id
    WHERE e.work_id = %s
    ORDER BY f.id"""


def resolve(storage_root, mode, root, path):
    """Where the bytes of a file are, or None. A path is joined to its root and must stay inside it."""
    if not path:
        return None
    base = root if mode == 'referenced' else storage_root
    if not base:
        return None
    full = os.path.normpath(os.path.join(base, path))
    base = os.path.normpath(base)
    if full != base and not full.startswith(base + os.sep):
        return None
    return full


class TextIndexer:
    def __init__(self, db, storage_root, version=EXTRACTOR_VERSION, log=print):
        self.db = db
        self.storage_root = storage_root
        self.version = version
        self.log = log

    def needs_reading(self, row, force):
        _id, _fmt, sha, _lang, _avail, _mode, _root, _path, version, source_sha, status = row
        if force or version is None or version < self.version:
            return True
        if status == 'failed':
            return False  # it failed the same way: only asking again (force) or a new version tries it
        return bool(sha) and source_sha != sha  # the file is not the one the text came from

    def run(self, work_id, force=False, checkpoint=lambda: None):
        """Reads every file of the work that needs it. Returns {file_id: status}."""
        outcome = {}
        for row in self.db.fetchall(FILES_OF_WORK, (work_id,)):
            checkpoint()
            file_id, fmt, sha, language, availability, mode, root, path = row[:8]
            if availability != 'available' or not self.needs_reading(row, force):
                continue
            outcome[file_id] = self.read_file(file_id, fmt.lower(), sha, language, mode, root, path, checkpoint)
        return outcome

    def read_file(self, file_id, fmt, sha, language, mode, root, path, checkpoint):
        reader = READERS.get(fmt)
        generation = self.db.fetchone("SELECT text_extraction_begin(%s)", (file_id,))[0]
        try:
            if reader is None:
                return self.publish(file_id, generation, sha, 'unsupported', language)
            full = resolve(self.storage_root, mode, root, path)
            if full is None or not os.path.isfile(full):
                # Not here now (moved, or gone): nothing is published, and nothing is recorded as failed
                # either, because it may be back the next time. The job says so.
                raise FileNotFoundError(f'file {file_id} is not at its place')
            count = self.write(file_id, generation, reader(full, checkpoint), checkpoint)
            return self.publish(file_id, generation, sha, 'ready' if count else 'empty', language)
        except (ValueError, zipfile.BadZipFile) as err:
            # The file is what it is and will not read: recorded, and the job goes on to the next file.
            self.log(f'   ⚠️ text of file {file_id} could not be read: {err}')
            self.db.execute("SELECT text_extraction_fail(%s, %s, %s, %s)", (file_id, self.version, sha, str(err)[:500]))
            return 'failed'

    def write(self, file_id, generation, segments, checkpoint):
        rows, count = [], 0
        for sequence, seg in enumerate(segments):
            rows.append((file_id, generation, sequence, seg.origin, seg.section, seg.text,
                         json.dumps(seg.locator, ensure_ascii=False), LOCATOR_VERSION))
            count += 1
            if len(rows) >= BATCH:
                self.flush(rows)
                rows = []
                checkpoint()
        if rows:
            self.flush(rows)
        return count

    def flush(self, rows):
        self.db.insert_many(
            "INSERT INTO document_segments (file_id, generation, sequence, origin, section, text, locator, locator_version) VALUES %s",
            rows, template="(%s, %s, %s, %s, %s, %s, %s::jsonb, %s)")

    def publish(self, file_id, generation, sha, status, language):
        self.db.fetchone("SELECT text_extraction_publish(%s, %s, %s, %s, %s, 'native', %s)",
                         (file_id, generation, self.version, sha, status, language))
        return status
