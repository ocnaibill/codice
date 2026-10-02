"""The statements of the OCR job, run for real (#24).

They need a PostgreSQL to run against (TEST_DATABASE_URL, as the backend's tests do) and skip without one. They make their
own temporary tables, with only the columns the statements touch (the constraints of ocr_pages are the migration's),
so they change nothing that exists and disappear with the connection."""
import json
import os

import psycopg2
import pytest

import ocr
from ocr import OcrIndexer, SAVE_PAGE
from textindex.store import TextIndexer

URL = os.environ.get('TEST_DATABASE_URL')
pytestmark = pytest.mark.skipif(not URL, reason='TEST_DATABASE_URL is not set')


class Db:
    def __init__(self, conn):
        self.cur = conn.cursor()

    def execute(self, query, params=()):
        self.cur.execute(query, params)

    def fetchone(self, query, params=()):
        self.cur.execute(query, params)
        return self.cur.fetchone()

    def fetchall(self, query, params=()):
        self.cur.execute(query, params)
        return self.cur.fetchall()


class NoEngine:
    name = 'tesseract'

    def installed(self):
        return True

    def version(self):
        return '5.3.0'

    def languages(self):
        return ['eng', 'por']


@pytest.fixture
def db():
    conn = psycopg2.connect(URL)
    cur = conn.cursor()
    cur.execute("""
        CREATE TEMP TABLE settings (key TEXT PRIMARY KEY, value JSONB NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
        CREATE TEMP TABLE works (id INTEGER PRIMARY KEY, retired_at TIMESTAMPTZ);
        CREATE TEMP TABLE editions (id INTEGER PRIMARY KEY, work_id INTEGER);
        CREATE TEMP TABLE files (id INTEGER PRIMARY KEY, edition_id INTEGER, availability VARCHAR(16) NOT NULL DEFAULT 'available', sha256 CHAR(64));
        CREATE TEMP TABLE text_layers (file_id INTEGER PRIMARY KEY, needs_ocr BOOLEAN NOT NULL, pages_without_text INTEGER[] NOT NULL);
        CREATE TEMP TABLE jobs (id BIGSERIAL PRIMARY KEY, type VARCHAR(32) NOT NULL, work_id INTEGER, payload JSONB NOT NULL DEFAULT '{}',
            priority SMALLINT NOT NULL DEFAULT 0, state VARCHAR(12) NOT NULL DEFAULT 'pending', updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
        CREATE UNIQUE INDEX ON jobs (type, work_id) WHERE state IN ('pending', 'running');
        CREATE TEMP TABLE ocr_pages (
            file_id BIGINT NOT NULL, page INTEGER NOT NULL CHECK (page >= 0), source_sha256 CHAR(64),
            state VARCHAR(8) NOT NULL CHECK (state IN ('done', 'blank', 'failed')), text TEXT NOT NULL DEFAULT '',
            engine VARCHAR(32) NOT NULL, engine_version VARCHAR(32) NOT NULL DEFAULT '', language VARCHAR(32) NOT NULL DEFAULT '',
            dpi SMALLINT, error TEXT, recognized_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            PRIMARY KEY (file_id, page), CHECK (state <> 'done' OR text <> ''));
        INSERT INTO settings (key, value) VALUES ('ocr', '{"enabled": true, "language": "por+eng"}');
    """)
    yield Db(conn)
    conn.rollback()
    conn.close()


SHA = 'a' * 64


def book(db, work=1, pages=(1, 2, 3), needs=True, retired=False, available=True, sha=SHA):
    db.execute("INSERT INTO works (id, retired_at) VALUES (%s, %s)", (work, '2026-01-01' if retired else None))
    db.execute("INSERT INTO editions (id, work_id) VALUES (%s, %s)", (work * 10, work))
    db.execute("INSERT INTO files (id, edition_id, availability, sha256) VALUES (%s, %s, %s, %s)",
               (work * 100, work * 10, 'available' if available else 'missing', sha))
    db.execute("INSERT INTO text_layers (file_id, needs_ocr, pages_without_text) VALUES (%s, %s, %s)", (work * 100, needs, list(pages)))
    return work * 100


def keep(db, file_id, page, state='done', sha=SHA, text='um texto'):
    db.execute(SAVE_PAGE, (file_id, page, sha, state, text if state == 'done' else '', 'tesseract', '5.3.0', 'por', 200, None))


def queued(db):
    return [r[0] for r in db.fetchall("SELECT work_id FROM jobs WHERE type = 'ocr' ORDER BY work_id")]


def enqueue(db):
    OcrIndexer(db, '/nowhere', NoEngine(), log=lambda *a: None).enqueue_missing()


class TestQueue:
    def test_a_work_with_pages_nobody_has_read_is_queued_once_however_often_it_is_asked(self, db):
        book(db)
        enqueue(db)
        enqueue(db)
        assert queued(db) == [1]
        assert db.fetchone("SELECT priority FROM jobs")[0] == -20  # behind what people asked for

    def test_what_has_no_pages_to_read_is_not_queued(self, db):
        book(db, work=1, needs=False)
        book(db, work=2, retired=True)
        book(db, work=3, available=False)
        enqueue(db)
        assert queued(db) == []

    def test_a_work_whose_pages_were_all_asked_about_is_left_alone_failures_included(self, db):
        f = book(db, pages=(1, 2, 3))
        keep(db, f, 0, 'done')
        keep(db, f, 1, 'blank')
        keep(db, f, 2, 'failed')
        enqueue(db)
        assert queued(db) == []

    def test_a_work_with_a_page_left_is_queued(self, db):
        f = book(db, pages=(1, 2, 3))
        keep(db, f, 0)
        keep(db, f, 2)
        enqueue(db)
        assert queued(db) == [1]

    def test_the_pages_are_counted_from_one_there_and_from_zero_here(self, db):
        f = book(db, pages=(5,))
        keep(db, f, 4)   # the fifth page
        enqueue(db)
        assert queued(db) == []
        keep(db, f, 0)   # the first page is not one of them: it does not count as the one asked about
        db.execute("DELETE FROM ocr_pages WHERE page = 4")
        enqueue(db)
        assert queued(db) == [1]

    def test_what_was_read_of_another_version_of_the_file_does_not_count(self, db):
        f = book(db, pages=(1, 2), sha=SHA)
        keep(db, f, 0, sha='b' * 64)
        keep(db, f, 1, sha='b' * 64)
        enqueue(db)
        assert queued(db) == [1]

    def test_a_file_without_a_hash_is_matched_with_pages_without_one(self, db):
        f = book(db, pages=(1,), sha=None)
        keep(db, f, 0, sha=None)
        enqueue(db)
        assert queued(db) == []

    def test_a_file_with_more_pages_than_one_job_reads_is_not_queued(self, db, monkeypatch):
        book(db, pages=(1, 2, 3))
        monkeypatch.setattr(ocr, 'MAX_PAGES', 2)
        enqueue(db)
        assert queued(db) == []

    def test_a_work_whose_job_just_failed_is_left_alone_for_an_hour_and_then_tried_again(self, db):
        book(db)
        db.execute("INSERT INTO jobs (type, work_id, state, updated_at) VALUES ('ocr', 1, 'failed', now() - interval '10 minutes')")
        enqueue(db)
        assert db.fetchall("SELECT state FROM jobs WHERE type = 'ocr'") == [('failed',)]
        db.execute("UPDATE jobs SET updated_at = now() - interval '2 hours'")
        enqueue(db)
        assert sorted(r[0] for r in db.fetchall("SELECT state FROM jobs WHERE type = 'ocr'")) == ['failed', 'pending']

    def test_a_job_of_another_kind_does_not_stop_it(self, db):
        book(db)
        db.execute("INSERT INTO jobs (type, work_id, state) VALUES ('extract_text', 1, 'pending')")
        enqueue(db)
        assert queued(db) == [1]

    def test_nothing_is_queued_while_it_is_off(self, db):
        book(db)
        db.execute("UPDATE settings SET value = '{\"enabled\": false}' WHERE key = 'ocr'")
        enqueue(db)
        assert queued(db) == []
        assert json.loads(json.dumps(db.fetchone("SELECT value FROM settings WHERE key = 'ocr.worker'")[0]))['engine'] == 'tesseract'


class TestPages:
    def test_a_page_read_again_replaces_what_was_kept(self, db):
        keep(db, 1, 0, 'failed')
        keep(db, 1, 0, 'done', text='agora sim')
        assert db.fetchall("SELECT state, text, error FROM ocr_pages") == [('done', 'agora sim', None)]

    def test_the_kept_page_says_how_it_was_read(self, db):
        keep(db, 1, 3)
        assert db.fetchone("SELECT engine, engine_version, language, dpi FROM ocr_pages") == ('tesseract', '5.3.0', 'por', 200)

    def test_a_done_page_has_text_a_blank_one_has_none_and_a_failed_one_says_why(self, db):
        with pytest.raises(psycopg2.errors.CheckViolation):
            keep(db, 1, 0, 'done', text='')
        db.cur.connection.rollback()

    def test_the_text_of_a_file_is_made_from_the_done_pages_of_the_file_as_it_is(self, db):
        keep(db, 1, 0, 'done', text='primeira')
        keep(db, 1, 1, 'blank')
        keep(db, 1, 2, 'failed')
        keep(db, 1, 3, 'done', sha='b' * 64, text='de outra versão')
        keep(db, 2, 0, 'done', text='de outro arquivo')
        assert TextIndexer(db, '/nowhere').recognised_pages(1, SHA) == {0: 'primeira'}

    def test_a_file_without_a_hash_gets_the_pages_kept_without_one(self, db):
        keep(db, 1, 0, 'done', sha=None, text='sem hash')
        keep(db, 1, 1, 'done', sha=SHA, text='com hash')
        assert TextIndexer(db, '/nowhere').recognised_pages(1, None) == {0: 'sem hash'}
