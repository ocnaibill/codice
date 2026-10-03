"""The worker takes the dictionary job like any other, hands it to the importer, and tells the package how it ended (#109)."""
from unittest.mock import patch

import pytest

from runner import JobsClient


def load_main():
    with patch('dotenv.load_dotenv', return_value=False):
        import main
    return main


class FakeDB:
    def __init__(self, job):
        self.job = job
        self.executed = []
        self.failures = []

    def fetchone(self, query, params=None):
        if 'jobs_claim' in query:
            job, self.job = self.job, None
            self.claimed_types = params[3]
            return job
        if 'jobs_complete' in query:
            return (True,)
        if 'jobs_fail' in query:
            self.failures.append(params)
            return (self.outcome,)
        return (1,)

    outcome = 'failed'

    def fetchall(self, query, params=None):
        return []

    def execute(self, query, params=None):
        self.executed.append(query)

    def insert_many(self, *args, **kwargs):
        pass


class FakeImporter:
    instances = []

    def __init__(self, connect, workdir=None, **kwargs):
        self.workdir = workdir
        self.calls = []
        self.error = None
        FakeImporter.instances.append(self)

    def run(self, job, checkpoint):
        self.calls.append(('run', job['payload']))
        checkpoint()
        if self.error:
            raise self.error
        return {'entries': 3}

    def fail(self, job, message):
        self.calls.append(('fail', message))

    def retrying(self, job, message):
        self.calls.append(('retrying', message))


@pytest.fixture
def worker(tmp_path, monkeypatch):
    monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
    FakeImporter.instances = []
    main = load_main()
    monkeypatch.setattr(main, 'DictionaryImporter', FakeImporter)
    payload = {'package': 'wikt-pt', 'url': 'https://kaikki.org/dictionary/downloads/pt/pt-extract.jsonl.gz', 'edition': 'pt'}

    def make(error=None, outcome='failed'):
        db = FakeDB((51, None, payload, 1, 3, 'dictionary'))
        db.outcome = outcome
        runner = main.build_runner(db, None)
        importer = FakeImporter.instances[-1]
        importer.error = error
        return db, runner, importer
    make.tmp = tmp_path
    return make


def test_the_default_worker_takes_dictionary_jobs():
    assert 'dictionary' in JobsClient.JOB_TYPES


def test_the_job_goes_to_the_importer_and_never_touches_a_work(worker):
    db, runner, importer = worker()
    assert runner.run_one() is True
    assert importer.calls == [('run', {'package': 'wikt-pt', 'url': 'https://kaikki.org/dictionary/downloads/pt/pt-extract.jsonl.gz', 'edition': 'pt'})]
    assert not [q for q in db.executed if 'media_status' in q], db.executed
    assert 'dictionary' in db.claimed_types


def test_what_is_downloaded_is_kept_in_the_storage_and_not_on_the_system_temp(worker):
    _, _, importer = worker()
    assert importer.workdir == str(worker.tmp / 'tmp')
    assert (worker.tmp / 'tmp').is_dir()


def test_a_failure_that_will_not_change_says_so_in_the_package(worker):
    db, runner, importer = worker(error=ValueError('not the dictionary'))
    runner.run_one()
    assert [c for c in importer.calls if c[0] == 'fail'] == [('fail', 'ValueError: not the dictionary')]
    assert not [q for q in db.executed if 'media_status' in q]


def test_a_failure_that_will_be_tried_again_says_so_in_the_package(worker):
    db, runner, importer = worker(error=OSError('connection reset'), outcome='retry')
    runner.run_one()
    assert [c for c in importer.calls if c[0] == 'retrying'] == [('retrying', 'OSError: connection reset')]
    assert not [c for c in importer.calls if c[0] == 'fail']
