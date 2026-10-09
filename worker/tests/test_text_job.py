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


class SettingsDB(FakeDB):
    """The same database, answering what the owner allowed (#68)."""

    def __init__(self, allowed=None):
        super().__init__()
        self.allowed = allowed

    def fetchone(self, query, params=None):
        if 'FROM settings' in query:
            return None if self.allowed is None else (self.allowed,)
        if 'w.title_lock' in query:  # the state of a work that has nothing yet
            from tests.test_analyzer import LOCK_NAMES, VALUE_NAMES
            return tuple('' for _ in VALUE_NAMES) + tuple(False for _ in LOCK_NAMES)
        return super().fetchone(query, params)


def _ingest_with_network_spies(tmp_path, monkeypatch, allowed):
    """Analyses a small text file and returns the URLs the worker tried to reach."""
    monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
    book = tmp_path / 'livro.txt'
    book.write_text('Um livro de teste.\n' * 20, encoding='utf-8')
    db = SettingsDB(allowed)
    db.job = (43, 7, {'file_path': str(book)}, 1, 3, 'ingest')
    asked = []

    class Reply:
        status_code = 200

        def json(self):
            return {'docs': [], 'items': []}

    def spy(url, *args, **kwargs):
        asked.append(url)
        return Reply()

    with patch('providers.http.requests.get', spy), patch('providers.google_books.requests.get', spy):
        load_main().build_runner(db, None).run_one()
    return asked


def test_an_analysis_asks_no_provider_the_owner_did_not_turn_on(tmp_path, monkeypatch):
    assert _ingest_with_network_spies(tmp_path, monkeypatch, None) == []
    assert _ingest_with_network_spies(tmp_path, monkeypatch, {'openlibrary': False, 'google_books': False}) == []


def test_an_analysis_asks_the_providers_the_owner_turned_on_and_only_those(tmp_path, monkeypatch):
    asked = _ingest_with_network_spies(tmp_path, monkeypatch, {'openlibrary': True})
    assert asked and all('openlibrary.org' in url for url in asked)
