"""Provider registry with format-based priority selection.

Inspired by Calibre-Web's provider selection pattern:
different formats get different provider priority ordering.
"""
from typing import Dict, List, Optional
from .base import BaseProvider, MetadataRecord
from .google_books import GoogleBooksProvider
from .openlibrary import OpenLibraryProvider
from .comicvine import ComicVineProvider


class ProviderRegistry:
    """Registry that selects providers by format with priority ordering."""

    def __init__(self):
        self._providers: Dict[str, List[BaseProvider]] = {
            'default': [
                GoogleBooksProvider(),
                OpenLibraryProvider(),
            ],
            'cbz': [
                ComicVineProvider(),
                GoogleBooksProvider(),
                OpenLibraryProvider(),
            ],
            'cbr': [
                ComicVineProvider(),
                GoogleBooksProvider(),
                OpenLibraryProvider(),
            ],
        }

    def search(self, query: str, format: str = 'default') -> Optional[MetadataRecord]:
        """Search for metadata using the best provider for the given format."""
        providers = self._providers.get(format, self._providers['default'])

        for provider in providers:
            try:
                result = provider.search(query)
                if result is not None and (result.title or result.author):
                    print(f"   ✨ Found metadata from {result.source}")
                    return result
            except Exception as e:
                print(f"   ⚠️ {provider.name} failed: {e}")

        print("   ❌ No metadata found from any provider")
        return None