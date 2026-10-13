from embeddings import EmbeddingIndexer, SentenceTransformersProvider


class Provider:
    name = 'fake'
    model = 'multilingual-test'
    revision = 'r1'

    def encode(self, texts):
        return [[float(len(text)), 1.0] for text in texts]


class DB:
    def __init__(self):
        self.executed = []
        self.inserted = []

    def execute(self, query, params=None):
        self.executed.append((query, params))

    def fetchone(self, query, params=None):
        return (True,)

    def fetchall(self, query, params=None):
        if 'SELECT f.id, tx.generation' in query:
            return [(7, 3)]
        if 'SELECT id, text FROM document_segments' in query:
            return [(70, 'um trecho'), (71, 'outro trecho')]
        return []

    def insert_many(self, query, rows, template=None):
        self.inserted.extend(rows)


def test_embedding_indexer_stores_versioned_vectors_and_publishes_status():
    db = DB()
    outcome = EmbeddingIndexer(db, Provider()).run(12)
    assert outcome == {7: 2}
    assert [row[0] for row in db.inserted] == [70, 71]
    assert all(row[1:5] == ('fake', 'multilingual-test', 'r1', 1) for row in db.inserted)
    assert any('INSERT INTO text_embedding_status' in query for query, _params in db.executed)


def test_enqueue_missing_is_provider_and_version_specific():
    db = DB()
    EmbeddingIndexer(db, Provider()).enqueue_missing()
    query, params = db.executed[-1]
    assert "'embed_text'" in query
    assert params == ('fake', 'multilingual-test', 'r1', 1)


def test_e5_uses_the_symmetric_task_prefix():
    class Encoder:
        def encode(self, texts, **_options):
            self.texts = texts
            return type('Vectors', (), {'tolist': lambda self: [[1.0]]})()

    provider = SentenceTransformersProvider(model='intfloat/multilingual-e5-small')
    provider._encoder = Encoder()
    provider.encode(['uma passagem'])
    assert provider._encoder.texts == ['query: uma passagem']


def test_selectable_indexer_follows_the_owner_model_choice():
    class ConfigDB(DB):
        def fetchone(self, query, params=None):
            if "value->>'model'" in query:
                return ('intfloat/multilingual-e5-small',)
            return super().fetchone(query, params)

    indexer = EmbeddingIndexer(ConfigDB(), SentenceTransformersProvider(), selectable=True)
    indexer.select_configured_model()
    assert indexer.provider.model == 'intfloat/multilingual-e5-small'


# --- the percentage of the download of a model (DEC-176) ---

import json
import os

import embeddings
from embeddings import DEFAULT_MODEL, DownloadWatcher, MODEL_MB, cache_bytes, progress_of


class TestTheDownloadPercentage:
    def test_it_is_what_is_on_disk_over_what_the_model_weighs(self, tmp_path, monkeypatch):
        monkeypatch.setattr(embeddings, 'hf_cache_root', lambda: str(tmp_path))
        blobs = tmp_path / 'models--intfloat--multilingual-e5-small' / 'blobs'
        blobs.mkdir(parents=True)
        (blobs / 'a').write_bytes(b'x' * 1024)
        (blobs / 'b.incomplete').write_bytes(b'x' * 2048)  # a file that is still arriving counts
        assert cache_bytes('intfloat/multilingual-e5-small') == 3072
        half = int(MODEL_MB['intfloat/multilingual-e5-small'] * 1024 * 1024 / 2)
        assert progress_of('intfloat/multilingual-e5-small', have=half) == 49 or progress_of('intfloat/multilingual-e5-small', have=half) == 50

    def test_it_never_says_one_hundred_and_never_less_than_nothing(self):
        huge = MODEL_MB[DEFAULT_MODEL] * 1024 * 1024 * 3
        assert progress_of(DEFAULT_MODEL, have=huge) == 99
        assert progress_of(DEFAULT_MODEL, have=0) == 0

    def test_a_model_of_unknown_size_has_no_percentage(self):
        assert progress_of('some/other-model', have=10) is None

    def test_a_model_that_was_never_downloaded_has_nothing_on_disk(self, tmp_path, monkeypatch):
        monkeypatch.setattr(embeddings, 'hf_cache_root', lambda: str(tmp_path))
        assert cache_bytes('intfloat/multilingual-e5-small') == 0
        monkeypatch.setattr(embeddings, 'hf_cache_root', lambda: None)
        assert cache_bytes('intfloat/multilingual-e5-small') == 0


class TestTheWatcher:
    def test_each_tick_reports_what_it_measures(self):
        seen = []
        DownloadWatcher('m', seen.append, measure=lambda model: 37).tick()
        assert seen == [37]

    def test_a_model_of_unknown_size_reports_nothing(self):
        seen = []
        DownloadWatcher('m', seen.append, measure=lambda model: None).tick()
        assert seen == []

    def test_it_stops_when_it_is_told_to_and_a_failing_report_does_not_stop_the_work(self, capsys):
        calls = []

        def report(progress):
            calls.append(progress)
            raise RuntimeError('the connection is gone')

        watcher = DownloadWatcher('m', report, interval=0.01, measure=lambda model: 5).start()
        deadline = __import__('time').time() + 2
        while not calls and __import__('time').time() < deadline:
            __import__('time').sleep(0.01)
        watcher.stop()
        assert calls and not watcher._thread.is_alive()
        assert 'could not report the download' in capsys.readouterr().out


class LoadingProvider(Provider):
    """A provider that has not downloaded its model yet."""
    loaded = False
    model = 'intfloat/multilingual-e5-small'


class TestWhatTheHeartbeatSays:
    def written(self, db):
        return [json.loads(params[0]) for query, params in db.executed if 'embeddings.worker' in query]

    def test_the_percentage_goes_in_the_heartbeat_only_while_the_model_is_being_prepared(self):
        db = DB()
        indexer = EmbeddingIndexer(db, LoadingProvider())
        indexer.progress = 42
        indexer.heartbeat('preparing')
        indexer.heartbeat('ready')
        first, second = self.written(db)
        assert first['progress'] == 42 and first['state'] == 'preparing'
        assert 'progress' not in second and second['state'] == 'ready'

    def test_the_download_is_told_through_the_connection_of_the_watcher(self):
        job, watch = DB(), DB()
        indexer = EmbeddingIndexer(job, LoadingProvider(), watch_db=watch)
        indexer.state = 'preparing'
        indexer._report_download(64)
        assert self.written(job) == []
        assert self.written(watch)[0]['progress'] == 64 and indexer.progress == 64

    def test_a_run_clears_the_percentage_when_it_ends(self, monkeypatch):
        db = DB()
        indexer = EmbeddingIndexer(db, Provider())
        monkeypatch.setattr(indexer, 'enabled', lambda: True)
        indexer.run(12)
        assert indexer.progress is None
        bodies = self.written(db)
        assert bodies[-1]['state'] == 'ready' and 'progress' not in bodies[-1]
