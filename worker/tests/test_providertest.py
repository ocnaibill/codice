"""Testing one provider, when the owner asks (DEC-145)."""
import json
from unittest.mock import MagicMock, patch

import pytest

import providertest
from providers.base import MetadataRecord
from providers.health import HealthRecorder, install
from providers.registry import ProviderRegistry
from providertest import QUESTIONS, SETTING, ProviderTester
from runner import classify
from tests.test_google_comicvine import KEY, Net, book, patched


class DB:
    def __init__(self):
        self.statements = []

    def execute(self, query, params=()):
        self.statements.append((' '.join(query.split()), params))


def tester(keys=True, installed=True):
    """The real providers, all off (the owner has turned none on), with their keys or without."""
    db = DB()
    registry = ProviderRegistry()   # nothing is allowed: a test is not an analysis
    for provider in registry._providers['default'] + registry._providers['cbz']:
        if hasattr(provider, 'api_key'):
            provider.api_key = KEY if keys else ''
    if installed:
        install(db, registry)
        db.statements.clear()
    return ProviderTester(db, registry), db, registry


def saved(db):
    query, params = [s for s in db.statements if 'INSERT INTO settings' in s[0]][-1]
    return params, json.loads(params[2])


def ask(t, provider, answer, **kw):
    net = Net(answer)
    with patched(net):
        return t.run(9, provider, **kw), net


class TestTheQuestionIsFixedAndPublic:
    def test_each_provider_is_asked_a_title_it_must_know_and_nothing_of_the_library(self):
        t, db, _ = tester()
        everything = {'items': [book()], 'docs': [{'key': '/works/OL1W', 'title': 'Dune'}], 'search': [], 'error': 'OK', 'results': [], 'data': {'data': []}}
        for provider, expect in (('google_books', 'Dune'), ('openlibrary', 'Dune'), ('wikidata', 'Dune'), ('comicvine', 'Absolute Batman'), ('mangadex', 'Berserk')):
            net = Net(lambda url, p: everything)
            with patched(net):
                t.run(1, provider)
            assert expect in json.dumps(net.params()), provider
        assert set(QUESTIONS) == {'google_books', 'openlibrary', 'wikidata', 'comicvine', 'anilist', 'mangadex'}

    def test_anilist_is_asked_its_question_in_the_body_of_the_request(self):
        t, _, _ = tester()
        sent = []

        def post(url, json=None, headers=None, timeout=None):
            sent.append(json)
            return MagicMock(status_code=200, json=MagicMock(return_value={'data': {'Page': {'media': []}}}))
        with patch('providers.http.requests.post', post):
            result = t.run(1, 'anilist')
        assert 'Berserk' in str(sent) and result['state'] == 'empty' and len(sent) >= 1

    def test_the_setting_the_administration_reads_and_the_series_question_of_comicvine(self):
        assert SETTING == 'metadata.providers.tests'   # the server reads it by this name (metaproviders.TestsSettingKey)
        t, _, _ = tester()
        net = Net(lambda url, p: {'error': 'OK', 'results': []})
        with patched(net):
            t.run(1, 'comicvine')
        assert net.params()[0]['query'] == 'Absolute Batman' and net.params()[0]['resources'] == 'volume'   # asked as a file of a series: without the number

    def test_the_request_of_google_books_is_the_fixed_title_with_the_key_of_the_owner(self):
        t, _, _ = tester()
        _, net = ask(t, 'google_books', lambda url, p: {'items': [book()]})
        (params,) = net.params()
        assert params['q'] == 'Dune' and params['key'] == KEY

    def test_it_is_asked_whether_the_provider_is_on_or_not(self):
        t, _, registry = tester()
        assert not any(registry._enabled(p) for p in ('google_books', 'openlibrary', 'comicvine'))
        result, net = ask(t, 'openlibrary', lambda url, p: {'docs': [{'key': '/works/OL1W', 'title': 'Dune'}]})
        assert net.calls and result['ok'] is True


class TestHowItCameOut:
    def test_a_provider_that_answers_with_results_is_well(self):
        t, db, _ = tester()
        result, _ = ask(t, 'google_books', lambda url, p: {'items': [book('Dune'), book('Dune Messiah')]})
        assert (result['ok'], result['state'], result['status'], result['results']) == (True, 'ok', 200, 2)
        assert result['at'].endswith('Z') and len(result['at']) == 20
        params, body = saved(db)
        assert body == result and params[0] == SETTING and params[1] == 'google_books' and params[3] == 'google_books'

    def test_it_is_kept_by_merging_into_what_is_there(self):
        t, db, _ = tester()
        ask(t, 'google_books', lambda url, p: {'items': [book()]})
        (query, params), = [s for s in db.statements if 'INSERT INTO settings' in s[0]]
        assert 'updated_at = now()' in query and json.loads(params[2]) == json.loads(params[4])   # the same outcome on both sides of the merge
        assert 'ON CONFLICT (key) DO UPDATE' in query and "|| jsonb_build_object(%s::text, %s::jsonb)" in query
        assert "CASE WHEN jsonb_typeof(settings.value) = 'object' THEN settings.value ELSE '{}'::jsonb END" in query

    def test_a_key_that_is_not_valid(self):
        t, db, _ = tester()
        body = {'error': {'message': 'API key not valid. Please pass a valid API key.'}}
        result, _ = ask(t, 'google_books', lambda url, p: (400, body))
        assert (result['ok'], result['state'], result['status'], result['results']) == (False, 'key', 400, 0)
        assert saved(db)[1]['state'] == 'key'

    def test_a_key_that_is_refused_and_a_quota_that_is_used_up_and_a_service_that_is_down_and_an_error(self):
        t, _, _ = tester()
        for answer, state, status in (((401, {}), 'key', 401), ((403, {}), 'key', 403), ((429, {}), 'quota', 429), ((503, {}), 'down', 503),
                                      ((404, {}), 'error', 404), (RuntimeError('refused'), 'down', 0)):
            result, _ = ask(t, 'openlibrary', lambda url, p, a=answer: a)
            assert (result['ok'], result['state'], result['status'], result['results']) == (False, state, status, 0), answer

    def test_a_provider_that_answers_with_nothing_to_a_question_that_has_an_answer_is_not_well(self):
        t, _, _ = tester()
        result, _ = ask(t, 'openlibrary', lambda url, p: {'docs': []})
        assert (result['ok'], result['state'], result['status'], result['results']) == (False, 'empty', 200, 0)

    def test_a_provider_that_needs_a_key_it_has_not_got_is_not_asked(self, capsys):
        t, db, _ = tester(keys=False)
        for provider in ('google_books', 'comicvine'):
            result, net = ask(t, provider, lambda url, p: {'items': [book()]})
            assert net.calls == [] and (result['ok'], result['state'], result['results'], result['ms']) == (False, 'nokey', 0, 0), provider
        assert 'no API key' in capsys.readouterr().out
        assert saved(db)[1]['state'] == 'nokey'

    def test_a_provider_that_has_no_key_to_ask_for_is_asked_anyway(self):
        t, _, _ = tester(keys=False)
        result, net = ask(t, 'openlibrary', lambda url, p: {'docs': [{'key': '/works/OL1W', 'title': 'Dune'}]})
        assert net.calls and result['state'] == 'ok'

    def test_a_provider_that_breaks_is_a_finding_and_not_a_job_to_try_again(self, capsys):
        t, db, registry = tester()
        with patch.object(registry.provider('openlibrary'), 'lookup', side_effect=RuntimeError('boom')):
            result = t.run(1, 'openlibrary')
        assert (result['ok'], result['state'], result['results']) == (False, 'error', 0)
        assert 'it broke (boom)' in capsys.readouterr().out and saved(db)[1]['state'] == 'error'

    def test_a_provider_that_made_no_request_has_no_state_of_its_own_to_blame(self):
        t, _, registry = tester()
        registry.health.last['openlibrary'], registry.health.status['openlibrary'] = 'key', 403   # from an analysis, long ago
        with patch.object(registry.provider('openlibrary'), 'lookup', return_value=[]):
            result = t.run(1, 'openlibrary')
        assert (result['state'], result['status']) == ('empty', 0)

    def test_how_long_it_took_is_told_in_milliseconds(self):
        t, _, _ = tester()
        with patch.object(providertest.time, 'monotonic', side_effect=[10.0, 10.84]):
            result, _ = ask(t, 'google_books', lambda url, p: {'items': [book()]})
        assert result['ms'] == 840

    def test_what_the_provider_said_before_does_not_count(self):
        t, _, registry = tester()
        registry.health.last['openlibrary'], registry.health.status['openlibrary'] = 'key', 403
        result, _ = ask(t, 'openlibrary', lambda url, p: {'docs': [{'key': '/works/OL1W', 'title': 'Dune'}]})
        assert (result['ok'], result['state'], result['status']) == (True, 'ok', 200)

    def test_without_a_recorder_the_answers_are_all_there_is_to_go_by(self):
        t, _, _ = tester(installed=False)
        result, _ = ask(t, 'openlibrary', lambda url, p: {'docs': [{'key': '/works/OL1W', 'title': 'Dune'}]})
        assert (result['ok'], result['state'], result['status'], result['results']) == (True, 'ok', 0, 1)
        result, _ = ask(t, 'openlibrary', lambda url, p: {'docs': []})
        assert (result['ok'], result['state']) == (False, 'empty')


class TestWikipedia:
    PAGE = {'type': 'standard', 'extract': 'Dune is a 1965 science fiction novel.', 'content_urls': {'desktop': {'page': 'https://en.wikipedia.org/wiki/Dune_(novel)'}}}

    def test_it_is_asked_for_a_page_that_exists_and_is_well_when_it_gives_it(self):
        t, _, _ = tester()
        result, net = ask(t, 'wikipedia', lambda url, p: self.PAGE)
        assert (result['ok'], result['state'], result['results']) == (True, 'ok', 1)
        assert net.calls[0][0] == 'https://en.wikipedia.org/api/rest_v1/page/summary/Dune%20%28novel%29'

    def test_a_page_it_does_not_give_is_not_well(self):
        t, _, _ = tester()
        result, _ = ask(t, 'wikipedia', lambda url, p: (404, {}))
        assert (result['ok'], result['state'], result['status'], result['results']) == (False, 'error', 404, 0)
        result, _ = ask(t, 'wikipedia', lambda url, p: {'type': 'disambiguation', 'extract': 'x'})
        assert (result['ok'], result['state'], result['results']) == (False, 'empty', 0)


class TestWhatCannotBeTested:
    def test_a_provider_that_does_not_exist_is_a_permanent_error_and_nothing_is_kept(self):
        t, db, _ = tester()
        for name in ('nobody', '', None, 'GOOGLE_BOOKS'):
            with pytest.raises(ValueError) as caught:
                t.run(1, name)
            assert classify(caught.value) == 'permanent'
        assert [s for s in db.statements if 'INSERT INTO settings' in s[0]] == []

    def test_a_provider_with_no_question_is_not_asked_anything(self):
        class Other:
            id, name = 'other', 'Other'

            def lookup(self, query):
                raise AssertionError('asked')
        t, db, registry = tester()
        registry._providers['default'].append(Other())
        with pytest.raises(ValueError):
            t.run(1, 'other')

    def test_the_job_can_be_cancelled_before_the_question_is_asked(self):
        class Cancelled(Exception):
            pass

        def stop():
            raise Cancelled()
        t, db, _ = tester()
        net = Net(lambda url, p: {'items': [book()]})
        with patched(net), pytest.raises(Cancelled):
            t.run(1, 'google_books', checkpoint=stop)
        assert net.calls == [] and [s for s in db.statements if 'INSERT INTO settings' in s[0]] == []


class TestTheRegistryKnowsItsProviders:
    def test_a_provider_by_its_id(self):
        registry = ProviderRegistry()
        assert registry.provider('google_books').name == 'Google Books' and registry.provider('anilist').id == 'anilist' and registry.provider('wikipedia').name == 'Wikipedia'
        assert registry.provider('nobody') is None and registry.provider(None) is None

    def test_the_recorder_keeps_the_status_of_the_last_request_too(self):
        from providers.http import Reply
        recorder = HealthRecorder(DB(), {'Google Books': 'google_books'})
        recorder.reply('Google Books', Reply(429, None, 'x'))
        assert recorder.status == {'google_books': 429}


class TestTheWorkerDoesTheJob:
    def test_a_provider_test_job_asks_the_provider_and_never_touches_a_work(self, tmp_path, monkeypatch):
        from tests.test_text_job import SettingsDB, load_main
        monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
        db = SettingsDB(None)
        db.job = (51, None, {'provider': 'openlibrary'}, 1, 3, 'provider_test')
        net = Net(lambda url, p: {'docs': [{'key': '/works/OL1W', 'title': 'Dune'}]})
        with patched(net):
            assert load_main().build_runner(db, None).run_one() is True
        assert net.calls and 'Dune' in json.dumps(net.params())
        assert [q for q in db.executed if 'INSERT INTO settings' in q and 'jsonb_build_object' in q]
        assert not [q for q in db.executed if 'media_status' in q]

    def test_the_provider_of_the_job_is_the_one_asked(self, tmp_path, monkeypatch):
        from tests.test_text_job import SettingsDB, load_main
        monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
        db = SettingsDB(None)
        db.job = (52, None, {'provider': 'wikidata'}, 1, 3, 'provider_test')
        net = Net(lambda url, p: {'search': []})
        with patched(net):
            load_main().build_runner(db, None).run_one()
        assert net.calls and all('wikidata.org' in url for url, _, _ in net.calls)
