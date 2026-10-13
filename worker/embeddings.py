"""Optional, local embeddings for equivalent-position evidence (#31)."""
import json
import os
import threading


PREPROCESSING_VERSION = 1
BATCH = 32
DEFAULT_MODEL = 'sentence-transformers/LaBSE'
SUPPORTED_MODELS = {
    DEFAULT_MODEL,
    'intfloat/multilingual-e5-small',
}
# What each model weighs when downloaded (MB), as the administration says it: the percentage of a download is what is on disk of it.
MODEL_MB = {DEFAULT_MODEL: 1880, 'intfloat/multilingual-e5-small': 471}


def hf_cache_root():
    """The folder the models are downloaded to (the one huggingface_hub uses)."""
    try:
        from huggingface_hub.constants import HF_HUB_CACHE
        return HF_HUB_CACHE
    except Exception:
        return None


def cache_bytes(model):
    """What is on disk of the download of a model, finished or not (a file that is still arriving counts)."""
    root = hf_cache_root()
    if not root:
        return 0
    blobs = os.path.join(root, 'models--' + model.replace('/', '--'), 'blobs')
    try:
        return sum(os.path.getsize(os.path.join(blobs, name)) for name in os.listdir(blobs))
    except OSError:
        return 0


def progress_of(model, have=None):
    """The percentage of a download, 0 to 99 (100 is the model being ready, which is said by the state), or None when its size is not known."""
    expected = MODEL_MB.get(model, 0) * 1024 * 1024
    if not expected:
        return None
    have = cache_bytes(model) if have is None else have
    return max(0, min(99, int(100 * have / expected)))


class SentenceTransformersProvider:
    name = 'sentence-transformers'

    @property
    def loaded(self):
        return self._encoder is not None

    def __init__(self, model=None, revision=None):
        self.model = model or os.getenv('EMBEDDINGS_MODEL', DEFAULT_MODEL)
        self.revision = revision or os.getenv('EMBEDDINGS_MODEL_REVISION', 'default')
        self._encoder = None

    def encode(self, texts):
        if self._encoder is None:
            from sentence_transformers import SentenceTransformer
            self._encoder = SentenceTransformer(self.model, revision=None if self.revision == 'default' else self.revision)
        # E5 was trained with task prefixes. Equivalent-position matching is symmetric, so both
        # sides use the same query prefix as recommended for symmetric similarity tasks.
        if self.model == 'intfloat/multilingual-e5-small':
            texts = [f'query: {text}' for text in texts]
        return self._encoder.encode(texts, batch_size=BATCH, normalize_embeddings=True, show_progress_bar=False).tolist()


class DownloadWatcher:
    """While a model is being downloaded, says how much of it is on disk, now and then, until it is stopped. The download happens inside a
    call that does not return until it is done, so this has a thread of its own, and the one that reports has a connection of its own."""

    def __init__(self, model, report, interval=2.0, measure=progress_of):
        self.model, self.report, self.interval, self.measure = model, report, interval, measure
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._loop, daemon=True)

    def tick(self):
        progress = self.measure(self.model)
        if progress is not None:
            self.report(progress)

    def _loop(self):
        while not self._stop.wait(self.interval):
            try:
                self.tick()
            except Exception as err:  # never a reason to stop preparing the model
                print(f"   ⚠️ could not report the download of {self.model}: {err}")

    def start(self):
        self._thread.start()
        return self

    def stop(self):
        self._stop.set()
        if self._thread.is_alive():
            self._thread.join(timeout=5)


class EmbeddingIndexer:
    def __init__(self, db, provider, log=print, selectable=False, watch_db=None):
        self.db, self.provider, self.log, self.selectable = db, provider, log, selectable
        self.state, self.error = 'idle', ''
        self.progress = None
        # A connection for the thread that watches a download (the job's own is busy); made when it is first needed.
        self.watch_db = watch_db

    def select_configured_model(self):
        if not self.selectable:
            return
        row = self.db.fetchone("""SELECT COALESCE(
            (SELECT value->>'model' FROM settings WHERE key = 'equivalence.embeddings'), %s)""",
                               (DEFAULT_MODEL,))
        model = row[0] if row and row[0] in SUPPORTED_MODELS else DEFAULT_MODEL
        if model != self.provider.model:
            self.provider = SentenceTransformersProvider(model=model)
            self.state, self.error = 'idle', ''

    def heartbeat(self, state=None, error=None):
        if state is not None:
            self.state = state
        if error is not None:
            self.error = error
        self._write(self.db, self.progress)

    def _write(self, db, progress):
        body = {'provider': self.provider.name, 'model': self.provider.model,
                'revision': self.provider.revision, 'state': self.state, 'error': self.error}
        if progress is not None and self.state == 'preparing':
            body['progress'] = progress  # the administration shows the percentage of the download (DEC-176)
        db.execute("""INSERT INTO settings (key, value) VALUES ('embeddings.worker', %s::jsonb)
            ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()""", (json.dumps(body),))

    def _report_download(self, progress):
        self.progress = progress
        if self.watch_db is None:
            from db import CodiceDatabase
            self.watch_db = CodiceDatabase()
        self._write(self.watch_db, progress)

    def enabled(self):
        row = self.db.fetchone("""SELECT COALESCE(
            (SELECT (value->>'enabled')::boolean FROM settings WHERE key = 'equivalence.embeddings'), false)""")
        return bool(row and row[0])

    def enqueue_missing(self):
        self.select_configured_model()
        self.heartbeat()
        if not self.enabled():
            return
        self.db.execute("""
            INSERT INTO jobs (type, work_id, payload, priority)
            SELECT DISTINCT 'embed_text', e.work_id, '{}'::jsonb, -20
            FROM editions e JOIN files f ON f.edition_id = e.id
            JOIN text_extractions tx ON tx.file_id = f.id AND tx.status = 'ready'
            LEFT JOIN text_embedding_status es ON es.file_id = f.id
              AND es.generation = tx.generation AND es.provider = %s AND es.model = %s
              AND es.revision = %s AND es.preprocessing_version = %s
            WHERE es.file_id IS NULL
            ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING
        """, (self.provider.name, self.provider.model, self.provider.revision, PREPROCESSING_VERSION))

    def run(self, work_id, checkpoint=lambda: None):
        self.select_configured_model()
        if not self.enabled():
            self.heartbeat('idle', '')
            return {}
        self.heartbeat('preparing', '')
        # The model is downloaded the first time it is used, inside the first encoding: while that lasts, the percentage is reported.
        watcher = None if getattr(self.provider, 'loaded', True) else DownloadWatcher(self.provider.model, self._report_download).start()
        try:
            outcome = self._run(work_id, checkpoint)
            if watcher:
                watcher.stop()
                watcher = None
            self.progress = None
            self.heartbeat('ready')
            return outcome
        except Exception as err:
            self.progress = None
            self.heartbeat('error', f'{type(err).__name__}: {err}'[:500])
            raise
        finally:
            if watcher:
                watcher.stop()

    def _run(self, work_id, checkpoint=lambda: None):
        files = self.db.fetchall("""
            SELECT f.id, tx.generation FROM files f JOIN editions e ON e.id = f.edition_id
            JOIN text_extractions tx ON tx.file_id = f.id AND tx.status = 'ready'
            LEFT JOIN text_embedding_status es ON es.file_id = f.id
              AND es.generation = tx.generation AND es.provider = %s AND es.model = %s
              AND es.revision = %s AND es.preprocessing_version = %s
            WHERE e.work_id = %s AND es.file_id IS NULL ORDER BY f.id
        """, (self.provider.name, self.provider.model, self.provider.revision, PREPROCESSING_VERSION, work_id))
        outcome = {}
        for file_id, generation in files:
            checkpoint()
            outcome[file_id] = self.embed_file(file_id, generation, checkpoint)
        return outcome

    def embed_file(self, file_id, generation, checkpoint):
        rows = self.db.fetchall("""
            SELECT id, text FROM document_segments
            WHERE file_id = %s AND generation = %s ORDER BY sequence
        """, (file_id, generation))
        vectors = []
        for start in range(0, len(rows), BATCH):
            checkpoint()
            batch = rows[start:start + BATCH]
            encoded = self.provider.encode([text for _id, text in batch])
            vectors.extend((segment_id, vector) for (segment_id, _text), vector in zip(batch, encoded))
        dimensions = len(vectors[0][1]) if vectors else 0
        if vectors:
            self.db.insert_many("""
                INSERT INTO document_segment_embeddings
                  (document_segment_id, provider, model, revision, preprocessing_version, dimensions, normalized, embedding)
                VALUES %s
                ON CONFLICT (document_segment_id) DO UPDATE SET provider = EXCLUDED.provider,
                  model = EXCLUDED.model, revision = EXCLUDED.revision,
                  preprocessing_version = EXCLUDED.preprocessing_version, dimensions = EXCLUDED.dimensions,
                  normalized = EXCLUDED.normalized, embedding = EXCLUDED.embedding, embedded_at = now()
            """, [(segment_id, self.provider.name, self.provider.model, self.provider.revision,
                    PREPROCESSING_VERSION, dimensions, True, json.dumps(vector)) for segment_id, vector in vectors],
                template='(%s, %s, %s, %s, %s, %s, %s, %s::jsonb)')
        self.db.execute("""
            INSERT INTO text_embedding_status
              (file_id, generation, provider, model, revision, preprocessing_version, segment_count)
            VALUES (%s, %s, %s, %s, %s, %s, %s)
            ON CONFLICT (file_id) DO UPDATE SET generation = EXCLUDED.generation,
              provider = EXCLUDED.provider, model = EXCLUDED.model, revision = EXCLUDED.revision,
              preprocessing_version = EXCLUDED.preprocessing_version,
              segment_count = EXCLUDED.segment_count, embedded_at = now()
        """, (file_id, generation, self.provider.name, self.provider.model, self.provider.revision,
              PREPROCESSING_VERSION, len(vectors)))
        return len(vectors)
