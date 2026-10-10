"""How each provider answers, told to the administration (DEC-144)."""
from unittest.mock import MagicMock, patch

from providers import http
from providers.health import DOWN, ERROR, KEY, OK, QUOTA, HealthRecorder, install, state_of
from providers.http import MAX_SAID, Reply, _said, get_json
from providers.registry import ProviderRegistry
from tests.test_google_comicvine import KEY as SECRET, Net, book, patched
from tests.test_wikidata_wikipedia import Books, Completer, Resolver, registry as fake_registry, work


class DB:
    def __init__(self, boom=False):
        self.statements, self.boom = [], boom

    def execute(self, query, params=()):
        if self.boom:
            raise RuntimeError('database down')
        self.statements.append((' '.join(query.split()), params))


class TestWhatARequestCameTo:
    def test_the_kind_of_answer_by_the_status(self):
        assert state_of(200) == OK and state_of(200, ok=True) == OK
        assert state_of(200, ok=False) == ERROR   # it answered, but not something that can be read
        assert state_of(401) == KEY and state_of(403) == KEY
        assert state_of(429) == QUOTA
        assert state_of(0) == DOWN and state_of(500) == DOWN and state_of(502) == DOWN and state_of(503) == DOWN
        for status in (400, 404, 410, 418, 499):
            assert state_of(status) == ERROR, status

    def test_a_key_that_is_not_valid_is_a_refused_key_though_google_calls_it_a_bad_request(self):
        said = 'HTTP 400: the request was refused as badly made (API key not valid. Please pass a valid API key.)'
        assert state_of(400, False, said) == KEY
        for text in ('API key expired. Please renew the API key.', 'The API KEY is invalid', 'api key not valid'):
            assert state_of(400, False, text) == KEY, text
        assert state_of(403, False, 'whatever') == KEY and state_of(404, False, 'API key not valid') == ERROR   # only the 400 that Google gives
        # what is not about the key is not
        for text in ('', 'HTTP 400: the request was refused as badly made', 'Invalid value at q', 'the API is not valid here', 'key is not valid', 'API key quota', 'api key in the query'):
            assert state_of(400, False, text) == ERROR, text
        assert state_of(500, False, 'API key not valid') == DOWN and state_of(429, False, 'API key not valid') == QUOTA and state_of(0, False, 'API key not valid') == DOWN
        assert state_of(200, True, 'API key not valid') == OK


class TestRecording:
    def setup_method(self):
        self.db = DB()
        self.recorder = HealthRecorder(self.db, {'Google Books': 'google_books', 'OpenLibrary': 'openlibrary'})

    def test_a_request_leaves_how_it_came_out_by_the_id_of_the_provider(self):
        self.recorder.reply('Google Books', Reply(429, None, 'HTTP 429: too many requests'))
        (query, params), = self.db.statements
        assert 'INSERT INTO provider_health' in query and 'ON CONFLICT (provider) DO UPDATE' in query
        assert params == ('google_books', 'quota', 429, 'HTTP 429: too many requests', 'quota')

    def test_the_last_time_it_was_well_is_kept_through_the_bad_times(self):
        self.recorder.reply('OpenLibrary', Reply(200, {'docs': []}))
        query, params = self.db.statements[-1]
        assert params[1] == 'ok' and "CASE WHEN %s = 'ok' THEN now() END" in query
        assert "last_ok_at = CASE WHEN EXCLUDED.state = 'ok' THEN now() ELSE provider_health.last_ok_at END" in query

    def test_what_is_said_of_a_problem_is_short(self):
        self.recorder.reply('Google Books', Reply(0, None, 'x' * 1000))
        assert len(self.db.statements[-1][1][3]) == 300
        self.recorder.reply('Google Books', Reply(200, {}, ''))
        assert self.db.statements[-1][1][3] == ''

    def test_an_answer_that_could_not_be_read_is_an_error(self):
        self.recorder.reply('Google Books', Reply(200, None, 'the answer is not JSON'))
        assert self.db.statements[-1][1][1] == 'error'

    def test_a_provider_that_is_not_one_of_ours_is_not_recorded(self):
        self.recorder.reply('Some Other Service', Reply(200, {}))
        assert self.db.statements == []

    def test_a_database_that_fails_never_fails_the_request(self, capsys):
        recorder = HealthRecorder(DB(boom=True), {'Google Books': 'google_books'})
        recorder.reply('Google Books', Reply(200, {}))
        recorder.answered('google_books', 0)
        assert 'Could not record how Google Books answered' in capsys.readouterr().out
        assert recorder.last['google_books'] == OK

    def test_what_the_provider_said_of_a_refusal_decides_when_the_status_does_not(self):
        self.recorder.reply('Google Books', Reply(400, None, 'HTTP 400: the request was refused as badly made (API key not valid. Please pass a valid API key.)'))
        assert self.db.statements[-1][1][1] == 'key'
        self.recorder.reply('Google Books', Reply(400, None, 'HTTP 400: the request was refused as badly made'))
        assert self.db.statements[-1][1][1] == 'error'

    def test_the_state_of_each_provider_is_kept_in_the_process(self):
        self.recorder.reply('Google Books', Reply(403, None, 'x'))
        self.recorder.reply('OpenLibrary', Reply(200, {}))
        assert self.recorder.last == {'google_books': KEY, 'openlibrary': OK}


class TestSearchesThatComeBackEmpty:
    def setup_method(self):
        self.db = DB()
        self.recorder = HealthRecorder(self.db, {'Google Books': 'google_books'})
        self.recorder.reply('Google Books', Reply(200, {}))
        self.db.statements.clear()

    def test_an_empty_search_adds_to_the_streak_and_any_answer_ends_it(self):
        self.recorder.answered('google_books', 0)
        self.recorder.answered('google_books', 3)
        (q1, p1), (q2, p2) = self.db.statements
        assert 'UPDATE provider_health SET empty_streak' in q1 and 'WHERE provider = %s' in q1
        assert p1 == (0, 'google_books') and p2 == (3, 'google_books')
        assert 'empty_streak + 1 ELSE 0' in q1

    def test_a_provider_whose_request_failed_is_not_blamed_for_giving_nothing(self):
        self.recorder.reply('Google Books', Reply(429, None, 'HTTP 429'))
        self.db.statements.clear()
        self.recorder.answered('google_books', 0)
        assert self.db.statements == []

    def test_a_provider_that_was_not_asked_has_no_streak(self):
        self.recorder.answered('openlibrary', 0)
        assert self.db.statements == []


class TestEveryRequestIsReported:
    def test_a_request_is_told_to_the_reporter_with_the_name_of_the_provider(self):
        told = []
        with patch.object(http, 'reporter', lambda name, reply: told.append((name, reply.status))), patched(Net(lambda url, p: {'items': []})):
            get_json('Google Books', 'https://x.test/v1', params={'q': 'Duna'})
            get_json('OpenLibrary', 'https://y.test/s')
        assert told == [('Google Books', 200), ('OpenLibrary', 200)]

    def test_so_is_a_refusal_and_a_request_that_never_got_there(self, capsys):
        told = []
        with patch.object(http, 'reporter', lambda name, reply: told.append((reply.status, reply.problem))):
            with patched(Net(lambda url, p: (429, {}))):
                get_json('Google Books', 'https://x.test/v1')
            with patched(Net(lambda url, p: RuntimeError(f'down key={SECRET}'))):
                get_json('Google Books', 'https://x.test/v1', secrets=(SECRET,))
        assert told[0][0] == 429 and 'HTTP 429' in told[0][1] and told[1][0] == 0 and 'SECRET' not in str(told)

    def test_a_reporter_that_breaks_does_not_lose_the_answer(self, capsys):
        def boom(name, reply):
            raise RuntimeError('cannot')
        with patch.object(http, 'reporter', boom), patched(Net(lambda url, p: {'ok': 1})):
            reply = get_json('Google Books', 'https://x.test/v1')
        assert reply.ok and reply.data == {'ok': 1} and 'could not report how it answered' in capsys.readouterr().out

    def test_without_a_reporter_nothing_is_told(self):
        assert http.reporter is None
        with patched(Net(lambda url, p: {'ok': 1})):
            assert get_json('Google Books', 'https://x.test/v1').ok

    def test_the_key_never_reaches_the_administration(self):
        db = DB()
        recorder = HealthRecorder(db, {'Google Books': 'google_books'})
        with patch.object(http, 'reporter', recorder.reply), patched(Net(lambda url, p: RuntimeError(f'Max retries url: /v1?key={SECRET}'))):
            get_json('Google Books', 'https://x.test/v1', params={'key': SECRET}, secrets=(SECRET,))
        assert 'SECRET' not in str(db.statements) and '7890' not in str(db.statements)


class TestWhatAProviderSaysOfARefusal:
    def reply(self, body, status=400):
        resp = MagicMock(status_code=status)
        if isinstance(body, Exception):
            resp.json.side_effect = body
        else:
            resp.json.return_value = body
        return resp

    def test_google_says_it_in_error_message(self):
        assert _said(self.reply({'error': {'code': 400, 'message': 'API key not valid. Please pass a valid API key.', 'status': 'INVALID_ARGUMENT'}})) == 'API key not valid. Please pass a valid API key.'

    def test_others_say_it_in_error(self):
        assert _said(self.reply({'error': 'Invalid API Key', 'status_code': 100})) == 'Invalid API Key'

    def test_what_is_not_a_reason_is_nothing(self):
        for body in ({}, {'error': None}, {'error': 5}, {'error': {'message': 7}}, {'error': {}}, ['error'], 'text', None, ValueError('not json')):
            assert _said(self.reply(body)) == '', body

    def test_it_is_short_on_one_line_and_never_has_the_key(self):
        long = _said(self.reply({'error': {'message': 'word ' * 100}}))
        assert len(long) == 120 == MAX_SAID and '\n' not in long
        assert _said(self.reply({'error': {'message': 'line one\n\n  line   two'}})) == 'line one line two'
        assert _said(self.reply({'error': {'message': f'bad key {SECRET} here'}}), (SECRET,)) == 'bad key *** here'

    def test_a_refusal_is_told_in_the_log_with_what_the_provider_said_and_to_the_administration(self, capsys):
        told = []
        body = {'error': {'message': 'API key not valid. Please pass a valid API key.'}}
        with patch.object(http, 'reporter', lambda name, reply: told.append(reply)), patched(Net(lambda url, p: (400, body))):
            reply = get_json('Google Books', 'https://x.test/v1')
        assert reply.status == 400 and not reply.ok
        assert reply.problem == 'HTTP 400: the request was refused as badly made (API key not valid. Please pass a valid API key.)'
        assert told == [reply] and state_of(reply.status, False, reply.problem) == KEY
        assert reply.problem in capsys.readouterr().out

    def test_the_key_a_provider_repeats_in_its_reason_is_never_written(self, capsys):
        body = {'error': {'message': f'API key {SECRET} not valid'}}
        told = []
        with patch.object(http, 'reporter', lambda name, reply: told.append(reply.problem)), patched(Net(lambda url, p: (400, body))):
            reply = get_json('Google Books', 'https://x.test/v1', secrets=(SECRET,))
        assert 'SECRET' not in reply.problem and 'SECRET' not in capsys.readouterr().out and 'SECRET' not in str(told) and '***' in reply.problem

    def test_a_refusal_with_nothing_said_is_as_it_was(self):
        with patched(Net(lambda url, p: (429, {}))):
            assert get_json('Google Books', 'https://x.test/v1').problem == 'HTTP 429: too many requests, or the daily quota is used up (a key of your own has a quota of its own)'


class TestTheRegistryTellsHowManyAnswers:
    def test_each_search_tells_how_many_answers_each_provider_gave(self):
        told = []
        recorder = MagicMock()
        recorder.answered.side_effect = lambda pid, n: told.append((pid, n))
        r = fake_registry({'openlibrary', 'wikidata', 'wikipedia'}, Books([('Dune', work())]), Resolver([]), Completer())
        r.health = recorder
        r.search_best('Dune', 'epub')
        assert ('openlibrary', 1) in told and all(pid != 'wikipedia' for pid, _ in told)   # one that only completes is never asked to search
        told.clear()
        r.search_best('Outra Obra', 'epub')
        assert ('openlibrary', 0) in told

    def test_a_provider_that_is_off_is_not_counted_and_no_recorder_is_fine(self):
        told = []
        recorder = MagicMock()
        recorder.answered.side_effect = lambda pid, n: told.append((pid, n))
        r = fake_registry({'wikidata'}, Books([('Dune', work())]), Resolver([]))
        r.health = recorder
        r.search_best('Dune', 'epub')
        assert all(pid != 'openlibrary' for pid, _ in told)
        r = fake_registry({'openlibrary'}, Books([('Dune', work())]))
        assert r.health is None and r.search_best('Dune', 'epub') is not None


class TestInstalling:
    def test_the_names_of_the_real_providers_are_known_to_the_registry(self):
        names = ProviderRegistry().names()
        assert names['Google Books'] == 'google_books' and names['OpenLibrary'] == 'openlibrary' and names['ComicVine'] == 'comicvine'
        assert {'AniList', 'MangaDex', 'Wikidata', 'Wikipedia'} <= set(names) and len(set(names.values())) == len(names)

    def test_installing_makes_every_request_and_every_search_count(self):
        db = DB()
        registry = ProviderRegistry()
        recorder = install(db, registry)
        try:
            assert registry.health is recorder and http.reporter == recorder.reply
            with patched(Net(lambda url, p: (403, {}))):
                get_json('Google Books', 'https://x.test/v1')
            assert db.statements[-1][1][:3] == ('google_books', 'key', 403)
        finally:
            http.reporter = None


class TestTheWorkerStartsReporting:
    def test_the_runner_and_the_search_server_both_tell_the_administration_how_the_providers_answer(self, tmp_path, monkeypatch):
        from tests.test_text_job import SettingsDB, load_main
        monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
        installed = []
        main = load_main()
        monkeypatch.setattr(main, 'report_provider_health', lambda db, registry: installed.append(('main', registry)))
        main.build_runner(SettingsDB(None), None)
        assert [who for who, _ in installed] == ['main'] and isinstance(installed[0][1], ProviderRegistry)

        import server
        monkeypatch.setattr(server, 'report_provider_health', lambda db, registry: installed.append(('server', registry)))
        monkeypatch.setattr(server, 'CodiceDatabase', lambda: object())

        class Stop(Exception):
            pass

        def boom(*a, **k):
            raise Stop()
        monkeypatch.setattr(server, 'HTTPServer', boom)
        try:
            server.run_server()
        except Stop:
            pass
        assert [who for who, _ in installed] == ['main', 'server'] and installed[1][1] is server.SearchHandler.registry
