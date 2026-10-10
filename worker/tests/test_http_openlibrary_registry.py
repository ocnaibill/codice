"""How the providers are asked, Open Library's way of answering, and the choice of the answer that is the work."""
from unittest.mock import MagicMock, patch

import pytest

from providers import http
from providers.base import BaseProvider, Credit, MetadataRecord
from providers.openlibrary import CANDIDATES, FIELDS, OpenLibraryProvider, clean_subjects
from providers.query import read_file_title
from providers.registry import ProviderRegistry


def reply(status=200, data=None, raises=None, bad_json=False):
    response = MagicMock()
    response.status_code = status
    if bad_json:
        response.json.side_effect = ValueError('not json')
    else:
        response.json.return_value = data
    return response


class TestWhoIsAsking:
    def test_the_project_is_always_said_and_the_contact_only_when_the_owner_gave_one(self):
        assert http.user_agent({}) == 'Codice (+https://github.com/ocnaibill/codice)'
        assert http.user_agent({'CODICE_CONTACT': '  '}) == 'Codice (+https://github.com/ocnaibill/codice)'
        assert http.user_agent({'CODICE_CONTACT': 'dono@exemplo.org'}) == 'Codice (+https://github.com/ocnaibill/codice; dono@exemplo.org)'

    def test_with_a_contact_the_pause_is_the_shorter_one(self):
        assert http.min_interval({}) == 1.05
        assert http.min_interval({'CODICE_CONTACT': 'a@b.c'}) == 0.35
        assert http.min_interval({'CODICE_CONTACT': ' '}) == 1.05

    def test_two_requests_to_the_same_host_are_apart_and_two_hosts_are_not(self, monkeypatch):
        now, slept = [100.0], []
        monkeypatch.setattr(http, '_clock', lambda: now[0])
        monkeypatch.setattr(http, '_sleep', lambda s: (slept.append(round(s, 2)), now.__setitem__(0, now[0] + s)))
        http._last_call.clear()
        http._wait('a.org', 1.0)
        http._wait('a.org', 1.0)
        http._wait('b.org', 1.0)
        now[0] += 5
        http._wait('a.org', 1.0)
        assert slept == [1.0]

    def test_a_half_waited_pause_is_only_what_is_left(self, monkeypatch):
        now, slept = [10.0], []
        monkeypatch.setattr(http, '_clock', lambda: now[0])
        monkeypatch.setattr(http, '_sleep', lambda s: slept.append(round(s, 2)))
        http._last_call.clear()
        http._wait('a.org', 1.0)
        now[0] += 0.4
        http._wait('a.org', 1.0)
        assert slept == [0.6]


class TestAskingAProvider:
    @patch('providers.http.requests.get')
    def test_an_answer_is_the_json_and_the_request_says_who_asks(self, mock_get, monkeypatch):
        monkeypatch.setenv('CODICE_CONTACT', 'dono@exemplo.org')
        mock_get.return_value = reply(200, {'a': 1})
        got = http.get_json('X', 'https://x.org/a', params={'q': 'dune'}, headers={'X-Key': '1'})
        assert got.ok and got.status == 200 and got.data == {'a': 1}
        _, kwargs = mock_get.call_args
        assert kwargs['params'] == {'q': 'dune'} and kwargs['timeout'] == 10
        assert kwargs['headers']['User-Agent'].endswith('; dono@exemplo.org)') and kwargs['headers']['X-Key'] == '1'
        assert kwargs['headers']['Accept'] == 'application/json'

    @patch('providers.http.requests.get')
    def test_two_requests_to_the_same_host_are_apart(self, mock_get, monkeypatch):
        now, slept = [50.0], []
        monkeypatch.setattr(http, '_clock', lambda: now[0])
        monkeypatch.setattr(http, '_sleep', lambda s: (slept.append(round(s, 2)), now.__setitem__(0, now[0] + s)))
        monkeypatch.delenv('CODICE_CONTACT', raising=False)
        http._last_call.clear()
        mock_get.return_value = reply(200, {})
        http.get_json('X', 'https://x.org/a')
        http.get_json('X', 'https://x.org/b')
        http.get_json('Y', 'https://y.org/a')
        assert slept == [1.05]
        http.get_json('Z', 'https://z.org/a', interval=0)
        http.get_json('Z', 'https://z.org/b', interval=0)
        assert slept == [1.05]

    @patch('providers.http.requests.get')
    def test_a_header_given_can_replace_the_default_one(self, mock_get):
        mock_get.return_value = reply(200, {})
        http.get_json('X', 'https://x.org/a', headers={'Accept': 'text/plain'})
        assert mock_get.call_args[1]['headers']['Accept'] == 'text/plain'

    @patch('providers.http.requests.get')
    def test_a_status_that_is_not_200_is_said_in_the_log_with_what_it_means(self, mock_get, capsys):
        for status, words in [(429, 'quota'), (401, 'key'), (403, 'not allowed'), (404, 'nothing there'), (400, 'badly made')]:
            mock_get.return_value = reply(status)
            got = http.get_json('Google Books', 'https://x.org/a')
            assert not got.ok and got.status == status and got.data is None
            assert f'Google Books: HTTP {status}' in capsys.readouterr().out and words in got.problem

    @patch('providers.http.requests.get')
    def test_another_status_is_said_without_a_hint(self, mock_get, capsys):
        mock_get.return_value = reply(503)
        got = http.get_json('X', 'https://x.org/a')
        assert got.problem == 'HTTP 503' and 'X: HTTP 503' in capsys.readouterr().out

    @patch('providers.http.requests.get')
    def test_an_answer_that_is_not_json_is_a_problem(self, mock_get, capsys):
        mock_get.return_value = reply(200, bad_json=True)
        got = http.get_json('X', 'https://x.org/a')
        assert not got.ok and got.status == 200 and got.data is None and 'not JSON' in capsys.readouterr().out

    @patch('providers.http.requests.get')
    def test_a_request_that_never_got_there_is_status_zero_and_never_writes_the_secret(self, mock_get, capsys):
        mock_get.side_effect = RuntimeError('boom https://x.org/a?key=SEGREDO123')
        got = http.get_json('X', 'https://x.org/a', secrets=('SEGREDO123',))
        out = capsys.readouterr().out
        assert got.status == 0 and not got.ok and 'SEGREDO123' not in out and 'SEGREDO123' not in got.problem and '***' in out

    @patch('providers.http.requests.post')
    def test_a_post_sends_the_body_as_json(self, mock_post):
        mock_post.return_value = reply(200, {'ok': True})
        got = http.get_json('X', 'https://x.org/graphql', post={'query': 'q'})
        assert got.ok and mock_post.call_args[1]['json'] == {'query': 'q'}

    @patch('providers.http.requests.get')
    def test_a_reply_with_no_data_is_not_ok(self, mock_get):
        mock_get.return_value = reply(200, None)
        assert not http.get_json('X', 'https://x.org/a').ok


DUNE = {'key': '/works/OL1W', 'title': 'Dune', 'author_name': ['Frank Herbert'], 'author_key': ['OL79034A'], 'first_publish_year': 1965,
        'isbn': ['0801950775', '9780340839935'], 'publisher': ['Ace', 'Hodder'], 'cover_i': 99, 'edition_count': 40,
        'subject': ['Fiction', 'Science fiction', 'nyt:list=2021', 'Large type books'], 'series_name': ['Dune'], 'series_position': ['1']}


def asked(mock_get):
    return [(c[0][0], c[1]['params']) for c in mock_get.call_args_list]


class TestOpenLibrary:
    def setup_method(self):
        self.provider = OpenLibraryProvider()

    @patch('providers.http.requests.get')
    def test_the_search_asks_for_every_field_the_record_is_made_of_and_for_several_works(self, mock_get):
        mock_get.return_value = reply(200, {'docs': [DUNE]})
        self.provider.lookup(read_file_title('Dune', 'Frank Herbert', 'epub'))
        url, params = asked(mock_get)[0]
        assert url == 'https://openlibrary.org/search.json'
        assert params == {'title': 'Dune', 'limit': CANDIDATES, 'fields': FIELDS} and CANDIDATES == 20
        for field in ('isbn', 'publisher', 'subject', 'subtitle', 'cover_i', 'author_key', 'series_name'):
            assert field in FIELDS.split(',')

    @patch('providers.http.requests.get')
    def test_a_record_is_made_of_the_work(self, mock_get):
        mock_get.return_value = reply(200, {'docs': [DUNE]})
        (r,) = self.provider.lookup(read_file_title('Dune', None, 'epub'))
        assert (r.title, r.author, r.publisher, r.original_year, r.isbn, r.source) == ('Dune', 'Frank Herbert', 'Ace', '1965', '9780340839935', 'OpenLibrary')
        assert r.publication_date is None   # the first year of the work is not the date of an edition (DEC-156)
        assert r.cover_url == 'https://covers.openlibrary.org/b/id/99-L.jpg'
        assert r.credits[0].ids == {'openlibrary': 'OL79034A'}
        assert r.tags == ['Fiction', 'Science fiction'] and (r.series, r.series_index) == ('Dune', 1.0)
        assert r.prior == 10.0 and r.raw['openlibrary_work'] == '/works/OL1W'

    @patch('providers.http.requests.get')
    def test_a_work_with_little_in_it_is_a_record_with_little_in_it(self, mock_get):
        mock_get.return_value = reply(200, {'docs': [{'key': '/works/OL2W', 'title': 'Obscure'}]})
        (r,) = self.provider.lookup(read_file_title('Obscure', None))
        assert (r.author, r.publisher, r.original_year, r.isbn, r.cover_url, r.tags, r.series, r.series_index, r.prior) == (None, None, None, None, None, [], None, None, 0.0)

    @patch('providers.http.requests.get')
    def test_a_position_that_is_not_a_number_is_left_out(self, mock_get):
        mock_get.return_value = reply(200, {'docs': [{'key': '/works/OL2W', 'title': 'X', 'series_name': ['S'], 'series_position': ['one']}]})
        (r,) = self.provider.lookup(read_file_title('X', None))
        assert r.series == 'S' and r.series_index is None

    @patch('providers.http.requests.get')
    def test_when_one_answer_is_close_it_does_not_ask_again(self, mock_get):
        mock_get.return_value = reply(200, {'docs': [DUNE]})
        self.provider.lookup(read_file_title('Dune', None, 'epub'))
        assert len(asked(mock_get)) == 1

    @patch('providers.http.requests.get')
    def test_when_none_is_close_it_asks_again_by_the_whole_text_and_keeps_each_work_once(self, mock_get):
        other = {'key': '/works/OL3W', 'title': 'Harry Potter and the Philosopher\'s Stone', 'author_name': ['J. K. Rowling']}
        mock_get.side_effect = [reply(200, {'docs': [{'key': '/works/OL9W', 'title': 'Cartas'}]}),
                                reply(200, {'docs': [{'key': '/works/OL9W', 'title': 'Cartas'}, other]})]
        records = self.provider.lookup(read_file_title('Harry Potter e a Pedra Filosofal', None, 'epub'))
        assert [p[1].get('q') for p in asked(mock_get)] == [None, 'Harry Potter e a Pedra Filosofal']
        assert [r.title for r in records] == ['Cartas', "Harry Potter and the Philosopher's Stone"]

    @patch('providers.http.requests.get')
    def test_a_title_close_to_a_part_of_the_file_s_text_counts(self, mock_get):
        mock_get.return_value = reply(200, {'docs': [{'key': '/works/OL4W', 'title': 'Sapiens'}]})
        self.provider.lookup(read_file_title('Sapiens: Uma breve história da humanidade', 'Yuval Noah Harari', 'epub'))
        assert len(asked(mock_get)) == 1

    @patch('providers.http.requests.get')
    def test_only_the_title_is_sent_never_the_author(self, mock_get):
        mock_get.return_value = reply(200, {'docs': []})
        self.provider.lookup(read_file_title('Dune - Frank Herbert', 'Frank Herbert', 'epub'))
        for url, params in asked(mock_get):
            assert 'Herbert' not in url and 'Herbert' not in str(params) and 'author' not in params

    @patch('providers.http.requests.get')
    def test_a_title_with_nothing_in_it_asks_nothing(self, mock_get):
        assert self.provider.lookup(read_file_title('', None)) == [] and mock_get.call_count == 0

    @patch('providers.http.requests.get')
    def test_a_search_that_fails_has_no_answers(self, mock_get):
        mock_get.return_value = reply(429)
        assert self.provider.lookup(read_file_title('Dune', None)) == []
        mock_get.return_value = reply(200, {})
        assert self.provider.lookup(read_file_title('Dune', None)) == []

    @patch('providers.http.requests.get')
    def test_the_manual_search_still_gives_the_first_work_of_a_text(self, mock_get):
        mock_get.return_value = reply(200, {'docs': [DUNE]})
        assert self.provider.search('dune').title == 'Dune'
        assert asked(mock_get)[0][1]['q'] == 'dune'
        assert self.provider.search('') is None
        mock_get.return_value = reply(200, {'docs': []})
        assert self.provider.search('x') is None

    @patch('providers.http.requests.get')
    def test_the_description_is_asked_for_in_the_work_and_the_subjects_join_the_tags(self, mock_get):
        mock_get.return_value = reply(200, {'description': {'value': '  A desert planet.  '}, 'subjects': ['Ecology', 'Science fiction', 'Award:hugo=1966']})
        record = MetadataRecord(title='Dune', tags=['Fiction', 'Science fiction'], raw={'openlibrary_work': '/works/OL1W'})
        out = self.provider.enrich(record)
        assert asked(mock_get)[0][0] == 'https://openlibrary.org/works/OL1W.json'
        assert out is record and out.description == 'A desert planet.'
        assert out.tags == ['Fiction', 'Science fiction', 'Ecology'] and 'Award:hugo=1966' not in out.tags

    @patch('providers.http.requests.get')
    def test_a_description_that_is_plain_text_is_kept_and_an_empty_one_is_not(self, mock_get):
        mock_get.return_value = reply(200, {'description': 'Plain.'})
        assert self.provider.enrich(MetadataRecord(raw={'openlibrary_work': '/works/OL1W'})).description == 'Plain.'
        for empty in ('   ', None, 5, {'type': 'x'}):
            mock_get.return_value = reply(200, {'description': empty})
            assert self.provider.enrich(MetadataRecord(description='before', raw={'openlibrary_work': '/works/OL1W'})).description == 'before'

    @patch('providers.http.requests.get')
    def test_a_work_that_cannot_be_read_leaves_the_answer_as_it_was(self, mock_get):
        mock_get.return_value = reply(500)
        record = MetadataRecord(title='Dune', tags=['a'], raw={'openlibrary_work': '/works/OL1W'})
        assert self.provider.enrich(record).tags == ['a'] and record.description is None
        mock_get.reset_mock()
        assert self.provider.enrich(MetadataRecord(title='x')).title == 'x' and mock_get.call_count == 0

    def test_the_names_of_the_provider_are_the_ones_the_owner_chooses_by(self):
        assert (self.provider.id, self.provider.name) == ('openlibrary', 'OpenLibrary')

    def test_the_subjects_that_are_about_the_book_are_kept_each_once(self):
        got = clean_subjects(['Dune (Imaginary place)', 'Fiction', 'Science fiction', 'Science-fiction', 'SCIENCE FICTION', 'nyt:mass=2021', 'award:hugo=1966',
                              'Large type books', 'Translations into Russian', 'Translation from French', 'Accessible book', 'Protected DAISY', 'In library',
                              'Lending library', 'overdrive', 'New York Times bestseller', '', '  ', 5, None, 'American literature'])
        assert got == ['Dune (Imaginary place)', 'Fiction', 'Science fiction', 'American literature']

    def test_only_the_first_eight_subjects_are_kept(self):
        assert len(clean_subjects([f'Assunto {i}' for i in range(20)])) == 8
        assert len(clean_subjects([f'Assunto {i}' for i in range(20)], limit=3)) == 3
        assert clean_subjects(None) == []


class Fake(BaseProvider):
    """A provider with a fixed list of answers, which says what it was asked and whether it was asked to complete one."""

    def __init__(self, pid, records, fail=False, enrich_fails=False):
        self._id, self.records, self.fail, self.enrich_fails = pid, records, fail, enrich_fails
        self.asked, self.enriched = [], []

    id = property(lambda self: self._id)
    name = property(lambda self: self._id.title())

    def search(self, query):
        raise AssertionError('a provider with a lookup is asked by it')

    def lookup(self, query):
        self.asked.append(query)
        if self.fail:
            raise RuntimeError('down')
        return list(self.records)

    def enrich(self, record):
        self.enriched.append(record)
        if self.enrich_fails:
            raise RuntimeError('cannot')
        record.description = 'completed'
        return record


def book(title, author, prior=0, **kw):
    r = MetadataRecord(title=title, credits=[Credit(author)] if author else [], source='x', prior=prior, **kw)
    r.author = author
    return r


def registry(allowed, *providers):
    r = ProviderRegistry(enabled=(lambda pid: pid in allowed) if allowed is not None else None)
    r._providers = {'default': list(providers), 'cbz': list(providers)}
    return r


class TestChoosingTheAnswer:
    def test_the_answer_that_is_the_work_is_the_one(self):
        a = Fake('google_books', [book('Duna', 'Brian Herbert'), book('Dune', 'Frank Herbert')])
        got = registry({'google_books'}, a).search_best('Dune', 'epub', author='Frank Herbert')
        assert got.author == 'Frank Herbert' and got.match['accepted'] and got.match['title'] == 1.0

    def test_nothing_close_is_none_and_it_says_why(self, capsys):
        a = Fake('openlibrary', [book('Herdeiras de Duna', 'Frank Herbert')])
        assert registry({'openlibrary'}, a).search_best('Duna', 'epub', author='Frank Herbert') is None
        assert "Nothing close enough: the nearest, 'Herdeiras de Duna'" in capsys.readouterr().out

    def test_an_answer_by_someone_else_is_none(self, capsys):
        a = Fake('openlibrary', [book('A nuvem', 'Carlos Poças Falcão')])
        assert registry({'openlibrary'}, a).search_best('A Nuvem', 'epub', author='Neal Shusterman') is None
        assert '(author,' in capsys.readouterr().out

    def test_no_answer_at_all_is_none(self, capsys):
        a = Fake('openlibrary', [])
        assert registry({'openlibrary'}, a).search_best('Dune', 'epub') is None
        out = capsys.readouterr().out
        assert 'Openlibrary: no results' in out and 'No metadata found' in out

    def test_the_best_of_the_providers_wins_and_only_it_is_completed(self):
        a = Fake('google_books', [book('Dune', None)])
        b = Fake('openlibrary', [book('Dune', 'Frank Herbert', prior=10)])
        got = registry({'google_books', 'openlibrary'}, a, b).search_best('Dune', 'epub', author='Frank Herbert')
        assert got.author == 'Frank Herbert' and got.description == 'completed'
        assert a.enriched == [] and len(b.enriched) == 1

    def test_the_description_is_only_asked_for_the_chosen_one(self):
        a = Fake('openlibrary', [book('Dune', 'Frank Herbert', prior=2), book('Dune', 'Frank Herbert', prior=9), book('Dune', 'Frank Herbert', prior=5)])
        got = registry({'openlibrary'}, a).search_best('Dune', 'epub')
        assert len(a.enriched) == 1 and a.enriched[0] is got and got.prior == 9

    def test_an_answer_that_cannot_be_completed_is_still_the_answer(self, capsys):
        a = Fake('openlibrary', [book('Dune', 'Frank Herbert')], enrich_fails=True)
        got = registry({'openlibrary'}, a).search_best('Dune', 'epub')
        assert got is not None and got.description is None and 'could not complete the answer' in capsys.readouterr().out

    def test_a_provider_that_fails_does_not_stop_the_others(self, capsys):
        a, b = Fake('google_books', [], fail=True), Fake('openlibrary', [book('Dune', None)])
        got = registry({'google_books', 'openlibrary'}, a, b).search_best('Dune', 'epub')
        assert got is not None and 'Google_Books failed: down' in capsys.readouterr().out

    def test_only_the_providers_that_are_on_are_asked(self):
        a, b = Fake('google_books', [book('Dune', None)]), Fake('openlibrary', [book('Dune', None)])
        registry({'openlibrary'}, a, b).search_best('Dune', 'epub')
        assert a.asked == [] and len(b.asked) == 1
        c = Fake('openlibrary', [book('Dune', None)])
        assert registry(None, c).search_best('Dune', 'epub') is None and c.asked == []

    def test_the_provider_is_given_what_the_file_says_but_only_the_title_is_for_sending(self):
        a = Fake('openlibrary', [])
        registry({'openlibrary'}, a).search_best('Absolute Batman 006 (2025)', 'cbz', author='Unknown Author')
        (q,) = a.asked
        assert q.search_title == 'Absolute Batman' and q.number == 6 and q.author is None and q.serial

    def test_the_title_the_file_has_with_the_author_after_a_dash_is_found(self):
        a = Fake('openlibrary', [book('Scythe', 'Neal Shusterman')])
        assert registry({'openlibrary'}, a).search_best('Scythe - Neal Shusterman', 'pdf') is not None

    def test_a_record_with_no_title_nor_author_is_ignored(self):
        a = Fake('openlibrary', [MetadataRecord(source='x'), book('Dune', None)])
        assert registry({'openlibrary'}, a).search_best('Dune', 'epub').title == 'Dune'

    def test_a_provider_that_only_knows_how_to_search_is_asked_by_the_title(self):
        class OldWay:
            id, name = 'google_books', 'Google Books'

            def __init__(self):
                self.asked = []

            def search(self, text):
                self.asked.append(text)
                return book('Dune', 'Frank Herbert')
        old = OldWay()
        assert registry({'google_books'}, old).search_best('Dune - Frank Herbert', 'epub', author='Frank Herbert').title == 'Dune'
        assert old.asked == ['Dune']
        silent = type('Silent', (), {'id': 'google_books', 'name': 'G', 'search': lambda self, t: None})()
        assert registry({'google_books'}, silent).search_best('Dune', 'epub') is None

    def test_the_manual_search_gives_the_closest_first_up_to_five_a_provider_close_or_not(self):
        a = Fake('openlibrary', [book(f'Livro {i}', None, prior=i) for i in range(8)] + [book('Dune', None)])
        got = registry({'openlibrary'}, a).search_all('Dune', 'epub')
        assert got[0].title == 'Dune' and len(got) == 5 and all(r.match for r in got)

    def test_the_manual_search_keeps_each_providers_five_and_orders_all_by_closeness(self):
        a = Fake('google_books', [book('Dune Messiah', None), book('Dune', None)])
        b = Fake('openlibrary', [book('Children of Dune', None), book('Dune', None, prior=5)])
        got = registry({'google_books', 'openlibrary'}, a, b).search_all('Dune', 'epub', per_provider=1)
        assert [r.title for r in got] == ['Dune', 'Dune'] and got[0].prior == 5

    def test_the_manual_search_leaves_out_an_answer_with_nothing_in_it(self):
        a = Fake('openlibrary', [MetadataRecord(source='x'), book('Dune', None)])
        assert [r.title for r in registry({'openlibrary'}, a).search_all('Dune', 'epub')] == ['Dune']

    def test_the_manual_search_orders_every_answer_of_every_provider_by_closeness(self):
        a = Fake('google_books', [book('Dune', None)])
        b = Fake('openlibrary', [book('Dune', None, prior=5), book('Dune Messiah', None)])
        got = registry({'google_books', 'openlibrary'}, a, b).search_all('Dune', 'epub', per_provider=2)
        assert [(r.title, r.prior) for r in got] == [('Dune', 5), ('Dune', 0), ('Dune Messiah', 0)]

    def test_the_manual_search_finds_nothing_when_no_provider_is_on(self, capsys):
        assert registry(None, Fake('openlibrary', [book('Dune', None)])).search_all('Dune', 'epub') == []
        assert 'No metadata found' in capsys.readouterr().out

    def test_search_gives_the_first_answer_of_the_first_provider_that_has_one(self):
        a, b = Fake('google_books', []), Fake('openlibrary', [book('Dune', None)])
        got = registry({'google_books', 'openlibrary'}, a, b).search('Dune', 'epub')
        assert got.title == 'Dune' and got.match
        assert registry({'google_books'}, Fake('google_books', [])).search('Dune', 'epub') is None

    def test_search_goes_on_when_a_provider_fails(self):
        a, b = Fake('google_books', [], fail=True), Fake('openlibrary', [book('Dune', None)])
        assert registry({'google_books', 'openlibrary'}, a, b).search('Dune', 'epub').title == 'Dune'


class TestTheAuthorNeverLeaves:
    @patch('providers.http.requests.get')
    def test_what_is_sent_to_open_library_is_the_title_and_nothing_of_the_author(self, mock_get):
        mock_get.side_effect = [reply(200, {'docs': [DUNE]}), reply(200, {'description': 'A desert planet.', 'subjects': []})]
        r = ProviderRegistry(enabled=lambda pid: pid == 'openlibrary')
        r._providers = {'default': [OpenLibraryProvider()]}
        got = r.search_best('Dune', 'epub', author='Frank Herbert')
        assert got is not None and got.description == 'A desert planet.'
        assert len(asked(mock_get)) == 2   # the title, and the description of the work that was chosen
        for url, params in asked(mock_get):
            assert 'Herbert' not in url and 'Herbert' not in str(params)

    @patch('providers.http.requests.get')
    def test_not_even_when_the_author_is_in_the_title_of_the_file(self, mock_get):
        mock_get.return_value = reply(200, {'docs': []})
        r = ProviderRegistry(enabled=lambda pid: pid == 'openlibrary')
        r._providers = {'default': [OpenLibraryProvider()]}
        r.search_best('A Nuvem 2 - Neal Shusterman', 'pdf', author=None)
        for url, params in asked(mock_get):
            assert 'Shusterman' not in url and 'Shusterman' not in str(params)
