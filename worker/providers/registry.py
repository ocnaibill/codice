"""Provider registry with format-based priority selection.

Inspired by Calibre-Web's provider selection pattern:
different formats get different provider priority ordering.
"""
from typing import Dict, List, Optional
from .base import BaseProvider, MetadataRecord
from .google_books import GoogleBooksProvider
from .openlibrary import OpenLibraryProvider
from .comicvine import ComicVineProvider
from .gate import nothing_allowed
from .match import judge
from .query import read_file_title


class ProviderRegistry:
    """Registry that selects providers by format with priority ordering."""

    def __init__(self, enabled=None):
        # Which providers may be asked (the owner's choice, gate.py). Without one, none is: asking a third party
        # is never the default.
        self._enabled = enabled or nothing_allowed
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

    def _allowed(self, providers):
        """The providers the owner turned on, in their order. The others are not asked."""
        allowed = [p for p in providers if self._enabled(p.id)]
        if not allowed:
            print("   🔒 No external metadata provider is turned on")
        return allowed

    @staticmethod
    def _answers(provider, query):
        """What a provider answers to the file: its several answers, or the one it gives (a provider that only knows how to `search`)."""
        if hasattr(provider, 'lookup'):
            return provider.lookup(query) or []
        record = provider.search(query.search_title)
        return [record] if record is not None else []

    def _judged(self, query, format):
        """Every answer of every provider that is on, judged against the file, with the provider that gave it. Only the title is sent
        (`query.search_title`); the author and the number of the file are used here, to tell which answer is the work."""
        out = []
        for provider in self._allowed(self._providers.get(format, self._providers['default'])):
            try:
                answers = self._answers(provider, query)
            except Exception as e:
                print(f"   ⚠️ {provider.name} failed: {e}")
                continue
            if not answers:
                print(f"   ⚠️ {provider.name}: no results")
            for record in answers:
                if record is None or not (record.title or record.author):
                    continue
                match = judge(query, record)
                record.match = match.as_dict()
                out.append((match, provider, record))
        return out

    def search(self, query: str, format: str = 'default') -> Optional[MetadataRecord]:
        """The first answer that is the work, asking the providers in their order and stopping at the first that has one."""
        q = read_file_title(query, None, format)
        for provider in self._allowed(self._providers.get(format, self._providers['default'])):
            try:
                for record in self._answers(provider, q):
                    if record is not None and (record.title or record.author):
                        record.match = judge(q, record).as_dict()
                        print(f"   ✨ Found metadata from {record.source}")
                        return record
            except Exception as e:
                print(f"   ⚠️ {provider.name} failed: {e}")
        print("   ❌ No metadata found from any provider")
        return None

    def search_all(self, query: str, format: str = 'default', per_provider: int = 5) -> List[MetadataRecord]:
        """The answers of every provider that is on, up to `per_provider` of each, the closest to the text first, close or not: it is for a
        person to choose among (the manual search), who knows better than the threshold."""
        q = read_file_title(query, None, format)
        judged = self._judged(q, format)
        by_provider = {}
        for match, provider, record in sorted(judged, key=lambda x: -x[0].score):
            kept = by_provider.setdefault(provider.id, [])
            if len(kept) < per_provider:
                kept.append((match, provider, record))
        results = [r for match, _, r in sorted((x for kept in by_provider.values() for x in kept), key=lambda x: -x[0].score)]
        if not results:
            print("   ❌ No metadata found from any provider")
        return results

    def search_best(self, query: str, format: str = 'default', author: Optional[str] = None) -> Optional[MetadataRecord]:
        """The answer that is the work the file is, or none. Every provider that is on is asked for the title, the answers are judged against
        what the file says (the title, and the author when the file's metadata has one), and only an answer that is close enough is a
        suggestion: nothing is better than the wrong work. The one chosen is completed (the description of the work, say) and carries
        how close it was in `match`."""
        q = read_file_title(query, author, format)
        judged = self._judged(q, format)
        accepted = [x for x in judged if x[0].accepted]
        if not accepted:
            if judged:
                match, _, record = max(judged, key=lambda x: x[0].score)
                print(f"   🚫 Nothing close enough: the nearest, '{record.title}' ({record.source}), is not the title ({match.reason}, title {match.title}, author {match.author})")
            else:
                print("   ❌ No metadata found from any provider")
            return None
        match, provider, best = max(accepted, key=lambda x: x[0].score)
        print(f"   🏆 Best match: {best.source} (score={match.score}, title={match.title}, author={match.author}) - '{best.title}'")
        try:
            best = provider.enrich(best) if hasattr(provider, 'enrich') else best
        except Exception as e:
            print(f"   ⚠️ {provider.name} could not complete the answer: {e}")
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
