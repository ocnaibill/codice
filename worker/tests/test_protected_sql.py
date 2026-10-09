"""The statement that marks the file of a work as asking for a password, run for real (#89).

It needs a PostgreSQL to run against (TEST_DATABASE_URL, as the backend's tests do) and skips without one. It makes its own
temporary tables and a temporary view with the shape of `work_primary`, so it changes nothing that exists."""
import os

import psycopg2
import pytest

from analyzer import Analyzer

URL = os.environ.get('TEST_DATABASE_URL')
pytestmark = pytest.mark.skipif(not URL, reason='TEST_DATABASE_URL is not set')


class Db:
    def __init__(self, conn):
        self.cur = conn.cursor()

    def execute(self, query, params=()):
        self.cur.execute(query, params)

    def protected(self):
        self.cur.execute("SELECT id FROM files WHERE protected ORDER BY id")
        return [row[0] for row in self.cur.fetchall()]


@pytest.fixture
def db():
    conn = psycopg2.connect(URL)
    cur = conn.cursor()
    cur.execute("""
        CREATE TEMP TABLE works (id INTEGER PRIMARY KEY);
        CREATE TEMP TABLE editions (id INTEGER PRIMARY KEY, work_id INTEGER, is_primary BOOLEAN NOT NULL DEFAULT TRUE);
        CREATE TEMP TABLE files (id INTEGER PRIMARY KEY, edition_id INTEGER, protected BOOLEAN NOT NULL DEFAULT FALSE);
        CREATE TEMP VIEW work_primary AS
            SELECT w.id AS work_id, e.id AS edition_id, f.id AS file_id
            FROM works w
            LEFT JOIN editions e ON e.work_id = w.id AND e.is_primary
            LEFT JOIN LATERAL (SELECT id FROM files WHERE edition_id = e.id ORDER BY id LIMIT 1) f ON TRUE;
        INSERT INTO works (id) VALUES (1), (2);
        INSERT INTO editions (id, work_id, is_primary) VALUES (10, 1, TRUE), (11, 1, FALSE), (20, 2, TRUE);
        INSERT INTO files (id, edition_id) VALUES (100, 10), (101, 11), (200, 20);
    """)
    yield Db(conn)
    conn.rollback()
    conn.close()


def test_marks_the_primary_file_of_the_work_and_nothing_else(db):
    Analyzer(db).mark_protected(1)
    assert db.protected() == [100]


def test_marks_the_work_that_was_asked_for_only(db):
    Analyzer(db).mark_protected(2)
    assert db.protected() == [200]


def test_marking_twice_is_the_same_as_once_and_a_work_with_no_file_marks_nothing(db):
    Analyzer(db).mark_protected(1)
    Analyzer(db).mark_protected(1)
    assert db.protected() == [100]
    db.execute("INSERT INTO works (id) VALUES (3)")
    Analyzer(db).mark_protected(3)
    assert db.protected() == [100]
