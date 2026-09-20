"""The job that reads the text of a work must not touch the work itself: reading stays available while
its text is being read, and when reading it fails."""
from unittest.mock import patch


def load_main():
    # main.py reads a .env when it is imported: not in a test, where it would leak into the environment.
    with patch('dotenv.load_dotenv', return_value=False):
        import main
    return main


class FakeDB:
    def __init__(self):
        self.executed = []
        self.job = (41, 7, {}, 1, 3, 'extract_text')

    def fetchone(self, query, params=None):
        if 'jobs_claim' in query:
            job, self.job = self.job, None
            return job
        if 'jobs_complete' in query:
            return (True,)
        return (1,)

    def fetchall(self, query, params=None):
        return []

    def execute(self, query, params=None):
        self.executed.append(query)

    def insert_many(self, *args, **kwargs):
        pass


def test_a_text_job_never_changes_the_status_of_the_work(tmp_path, monkeypatch):
    monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
    db = FakeDB()
    runner = load_main().build_runner(db, None)
    assert runner.run_one() is True
    assert not [q for q in db.executed if 'media_status' in q], db.executed


def test_an_ingest_job_still_does(tmp_path, monkeypatch):
    monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
    db = FakeDB()
    db.job = (42, 7, {'file_path': str(tmp_path / 'missing.epub')}, 1, 3, 'ingest')
    runner = load_main().build_runner(db, None)
    runner.run_one()
    assert [q for q in db.executed if 'media_status' in q], 'an ingest job reports on the work'
