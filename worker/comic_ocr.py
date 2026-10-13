"""Reads the pages of comics and manga by OCR (DEC-176, #27).

A comic is a folder of pictures: its text is in the balloons. Here each page is read by PaddleOCR (the engine that measured best in the
laboratory, lab/ocr) and what it found is turned into regions of speech (comic_regions.py): balloons and captions, without the
onomatopoeia, each with its box in the pixels of the page. A page is read once and kept, in `comic_ocr_pages`; the text the search sees is
made from those rows every time the text of the file is read (textindex/comic.py), in the order the direction of the series asks for, so
choosing a direction later reads nothing again.

It is an opt-in processing of its own, chosen by the owner in the administration (setting `comic_ocr`: none, mobile or server), and this
service only does anything when a model is chosen. The model is downloaded when it is chosen, with the percentage of the download told in
`settings` (key `comic_ocr.worker`), which also says how many pages it has read. A page it cannot read is kept as failed, with the reason,
and is not tried again by itself. Pages read with one model are not read again when another is chosen: that is a button of its own.
"""
import io
import json
import os
import re
import time
import zipfile

from comic_regions import speech
from embeddings import DownloadWatcher
from textindex import Limits
from textindex.store import TextIndexer, resolve

SETTING = 'comic_ocr'
WORKER_SETTING = 'comic_ocr.worker'
ENGINE_NAME = 'paddleocr'
NONE = 'none'
FORMATS = ('cbz', 'cbr')
IMAGES = ('.jpg', '.jpeg', '.png', '.webp')

# The two models the owner chooses between: the pair of PaddleOCR models each one is (detector and recogniser) and what it weighs, in MB.
# The recogniser of the light one is the Latin one, which reads Portuguese and English alike.
MODELS = {
    'mobile': {'det': 'PP-OCRv5_mobile_det', 'rec': 'latin_PP-OCRv5_mobile_rec', 'mb': 13},
    'server': {'det': 'PP-OCRv6_medium_det', 'rec': 'PP-OCRv6_medium_rec', 'mb': 132},
}
MAX_SIDE = 1600           # a page is read at most this wide or tall: the detector is slow on 3000 pixels, and balloons survive it
MAX_PAGES = int(os.getenv('COMIC_OCR_MAX_PAGES', '2000'))
MAX_CONSECUTIVE_FAILURES = 5
HEARTBEAT_EVERY = 5       # pages


class EngineMissing(Exception):
    """PaddleOCR is not installed in this image."""


def models_dir():
    """Where the models are kept (the cache of PaddleX): a folder that survives the container."""
    return os.getenv('PADDLE_PDX_CACHE_HOME') or os.path.join(os.path.expanduser('~'), '.paddlex')


def model_bytes(model, root=None):
    """What is on disk of the two models of one choice, finished or not."""
    base = os.path.join(root or models_dir(), 'official_models')
    total = 0
    for name in (MODELS[model]['det'], MODELS[model]['rec']):
        for folder, _dirs, files in os.walk(os.path.join(base, name)):
            for f in files:
                try:
                    total += os.path.getsize(os.path.join(folder, f))
                except OSError:
                    pass
    return total


def download_progress(model, root=None):
    """The percentage of the download of a model: 0 to 99 (100 is the state "ready")."""
    return max(0, min(99, int(100 * model_bytes(model, root) / (MODELS[model]['mb'] * 1024 * 1024))))


class PaddleEngine:
    """The detector and the recogniser of one choice, loaded when they are first needed (and downloaded then, if they are not here)."""
    name = ENGINE_NAME

    def __init__(self, model):
        self.model = model
        self._ocr = None

    @property
    def loaded(self):
        return self._ocr is not None

    def version(self):
        try:
            import paddleocr
            return str(paddleocr.__version__)[:32]
        except Exception:
            return ''

    def load(self):
        try:
            from paddleocr import PaddleOCR
        except ImportError as err:
            raise EngineMissing(f'PaddleOCR is not installed: {err}')
        spec = MODELS[self.model]
        self._ocr = PaddleOCR(text_detection_model_name=spec['det'], text_recognition_model_name=spec['rec'],
                              use_doc_orientation_classify=False, use_doc_unwarping=False, use_textline_orientation=False)

    def read(self, data):
        """([lines], (width, height)) of a page given as the bytes of its picture. A line is {'x', 'y', 'w', 'h', 'text', 'conf'} in the
        pixels of the page as it is, not as it was shrunk to be read."""
        import numpy as np
        from PIL import Image
        image = Image.open(io.BytesIO(data)).convert('RGB')
        width, height = image.size
        scale = min(1.0, MAX_SIDE / max(width, height))
        if scale < 1:
            image = image.resize((max(1, int(width * scale)), max(1, int(height * scale))))
        result = self._ocr.predict(np.array(image)[:, :, ::-1].copy())
        return lines_of(result, scale), (width, height)


def lines_of(result, scale=1.0):
    """The lines PaddleOCR found, with confidence from 0 to 100, in the pixels of the page."""
    lines = []
    for item in result:
        data = item.json.get('res', item.json) if hasattr(item, 'json') else item
        texts, scores = data.get('rec_texts', []), data.get('rec_scores', [])
        polys = data.get('rec_polys', data.get('dt_polys', []))
        for text, score, poly in zip(texts, scores, polys):
            text = (text or '').strip()
            if not text:
                continue
            xs, ys = [p[0] for p in poly], [p[1] for p in poly]
            x0, y0, x1, y1 = min(xs) / scale, min(ys) / scale, max(xs) / scale, max(ys) / scale
            lines.append({'x': int(x0), 'y': int(y0), 'w': int(x1 - x0), 'h': int(y1 - y0), 'text': text, 'conf': float(score) * 100})
    return lines


def natural_names(names):
    """The pictures of an archive in the order the rest of the system counts them (the index of a page in a locator)."""
    return sorted(n for n in names if n.lower().endswith(IMAGES) and not n.endswith('/'))


class Archive:
    """The pictures of a CBZ or a CBR, in page order, read one at a time."""

    def __init__(self, path, fmt):
        self.path, self.fmt, self._a = path, fmt, None

    def __enter__(self):
        if self.fmt == 'cbr':
            try:
                import rarfile
            except ImportError:
                raise ValueError('CBR files cannot be read here (rarfile is not installed)')
            try:
                self._a = rarfile.RarFile(self.path)
            except (rarfile.Error, OSError) as err:
                raise ValueError(f'not a readable CBR: {err}')
        else:
            try:
                self._a = zipfile.ZipFile(self.path)
            except zipfile.BadZipFile as err:
                raise ValueError(f'not a readable CBZ: {err}')
        self.names = natural_names(self._a.namelist())
        return self

    def __exit__(self, *exc):
        if self._a is not None:
            self._a.close()

    def read(self, index):
        info = self._a.getinfo(self.names[index])
        if info.file_size > Limits.MAX_ENTRY_BYTES:
            raise ValueError(f'page {index + 1} is too large to be read')
        return self._a.read(info)


FILES_TO_READ = """
    SELECT f.id, f.sha256, lower(COALESCE(f.format, '')), l.mode, l.root, l.path
    FROM files f
    JOIN editions e ON e.id = f.edition_id
    JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
    LEFT JOIN LATERAL (
        SELECT mode, root, path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1
    ) l ON TRUE
    WHERE e.work_id = %s AND f.availability = 'available' AND lower(COALESCE(f.format, '')) IN ('cbz', 'cbr')
    ORDER BY f.id"""

SAVE_PAGE = """
    INSERT INTO comic_ocr_pages (file_id, page, source_sha256, state, regions, width, height, model, engine, engine_version, seconds, error)
    VALUES (%s, %s, %s, %s, %s::jsonb, %s, %s, %s, %s, %s, %s, %s)
    ON CONFLICT (file_id, page) DO UPDATE SET source_sha256 = EXCLUDED.source_sha256, state = EXCLUDED.state,
        regions = EXCLUDED.regions, width = EXCLUDED.width, height = EXCLUDED.height, model = EXCLUDED.model, engine = EXCLUDED.engine,
        engine_version = EXCLUDED.engine_version, seconds = EXCLUDED.seconds, error = EXCLUDED.error, recognized_at = now()"""


class ComicOcrIndexer:
    def __init__(self, db, storage_root, engine_factory=PaddleEngine, log=print, watch_db=None, root=None):
        self.db, self.storage_root, self.engine_factory, self.log = db, storage_root, engine_factory, log
        self.watch_db, self.root = watch_db, root
        self.engines = {}
        self.state, self.error, self.progress = 'idle', '', None
        self.model = NONE

    # --- what the owner decided, and what this service says of itself ---

    def choice(self):
        row = self.db.fetchone("SELECT value FROM settings WHERE key = %s", (SETTING,))
        value = row[0] if row else None
        if isinstance(value, (str, bytes)):
            try:
                value = json.loads(value)
            except ValueError:
                value = None
        model = value.get('model') if isinstance(value, dict) else None
        return model if model in MODELS else NONE

    def pages_read(self, model):
        row = self.db.fetchone("SELECT count(*) FROM comic_ocr_pages WHERE model = %s AND state IN ('done', 'blank')", (model,))
        return int(row[0]) if row else 0

    def _write(self, db, progress):
        body = {'model': self.model, 'state': self.state, 'error': self.error, 'engine': ENGINE_NAME,
                'pages': self.pages_read(self.model) if self.model in MODELS else 0}
        if progress is not None and self.state == 'preparing':
            body['progress'] = progress  # the administration shows the percentage of the download
        db.execute("""INSERT INTO settings (key, value) VALUES (%s, %s::jsonb)
            ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()""", (WORKER_SETTING, json.dumps(body)))

    def heartbeat(self, state=None, error=None):
        """Tells the administration that the service is here, with the model it has, how far its download is and how many pages it read."""
        if state is not None:
            self.state = state
        if error is not None:
            self.error = error
        self._write(self.db, self.progress)

    def _report_download(self, progress):
        self.progress = progress
        if self.watch_db is None:
            from db import CodiceDatabase
            self.watch_db = CodiceDatabase()
        self._write(self.watch_db, progress)

    # --- the model ---

    def prepare(self, model):
        """The engine of a model, downloaded and loaded. While it is downloaded the percentage is reported, from a thread of its own."""
        engine = self.engines.get(model)
        if engine is None:
            engine = self.engines[model] = self.engine_factory(model)
        if engine.loaded:
            return engine
        self.model = model
        self.progress = 0
        self.heartbeat('preparing', '')
        watcher = DownloadWatcher(model, self._report_download, measure=lambda m: download_progress(m, self.root)).start()
        try:
            engine.load()
        except Exception as err:
            self.progress = None
            self.engines.pop(model, None)
            self.heartbeat('error', f'{type(err).__name__}: {err}'[:300])
            raise
        finally:
            watcher.stop()
        self.progress = None
        self.heartbeat('ready', '')
        return engine

    # --- what there is to read ---

    def enqueue_missing(self):
        """With nothing else to do: says it is here, prepares the model that was chosen (so it is downloaded when it is chosen) and queues the
        works that have a comic with pages nobody has read. A work whose job failed is left alone for an hour."""
        model = self.choice()
        if model == NONE:
            self.model, self.progress = NONE, None
            self.heartbeat('idle', '')
            return
        try:
            self.prepare(model)
        except Exception as err:
            self.log(f'   ⚠️ comic OCR: the model {model} could not be prepared ({type(err).__name__}: {err})')
            return
        self.model = model
        self.heartbeat()
        self.db.execute("""
            INSERT INTO jobs (type, work_id, payload, priority)
            SELECT DISTINCT 'comic_ocr', e.work_id, '{}'::jsonb, -20
            FROM files f
            JOIN editions e ON e.id = f.edition_id
            JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
            WHERE f.availability = 'available' AND lower(COALESCE(f.format, '')) IN ('cbz', 'cbr')
              AND COALESCE(w.page_count, 0) > 0
              AND (SELECT count(*) FROM comic_ocr_pages p WHERE p.file_id = f.id AND p.source_sha256 IS NOT DISTINCT FROM f.sha256) < w.page_count
              AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.type = 'comic_ocr' AND j.work_id = e.work_id
                              AND j.state = 'failed' AND j.updated_at > now() - interval '1 hour')
            ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING
        """)

    # --- the job ---

    def run(self, work_id, checkpoint=lambda: None):
        """Reads the pages of every comic of the work that nobody has read, then makes the text of each file from everything read so far.
        Returns {file_id: {'read': n, 'failed': n}}."""
        model = self.choice()
        if model == NONE:
            self.heartbeat('idle', '')
            return {}
        engine = self.prepare(model)
        self.model = model
        outcome = {}
        self.heartbeat('working', '')
        try:
            for file_id, sha, fmt, mode, root, path in self.db.fetchall(FILES_TO_READ, (work_id,)):
                checkpoint()
                if self.choice() != model:
                    break
                outcome[file_id] = self.read_file(engine, model, file_id, sha, fmt, mode, root, path, checkpoint)
                # The text of the file, from everything OCR has read of it so far.
                TextIndexer(self.db, self.storage_root).run(work_id, force=True, checkpoint=checkpoint, only={file_id})
            self.heartbeat('ready', '')
            return outcome
        except Exception as err:
            self.heartbeat('error', f'{type(err).__name__}: {err}'[:300])
            raise

    def read_file(self, engine, model, file_id, sha, fmt, mode, root, path, checkpoint):
        full = resolve(self.storage_root, mode, root, path)
        if full is None or not os.path.isfile(full):
            raise FileNotFoundError(f'file {file_id} is not at its place')
        # What was read of another version of the file is not of this one.
        self.db.execute("DELETE FROM comic_ocr_pages WHERE file_id = %s AND source_sha256 IS DISTINCT FROM %s", (file_id, sha))
        known = {int(p) for (p,) in self.db.fetchall(
            "SELECT page FROM comic_ocr_pages WHERE file_id = %s AND source_sha256 IS NOT DISTINCT FROM %s", (file_id, sha))}
        read = failed = in_a_row = 0
        version = engine.version()
        with Archive(full, fmt) as archive:
            if len(archive.names) > MAX_PAGES:
                raise ValueError(f'{len(archive.names)} pages: more than the {MAX_PAGES} that are read in one file')
            for index in range(len(archive.names)):
                if index in known:
                    continue
                checkpoint()
                # The owner chose another model (or none) while the job was running: what is still to read is left to the next job,
                # that reads it with the model chosen now, so that reading again (DEC-176) is not undone by the job that was running.
                if self.choice() != model:
                    break
                started = time.time()
                try:
                    lines, (width, height) = engine.read(archive.read(index))
                    regions = speech(lines, width, height)
                    state, error = ('done' if regions else 'blank'), None
                except Exception as err:  # a page that will not read is kept as failed, with the reason, and the file goes on
                    regions, width, height, state, error = [], None, None, 'failed', f'{type(err).__name__}: {err}'[:300]
                self.db.execute(SAVE_PAGE, (file_id, index, sha, state, json.dumps(regions, ensure_ascii=False), width, height, model,
                                            ENGINE_NAME, version, round(time.time() - started, 2), error))
                if state == 'failed':
                    failed += 1
                    in_a_row += 1
                    if in_a_row >= MAX_CONSECUTIVE_FAILURES:
                        raise RuntimeError(f'{in_a_row} pages in a row could not be read: {error}')
                else:
                    read += 1
                    in_a_row = 0
                if (read + failed) % HEARTBEAT_EVERY == 0:
                    self.heartbeat()
        return {'read': read, 'failed': failed}
