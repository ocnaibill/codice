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
        """Clean up a title before sending to providers.
        
        Removes parenthesized groups like (2025) (Digital) (Group-Name)
        but keeps the series title and issue number.
        """
        import re
        # Remove parenthesized groups: (2025) (Digital) (Group-Name)
        cleaned = re.sub(r'\s*\([^)]*\)\s*', ' ', raw_title)
        # Remove leading/trailing whitespace
        cleaned = cleaned.strip()
        # Collapse multiple spaces
        cleaned = re.sub(r'\s+', ' ', cleaned)
        return cleaned

    def search(self, query: str, format: str = 'default') -> Optional[MetadataRecord]:
        """Search for metadata using the best provider for the given format.
        
        Returns the first match (original behavior for backward compatibility).
        For getting results from ALL providers, use search_all() instead.
        """
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

    def search_all(self, query: str, format: str = 'default') -> List[MetadataRecord]:
        """Search for metadata using ALL providers for the given format.
        
        Returns a list of MetadataRecord from every provider that returned
        a result, regardless of match quality. Useful for manual selection.
        """
        sanitized = self._sanitize_query(query)
        if sanitized != query:
            print(f"   🧹 Sanitized query: '{query}' → '{sanitized}'")
        query = sanitized

        providers = self._providers.get(format, self._providers['default'])
        results: List[MetadataRecord] = []

        for provider in providers:
            try:
                result = provider.search(query)
                if result is not None and (result.title or result.author):
                    print(f"   ✨ Found metadata from {result.source}")
                    results.append(result)
                else:
                    print(f"   ⚠️ {provider.name}: no results")
            except Exception as e:
                print(f"   ⚠️ {provider.name} failed: {e}")

        if not results:
            print("   ❌ No metadata found from any provider")

        return results

    @staticmethod
    def _rank_results(query: str, results: List[MetadataRecord]) -> List[MetadataRecord]:
        """Rank metadata results by quality and relevance.
        
        Returns a sorted list (best first).
        """
        import re
        query_lower = query.lower().strip()
        # Extract issue number from query: "Absolute Batman 006" → "006"
        issue_match = re.search(r'(\d{2,4})$', query_lower)
        query_issue = issue_match.group(1) if issue_match else None

        def score(r: MetadataRecord) -> int:
            s = 0
            title_lower = (r.title or '').lower().strip()
            series_lower = (r.series or '').lower().strip()

            # Exact title match
            if title_lower == query_lower:
                s += 100
            # Title contains query or vice versa
            if title_lower and (title_lower in query_lower or query_lower in title_lower):
                s += 50
            # Series match
            if series_lower and (series_lower in query_lower or query_lower in series_lower):
                s += 40

            # Issue number match
            if issue_match and r.series_index is not None:
                try:
                    if int(r.series_index) == int(query_issue):
                        s += 60
                    elif abs(int(r.series_index) - int(query_issue)) <= 2:
                        s += 20
                except ValueError:
                    pass

            # Completeness bonus
            if r.author:
                s += 15
            if r.cover_url:
                s += 10
            if r.description:
                s += 10
            if r.publisher:
                s += 5
            if r.tags:
                s += 5

            return s

        results_with_scores = [(r, score(r)) for r in results]
        results_with_scores.sort(key=lambda x: x[1], reverse=True)

        for r, s in results_with_scores:
            print(f"   📊 Rank: {r.source} (score={s}) - '{r.title}'")

        return [r for r, _ in results_with_scores]

    def search_best(self, query: str, format: str = 'default') -> Optional[MetadataRecord]:
        """Search ALL providers and return the best ranked result."""
        results = self.search_all(query, format)
        if not results:
            return None
        ranked = self._rank_results(query, results)
        best = ranked[0]
        print(f"   🏆 Best match: {best.source} (score={self._rank_results(query, results)[0] if results else 0})")
        return best

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
