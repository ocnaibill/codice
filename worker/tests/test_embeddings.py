from embeddings import EmbeddingIndexer


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
