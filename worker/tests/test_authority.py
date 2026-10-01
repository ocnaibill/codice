"""The lookup of what Open Library knows about an author (#63): the identifiers, never a decision."""
import pytest
from unittest.mock import MagicMock

import authority
from authority import fetch, parse, resolve_pending

HERBERT = {'type': {'key': '/type/author'}, 'name': 'Frank Herbert',
           'remote_ids': {'wikidata': 'Q7934', 'viaf': '59083797', 'isni': '0000000121347853', 'goodreads': '58'}}


class TestParse:
    def test_keeps_the_identifiers_of_the_three_kinds_as_the_source_writes_them(self):
        assert parse(HERBERT) == ('ids', {'wikidata': 'Q7934', 'viaf': '59083797', 'isni': '0000000121347853'})

    def test_an_isni_may_end_in_x_and_whitespace_is_not_part_of_an_identifier(self):
        doc = {'remote_ids': {'isni': ' 000000012146227X ', 'wikidata': ' Q210059'}}
        assert parse(doc) == ('ids', {'isni': '000000012146227X', 'wikidata': 'Q210059'})

    @pytest.mark.parametrize('ids', [
        {'wikidata': 'Q'}, {'wikidata': '7934'}, {'wikidata': 'Q79x4'}, {'wikidata': 'Q' + '1' * 13},
        {'viaf': 'abc'}, {'viaf': ''}, {'viaf': '1' * 13}, {'viaf': 59083797},
        {'isni': '123'}, {'isni': '0000000121347853X'}, {'isni': 'x' * 16}, {'isni': None},
        {'wikidata': ['Q1']}, {'goodreads': '58', 'amazon': 'B000'},
    ])
    def test_what_is_not_an_identifier_of_its_kind_is_not_kept(self, ids):
        assert parse({'remote_ids': ids}) == ('ids', {})

    @pytest.mark.parametrize('doc', [None, [], 'text', {}, {'remote_ids': None}, {'remote_ids': 'Q1'}, {'type': 'author'}])
    def test_a_record_that_has_none_gives_none(self, doc):
        assert parse(doc) == ('ids', {})

    def test_an_author_merged_into_another_says_which(self):
        assert parse({'type': {'key': '/type/redirect'}, 'location': '/authors/OL34184A'}) == ('redirect', 'OL34184A')

    @pytest.mark.parametrize('location', [None, '', '/authors/', '/authors/OL1', '/authors/../x', '/authors/ol1a', 7])
    def test_a_redirect_to_something_that_is_not_an_author_key_is_not_followed(self, location):
        assert parse({'type': {'key': '/type/redirect'}, 'location': location}) == ('ids', {})


def reply(status, doc=None):
    r = MagicMock()
    r.status_code = status
    r.json.return_value = doc
    return r


class TestFetch:
    def test_asks_open_library_for_the_author_and_nothing_else(self):
        get = MagicMock(return_value=reply(200, HERBERT))
        assert fetch('OL79034A', get) == ('ok', HERBERT)
        (url,), kwargs = get.call_args
        assert url == 'https://openlibrary.org/authors/OL79034A.json'
        assert kwargs['timeout'] == 10 and 'User-Agent' in kwargs['headers'] and 'params' not in kwargs

    @pytest.mark.parametrize('key', ['', None, 'ol79034a', 'OL79034', 'OL79034A/../../x', 'OL79034A.json?x=1', 'OL' + '9' * 13 + 'A', ' OL1A', 'OL1A\n'])
    def test_a_key_that_is_not_one_is_never_put_in_a_url(self, key):
        get = MagicMock()
        assert fetch(key, get) == ('missing', None)
        get.assert_not_called()

    def test_not_found_is_an_answer_and_other_trouble_is_not(self):
        assert fetch('OL1A', MagicMock(return_value=reply(404))) == ('missing', None)
        assert fetch('OL1A', MagicMock(return_value=reply(500))) == ('failed', None)
        assert fetch('OL1A', MagicMock(return_value=reply(429))) == ('failed', None)
        assert fetch('OL1A', MagicMock(side_effect=RuntimeError('timeout'))) == ('failed', None)
        assert fetch('OL1A', MagicMock(return_value=MagicMock(status_code=200, json=MagicMock(side_effect=ValueError('not json'))))) == ('failed', None)


class FakeDb:
    def __init__(self, keys):
        self.keys, self.executed, self.queries = list(keys), [], []

    def fetchall(self, query, params=()):
        self.queries.append((query, params))
        return [(k,) for k in self.keys]

    def execute(self, query, params=()):
        self.executed.append((' '.join(query.split()), params))


def gate(*states):
    """A gate that answers with each state in turn, then the last one."""
    calls = iter(states)
    last = {'state': states[-1]}

    def allowed(provider_id):
        assert provider_id == 'openlibrary'
        return next(calls, last['state'])
    return allowed


def run(db, allowed, answers, limit=5):
    paused = []
    get = MagicMock(side_effect=lambda url, **kw: answers[url.rsplit('/', 1)[-1].replace('.json', '')])
    n = resolve_pending(db, allowed, limit=limit, get=get, sleep=paused.append)
    return n, get, paused


class TestResolvePending:
    def test_asks_nobody_while_open_library_is_off(self):
        db = FakeDb(['OL1A'])
        n, get, _ = run(db, gate(False), {})
        assert n == 0 and db.queries == [] and db.executed == [] and not get.called

    def test_keeps_what_it_found_for_whoever_holds_the_key_and_remembers_it_in_one_statement(self):
        db = FakeDb(['OL79034A'])
        n, get, paused = run(db, gate(True), {'OL79034A': reply(200, HERBERT)})
        assert n == 1
        (query, params), = [e for e in db.executed if 'authority_lookups' in e[0]]
        assert query.count('INSERT INTO person_authority') == 3  # wikidata, viaf, isni
        assert params == ('wikidata', 'Q7934', 'OL79034A', 'viaf', '59083797', 'OL79034A', 'isni', '0000000121347853', 'OL79034A',
                          'OL79034A', 'done')
        assert paused == [0.5]

    def test_asks_for_a_comparison_once_when_something_was_found_and_not_when_nothing_was(self):
        db = FakeDb(['OL1A', 'OL2A'])
        run(db, gate(True), {'OL1A': reply(200, HERBERT), 'OL2A': reply(200, HERBERT)})
        assert len([e for e in db.executed if 'INTO jobs' in e[0]]) == 1
        assert "WHERE NOT EXISTS" in [e for e in db.executed if 'INTO jobs' in e[0]][0][0]
        db = FakeDb(['OL3A'])
        run(db, gate(True), {'OL3A': reply(200, {'remote_ids': {}})})
        assert not [e for e in db.executed if 'INTO jobs' in e[0]]
        assert db.executed[-1][1] == ('OL3A', 'done')  # nothing to keep, but it was answered

    def test_an_author_with_no_such_key_is_remembered_as_missing_and_keeps_nothing(self):
        db = FakeDb(['OL9A'])
        n, _, _ = run(db, gate(True), {'OL9A': reply(404)})
        assert n == 1
        assert [e[1] for e in db.executed] == [('OL9A', 'missing')]

    def test_trouble_is_remembered_to_try_again_later_and_is_not_an_answer(self):
        db = FakeDb(['OL9A', 'OL1A'])
        n, _, _ = run(db, gate(True), {'OL9A': reply(503), 'OL1A': reply(200, HERBERT)})
        assert n == 1
        assert db.executed[0][1] == ('OL9A', 'failed')

    def test_a_merged_author_gives_the_key_it_was_merged_into_to_whoever_holds_this_one(self):
        db = FakeDb(['OLOLDA'.replace('OLOLDA', 'OL5A')])
        n, _, _ = run(db, gate(True), {'OL5A': reply(200, {'type': {'key': '/type/redirect'}, 'location': '/authors/OL6A'})})
        (query, params), = [e for e in db.executed if 'authority_lookups' in e[0]]
        assert params == ('openlibrary', 'OL6A', 'OL5A', 'OL5A', 'redirect')
        assert [e for e in db.executed if 'INTO jobs' in e[0]]

    def test_stops_at_once_when_the_owner_turns_it_off_meanwhile(self):
        db = FakeDb(['OL1A', 'OL2A', 'OL3A'])
        # allowed at the start and before the first key, not before the second.
        n, get, _ = run(db, gate(True, True, False), {'OL1A': reply(200, HERBERT)})
        assert n == 1 and get.call_count == 1

    def test_only_asks_for_keys_nobody_looked_up_and_only_as_many_as_it_was_told(self):
        db = FakeDb([])
        run(db, gate(True), {}, limit=3)
        (query, params), = db.queries
        assert params == (authority.MAX_ATTEMPTS, 3)
        assert "scheme = 'openlibrary'" in query and 'authority_lookups' in query
        assert "l.state <> 'failed'" in query and 'l.attempts >= %s' in query and "interval '1 day'" in query


class TestWiring:
    def main(self):
        from tests.test_text_job import load_main
        return load_main()

    def test_the_idle_worker_that_asks_the_providers_looks_up_authors(self, monkeypatch):
        from unittest.mock import patch
        main = self.main()
        monkeypatch.delenv('WORKER_JOB_TYPES', raising=False)
        calls = []
        with patch.object(main, 'resolve_pending', lambda db, allowed: calls.append((db, allowed))):
            main.resolve_authors('db', 'gate')
        assert calls == [('db', 'gate')]

    def test_another_worker_that_runs_this_code_does_not(self, monkeypatch):
        from unittest.mock import patch
        main = self.main()
        monkeypatch.setenv('WORKER_JOB_TYPES', 'embed_text')
        calls = []
        with patch.object(main, 'resolve_pending', lambda db, allowed: calls.append(1)):
            main.resolve_authors('db', 'gate')
        assert calls == []

    def test_a_failure_never_stops_the_worker(self, monkeypatch, capsys):
        from unittest.mock import patch
        main = self.main()
        monkeypatch.delenv('WORKER_JOB_TYPES', raising=False)

        def boom(db, allowed):
            raise RuntimeError('database down')
        with patch.object(main, 'resolve_pending', boom):
            main.resolve_authors('db', 'gate')
        assert 'Author lookup failed' in capsys.readouterr().out

    def test_the_loop_looks_up_authors_when_it_has_nothing_else_to_do_and_not_when_it_worked(self):
        from types import SimpleNamespace
        from unittest.mock import patch
        main = self.main()
        events = []
        outcomes = iter(['worked', 'idle'])

        class Stop(Exception):
            pass

        def wait(client, last_id):
            events.append('wait')
            raise Stop()

        with patch.object(main, 'CodiceDatabase', lambda: 'db'), patch.object(main, 'report_keys', lambda db: None), \
                patch.object(main, 'db_gate', lambda db: 'gate'), patch.object(main, 'connect_redis', lambda: 'redis'), \
                patch.object(main, 'Heartbeat', lambda: SimpleNamespace(beat=lambda *a, **k: None)), \
                patch.object(main, 'build_runner', lambda db, client, heartbeat: SimpleNamespace(embeddings=None)), \
                patch.object(main, 'poll_once', lambda runner, heartbeat: (events.append('poll'), next(outcomes))[1]), \
                patch.object(main, 'resolve_authors', lambda db, allowed: events.append(('authors', db, allowed))), \
                patch.object(main, 'wait_for_work', wait):
            with pytest.raises(Stop):
                main.listen_for_tasks()
        assert events == ['poll', 'poll', ('authors', 'db', 'gate'), 'wait']
