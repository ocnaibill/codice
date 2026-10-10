"""The statements of the author profile, run for real against the real migration (DEC-146). Needs a PostgreSQL (TEST_DATABASE_URL) and skips without
one; it works in a schema of its own, which it drops, so it changes nothing that exists."""
import os
import re
import uuid

import psycopg2
import pytest

from profiles import resolve_pending

URL = os.environ.get('TEST_DATABASE_URL')
pytestmark = pytest.mark.skipif(not URL, reason='TEST_DATABASE_URL is not set')
MIGRATION = os.path.join(os.path.dirname(__file__), '..', '..', 'backend', 'internal', 'database', 'migrations', '00064_person_profile.sql')

PROFILE = {'description': 'escritor americano', 'born': '1920-10-08', 'died': '1986-02-11', 'image': 'A.jpg', 'pages': {'pt': 'Frank Herbert'},
           'bio': ('pt', 'Frank Herbert', 'https://pt.wikipedia.org/wiki/Frank_Herbert', 'Frank Herbert foi um escritor.'), 'bio_state': 'done',
           'photo': {'path': '/covers/person_Q7934.jpg', 'credit': 'Unknown', 'license': 'Public domain', 'license_url': '', 'page': 'https://c.org/File:A.jpg'}}


class Db:
    def __init__(self, conn):
        self.cur = conn.cursor()

    def execute(self, query, params=()):
        self.cur.execute(query, params)

    def fetchall(self, query, params=()):
        self.cur.execute(query, params)
        return self.cur.fetchall()


def up_section():
    text = open(MIGRATION, encoding='utf-8').read()
    return text.split('-- +goose Up', 1)[1].split('-- +goose Down', 1)[0]


@pytest.fixture
def db():
    conn = psycopg2.connect(URL)
    schema = 'profiles_' + uuid.uuid4().hex[:10]
    cur = conn.cursor()
    cur.execute(f'CREATE SCHEMA {schema}')
    cur.execute(f'SET search_path TO {schema}')
    cur.execute("""
        CREATE TABLE users (id UUID PRIMARY KEY);
        CREATE TABLE person (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
        CREATE TABLE person_authority (
            person_id INTEGER NOT NULL REFERENCES person(id) ON DELETE CASCADE, scheme VARCHAR(32) NOT NULL, value VARCHAR(64) NOT NULL,
            source VARCHAR(64) NOT NULL DEFAULT '', PRIMARY KEY (person_id, scheme, value));
        CREATE TABLE authority_lookups (
            source VARCHAR(32) NOT NULL, key VARCHAR(64) NOT NULL,
            state VARCHAR(12) NOT NULL CHECK (state IN ('done', 'missing', 'redirect', 'failed')),
            attempts SMALLINT NOT NULL DEFAULT 1, attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (source, key));""")
    cur.execute(up_section())   # the migration itself
    yield Db(conn)
    conn.rollback()
    cur = conn.cursor()
    cur.execute(f'DROP SCHEMA IF EXISTS {schema} CASCADE')
    conn.commit()
    conn.close()


def person(db, name, qid=None):
    db.execute('INSERT INTO person (name) VALUES (%s) RETURNING id', (name,))
    pid = db.cur.fetchone()[0]
    if qid:
        db.execute("INSERT INTO person_authority (person_id, scheme, value, source) VALUES (%s, 'wikidata', %s, 'Open Library')", (pid, qid))
    return pid


def fetcher(results):
    calls = []

    def fetch(qid, covers_dir, allow_bio, allow_image=True):
        calls.append((qid, allow_bio, allow_image))
        return results.get(qid, ('failed', None))
    fetch.calls = calls
    return fetch


def read(db, query):
    return db.fetchall(query)


def run(db, allowed, fetch, **kw):
    return resolve_pending(db, lambda pid: pid in allowed, '/tmp/covers', sleep=lambda s: None, fetch=fetch, **kw)


class TestTheMigration:
    def test_a_person_has_one_profile_that_goes_with_the_person(self, db):
        a = person(db, 'Frank Herbert', 'Q7934')
        db.execute("INSERT INTO person_profile (person_id, wikidata_id) VALUES (%s, 'Q7934')", (a,))
        with pytest.raises(psycopg2.errors.UniqueViolation):
            db.execute("INSERT INTO person_profile (person_id, wikidata_id) VALUES (%s, 'Q7934')", (a,))

    def test_the_biography_is_in_a_state_the_pages_know(self, db):
        a = person(db, 'A')
        with pytest.raises(psycopg2.errors.CheckViolation):
            db.execute("INSERT INTO person_profile (person_id, wikidata_id, bio_state) VALUES (%s, 'Q1', 'maybe')", (a,))

    def test_it_is_not_hidden_until_somebody_hides_it_and_the_biography_is_pending(self, db):
        a = person(db, 'A')
        db.execute("INSERT INTO person_profile (person_id, wikidata_id) VALUES (%s, 'Q1')", (a,))
        assert read(db, 'SELECT hidden, image_hidden, bio_state, description, bio FROM person_profile') == [(False, False, 'pending', '', '')]

    def test_going_back_takes_the_table_away(self, db):
        text = open(MIGRATION, encoding='utf-8').read().split('-- +goose Down', 1)[1]
        db.execute(text)
        with pytest.raises(psycopg2.errors.UndefinedTable):
            db.execute('SELECT 1 FROM person_profile')


class TestKeepingAProfile:
    def test_every_person_that_holds_the_identifier_gets_the_profile_and_the_lookup_is_remembered(self, db):
        a, b = person(db, 'Frank Herbert', 'Q7934'), person(db, 'Herbert, Frank', 'Q7934')
        other = person(db, 'Isaac Asimov', 'Q34981')
        n = run(db, {'wikidata', 'wikipedia'}, fetcher({'Q7934': ('ok', PROFILE)}))
        assert n == 1
        rows = read(db, 'SELECT person_id, wikidata_id, description, born, died, bio, bio_language, bio_state, image_path, image_credit, image_license FROM person_profile ORDER BY person_id')
        expected = ('Q7934', 'escritor americano', '1920-10-08', '1986-02-11', 'Frank Herbert foi um escritor.', 'pt', 'done', '/covers/person_Q7934.jpg', 'Unknown', 'Public domain')
        assert [(r[0],) + r[1:] for r in rows if r[0] in (a, b)] == [(a,) + expected, (b,) + expected]
        assert other not in [r[0] for r in rows]   # nobody else's profile was read for it
        assert read(db, "SELECT key, state, attempts FROM authority_lookups WHERE source = 'wikidata' AND key = 'Q7934'") == [('Q7934', 'done', 1)]
        assert read(db, "SELECT count(*) FROM person_profile WHERE fetched_at > now()") == [(0,)]   # read now, and not at some other time

    def test_a_second_reading_changes_what_was_read_but_never_what_staff_hid(self, db):
        a = person(db, 'Frank Herbert', 'Q7934')
        db.execute("INSERT INTO person_profile (person_id, wikidata_id, description, hidden, image_hidden) VALUES (%s, 'Q7934', 'old', TRUE, TRUE)", (a,))
        db.execute("DELETE FROM authority_lookups")
        # the insert above makes the person have a profile, so the statement under test is run by hand, as the worker does for a new reading
        import profiles
        db.execute(profiles._KEEP + profiles._REMEMBER, profiles._keep_params(PROFILE, 'Q7934') + ('Q7934', 'done'))
        assert read(db, 'SELECT description, hidden, image_hidden, born FROM person_profile') == [('escritor americano', True, True, '1920-10-08')]
        assert read(db, "SELECT attempts FROM authority_lookups WHERE key = 'Q7934'") == [(1,)]
        db.execute(profiles._REMEMBER, ('Q7934', 'done'))
        assert read(db, "SELECT attempts FROM authority_lookups WHERE key = 'Q7934'") == [(2,)]


class TestWhoIsStillToRead:
    def test_a_person_with_a_profile_or_with_an_answered_lookup_is_not_read_again(self, db):
        a, b, c = person(db, 'A', 'Q1'), person(db, 'B', 'Q2'), person(db, 'C', 'Q3')
        db.execute("INSERT INTO person_profile (person_id, wikidata_id) VALUES (%s, 'Q1')", (a,))
        db.execute("INSERT INTO authority_lookups (source, key, state) VALUES ('wikidata', 'Q2', 'missing')")
        fetch = fetcher({'Q3': ('ok', PROFILE)})
        run(db, {'wikidata'}, fetch)
        assert [c[0] for c in fetch.calls] == ['Q3']

    def test_a_lookup_that_failed_is_tried_again_after_a_day_a_few_times_and_not_before(self, db):
        person(db, 'A', 'Q1'), person(db, 'B', 'Q2'), person(db, 'C', 'Q3')
        db.execute("INSERT INTO authority_lookups (source, key, state, attempts, attempted_at) VALUES "
                   "('wikidata', 'Q1', 'failed', 1, now()), "                       # just now: not yet
                   "('wikidata', 'Q2', 'failed', 2, now() - interval '2 days'), "   # a day has passed: again
                   "('wikidata', 'Q3', 'failed', 5, now() - interval '9 days')")   # too many attempts: left
        fetch = fetcher({'Q2': ('failed', None)})
        run(db, {'wikidata'}, fetch)
        assert [c[0] for c in fetch.calls] == ['Q2']
        assert read(db, "SELECT attempts FROM authority_lookups WHERE key = 'Q2'") == [(3,)]

    def test_a_lookup_that_was_answered_is_never_read_again_however_old_it_is(self, db):
        person(db, 'A', 'Q1')
        db.execute("INSERT INTO authority_lookups (source, key, state, attempts, attempted_at) VALUES ('wikidata', 'Q1', 'done', 1, now() - interval '30 days')")
        fetch = fetcher({})
        run(db, {'wikidata'}, fetch)
        assert fetch.calls == []

    def test_an_identifier_of_another_kind_is_not_an_authors_profile(self, db):
        a = person(db, 'A')
        db.execute("INSERT INTO person_authority (person_id, scheme, value, source) VALUES (%s, 'viaf', '59083797', 'Open Library')", (a,))
        db.execute("INSERT INTO person_authority (person_id, scheme, value, source) VALUES (%s, 'openlibrary', 'OL79034A', 'Open Library')", (a,))
        fetch = fetcher({})
        run(db, {'wikidata'}, fetch)
        assert fetch.calls == []

    def test_a_few_at_a_time_in_the_order_of_the_identifiers(self, db):
        for i in range(19, 9, -1):   # written the other way round
            person(db, f'P{i}', f'Q{i}')
        fetch = fetcher({f'Q{i}': ('missing', None) for i in range(10, 20)})
        run(db, {'wikidata'}, fetch, limit=3)
        assert [c[0] for c in fetch.calls] == ['Q10', 'Q11', 'Q12']

    def test_a_person_that_joins_one_who_has_the_profile_gets_it_without_a_request(self, db):
        a = person(db, 'Frank Herbert', 'Q7934')
        db.execute("INSERT INTO person_profile (person_id, wikidata_id, description) VALUES (%s, 'Q7934', 'escritor')", (a,))
        db.execute("INSERT INTO authority_lookups (source, key, state) VALUES ('wikidata', 'Q7934', 'done')")
        b = person(db, 'F. Herbert', 'Q7934')
        fetch = fetcher({})
        run(db, {'wikidata'}, fetch)
        assert fetch.calls == []
        assert read(db, f'SELECT description FROM person_profile WHERE person_id = {b}') == [('escritor',)]

    def test_the_biography_of_the_ones_read_without_wikipedia_is_filled_when_it_is_on(self, db):
        a = person(db, 'Frank Herbert', 'Q7934')
        db.execute("INSERT INTO person_profile (person_id, wikidata_id, description, bio_state, hidden) VALUES (%s, 'Q7934', 'escritor', 'pending', TRUE)", (a,))
        fetch = fetcher({'Q7934': ('ok', PROFILE)})
        run(db, {'wikidata'}, fetch)
        assert fetch.calls == []   # Wikipedia is off: it waits
        run(db, {'wikidata', 'wikipedia'}, fetch)
        assert fetch.calls == [('Q7934', True, False)]
        assert read(db, 'SELECT bio, bio_language, bio_state, description, hidden FROM person_profile') == [('Frank Herbert foi um escritor.', 'pt', 'done', 'escritor', True)]
        run(db, {'wikidata', 'wikipedia'}, fetch)
        assert len(fetch.calls) == 1   # done: not asked again
