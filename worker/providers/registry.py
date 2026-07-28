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

    @staticmethod
    def _sanitize_query(raw_title: str) -> str:
        """Clean up a title before sending to providers."""
        # Remove parenthesized groups: (2025) (Digital) (Group-Name)
        import re
        cleaned = re.sub(r'\s*\([^)]*\)\s*', ' ', raw_title)
        # Remove leading/trailing whitespace
        cleaned = cleaned.strip()
        # Remove issue numbers at the end: "Absolute Batman 006" → "Absolute Batman"
        cleaned = re.sub(r'\s+\d{2,4}\s*$', '', cleaned)
        # Collapse multiple spaces
        cleaned = re.sub(r'\s+', ' ', cleaned)
        return cleaned

    def search(self, query: str, format: str = 'default') -> Optional[MetadataRecord]:
        """Search for metadata using the best provider for the given format."""
        sanitized = self._sanitize_query(query)
        if sanitized != query:
            print(f"   🧹 Sanitized query: '{query}' → '{sanitized}'")
        query = sanitized

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

    def download_cover(self, cover_url: str, file_path: str, covers_dir: str) -> str:
        """Download cover image using the first available provider."""
        print(f"   📥 Downloading cover from: {cover_url[:80]}...")
        for provider_list in self._providers.values():
            for provider in provider_list:
                if hasattr(provider, 'download_cover'):
                    result = provider.download_cover(cover_url, file_path, covers_dir)
                    if result:
                        print(f"   ✅ Cover saved to: {result}")
                        return result
        print(f"   ⚠️ Cover download failed for: {cover_url[:80]}...")
        return ""
