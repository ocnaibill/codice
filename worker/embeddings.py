"""Optional, local embeddings for equivalent-position evidence (#31)."""
import json
import os


PREPROCESSING_VERSION = 1
BATCH = 32


class SentenceTransformersProvider:
    name = 'sentence-transformers'

    def __init__(self, model=None, revision=None):
        self.model = model or os.getenv('EMBEDDINGS_MODEL', 'sentence-transformers/LaBSE')
        self.revision = revision or os.getenv('EMBEDDINGS_MODEL_REVISION', 'default')
        self._encoder = None

    def encode(self, texts):
        if self._encoder is None:
            from sentence_transformers import SentenceTransformer
            self._encoder = SentenceTransformer(self.model, revision=None if self.revision == 'default' else self.revision)
        return self._encoder.encode(texts, batch_size=BATCH, normalize_embeddings=True, show_progress_bar=False).tolist()


class EmbeddingIndexer:
    def __init__(self, db, provider, log=print):
        self.db, self.provider, self.log = db, provider, log
        self.state, self.error = 'idle', ''

    def heartbeat(self, state=None, error=None):
        if state is not None:
            self.state = state
        if error is not None:
            self.error = error
        value = json.dumps({'provider': self.provider.name, 'model': self.provider.model,
                            'revision': self.provider.revision, 'state': self.state, 'error': self.error})
        self.db.execute("""INSERT INTO settings (key, value) VALUES ('embeddings.worker', %s::jsonb)
            ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()""", (value,))

    def enabled(self):
        row = self.db.fetchone("""SELECT COALESCE(
            (SELECT (value->>'enabled')::boolean FROM settings WHERE key = 'equivalence.embeddings'), false)""")
        return bool(row and row[0])

    def enqueue_missing(self):
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
        if not self.enabled():
            self.heartbeat('idle', '')
            return {}
        self.heartbeat('preparing', '')
        try:
            outcome = self._run(work_id, checkpoint)
            self.heartbeat('ready')
            return outcome
        except Exception as err:
            self.heartbeat('error', f'{type(err).__name__}: {err}'[:500])
            raise

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
