"""The statement that proposes the language found in a file's text, run for real (#35).

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


@pytest.fixture
def db():
    conn = psycopg2.connect(URL)
    cur = conn.cursor()
    cur.execute("""
        CREATE TEMP TABLE works (id INTEGER PRIMARY KEY, language_lock BOOLEAN NOT NULL DEFAULT FALSE);
        CREATE TEMP TABLE editions (id INTEGER PRIMARY KEY, work_id INTEGER, language VARCHAR(16), is_primary BOOLEAN NOT NULL DEFAULT TRUE);
        CREATE TEMP TABLE files (id INTEGER PRIMARY KEY, edition_id INTEGER);
        CREATE TEMP TABLE metadata_candidates (
            id BIGSERIAL PRIMARY KEY, work_id INTEGER NOT NULL, field VARCHAR(32) NOT NULL, value TEXT NOT NULL,
            source VARCHAR(32) NOT NULL, evidence JSONB NOT NULL DEFAULT '{}',
            state VARCHAR(12) NOT NULL DEFAULT 'pending', UNIQUE (work_id, field, source, value));
    """)
    yield Db(conn)
    conn.rollback()
    conn.close()


def seed(db, language=None, locked=False, primary=True):
    db.execute("INSERT INTO works (id, language_lock) VALUES (1, %s)", (locked,))
    db.execute("INSERT INTO editions (id, work_id, language, is_primary) VALUES (10, 1, %s, %s)", (language, primary))
    db.execute("INSERT INTO files (id, edition_id) VALUES (100, 10)")


def suggest(db, language='pt'):
    TextIndexer(db, '/nowhere').suggest_language(100, language)


def candidates(db):
    db.execute("SELECT work_id, field, value, source, state FROM metadata_candidates ORDER BY id")
    return db.cur.fetchall()


def edition_language(db):
    db.execute("SELECT language FROM editions WHERE id = 10")
    return db.cur.fetchone()[0]


def test_an_edition_with_no_language_gets_a_pending_suggestion_and_keeps_being_empty(db):
    seed(db)
    suggest(db)
    assert candidates(db) == [(1, 'language', 'pt', 'detected', 'pending')]
    assert edition_language(db) is None  # nothing is applied until someone accepts it


def test_an_empty_string_is_no_language_either(db):
    seed(db, language='')
    suggest(db)
    assert candidates(db) == [(1, 'language', 'pt', 'detected', 'pending')]


def test_a_declared_language_gets_no_suggestion(db):
    seed(db, language='en-GB')
    suggest(db)
    assert candidates(db) == []


def test_a_locked_language_gets_no_suggestion(db):
    seed(db, locked=True)
    suggest(db)
    assert candidates(db) == []


def test_only_the_primary_edition_is_suggested_for_because_that_is_what_accepting_changes(db):
    seed(db, primary=False)
    suggest(db)
    assert candidates(db) == []


def test_asking_twice_or_after_a_rejection_proposes_nothing_new(db):
    seed(db)
    suggest(db)
    db.execute("UPDATE metadata_candidates SET state = 'rejected'")
    suggest(db)
    suggest(db)
    assert candidates(db) == [(1, 'language', 'pt', 'detected', 'rejected')]


def test_a_different_guess_is_a_new_suggestion(db):
    seed(db)
    suggest(db, 'pt')
    db.execute("UPDATE metadata_candidates SET state = 'rejected'")
    suggest(db, 'es')
    assert [c[2:] for c in candidates(db)] == [('pt', 'detected', 'rejected'), ('es', 'detected', 'pending')]


def test_the_suggestion_says_how_it_was_made(db):
    seed(db)
    suggest(db)
    db.execute("SELECT evidence->>'method' FROM metadata_candidates")
    assert db.cur.fetchone()[0] == 'common words'
