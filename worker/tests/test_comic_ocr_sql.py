"""The statements of the reading of comics, run for real (DEC-176). Needs a PostgreSQL (TEST_DATABASE_URL) and skips without one; it makes
its own temporary tables (the one of the pages from the very text of its migration), so it changes nothing that exists."""
import json
import os
import re

import psycopg2
import pytest

from comic_ocr import ComicOcrIndexer, FILES_TO_READ, SAVE_PAGE
from textindex.store import TextIndexer

URL = os.environ.get('TEST_DATABASE_URL')
pytestmark = pytest.mark.skipif(not URL, reason='TEST_DATABASE_URL is not set')

MIGRATION = os.path.join(os.path.dirname(__file__), '..', '..', 'backend', 'internal', 'database', 'migrations', '00079_comic_ocr_pages.sql')


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


def pages_table_from_the_migration():
    up = open(MIGRATION).read().split('-- +goose Down')[0].replace('-- +goose Up', '')
    up = re.sub(r'^--.*$', '', up, flags=re.M)
    # a temporary table has no foreign key to a table that is not the real one
    up = re.sub(r'REFERENCES files\(id\) ON DELETE CASCADE', '', up)
    return up.replace('CREATE TABLE comic_ocr_pages', 'CREATE TEMP TABLE comic_ocr_pages').replace(
        'CREATE INDEX comic_ocr_pages_model', 'CREATE INDEX comic_ocr_pages_model_temp')


@pytest.fixture
def db():
    conn = psycopg2.connect(URL)
    cur = conn.cursor()
    cur.execute("""
        CREATE TEMP TABLE settings (key TEXT PRIMARY KEY, value JSONB NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
        CREATE TEMP TABLE works (id INTEGER PRIMARY KEY, retired_at TIMESTAMPTZ, page_count INTEGER DEFAULT 0);
        CREATE TEMP TABLE editions (id INTEGER PRIMARY KEY, work_id INTEGER NOT NULL);
        CREATE TEMP TABLE files (id BIGINT PRIMARY KEY, edition_id INTEGER NOT NULL, format TEXT, sha256 CHAR(64),
                                 availability TEXT NOT NULL DEFAULT 'available');
        CREATE TEMP TABLE storage_locations (id SERIAL PRIMARY KEY, file_id BIGINT NOT NULL, mode TEXT, root TEXT, path TEXT);
        CREATE TEMP TABLE jobs (id BIGSERIAL PRIMARY KEY, type VARCHAR(32) NOT NULL, work_id INTEGER, payload JSONB NOT NULL DEFAULT '{}',
                                priority SMALLINT NOT NULL DEFAULT 0, state VARCHAR(12) NOT NULL DEFAULT 'pending',
                                updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
        CREATE UNIQUE INDEX jobs_one_open ON jobs (type, work_id) WHERE state IN ('pending', 'running');
    """)
    cur.execute(pages_table_from_the_migration())
    yield Db(conn)
    conn.rollback()
    conn.close()


class Engine:
    name = 'paddleocr'
    loaded = True

    def __init__(self, model):
        self.model = model

    def version(self):
        return '3.7.0'


def work(db, work_id, fmt='cbz', pages=10, sha='a' * 64, file_id=None, retired=False, availability='available'):
    file_id = file_id or work_id * 10
    db.execute("INSERT INTO works (id, retired_at, page_count) VALUES (%s, %s, %s)", (work_id, 'now()' if retired else None, pages))
    db.execute("INSERT INTO editions (id, work_id) VALUES (%s, %s)", (work_id, work_id))
    db.execute("INSERT INTO files (id, edition_id, format, sha256, availability) VALUES (%s, %s, %s, %s, %s)",
               (file_id, work_id, fmt, sha, availability))
    db.execute("INSERT INTO storage_locations (file_id, mode, root, path) VALUES (%s, 'managed', NULL, %s)", (file_id, f'{file_id}.{fmt}'))
    return file_id


def read(db, file_id, pages, sha='a' * 64, model='mobile'):
    for page in range(pages):
        db.execute(SAVE_PAGE, (file_id, page, sha, 'blank', '[]', 100, 100, model, 'paddleocr', '3.7', 1.0, None))


def queue(db):
    db.execute("INSERT INTO settings (key, value) VALUES ('comic_ocr', '{\"model\": \"mobile\"}') ON CONFLICT (key) DO NOTHING")
    ix = ComicOcrIndexer(db, '/tmp', engine_factory=Engine, log=lambda *a: None)
    ix.enqueue_missing()
    return sorted(w for (w,) in db.fetchall("SELECT work_id FROM jobs WHERE type = 'comic_ocr' AND state = 'pending'"))


class TestWhatIsQueued:
    def test_a_comic_nobody_read_is_queued_and_asking_again_is_the_same_job(self, db):
        work(db, 1)
        assert queue(db) == [1]
        assert queue_again(db) == [1]

    def test_a_comic_whose_pages_were_all_read_is_not_queued_and_one_half_read_is(self, db):
        done = work(db, 1, pages=4)
        half = work(db, 2, pages=4)
        read(db, done, 4)
        read(db, half, 2)
        assert queue(db) == [2]

    def test_pages_read_with_another_model_count_as_read(self, db):
        file_id = work(db, 1, pages=3)
        read(db, file_id, 3, model='server')
        assert queue(db) == []

    def test_what_was_read_of_another_version_of_the_file_does_not_count(self, db):
        file_id = work(db, 1, pages=3, sha='b' * 64)
        read(db, file_id, 3, sha='a' * 64)
        assert queue(db) == [1]

    def test_only_comics_that_are_here_and_whose_work_is_not_retired(self, db):
        work(db, 1)
        work(db, 2, fmt='epub')
        work(db, 3, retired=True)
        work(db, 4, availability='missing')
        work(db, 5, pages=0)
        work(db, 6, fmt='cbr')
        assert queue(db) == [1, 6]

    def test_a_work_whose_job_failed_a_moment_ago_is_left_alone_for_a_while(self, db):
        work(db, 1)
        db.execute("INSERT INTO jobs (type, work_id, state) VALUES ('comic_ocr', 1, 'failed')")
        assert queue(db) == []
        db.execute("UPDATE jobs SET updated_at = now() - interval '2 hours'")
        assert queue(db) == [1]


def queue_again(db):
    ix = ComicOcrIndexer(db, '/tmp', engine_factory=Engine, log=lambda *a: None)
    ix.enqueue_missing()
    return sorted(w for (w,) in db.fetchall("SELECT work_id FROM jobs WHERE type = 'comic_ocr' AND state = 'pending'"))


class TestWhatIsRead:
    def test_the_files_of_a_work_are_its_comics_that_are_here(self, db):
        work(db, 1)
        work(db, 1 + 100, fmt='pdf')
        rows = db.fetchall(FILES_TO_READ, (1,))
        assert [(r[0], r[2], r[3], r[5]) for r in rows] == [(10, 'cbz', 'managed', '10.cbz')]
        assert db.fetchall(FILES_TO_READ, (101,)) == []

    def test_a_page_is_kept_once_and_read_again_replaces_it(self, db):
        file_id = work(db, 1)
        regions = json.dumps([{'box': [1, 2, 3, 4], 'text': 'OLÁ', 'conf': 98.0}], ensure_ascii=False)
        db.execute(SAVE_PAGE, (file_id, 0, 'a' * 64, 'done', regions, 800, 1200, 'mobile', 'paddleocr', '3.7', 2.5, None))
        db.execute(SAVE_PAGE, (file_id, 0, 'a' * 64, 'done', regions, 800, 1200, 'server', 'paddleocr', '3.7', 4.0, None))
        rows = db.fetchall("SELECT model, regions->0->>'text', width FROM comic_ocr_pages WHERE file_id = %s", (file_id,))
        assert rows == [('server', 'OLÁ', 800)]

    def test_a_page_with_speech_has_regions_and_the_count_of_pages_read_is_by_model(self, db):
        file_id = work(db, 1)
        with pytest.raises(psycopg2.errors.CheckViolation):
            db.execute(SAVE_PAGE, (file_id, 0, 'a' * 64, 'done', '[]', 10, 10, 'mobile', 'paddleocr', '3', 1.0, None))


class TestWhatTheTextIsMadeFrom:
    def test_only_the_pages_with_speech_of_the_file_as_it_is_now(self, db):
        file_id = work(db, 1)
        speech_ = json.dumps([{'box': [1, 2, 3, 4], 'text': 'OLÁ', 'conf': 98.0}], ensure_ascii=False)
        db.execute(SAVE_PAGE, (file_id, 0, 'a' * 64, 'done', speech_, 800, 1200, 'mobile', 'paddleocr', '3', 1.0, None))
        db.execute(SAVE_PAGE, (file_id, 1, 'a' * 64, 'blank', '[]', 800, 1200, 'mobile', 'paddleocr', '3', 1.0, None))
        db.execute(SAVE_PAGE, (file_id, 2, 'b' * 64, 'done', speech_, 800, 1200, 'mobile', 'paddleocr', '3', 1.0, None))
        got = TextIndexer(db, '/tmp').comic_pages(file_id, 'a' * 64)
        assert list(got) == [0]
        assert got[0] == {'regions': [{'box': [1, 2, 3, 4], 'text': 'OLÁ', 'conf': 98.0}], 'width': 800, 'height': 1200}
