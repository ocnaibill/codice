"""Tests for metadata providers.

Uses mocked HTTP responses to test each provider's parsing.
"""
import pytest
from unittest.mock import patch, MagicMock
from providers.base import MetadataRecord
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