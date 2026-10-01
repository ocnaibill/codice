"""The statements of the author lookup, run for real (#63). Needs a PostgreSQL (TEST_DATABASE_URL) and skips
without one; it makes its own temporary tables, so it changes nothing that exists."""
import os

import psycopg2
import pytest
from unittest.mock import MagicMock

from authority import resolve_pending

URL = os.environ.get('TEST_DATABASE_URL')
pytestmark = pytest.mark.skipif(not URL, reason='TEST_DATABASE_URL is not set')

HERBERT = {'type': {'key': '/type/author'}, 'remote_ids': {'wikidata': 'Q7934', 'viaf': '59083797', 'isni': '0000000121347853'}}


class Db:
    def __init__(self, conn):
        self.cur = conn.cursor()

    def execute(self, query, params=()):
        self.cur.execute(query, params)

    def fetchall(self, query, params=()):
        self.cur.execute(query, params)
        return self.cur.fetchall()


@pytest.fixture
def db():
    conn = psycopg2.connect(URL)
    conn.cursor().execute("""
        CREATE TEMP TABLE person_authority (
            person_id INTEGER NOT NULL, scheme VARCHAR(32) NOT NULL, value VARCHAR(64) NOT NULL,
            source VARCHAR(64) NOT NULL DEFAULT '', PRIMARY KEY (person_id, scheme, value));
        CREATE TEMP TABLE authority_lookups (
            source VARCHAR(32) NOT NULL, key VARCHAR(64) NOT NULL,
            state VARCHAR(12) NOT NULL CHECK (state IN ('done', 'missing', 'redirect', 'failed')),
            attempts SMALLINT NOT NULL DEFAULT 1, attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (source, key));
        CREATE TEMP TABLE jobs (id BIGSERIAL PRIMARY KEY, type VARCHAR(32) NOT NULL, work_id INTEGER,
            payload JSONB NOT NULL DEFAULT '{}', priority SMALLINT NOT NULL DEFAULT 0, state VARCHAR(12) NOT NULL DEFAULT 'pending');
    """)
    yield Db(conn)
    conn.rollback()
    conn.close()


def hold(db, person, scheme, value, source='Open Library'):
    db.execute("INSERT INTO person_authority (person_id, scheme, value, source) VALUES (%s, %s, %s, %s)", (person, scheme, value, source))


def rows(db, query):
    return db.fetchall(query)


def ask(db, answers, **kw):
    get = MagicMock(side_effect=lambda url, **k: answers[url.rsplit('/', 1)[-1].replace('.json', '')])
    return resolve_pending(db, lambda _id: True, get=get, sleep=lambda s: None, **kw), get


def reply(status, doc=None):
    return MagicMock(status_code=status, json=MagicMock(return_value=doc))


def identifiers(db):
    return rows(db, "SELECT person_id, scheme, value, source FROM person_authority WHERE scheme <> 'openlibrary' ORDER BY person_id, scheme")


def test_every_person_holding_the_key_gets_the_identifiers_and_the_lookup_is_remembered(db):
    hold(db, 1, 'openlibrary', 'OL79034A', 'Open Library')
    hold(db, 2, 'openlibrary', 'OL79034A', 'Open Library')
    hold(db, 3, 'openlibrary', 'OL1A', 'Open Library')
    n, get = ask(db, {'OL79034A': reply(200, HERBERT), 'OL1A': reply(404)})
    assert n == 2
    assert identifiers(db) == [
        (1, 'isni', '0000000121347853', 'Open Library'), (1, 'viaf', '59083797', 'Open Library'), (1, 'wikidata', 'Q7934', 'Open Library'),
        (2, 'isni', '0000000121347853', 'Open Library'), (2, 'viaf', '59083797', 'Open Library'), (2, 'wikidata', 'Q7934', 'Open Library')]
    assert rows(db, "SELECT key, state FROM authority_lookups ORDER BY key") == [('OL1A', 'missing'), ('OL79034A', 'done')]
    assert rows(db, "SELECT type, state FROM jobs") == [('dedupe', 'pending')]


def test_a_key_that_was_looked_up_is_not_asked_again_and_a_second_run_does_nothing(db):
    hold(db, 1, 'openlibrary', 'OL79034A')
    ask(db, {'OL79034A': reply(200, HERBERT)})
    n, get = ask(db, {})
    assert n == 0 and not get.called
    assert rows(db, "SELECT count(*) FROM jobs") == [(1,)]


def test_what_a_person_already_held_is_left_and_nothing_is_taken_from_other_sources(db):
    hold(db, 1, 'openlibrary', 'OL79034A')
    hold(db, 1, 'wikidata', 'Q7934', 'someone else')
    hold(db, 1, 'comicvine', '4050-1', 'ComicVine')
    ask(db, {'OL79034A': reply(200, HERBERT)})
    assert (1, 'wikidata', 'Q7934', 'someone else') in identifiers(db)  # the existing row stays as it was
    assert (1, 'comicvine', '4050-1', 'ComicVine') in identifiers(db)
    assert len(identifiers(db)) == 4


def test_a_comparison_is_not_asked_for_twice(db):
    hold(db, 1, 'openlibrary', 'OL1A')
    hold(db, 2, 'openlibrary', 'OL2A')
    db.execute("INSERT INTO jobs (type, state) VALUES ('dedupe', 'running')")
    ask(db, {'OL1A': reply(200, HERBERT), 'OL2A': reply(200, HERBERT)})
    assert rows(db, "SELECT count(*) FROM jobs") == [(1,)]
    db.execute("UPDATE jobs SET state = 'succeeded'")
    hold(db, 3, 'openlibrary', 'OL3A')
    ask(db, {'OL3A': reply(200, HERBERT)})
    assert rows(db, "SELECT count(*) FROM jobs") == [(2,)]


def test_trouble_is_tried_again_after_a_day_and_a_few_times_and_not_before(db):
    hold(db, 1, 'openlibrary', 'OL1A')
    ask(db, {'OL1A': reply(503)})
    assert rows(db, "SELECT state, attempts FROM authority_lookups") == [('failed', 1)]
    n, get = ask(db, {})
    assert not get.called  # it just failed: not again today

    db.execute("UPDATE authority_lookups SET attempted_at = now() - interval '2 days'")
    ask(db, {'OL1A': reply(503)})
    assert rows(db, "SELECT state, attempts FROM authority_lookups") == [('failed', 2)]

    db.execute("UPDATE authority_lookups SET attempted_at = now() - interval '2 days', attempts = 5")
    n, get = ask(db, {})
    assert not get.called  # given up after five

    db.execute("UPDATE authority_lookups SET attempts = 4")
    ask(db, {'OL1A': reply(200, HERBERT)})
    assert rows(db, "SELECT state, attempts FROM authority_lookups") == [('done', 5)]
    assert len(identifiers(db)) == 3


def test_a_merged_author_hands_its_new_key_to_whoever_held_the_old_one_and_the_new_one_is_looked_up_next(db):
    hold(db, 1, 'openlibrary', 'OL5A')
    ask(db, {'OL5A': reply(200, {'type': {'key': '/type/redirect'}, 'location': '/authors/OL6A'})})
    assert rows(db, "SELECT person_id, value, source FROM person_authority WHERE scheme = 'openlibrary' ORDER BY value") == [
        (1, 'OL5A', 'Open Library'), (1, 'OL6A', 'Open Library')]
    assert rows(db, "SELECT key, state FROM authority_lookups") == [('OL5A', 'redirect')]
    n, get = ask(db, {'OL6A': reply(200, HERBERT)})
    assert n == 1 and get.call_count == 1
    assert len(identifiers(db)) == 3


def test_only_as_many_keys_as_it_was_told_in_a_stable_order(db):
    for i, key in enumerate(['OL3A', 'OL1A', 'OL2A'], start=1):
        hold(db, i, 'openlibrary', key)
    n, get = ask(db, {'OL1A': reply(404), 'OL2A': reply(404)}, limit=2)
    assert n == 2
    assert rows(db, "SELECT key FROM authority_lookups ORDER BY key") == [('OL1A',), ('OL2A',)]


def test_only_open_library_keys_are_looked_up(db):
    hold(db, 1, 'comicvine', '4050-1', 'ComicVine')
    hold(db, 2, 'wikidata', 'Q1', 'Open Library')
    n, get = ask(db, {})
    assert n == 0 and not get.called
