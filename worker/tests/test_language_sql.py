"""The statement that gives an edition the language found in its text, run for real (#35).

It needs a PostgreSQL to run against (TEST_DATABASE_URL, as the backend's tests do) and skips without one.
It makes its own temporary tables, with only the columns the statement touches, so it changes nothing that
exists and disappears with the connection."""
import os

import psycopg2
import pytest

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
        return self.cur.fetchone() if self.cur.description else None


@pytest.fixture
def db():
    conn = psycopg2.connect(URL)
    cur = conn.cursor()
    cur.execute("""
        CREATE TEMP TABLE works (id INTEGER PRIMARY KEY, language_lock BOOLEAN NOT NULL DEFAULT FALSE);
        CREATE TEMP TABLE editions (id INTEGER PRIMARY KEY, work_id INTEGER, language VARCHAR(16));
        CREATE TEMP TABLE files (id INTEGER PRIMARY KEY, edition_id INTEGER);
        CREATE TEMP TABLE work_field_sources (
            work_id INTEGER, field VARCHAR(32), source VARCHAR(32), updated_at TIMESTAMPTZ DEFAULT now(),
            PRIMARY KEY (work_id, field));
    """)
    yield Db(conn)
    conn.rollback()
    conn.close()


def seed(db, language=None, locked=False, source=None):
    db.execute("INSERT INTO works (id, language_lock) VALUES (1, %s)", (locked,))
    db.execute("INSERT INTO editions (id, work_id, language) VALUES (10, 1, %s)", (language,))
    db.execute("INSERT INTO files (id, edition_id) VALUES (100, 10)")
    if source:
        db.execute("INSERT INTO work_field_sources (work_id, field, source) VALUES (1, 'language', %s)", (source,))


def state(db):
    db.execute("SELECT language FROM editions WHERE id = 10")
    language = db.cur.fetchone()[0]
    db.execute("SELECT source FROM work_field_sources WHERE work_id = 1 AND field = 'language'")
    row = db.cur.fetchone()
    return language, row[0] if row else None


def fill(db):
    TextIndexer(db, '/nowhere').fill_language(100, 'pt')


def test_an_edition_with_no_language_gets_it_and_the_source_says_it_was_detected(db):
    seed(db)
    fill(db)
    assert state(db) == ('pt', 'detected')


def test_an_empty_string_is_no_language_either(db):
    seed(db, language='')
    fill(db)
    assert state(db) == ('pt', 'detected')


def test_a_declared_language_is_not_replaced_and_nothing_is_recorded(db):
    seed(db, language='en-GB')
    fill(db)
    assert state(db) == ('en-GB', None)


def test_a_locked_language_is_not_filled(db):
    seed(db, locked=True)
    fill(db)
    assert state(db) == (None, None)


def test_the_source_always_says_where_a_value_that_was_just_written_came_from(db):
    seed(db, source='manual')  # someone cleared it: what is there now is not what they wrote
    fill(db)
    assert state(db) == ('pt', 'detected')


def test_an_earlier_guess_or_reading_is_refreshed(db):
    seed(db, source='file')
    fill(db)
    assert state(db) == ('pt', 'detected')


def test_only_the_edition_of_that_file_is_filled_not_the_others_of_the_work(db):
    seed(db)
    db.execute("INSERT INTO editions (id, work_id, language) VALUES (11, 1, NULL)")
    db.execute("INSERT INTO files (id, edition_id) VALUES (101, 11)")
    fill(db)
    db.execute("SELECT id, language FROM editions ORDER BY id")
    assert db.cur.fetchall() == [(10, 'pt'), (11, None)]
