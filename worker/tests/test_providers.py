"""Tests for metadata providers.

Uses mocked HTTP responses to test each provider's parsing.
"""
import pytest
from unittest.mock import patch, MagicMock
from providers.base import Credit, MetadataRecord
from providers.google_books import GoogleBooksProvider
from providers.openlibrary import OpenLibraryProvider
from providers.comicvine import ComicVineProvider
from providers.registry import ProviderRegistry


class TestGoogleBooksProvider:
    def setup_method(self):
        self.provider = GoogleBooksProvider()

    @patch('providers.google_books.requests.get')
    def test_search_returns_record(self, mock_get):
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "items": [{
                "volumeInfo": {
                    "title": "Test Book",
                    "authors": ["John Author"],
                    "publisher": "Test Publisher",
                    "language": "en",
                    "publishedDate": "2023",
                    "description": "A test book description.",
                    "categories": ["Fiction", "Science Fiction"],
                    "industryIdentifiers": [{"type": "ISBN_13", "identifier": "9781234567890"}],
                    "imageLinks": {"thumbnail": "http://example.com/cover.jpg"},
                }
            }]
        }
        mock_get.return_value = mock_response

        result = self.provider.search("Test Book")
        assert result is not None
        assert result.title == "Test Book"
        assert result.author == "John Author"
        assert result.isbn == "9781234567890"
        assert "Fiction" in result.tags
        assert result.cover_url is not None
        assert result.cover_url.startswith("https://")

    @patch('providers.google_books.requests.get')
    def test_search_empty_returns_none(self, mock_get):
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"items": []}
        mock_get.return_value = mock_response

        result = self.provider.search("Nonexistent Book XYZ")
        assert result is None


class TestOpenLibraryProvider:
    def setup_method(self):
        self.provider = OpenLibraryProvider()

    @patch('providers.openlibrary.requests.get')
    def test_search_returns_record(self, mock_get):
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "docs": [{
                "title": "Open Library Book",
                "author_name": ["Jane Author"],
                "publisher": ["Open Publisher"],
                "language": ["en"],
                "first_publish_year": 2022,
                "isbn": ["9780987654321"],
                "cover_i": 12345,
                "subject": ["Science", "Technology"],
            }]
        }
        mock_get.return_value = mock_response

        result = self.provider.search("Open Library Book")
        assert result is not None
        assert result.title == "Open Library Book"
        assert result.author == "Jane Author"
        assert result.cover_url == "https://covers.openlibrary.org/b/id/12345-L.jpg"

    @patch('providers.openlibrary.requests.get')
    def test_search_http_error(self, mock_get):
        mock_response = MagicMock()
        mock_response.status_code = 500
        mock_get.return_value = mock_response

        result = self.provider.search("Any Book")
        assert result is None


class TestComicVineProvider:
    def setup_method(self):
        self.provider = ComicVineProvider()

    def test_no_api_key_returns_none(self):
        import os
        key = os.environ.pop("COMICVINE_API_KEY", None)
        result = self.provider.search("Batman")
        assert result is None
        if key:
            os.environ["COMICVINE_API_KEY"] = key

    @patch('providers.comicvine.requests.get')
    @patch('providers.comicvine.ComicVineProvider.__init__', return_value=None)
    def test_search_returns_record(self, mock_init, mock_get):
        import os
        os.environ['COMICVINE_API_KEY'] = 'test_key'
        self.provider = ComicVineProvider()
        self.provider.api_key = 'test_key'
        self.provider.base_url = 'https://comicvine.gamespot.com/api'
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "results": [{
                "name": "Batman #1",
                "issue_number": "1",
                "description": "The first issue.",
                "volume": {"name": "Batman"},
                "image": {"super_url": "https://example.com/cover.jpg"},
                "cover_date": "2024-01-01",
            }]
        }
        mock_get.return_value = mock_response

        result = self.provider.search("Batman")
        assert result is not None
        assert result.title == "Batman #1"
        assert result.series == "Batman"
        assert result.series_index == 1.0


class TestProviderRegistry:
    def test_registry_default_has_providers(self):
        registry = ProviderRegistry()
        assert len(registry._providers['default']) > 0

    def test_registry_cbz_has_comicvine(self):
        registry = ProviderRegistry()
        provider_names = [p.name for p in registry._providers['cbz']]
        assert 'ComicVine' in provider_names

def _openlibrary(doc):
    response = MagicMock()
    response.status_code = 200
    response.json.return_value = {"docs": [doc]}
    with patch('providers.openlibrary.requests.get', return_value=response):
        return OpenLibraryProvider().search("Good Omens")


class TestCredits:
    def test_open_library_reads_every_author_with_the_key_of_each(self):
        result = _openlibrary({"title": "Good Omens", "author_name": ["Terry Pratchett", "Neil Gaiman"],
                               "author_key": ["OL25712A", "OL53305A"]})
        assert [c.name for c in result.credits] == ["Terry Pratchett", "Neil Gaiman"]
        assert [c.ids for c in result.credits] == [{"openlibrary": "OL25712A"}, {"openlibrary": "OL53305A"}]
        assert result.author == "Terry Pratchett"  # the first one stays what `author` has always been

    def test_open_library_uses_no_key_when_the_lists_do_not_line_up(self):
        # Which key belongs to which name is not known, so none is given: a wrong identifier is worse than none.
        result = _openlibrary({"title": "T", "author_name": ["A One", "B Two"], "author_key": ["OL1A"]})
        assert [c.name for c in result.credits] == ["A One", "B Two"]
        assert all(c.ids == {} for c in result.credits)

    def test_open_library_without_keys_still_names_the_authors(self):
        result = _openlibrary({"title": "T", "author_name": ["A One"]})
        assert result.credits == [Credit("A One")]

    def test_open_library_with_a_blank_name_does_not_shift_the_keys_of_the_others(self):
        result = _openlibrary({"title": "T", "author_name": ["A One", " ", "C Three"],
                               "author_key": ["OL1A", "OL2A", "OL3A"]})
        assert [c.name for c in result.credits] == ["A One", "C Three"]
        assert [c.ids for c in result.credits] == [{}, {}]  # the lists no longer line up once a name is dropped

    def test_open_library_without_authors_has_none(self):
        result = _openlibrary({"title": "Anonymous"})
        assert result.credits == [] and result.author is None

    @patch('providers.google_books.requests.get')
    def test_google_books_reads_every_author_and_no_identifier(self, mock_get):
        mock_get.return_value = MagicMock(status_code=200, json=MagicMock(return_value={"items": [
            {"volumeInfo": {"title": "T", "authors": ["A One", "  ", "B Two"]}}]}))
        result = GoogleBooksProvider().search("T")
        assert result.credits == [Credit("A One"), Credit("B Two")]
        assert result.author == "A One"

    @patch('providers.comicvine.requests.get')
    def test_comicvine_keeps_the_role_and_id_of_each_credit(self, mock_get):
        mock_get.return_value = MagicMock(status_code=200, json=MagicMock(return_value={"results": [{
            "name": "Batman #1", "issue_number": "1", "volume": {"name": "Batman"},
            "person_credits": [{"id": 11, "name": "Penciller Person", "role": "penciller"},
                               {"id": 12, "name": "Writer Person", "role": "writer, inker"},
                               {"name": "No Id", "role": ""}]}]}))
        provider = ComicVineProvider()
        provider.api_key = 'k'
        provider.base_url = 'https://comicvine.gamespot.com/api'
        result = provider.search("Batman")
        assert [(c.name, c.role, c.ids) for c in result.credits] == [
            ("Penciller Person", "penciller", {"comicvine": "11"}),
            ("Writer Person", "writer, inker", {"comicvine": "12"}),
            ("No Id", None, {})]

    def test_a_credit_leaves_out_what_the_provider_did_not_say(self):
        assert Credit("A One").as_dict() == {"name": "A One"}
        assert Credit("A One", "writer", {"openlibrary": "OL1A"}).as_dict() == {
            "name": "A One", "role": "writer", "ids": {"openlibrary": "OL1A"}}


class TestNoLanguageFromProviders:
    """The language of a file is read from the file (DEC-096): a provider says the language of some edition."""

    def test_open_library_does_not_give_the_language_of_an_edition_as_the_files(self):
        result = _openlibrary({"title": "Good Omens", "author_name": ["A One"], "language": ["cat", "eng"]})
        assert result.language is None

    @patch('providers.google_books.requests.get')
    def test_google_books_does_not_give_the_language_of_the_volume_it_matched(self, mock_get):
        mock_get.return_value = MagicMock(status_code=200, json=MagicMock(return_value={"items": [
            {"volumeInfo": {"title": "Duna", "authors": ["A One"], "language": "pt"}}]}))
        assert GoogleBooksProvider().search("Duna").language is None


class FakeProvider:
    """A provider that records that it was asked: what the gate is for is that it is not."""

    def __init__(self, pid, title):
        self.id = pid
        self.name = pid.title()
        self._title = title
        self.asked = []

    def search(self, query):
        self.asked.append(query)
        return MetadataRecord(title=self._title, source=self.name)


def _registry(allowed, *providers):
    registry = ProviderRegistry(enabled=(lambda pid: pid in allowed) if allowed is not None else None)
    registry._providers = {'default': list(providers)}
    return registry


class TestNothingIsAskedUnlessTheOwnerTurnedItOn:
    def test_without_a_gate_no_provider_is_asked(self):
        a = FakeProvider('openlibrary', 'A')
        registry = _registry(None, a)
        assert registry.search('Duna') is None
        assert registry.search_all('Duna') == []
        assert registry.search_best('Duna') is None
        assert a.asked == []

    def test_a_provider_that_is_off_is_not_asked_and_the_next_one_answers(self):
        a, b = FakeProvider('google_books', 'A'), FakeProvider('openlibrary', 'B')
        registry = _registry({'openlibrary'}, a, b)
        assert registry.search('Duna').title == 'B'
        assert a.asked == [] and b.asked == ['Duna']

    def test_search_all_asks_only_the_ones_that_are_on_in_their_order(self):
        a, b, c = FakeProvider('google_books', 'A'), FakeProvider('openlibrary', 'B'), FakeProvider('comicvine', 'C')
        registry = _registry({'google_books', 'comicvine'}, a, b, c)
        assert [r.title for r in registry.search_all('Duna')] == ['A', 'C']
        assert b.asked == []

    def test_every_provider_on_is_the_way_it_always_was(self):
        a, b = FakeProvider('google_books', 'A'), FakeProvider('openlibrary', 'B')
        registry = _registry({'google_books', 'openlibrary'}, a, b)
        assert registry.search('Duna').title == 'A'
        assert b.asked == []  # the first answer is enough, as before

    def test_a_provider_the_gate_does_not_know_is_off(self):
        a = FakeProvider('something_new', 'A')
        assert _registry({'openlibrary'}, a).search('Duna') is None
        assert a.asked == []

    def test_the_real_providers_have_the_ids_the_owner_chooses_by(self):
        assert [p.id for p in ProviderRegistry()._providers['default']] == ['google_books', 'openlibrary']
        assert [p.id for p in ProviderRegistry()._providers['cbz']] == ['comicvine', 'google_books', 'openlibrary']


class FakeSettings:
    def __init__(self, value=None, boom=False):
        self.value, self.boom, self.reads = value, boom, 0

    def fetchone(self, query, params=()):
        self.reads += 1
        if self.boom:
            raise RuntimeError('database down')
        assert 'FROM settings' in query and params == ('metadata.providers',)
        return None if self.value is None else (self.value,)


class TestTheGate:
    def test_only_a_provider_set_to_true_is_on(self):
        from providers.gate import db_gate
        gate = db_gate(FakeSettings({'openlibrary': True, 'google_books': False, 'comicvine': 'true', 'x': 1}))
        assert gate('openlibrary') is True
        assert gate('google_books') is False
        assert gate('comicvine') is False and gate('x') is False  # not a yes the owner gave
        assert gate('not_listed') is False

    def test_nothing_is_on_without_a_setting_or_with_one_that_is_not_a_choice(self):
        from providers.gate import db_gate
        for value in (None, [], 'null', '[1]', 5):
            assert db_gate(FakeSettings(value))('openlibrary') is False

    def test_a_setting_that_comes_as_text_is_read(self):
        from providers.gate import db_gate
        assert db_gate(FakeSettings('{"openlibrary": true}'))('openlibrary') is True
        assert db_gate(FakeSettings(b'{"openlibrary": true}'))('openlibrary') is True

    def test_when_the_setting_cannot_be_read_nothing_is_on(self):
        from providers.gate import db_gate
        assert db_gate(FakeSettings({'openlibrary': True}, boom=True))('openlibrary') is False
        assert db_gate(FakeSettings('not json'))('openlibrary') is False

    def test_it_reads_the_setting_every_time_so_turning_one_off_counts_for_the_next_work(self):
        from providers.gate import db_gate
        db = FakeSettings({'openlibrary': True})
        gate = db_gate(db)
        assert gate('openlibrary') is True
        db.value = {'openlibrary': False}
        assert gate('openlibrary') is False
        assert db.reads == 2


class FakeStateDB:
    def __init__(self, boom=False):
        self.boom, self.statements = boom, []

    def execute(self, query, params=()):
        if self.boom:
            raise RuntimeError('database down')
        self.statements.append((' '.join(query.split()), params))


class TestApiKeys:
    def test_a_key_is_configured_only_when_it_is_set_to_something(self):
        from providers.gate import key_configured
        assert key_configured('comicvine', {'COMICVINE_API_KEY': 'abc'}) is True
        for value in ('', '   ', None):
            assert key_configured('comicvine', {'COMICVINE_API_KEY': value}) is False
        assert key_configured('comicvine', {}) is False
        assert key_configured('google_books', {'COMICVINE_API_KEY': 'abc'}) is False  # each its own variable
        assert key_configured('google_books', {'GOOGLE_BOOKS_API_KEY': 'k'}) is True

    def test_the_worker_tells_whether_each_key_is_set_and_never_the_key(self):
        import json
        from providers.gate import report_keys
        db = FakeStateDB()
        state = report_keys(db, {'COMICVINE_API_KEY': 'super-secret-value', 'GOOGLE_BOOKS_API_KEY': ''})
        assert state == {'google_books': False, 'comicvine': True}
        (query, params), = db.statements
        assert 'INSERT INTO settings' in query and 'ON CONFLICT (key) DO UPDATE' in query
        assert params[0] == 'metadata.providers.keys'
        assert json.loads(params[1]) == {'google_books': False, 'comicvine': True}
        assert 'super-secret-value' not in str(db.statements)

    def test_a_worker_that_does_not_analyse_files_says_nothing_so_it_cannot_overwrite_the_one_that_does(self):
        from providers.gate import asks_providers, report_keys
        db = FakeStateDB()
        assert report_keys(db, {'WORKER_JOB_TYPES': 'embed_text'}) is None
        assert report_keys(db, {'WORKER_JOB_TYPES': 'extract_text, embed_text'}) is None
        assert db.statements == []
        for env in ({}, {'WORKER_JOB_TYPES': ''}, {'WORKER_JOB_TYPES': 'ingest'}, {'WORKER_JOB_TYPES': 'embed_text, ingest'}):
            assert asks_providers(env) is True, env
        assert report_keys(db, {'WORKER_JOB_TYPES': 'ingest', 'GOOGLE_BOOKS_API_KEY': 'k'}) == {'google_books': True, 'comicvine': False}

    def test_failing_to_tell_never_stops_the_worker(self):
        from providers.gate import report_keys
        assert report_keys(FakeStateDB(boom=True), {}) == {'google_books': False, 'comicvine': False}

    def test_the_worker_tells_it_at_start(self):
        from tests.test_text_job import load_main
        main = load_main()
        told = []

        class Stop(Exception):
            pass

        def boom(*args, **kwargs):
            raise Stop()

        with patch.object(main, 'CodiceDatabase', lambda: 'db'), patch.object(main, 'report_keys', lambda db: told.append(db)), \
                patch.object(main, 'connect_redis', boom):
            with pytest.raises(Stop):
                main.listen_for_tasks()
        assert told == ['db']

    def test_scrub_takes_the_secret_out_of_a_text_and_leaves_the_rest(self):
        from providers.gate import scrub
        assert scrub('GET /v1?q=Duna&key=abc123 failed', 'abc123') == 'GET /v1?q=Duna&key=*** failed'
        assert scrub('nothing here', 'abc123', '', None, '  ') == 'nothing here'
        assert scrub(ValueError('url=k1&x=k2'), 'k1', 'k2') == 'url=***&x=***'

    @patch('providers.comicvine.requests.get')
    def test_comicvine_never_writes_any_part_of_its_key_to_the_log(self, mock_get, capsys):
        mock_get.return_value = MagicMock(status_code=200, json=MagicMock(return_value={"results": []}))
        provider = ComicVineProvider()
        provider.api_key = 'SECRETKEY-1234567890'
        provider.search('Batman')
        mock_get.side_effect = RuntimeError('Max retries url: /api/search?api_key=SECRETKEY-1234567890&query=Batman')
        provider.search('Batman')
        mock_get.side_effect = None
        mock_get.return_value = MagicMock(status_code=401, text='bad key SECRETKEY-1234567890')
        provider.search('Batman')
        out = capsys.readouterr().out
        assert 'SECRET' not in out and '7890' not in out and 'ComicVine API error' in out and 'non-200' in out

    @patch('providers.google_books.requests.get')
    def test_google_books_never_writes_its_key_to_the_log(self, mock_get, capsys):
        mock_get.side_effect = RuntimeError('Max retries url: /v1/volumes?q=Duna&key=SECRETKEY-1234567890')
        provider = GoogleBooksProvider()
        provider.api_key = 'SECRETKEY-1234567890'
        provider.search('Duna')
        out = capsys.readouterr().out
        assert 'SECRET' not in out and '7890' not in out and 'Google Books API error' in out and 'key=***' in out
