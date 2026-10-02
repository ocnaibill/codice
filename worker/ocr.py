"""Reads the pages of scanned PDFs by OCR (#24, RF-019, DEC-044).

A page with no text layer is an image. Here it is read by an engine (Tesseract, run as a program with a time limit and one
CPU thread by default) and what it found is kept, per page, in `ocr_pages`: reading a scanned book takes minutes or hours,
so a page is read once and its text is the expensive part. The segments the search sees are made from those pages every
time the text of the file is read (textindex/pdf.py), which is why reading the native text again never loses them.

It is an opt-in processing of its own, apart from the switch of the AI areas (DEC-044): this worker only does anything when
the owner has turned OCR on, and it says in `settings` (key `ocr.worker`) what engine and languages it has, so the
administration can tell. A page it cannot read is kept as failed, with the reason, and is not tried again by itself.

What is recognised is not the faithful transcription of the original: the engine, its version and the language it was
told are recorded with every page.
"""
import json
import os
import shutil
import subprocess

import fitz  # PyMuPDF

from textindex.language import detect as detect_language
from textindex.pdf import MIN_TEXT_CHARS
from textindex.store import TextIndexer, resolve

SETTING = 'ocr'
WORKER_SETTING = 'ocr.worker'
ENGINE_NAME = 'tesseract'
DEFAULT_LANGUAGE = 'por+eng'

# What the engine is allowed to cost (RNF-003, RNF-007): the time of one page, the size of the picture it is given,
# the number of pages of one file and how many in a row may fail before the job stops trying.
PAGE_TIMEOUT = float(os.getenv('OCR_PAGE_TIMEOUT', '180'))
MAX_PIXELS = 40_000_000
MAX_PAGES = int(os.getenv('OCR_MAX_PAGES', '3000'))
MAX_CONSECUTIVE_FAILURES = 5
MIN_DPI, MAX_DPI, DEFAULT_DPI = 150, 300, 200
# Pages read, spread over the file, to tell its language when nothing declares it.
LANGUAGE_SAMPLES = 3

# The languages of an edition that map to a code of the engine; the ones it is not installed with are not used.
ENGINE_LANGUAGES = {
    'pt': 'por', 'por': 'por', 'en': 'eng', 'eng': 'eng', 'es': 'spa', 'spa': 'spa', 'fr': 'fra', 'fra': 'fra',
    'fre': 'fra', 'it': 'ita', 'ita': 'ita', 'ca': 'cat', 'cat': 'cat', 'ro': 'ron', 'ron': 'ron', 'rum': 'ron',
    'de': 'deu', 'deu': 'deu', 'ger': 'deu', 'nl': 'nld', 'nld': 'nld', 'dut': 'nld', 'la': 'lat', 'lat': 'lat',
}


class EngineError(Exception):
    """The engine could not read a page (a time out, a failure of its own)."""


class EngineMissing(Exception):
    """The engine is not here: no page can be read, which is not the fault of any of them."""


class Tesseract:
    name = ENGINE_NAME

    def __init__(self, binary=None, threads=None, timeout=None):
        self.binary = binary or os.getenv('TESSERACT_BIN', 'tesseract')
        self.threads = int(threads if threads is not None else os.getenv('OCR_THREADS', '1'))
        self.timeout = float(timeout if timeout is not None else PAGE_TIMEOUT)
        self._languages = None
        self._version = None

    def installed(self):
        return shutil.which(self.binary) is not None

    def _run(self, args, **kwargs):
        try:
            return subprocess.run([self.binary, *args], capture_output=True, **kwargs)
        except FileNotFoundError:
            raise EngineMissing(f'{self.binary} is not installed')

    def version(self):
        if self._version is None:
            out = self._run(['--version'], timeout=20)
            first = (out.stdout or out.stderr).decode('utf-8', 'replace').strip().splitlines()
            words = first[0].split() if first else []
            self._version = words[1] if len(words) > 1 else ''
        return self._version

    def languages(self):
        """The language codes the engine has (what it was installed with), without the one it uses for orientation."""
        if self._languages is None:
            out = self._run(['--list-langs'], timeout=20)
            lines = out.stdout.decode('utf-8', 'replace').splitlines()
            self._languages = sorted(l.strip() for l in lines[1:] if l.strip() and l.strip() != 'osd')
            self._has_osd = any(l.strip() == 'osd' for l in lines[1:])
        return self._languages

    def recognize(self, image, language, dpi):
        """The text of a picture of a page (PNG bytes), in reading order, paragraphs apart by blank lines."""
        self.languages()
        # Page layout with the orientation detected (a scan can be turned) needs the data for it; without, the plain one.
        psm = '1' if self._has_osd else '3'
        env = dict(os.environ, OMP_THREAD_LIMIT=str(self.threads))
        try:
            out = self._run(['stdin', 'stdout', '-l', language, '--psm', psm, '--dpi', str(dpi)], input=image,
                            timeout=self.timeout, env=env)
        except subprocess.TimeoutExpired:
            raise EngineError(f'timed out after {int(self.timeout)} s')
        if out.returncode != 0:
            raise EngineError((out.stderr.decode('utf-8', 'replace').strip() or f'exit code {out.returncode}')[:300])
        return out.stdout.decode('utf-8', 'replace')


def declared_language(code, available):
    """The code of the engine for a language of an edition (`pt-BR`, `eng`), when the engine has it; else None."""
    primary = (code or '').strip().lower().replace('_', '-').split('-')[0]
    mapped = ENGINE_LANGUAGES.get(primary)
    return mapped if mapped and mapped in set(available) else None


def engine_language(edition_language, fallback, available):
    """The language the engine is told for a file: the one its edition declares when the engine has it, else the
    owner's default (only its parts the engine has). Raises ValueError when there is nothing it can read."""
    declared = declared_language(edition_language, available)
    if declared:
        return declared
    have = set(available)
    parts = [p for p in (fallback or DEFAULT_LANGUAGE).split('+') if p in have]
    if not parts:
        raise ValueError('the OCR engine has none of the languages chosen')
    return '+'.join(parts)


def sample_pages(without_text, count=LANGUAGE_SAMPLES):
    """The indexes of the pages to read to tell the language: all of them for a short file, else a few spread over it
    (not the first or the last, which are often a cover or a blank)."""
    pages = sorted(n - 1 for n in without_text if n >= 1)
    if len(pages) <= count:
        return pages
    return [pages[len(pages) * (i + 1) // (count + 1)] for i in range(count)]


def effective_dpi(page):
    """The resolution to read a page at: the one its picture was scanned at (reading it at another only blurs it), kept
    between what the engine reads well and what costs too much, and never so large that the picture is huge."""
    dpi = DEFAULT_DPI
    best_area = 0
    try:
        for info in page.get_image_info():
            x0, y0, x1, y1 = info['bbox']
            width_inches = (x1 - x0) / 72
            area = (x1 - x0) * (y1 - y0)
            if width_inches > 0 and info.get('width') and area > best_area:
                best_area, dpi = area, info['width'] / width_inches
    except (RuntimeError, ValueError, KeyError):
        dpi = DEFAULT_DPI
    dpi = max(MIN_DPI, min(MAX_DPI, int(round(dpi))))
    rect = page.rect
    while dpi > MIN_DPI and rect.width * rect.height * (dpi / 72) ** 2 > MAX_PIXELS:
        dpi -= 25
    return dpi


def render(page):
    """(PNG bytes, dpi) of a page, in grey: colour does not help the engine and costs memory."""
    dpi = effective_dpi(page)
    pixmap = page.get_pixmap(dpi=dpi, colorspace=fitz.csGRAY, alpha=False)
    return pixmap.tobytes('png'), dpi


FILES_TO_READ = """
    SELECT f.id, f.sha256, COALESCE(e.language, ''), l.mode, l.root, l.path, tl.pages_without_text
    FROM files f
    JOIN editions e ON e.id = f.edition_id
    JOIN text_layers tl ON tl.file_id = f.id AND tl.needs_ocr
    LEFT JOIN LATERAL (
        SELECT mode, root, path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1
    ) l ON TRUE
    WHERE e.work_id = %s AND f.availability = 'available' AND lower(COALESCE(f.format, '')) = 'pdf'
    ORDER BY f.id"""

SAVE_PAGE = """
    INSERT INTO ocr_pages (file_id, page, source_sha256, state, text, engine, engine_version, language, dpi, error)
    VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
    ON CONFLICT (file_id, page) DO UPDATE SET source_sha256 = EXCLUDED.source_sha256, state = EXCLUDED.state,
        text = EXCLUDED.text, engine = EXCLUDED.engine, engine_version = EXCLUDED.engine_version,
        language = EXCLUDED.language, dpi = EXCLUDED.dpi, error = EXCLUDED.error, recognized_at = now()"""


class OcrIndexer:
    def __init__(self, db, storage_root, engine=None, log=print):
        self.db = db
        self.storage_root = storage_root
        self.engine = engine or Tesseract()
        self.log = log
        self.state, self.error = 'idle', ''

    # --- what the owner decided, and what this worker says of itself ---

    def settings(self):
        """(enabled, the default language) the owner chose."""
        row = self.db.fetchone("SELECT value FROM settings WHERE key = %s", (SETTING,))
        value = row[0] if row else None
        if isinstance(value, (str, bytes)):
            try:
                value = json.loads(value)
            except ValueError:
                value = None  # what cannot be read is not a yes
        value = value if isinstance(value, dict) else {}
        return value.get('enabled') is True, (value.get('language') or DEFAULT_LANGUAGE)

    def heartbeat(self, state=None, error=None):
        """Tells the administration that the engine is here, with what languages, and what it is doing."""
        if state is not None:
            self.state = state
        if error is not None:
            self.error = error
        try:
            languages = self.engine.languages() if self.engine.installed() else []
            version = self.engine.version() if self.engine.installed() else ''
            state, error = (self.state, self.error) if languages else ('error', 'O motor de OCR não está instalado.')
        except (EngineMissing, subprocess.SubprocessError, OSError) as err:
            languages, version, state, error = [], '', 'error', f'{type(err).__name__}: {err}'[:300]
        value = json.dumps({'engine': self.engine.name, 'version': version, 'languages': languages, 'state': state, 'error': error})
        self.db.execute("""INSERT INTO settings (key, value) VALUES (%s, %s::jsonb)
            ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()""", (WORKER_SETTING, value))

    # --- what there is to read ---

    def enqueue_missing(self):
        """With nothing else to do: queues the works that have a PDF with pages OCR has not been asked about yet. Pages that
        failed count as asked, so a page that will not read does not queue the work again and again; a work whose job
        failed is left alone for an hour."""
        self.heartbeat()
        enabled, _language = self.settings()
        if not enabled:
            return
        self.db.execute("""
            INSERT INTO jobs (type, work_id, payload, priority)
            SELECT DISTINCT 'ocr', e.work_id, '{}'::jsonb, -20
            FROM text_layers tl
            JOIN files f ON f.id = tl.file_id AND f.availability = 'available'
            JOIN editions e ON e.id = f.edition_id
            JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
            WHERE tl.needs_ocr AND cardinality(tl.pages_without_text) <= %s
              AND (SELECT count(*) FROM ocr_pages p
                   WHERE p.file_id = f.id AND p.source_sha256 IS NOT DISTINCT FROM f.sha256
                     AND p.page + 1 = ANY (tl.pages_without_text)) < cardinality(tl.pages_without_text)
              AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.type = 'ocr' AND j.work_id = e.work_id
                              AND j.state = 'failed' AND j.updated_at > now() - interval '1 hour')
            ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING
        """, (MAX_PAGES,))

    # --- the job ---

    def run(self, work_id, retry_failed=False, checkpoint=lambda: None):
        """Reads the pages without text of every PDF of the work that has some, then makes the text of each file from what
        was read. Returns {file_id: {'read': n, 'failed': n}}."""
        enabled, default = self.settings()
        if not enabled:
            self.heartbeat('idle', '')
            return {}
        outcome = {}
        self.heartbeat('working', '')
        try:
            for file_id, sha, language, mode, root, path, without_text in self.db.fetchall(FILES_TO_READ, (work_id,)):
                checkpoint()
                outcome[file_id] = self.read_file(file_id, sha, language, mode, root, path, list(without_text or []),
                                                  default, retry_failed, checkpoint)
                # The text of the file, from everything OCR has read of it so far.
                TextIndexer(self.db, self.storage_root).run(work_id, force=True, checkpoint=checkpoint, only={file_id})
            self.heartbeat('idle', '')
            return outcome
        except Exception as err:
            self.heartbeat('error', f'{type(err).__name__}: {err}'[:300])
            raise

    def read_file(self, file_id, sha, language, mode, root, path, without_text, default, retry_failed, checkpoint):
        if len(without_text) > MAX_PAGES:
            raise ValueError(f'{len(without_text)} pages without text: more than the {MAX_PAGES} that are read in one file')
        full = resolve(self.storage_root, mode, root, path)
        if full is None or not os.path.isfile(full):
            raise FileNotFoundError(f'file {file_id} is not at its place')
        if not self.engine.installed():
            raise EngineMissing('the OCR engine is not installed')
        version = self.engine.version()

        # What was read, and decided, of another version of the file is not of this one.
        self.db.execute("DELETE FROM ocr_pages WHERE file_id = %s AND source_sha256 IS DISTINCT FROM %s", (file_id, sha))
        self.db.execute("DELETE FROM ocr_files WHERE file_id = %s AND source_sha256 IS DISTINCT FROM %s", (file_id, sha))

        read = failed = in_a_row = 0
        try:
            doc = fitz.open(full)
        except (RuntimeError, ValueError) as err:
            raise ValueError(f'not a readable PDF: {err}')
        try:
            if doc.needs_pass:
                raise ValueError('the PDF is encrypted')
            lang = self.language_of(doc, file_id, sha, language, without_text, default, checkpoint)
            known = {int(page): state for page, state in self.db.fetchall(
                "SELECT page, state FROM ocr_pages WHERE file_id = %s AND source_sha256 IS NOT DISTINCT FROM %s", (file_id, sha))}
            todo = [n - 1 for n in without_text if n >= 1 and (n - 1 not in known or (retry_failed and known[n - 1] == 'failed'))]
            for index in todo:
                checkpoint()
                state, text, error, dpi = self.read_page(doc, index, lang)
                self.db.execute(SAVE_PAGE, (file_id, index, sha, state, text, self.engine.name, version, lang, dpi, error))
                if state == 'failed':
                    failed += 1
                    in_a_row += 1
                    if in_a_row >= MAX_CONSECUTIVE_FAILURES:
                        raise RuntimeError(f'{in_a_row} pages in a row could not be read: {error}')
                else:
                    read += 1
                    in_a_row = 0
        finally:
            doc.close()
        return {'read': read, 'failed': failed}

    def language_of(self, doc, file_id, sha, declared, without_text, default, checkpoint):
        """The language to read the file in, decided once and kept (ocr_files): what was decided for this version of the
        file (including a language a person on the staff chose), else the one its edition declares, else the one found by
        reading a few pages and telling the language from the text, else the owner's default."""
        have = self.engine.languages()
        row = self.db.fetchone("SELECT language, source FROM ocr_files WHERE file_id = %s AND source_sha256 IS NOT DISTINCT FROM %s",
                               (file_id, sha))
        # A fallback to the owner's default is not a decision: it is made again, so that a new default (or a better
        # reading of the pages) is used.
        if row and row[0] and row[1] != 'default' and all(part in have for part in row[0].split('+')):
            return row[0]
        lang = declared_language(declared, have)
        source = 'declared'
        if not lang:
            lang, source = self.detect_language(doc, without_text, default, have, checkpoint)
        self.db.execute("""INSERT INTO ocr_files (file_id, source_sha256, language, source) VALUES (%s, %s, %s, %s)
            ON CONFLICT (file_id) DO UPDATE SET source_sha256 = EXCLUDED.source_sha256, language = EXCLUDED.language,
                source = EXCLUDED.source, decided_at = now()""", (file_id, sha, lang, source))
        return lang

    def detect_language(self, doc, without_text, default, have, checkpoint):
        """(language, 'detected') for a few pages read with the owner's default and found to be in one language the engine
        has; else (the default, 'default'). The pages read for this are not kept: the file is read in the language found."""
        fallback = engine_language('', default, have)
        texts = []
        for index in sample_pages(without_text):
            checkpoint()
            state, text, _error, _dpi = self.read_page(doc, index, fallback)
            if state == 'done':
                texts.append(text)
        found = declared_language(detect_language('\n'.join(texts)), have)
        return (found, 'detected') if found else (fallback, 'default')

    def read_page(self, doc, index, lang):
        """(state, text, error, dpi) of one page: done with its text, blank when it has none, failed with why."""
        dpi = None
        try:
            page = doc.load_page(index)
            image, dpi = render(page)
            text = self.engine.recognize(image, lang, dpi)
        except EngineMissing:
            raise
        except (EngineError, RuntimeError, ValueError) as err:
            return 'failed', '', f'{type(err).__name__}: {err}'[:300], dpi
        if sum(1 for ch in text if not ch.isspace()) < MIN_TEXT_CHARS:
            return 'blank', '', None, dpi
        return 'done', text, None, dpi
