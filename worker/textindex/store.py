"""Reads the text of the files of a work and publishes it (job `extract_text`).

Reading never waits for this and never depends on it: it runs after the file is analysed, as a job
of its own, and touches nothing about the work itself (its status stays what it was). The queue
decides when; the rules for publishing (a new generation of a file's text is invisible until it is
published, and publishing replaces the old one at once) are SQL functions, migration 00019.
"""
import json
import os
import zipfile

from . import EXTRACTOR_VERSION, LOCATOR_VERSION, required_version
from .language import detect as detect_language
from .audio import audio_segments
from .comic import comic_segments
from .epub import epub_segments
from .pdf import pdf_segments
from .plain import plain_segments

BATCH = 200
SAMPLE_PIECE = 300       # characters taken from the start of each segment, to tell the language
SAMPLE_PIECES = 1200     # segments the sample is made from, spread over the whole file
# The formats that have text to read. A comic and an audio file have only what their metadata says (the ComicInfo.xml,
# the chapters and the description; the pages are for OCR and the speech for recognition); MOBI is not read here.
READERS = {
    'epub': lambda path, checkpoint, out: epub_segments(path, checkpoint, out),
    'pdf': lambda path, checkpoint, out: pdf_segments(path, checkpoint, out, ocr=out.get('ocr')),
    'txt': lambda path, checkpoint, out: plain_segments(path, 'txt', checkpoint),
    'md': lambda path, checkpoint, out: plain_segments(path, 'md', checkpoint),
    'cbz': lambda path, checkpoint, out: comic_segments(path, 'cbz', checkpoint),
    'cbr': lambda path, checkpoint, out: comic_segments(path, 'cbr', checkpoint),
    **{fmt: (lambda path, checkpoint, out, fmt=fmt: audio_segments(path, fmt, checkpoint))
       for fmt in ('mp3', 'm4a', 'm4b', 'ogg', 'wav', 'flac')},
}
# What these formats have is the description of the file, not its body: with none, there is no text to read yet (not
# an empty scan waiting for OCR), and what there is says nothing about the language the book is written in.
METADATA_ONLY = {'cbz', 'cbr', 'mp3', 'm4a', 'm4b', 'ogg', 'wav', 'flac'}

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


# The comparison of a work with the others goes after what people asked for (priority below 0), once however many of its
# files were read, and not while one is already waiting.
ENQUEUE_DEDUPE = """
    INSERT INTO jobs (type, work_id, payload, priority) VALUES ('dedupe', %s, '{}'::jsonb, -10)
    ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING"""


class TextIndexer:
    def __init__(self, db, storage_root, version=EXTRACTOR_VERSION, log=print):
        self.db = db
        self.storage_root = storage_root
        self.version = version
        self.log = log

    def needs_reading(self, row, force):
        _id, fmt, sha, _lang, _avail, _mode, _root, _path, version, source_sha, status = row
        if force or version is None or version < min(required_version(fmt), self.version):
            return True
        if status == 'failed':
            return False  # it failed the same way: only asking again (force) or a new version tries it
        return bool(sha) and source_sha != sha  # the file is not the one the text came from

    def run(self, work_id, force=False, checkpoint=lambda: None, only=None):
        """Reads every file of the work that needs it (or, with `only`, those of that list of file ids). Returns
        {file_id: status}."""
        outcome = {}
        for row in self.db.fetchall(FILES_OF_WORK, (work_id,)):
            checkpoint()
            file_id, fmt, sha, language, availability, mode, root, path = row[:8]
            if only is not None and file_id not in only:
                continue
            if availability != 'available' or not self.needs_reading(row, force):
                continue
            outcome[file_id] = self.read_file(file_id, fmt.lower(), sha, language, mode, root, path, checkpoint)
        if 'ready' in outcome.values():
            # There is text now (or new text): the work is compared, by its words, with the others (#38).
            self.db.execute(ENQUEUE_DEDUPE, (work_id,))
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
            found = {}  # what a reader learns besides the segments: the shape of the book
            if fmt == 'pdf':
                found['ocr'] = self.recognised_pages(file_id, sha)  # what OCR read of the pages that have no text
            pieces = []  # and a sample of the text, to tell the language when the file does not
            origins = set()
            count = self.write(file_id, generation, self.tracked(reader(full, checkpoint, found), origins), checkpoint, pieces)
            metadata_only = fmt in METADATA_ONLY
            detected = None if language or not count or metadata_only else self.detect(pieces)
            none = 'unsupported' if metadata_only else 'empty'
            origin = 'mixed' if len(origins) > 1 else ('ocr' if origins == {'ocr'} else 'native')
            status = self.publish(file_id, generation, sha, 'ready' if count else none, language or detected, found.get('structure'), origin)
            if detected:
                self.suggest_language(file_id, detected)
            return status
        except (ValueError, zipfile.BadZipFile) as err:
            # The file is what it is and will not read: recorded, and the job goes on to the next file.
            self.log(f'   ⚠️ text of file {file_id} could not be read: {err}')
            self.db.execute("SELECT text_extraction_fail(%s, %s, %s, %s)", (file_id, self.version, sha, str(err)[:500]))
            return 'failed'

    def recognised_pages(self, file_id, sha):
        """{page index: text} of the pages of a PDF that OCR has read, for the file as it is (a file whose bytes
        changed is not the one that was read)."""
        rows = self.db.fetchall(
            """SELECT page, text FROM ocr_pages
               WHERE file_id = %s AND source_sha256 IS NOT DISTINCT FROM %s AND state = 'done'""", (file_id, sha))
        return {int(page): text for page, text in rows}

    @staticmethod
    def tracked(segments, origins):
        """The segments as they are, noting where their text comes from."""
        for segment in segments:
            origins.add(segment.origin)
            yield segment

    def write(self, file_id, generation, segments, checkpoint, pieces=None):
        rows, count = [], 0
        for sequence, seg in enumerate(segments):
            if pieces is not None:
                pieces.append(seg.text[:SAMPLE_PIECE])
            rows.append((file_id, generation, sequence, seg.origin, seg.section, seg.text,
                         json.dumps(seg.locator, ensure_ascii=False), LOCATOR_VERSION, seg.node))
            count += 1
            if len(rows) >= BATCH:
                self.flush(rows)
                rows = []
                checkpoint()
        if rows:
            self.flush(rows)
        return count

    @staticmethod
    def detect(pieces):
        """The language of a file, from a sample spread over all of it (None when it cannot be told)."""
        if len(pieces) > SAMPLE_PIECES:
            step = len(pieces) / SAMPLE_PIECES
            pieces = [pieces[int(i * step)] for i in range(SAMPLE_PIECES)]
        return detect_language('\n'.join(pieces))

    def suggest_language(self, file_id, language):
        """Proposes the language found in the text of a file to whoever looks after the library (#35). It is
        a guess, so it is a suggestion like the ones external providers make: nothing changes until an owner
        or admin accepts it, and one that was rejected is not proposed again (the key is work, field, source
        and value). Only for an edition with no language at all, that nobody locked, and only for the primary
        edition of its work, because that is the edition a suggestion about a work applies to."""
        self.db.execute(
            """INSERT INTO metadata_candidates (work_id, field, value, source, evidence)
               SELECT e.work_id, 'language', %s, 'detected', %s::jsonb
               FROM files f JOIN editions e ON e.id = f.edition_id JOIN works w ON w.id = e.work_id
               WHERE f.id = %s AND e.is_primary AND COALESCE(e.language, '') = '' AND NOT w.language_lock
               ON CONFLICT (work_id, field, source, value) DO NOTHING""",
            (language, json.dumps({'method': 'common words', 'from': 'the text of the file'}), file_id))

    def flush(self, rows):
        self.db.insert_many(
            "INSERT INTO document_segments (file_id, generation, sequence, origin, section, text, locator, locator_version, node) VALUES %s",
            rows, template="(%s, %s, %s, %s, %s, %s, %s::jsonb, %s, %s)")

    def publish(self, file_id, generation, sha, status, language, structure=None, origin='native'):
        self.db.fetchone("SELECT text_extraction_publish(%s, %s, %s, %s, %s, %s, %s, %s::jsonb)",
                         (file_id, generation, self.version, sha, status, origin, language,
                          json.dumps(structure, ensure_ascii=False) if structure else None))
        return status
