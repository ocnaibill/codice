"""The installation of a dictionary, run for real against PostgreSQL (#109).

They need a PostgreSQL (TEST_DATABASE_URL, as the backend's tests do) and skip without one. Each test makes its own schema
and runs the migration's own statements in it, so the tables are the ones the application has, and nothing that exists is
touched: the schema is dropped when the test ends. The "download" is a function that answers with the bytes of the sample."""
import gzip
import hashlib
import io
import os
import re
import urllib.error
import uuid

import psycopg2
import pytest

import dictionary
from dictionary import DictionaryImporter, PermanentDownloadError
from runner import Cancelled, classify

URL = os.environ.get('TEST_DATABASE_URL')
pytestmark = pytest.mark.skipif(not URL, reason='TEST_DATABASE_URL is not set')

HERE = os.path.dirname(__file__)
SAMPLE = os.path.join(HERE, 'fixtures', 'dictionary-sample.jsonl.gz')
MIGRATION = os.path.join(HERE, '..', '..', 'backend', 'internal', 'database', 'migrations', '00042_dictionary.sql')
ADDRESS = 'https://kaikki.org/dictionary/downloads/pt/pt-extract.jsonl.gz'


def sample_bytes():
    with open(SAMPLE, 'rb') as f:
        return f.read()


class Response:
    def __init__(self, data, url=ADDRESS, length=None, modified='Mon, 28 Sep 2026 15:20:37 GMT'):
        self.stream = io.BytesIO(data)
        self.url = url
        self.headers = {'Content-Length': str(len(data) if length is None else length), 'Last-Modified': modified}

    def read(self, n=-1):
        return self.stream.read(n)

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


def serving(data=None, **kwargs):
    payload = sample_bytes() if data is None else data
    return lambda request, timeout=None: Response(payload, **kwargs)


@pytest.fixture
def world(tmp_path):
    schema = 'dict_' + uuid.uuid4().hex[:10]
    admin = psycopg2.connect(URL)
    admin.autocommit = True
    admin.cursor().execute(f'CREATE SCHEMA {schema}')

    def connect():
        conn = psycopg2.connect(URL)
        with conn.cursor() as cur:
            cur.execute(f'SET search_path TO {schema}')
        conn.commit()
        return conn

    up = open(MIGRATION, encoding='utf-8').read().split('-- +goose Down')[0].replace('-- +goose Up', '')
    conn = connect()
    conn.cursor().execute(up)
    conn.commit()
    conn.close()

    class World:
        pass

    w = World()
    w.connect = connect
    w.tmp = tmp_path

    def query(sql, params=()):
        c = connect()
        try:
            cur = c.cursor()
            cur.execute(sql, params)
            return cur.fetchall() if cur.description else None
        finally:
            c.close()

    def execute(sql, params=()):
        c = connect()
        try:
            c.cursor().execute(sql, params)
            c.commit()
        finally:
            c.close()

    w.query, w.execute = query, execute
    w.importer = lambda opener=None, **kw: DictionaryImporter(connect, workdir=str(tmp_path), opener=opener or serving(), **kw)

    def package(state='installing'):
        execute("INSERT INTO dictionary_packages (id, state, source_url) VALUES ('wikt-pt', %s, %s)", (state, ADDRESS))
    w.package = package
    w.job = lambda **extra: {'id': 1, 'payload': {'package': 'wikt-pt', 'url': ADDRESS, 'edition': 'pt', **extra}, 'attempts': 1, 'max_attempts': 3}
    try:
        yield w
    finally:
        admin.cursor().execute(f'DROP SCHEMA {schema} CASCADE')
        admin.close()


def nothing(): return None


class TestInstall:
    def test_downloads_imports_and_says_it_is_ready(self, world):
        world.package()
        outcome = world.importer().run(world.job(), nothing)
        assert outcome['entries'] > 30 and outcome['skipped'] == 1
        assert outcome['languages'] == ['de', 'en', 'es', 'fr', 'it', 'ja', 'pt', 'zh']
        state, stage, progress, entries, forms, links, languages, sha, date, error, installed = world.query(
            """SELECT state, stage, progress, entries, forms, links, languages, sha256, source_date, error, installed_at IS NOT NULL
               FROM dictionary_packages WHERE id = 'wikt-pt'""")[0]
        assert (state, stage, progress, error, installed) == ('ready', 'done', 1, '', True)
        assert entries == outcome['entries'] and forms == outcome['forms'] and links == outcome['links'] and forms > 0 and links > 0
        assert languages == ['de', 'en', 'es', 'fr', 'it', 'ja', 'pt', 'zh']
        assert sha == hashlib.sha256(sample_bytes()).hexdigest()
        assert date == 'Mon, 28 Sep 2026 15:20:37 GMT'

    def test_what_it_keeps_is_there_to_be_looked_up(self, world):
        world.package()
        world.importer().run(world.job(), nothing)
        # the lemma and the inflected form, by what they read as
        assert world.query("SELECT word, pos FROM dictionary_entries WHERE lang = 'pt' AND norm = 'correr'") == [('correr', 'verb')]
        assert world.query("SELECT data->'senses'->0->'form_of'->0->>'word' FROM dictionary_entries WHERE lang = 'pt' AND norm = 'correram'") == [('correr',)]
        assert world.query("SELECT word FROM dictionary_entries WHERE lang = 'pt' AND norm = 'acao' ORDER BY pos") == [('ação',), ('ação',)]
        # the forms of a lemma point back to it
        assert world.query("SELECT lemma FROM dictionary_forms WHERE lang = 'pt' AND norm = 'corro'") == [('correr',)]
        # a translation turned around finds the lemma, and the Japanese word is found as it is written
        assert ('pt', 'correr') in world.query("SELECT target_lang, target_word FROM dictionary_links WHERE lang = 'ja' AND norm = '走る'")
        # the words of the other languages are there, and the one that is not kept is not
        assert world.query("SELECT count(*) FROM dictionary_entries WHERE lang = 'zh'")[0][0] >= 4
        assert world.query("SELECT count(*) FROM dictionary_entries WHERE lang = 'gl'")[0][0] == 0
        # the accents of the data are the data's
        assert world.query("SELECT data->'senses'->0->'glosses'->>0 FROM dictionary_entries WHERE lang = 'pt' AND norm = 'saudade'")[0][0].startswith('memória')
        # every row belongs to the package
        assert world.query("SELECT count(*) FROM dictionary_entries WHERE package_id <> 'wikt-pt'")[0][0] == 0

    def test_a_second_installation_replaces_the_first_and_does_not_double_it(self, world):
        world.package()
        first = world.importer().run(world.job(), nothing)
        world.execute("UPDATE dictionary_packages SET state = 'installing' WHERE id = 'wikt-pt'")
        second = world.importer().run(world.job(), nothing)
        assert first['entries'] == second['entries']
        assert world.query('SELECT count(*) FROM dictionary_entries')[0][0] == second['entries']
        assert world.query('SELECT count(*) FROM dictionary_forms')[0][0] == second['forms']
        assert world.query('SELECT count(*) FROM dictionary_links')[0][0] == second['links']

    def test_the_file_it_downloaded_is_not_kept(self, world):
        world.package()
        world.importer().run(world.job(), nothing)
        assert os.listdir(world.tmp) == []

    def test_says_where_it_is_while_it_downloads_and_while_it_imports(self, world, monkeypatch):
        world.package()
        monkeypatch.setattr(dictionary, 'CHUNK', 1024)
        monkeypatch.setattr(dictionary, 'BATCH', 5)
        importer = world.importer()
        importer.PROGRESS_EVERY = 4
        seen = []

        def checkpoint():
            row = world.query("SELECT stage, progress, bytes_total, bytes_done, entries FROM dictionary_packages WHERE id = 'wikt-pt'")[0]
            seen.append(row)
        importer.run(world.job(), checkpoint)
        stages = [r[0] for r in seen]
        assert 'downloading' in stages and 'importing' in stages
        assert stages.index('downloading') < stages.index('importing')
        downloading = [r for r in seen if r[0] == 'downloading']
        assert downloading[0][2] == len(sample_bytes())  # the size the source said
        assert max(r[3] for r in downloading) > 0
        assert any(0 < r[1] < 1 for r in downloading)  # and how far it was while it was still coming
        importing = [r for r in seen if r[0] == 'importing']
        assert max(r[1] for r in importing) > 0 and max(r[4] for r in importing) > 0
        assert all(0 <= r[1] <= 1 for r in seen)


class TestWhatIsNotLost:
    def test_the_old_dictionary_stays_when_the_new_one_fails(self, world):
        world.package('ready')
        world.execute("INSERT INTO dictionary_entries (package_id, lang, word, norm, data) VALUES ('wikt-pt', 'pt', 'antigo', 'antigo', '{}')")
        world.execute("UPDATE dictionary_packages SET state = 'installing' WHERE id = 'wikt-pt'")
        broken = sample_bytes()[:-200]
        with pytest.raises(ValueError):
            world.importer(serving(broken)).run(world.job(), nothing)
        assert world.query("SELECT word FROM dictionary_entries") == [('antigo',)]

    def test_the_old_dictionary_stays_when_the_installation_is_cancelled_half_way(self, world, monkeypatch):
        world.package('ready')
        world.execute("INSERT INTO dictionary_entries (package_id, lang, word, norm, data) VALUES ('wikt-pt', 'pt', 'antigo', 'antigo', '{}')")
        world.execute("UPDATE dictionary_packages SET state = 'installing' WHERE id = 'wikt-pt'")
        monkeypatch.setattr(dictionary, 'BATCH', 5)
        calls = {'n': 0}

        def checkpoint():
            calls['n'] += 1
            if calls['n'] > 6:  # after the download, in the middle of the import
                raise Cancelled()
        with pytest.raises(Cancelled):
            world.importer().run(world.job(), checkpoint)
        assert world.query("SELECT word FROM dictionary_entries") == [('antigo',)]
        state, stage, error = world.query("SELECT state, stage, error FROM dictionary_packages WHERE id = 'wikt-pt'")[0]
        assert (state, stage) == ('failed', 'cancelled') and error
        assert os.listdir(world.tmp) == []

    def test_a_cancel_during_the_download_stops_it(self, world, monkeypatch):
        world.package()
        monkeypatch.setattr(dictionary, 'CHUNK', 512)
        calls = {'n': 0}

        def checkpoint():
            calls['n'] += 1
            if calls['n'] == 3:
                raise Cancelled()
        with pytest.raises(Cancelled):
            world.importer().run(world.job(), checkpoint)
        assert world.query('SELECT count(*) FROM dictionary_entries')[0][0] == 0
        assert world.query("SELECT state, stage FROM dictionary_packages")[0] == ('failed', 'cancelled')

    def test_the_failure_is_said_in_the_package_and_the_retry_too(self, world):
        world.package()
        importer = world.importer()
        importer.retrying(world.job(), 'ConnectionResetError: x')
        assert world.query("SELECT state, stage, error FROM dictionary_packages")[0] == ('installing', 'retrying', 'ConnectionResetError: x')
        importer.fail(world.job(), 'OSError: ' + 'y' * 800)
        state, stage, error = world.query("SELECT state, stage, error FROM dictionary_packages")[0]
        assert state == 'failed' and len(error) == 500


class TestWhatItRefuses:
    def test_an_address_that_is_not_the_catalogs(self, world):
        world.package()
        for url in ('http://kaikki.org/x.gz', 'https://evil.example/x.gz', 'https://kaikki.org.evil.example/x.gz',
                    'https://user@kaikki.org/x.gz', 'https://kaikki.org:8443/x.gz', 'file:///etc/passwd', 'ftp://kaikki.org/x'):
            with pytest.raises(ValueError):
                world.importer().run(world.job(url=url), nothing)
        assert world.query('SELECT count(*) FROM dictionary_entries')[0][0] == 0

    def test_a_redirect_that_leads_somewhere_else(self, world):
        world.package()
        with pytest.raises(ValueError):
            world.importer(serving(url='https://evil.example/pt-extract.jsonl.gz')).run(world.job(), nothing)

    def test_a_job_that_names_nothing(self, world):
        world.package()
        for payload in ({}, {'package': 'wikt-pt'}, {'url': ADDRESS}):
            with pytest.raises(ValueError, match='names no package or address'):
                world.importer().run({'id': 1, 'payload': payload}, nothing)

    def test_a_package_that_is_not_being_installed(self, world):
        world.package('ready')
        with pytest.raises(ValueError, match='not being installed'):
            world.importer().run(world.job(), nothing)
        with pytest.raises(ValueError, match='not being installed'):
            world.importer().run(world.job(package='wikt-xx'), nothing)

    def test_a_file_that_is_not_the_dictionary_asked_for(self, world):
        world.package()
        only_galician = gzip.compress(b'{"word": "casa", "lang_code": "gl", "senses": [{"glosses": ["lar"]}]}\n')
        with pytest.raises(ValueError, match='no entry'):
            world.importer(serving(only_galician)).run(world.job(), nothing)
        with pytest.raises(ValueError, match='no entry'):
            world.importer(serving(gzip.compress(b''))).run(world.job(), nothing)

    def test_a_file_that_is_not_gzip(self, world):
        world.package()
        with pytest.raises(ValueError, match='gzip'):
            world.importer(serving(b'<html>not found</html>')).run(world.job(), nothing)

    def test_the_lines_that_are_not_entries_are_counted_and_do_not_stop_it(self, world, monkeypatch):
        world.package()
        monkeypatch.setattr(dictionary, 'MAX_LINE', 400)
        outcome = world.importer().run(world.job(), nothing)
        assert outcome['skipped'] > 1  # the line that is not JSON, and the ones that are too long
        assert outcome['entries'] > 0


class TestHowItFails:
    def test_a_source_that_does_not_have_the_file_is_a_failure_that_will_not_change(self, world):
        world.package()

        def gone(request, timeout=None):
            raise urllib.error.HTTPError(ADDRESS, 404, 'Not Found', {}, None)
        with pytest.raises(PermanentDownloadError) as err:
            world.importer(gone).run(world.job(), nothing)
        assert classify(err.value) == 'permanent'

    def test_a_source_that_is_busy_or_limits_is_tried_again_later(self, world):
        world.package()
        for code in (408, 429, 500, 502, 503):
            def busy(request, timeout=None, code=code):
                raise urllib.error.HTTPError(ADDRESS, code, 'x', {}, None)
            with pytest.raises(urllib.error.HTTPError) as err:
                world.importer(busy).run(world.job(), nothing)
            assert classify(err.value) == 'temporary'

    def test_a_download_that_stops_short_is_tried_again(self, world):
        world.package()
        data = sample_bytes()
        with pytest.raises(OSError, match='stopped') as err:
            world.importer(serving(data[:1000], length=len(data))).run(world.job(), nothing)
        assert classify(err.value) == 'temporary'
        assert world.query('SELECT count(*) FROM dictionary_entries')[0][0] == 0

    def test_a_network_that_drops_is_tried_again(self, world):
        world.package()

        def down(request, timeout=None):
            raise urllib.error.URLError('no route')
        with pytest.raises(urllib.error.URLError) as err:
            world.importer(down).run(world.job(), nothing)
        assert classify(err.value) == 'temporary'
        assert os.listdir(world.tmp) == []

    def test_a_file_that_is_not_the_right_one_is_a_failure_that_will_not_change(self, world):
        world.package()
        with pytest.raises(ValueError) as err:
            world.importer(serving(b'nope')).run(world.job(), nothing)
        assert classify(err.value) == 'permanent'

    def test_asks_the_source_who_it_is(self, world):
        world.package()
        seen = {}

        def look(request, timeout=None):
            seen['agent'] = request.get_header('User-agent')
            seen['url'] = request.full_url
            seen['timeout'] = timeout
            return Response(sample_bytes())
        world.importer(look).run(world.job(), nothing)
        assert seen['agent'].startswith('Codice-dictionary-installer') and seen['url'] == ADDRESS and seen['timeout'] == 60
