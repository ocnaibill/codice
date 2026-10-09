"""Provider registry with format-based priority selection.

Inspired by Calibre-Web's provider selection pattern:
different formats get different provider priority ordering.
"""
from typing import Dict, List, Optional
from .base import BaseProvider, MetadataRecord
from .google_books import GoogleBooksProvider
from .openlibrary import OpenLibraryProvider
from .anilist import AniListProvider
from .comicvine import ComicVineProvider
from .mangadex import MangaDexProvider
from .wikidata import WikidataProvider
from .wikipedia import WikipediaProvider
from .gate import nothing_allowed
from .match import TITLE_MIN, judge
from .text import closeness
from .query import read_file_title

# The providers that answer about a series as a whole, and not about one book.
SERIES_PROVIDERS = ('anilist', 'mangadex')


class ProviderRegistry:
    """Registry that selects providers by format with priority ordering."""

    def __init__(self, enabled=None):
        # Which providers may be asked (the owner's choice, gate.py). Without one, none is: asking a third party
        # is never the default.
        self._enabled = enabled or nothing_allowed
        self._providers: Dict[str, List[BaseProvider]] = {
            # Wikidata translates a title the book providers do not know and says what the work is; Wikipedia completes with the text about it.
            'default': [
                GoogleBooksProvider(),
                OpenLibraryProvider(),
                WikidataProvider(),
                WikipediaProvider(),
            ],
            # A comic or a manga: the series first (ComicVine, AniList, MangaDex), then the books for the volume of a manga.
            'cbz': [
                ComicVineProvider(),
                AniListProvider(),
                MangaDexProvider(),
                GoogleBooksProvider(),
                OpenLibraryProvider(),
            ],
            'cbr': [
                ComicVineProvider(),
                AniListProvider(),
                MangaDexProvider(),
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

    def _judged(self, query, format, resolvers=False):
        """Every answer of every provider that is on, judged against the file, with the provider that gave it. Only the title is sent
        (`query.search_title`), and the ISBN to the providers that search by it (DEC-142); the author and the number of the file are used here,
        to tell which answer is the work. The providers that say what a work is (Wikidata) are not set against the book providers, unless
        `resolvers`, and the ones that only complete (Wikipedia) are never asked to look anything up: they are asked apart."""
        out = []
        for provider in self._allowed(self._providers.get(format, self._providers['default'])):
            if getattr(provider, 'completer', False) or (getattr(provider, 'resolver', False) and not resolvers):
                continue
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
                record.provider_id = getattr(provider, 'id', '')
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
        judged = self._judged(q, format, resolvers=True)
        by_provider = {}
        for match, provider, record in sorted(judged, key=lambda x: -x[0].score):
            kept = by_provider.setdefault(provider.id, [])
            if len(kept) < per_provider:
                kept.append((match, provider, record))
        results = [r for match, _, r in sorted((x for kept in by_provider.values() for x in kept), key=lambda x: -x[0].score)]
        if not results:
            print("   ❌ No metadata found from any provider")
        return results

    def search_best(self, query: str, format: str = 'default', author: Optional[str] = None, isbn: Optional[str] = None) -> Optional[MetadataRecord]:
        """The answer that is the work the file is, or none. Every provider that is on is asked for the title, the answers are judged against
        what the file says (the title, and the author when the file's metadata has one), and only an answer that is close enough is a
        suggestion: nothing is better than the wrong work. The one chosen is completed (the description of the work, say) and carries
        how close it was in `match`."""
        q = read_file_title(query, author, format, isbn)
        judged = self._judged(q, format)
        accepted = [x for x in judged if x[0].accepted]
        # A manga catalog that knows the title is the one to take: it has the genres, the authors and the synopsis of the series, and the volume
        # comes from the books. The comics database also answers for a manga, with an issue that scores higher only because it has a number.
        accepted = [x for x in accepted if x[1].id in SERIES_PROVIDERS] or accepted
        resolver = self._first(format, 'resolver')
        entity = None
        if accepted:
            match, provider, best = max(accepted, key=lambda x: x[0].score)
        else:
            match = provider = best = None
            if resolver is not None:
                # No book provider knows the work by this title: Wikidata may know it by another (the title it has in other languages).
                best, entity = self._translate(q, resolver, format)
                provider = self._provider_of(best, format)
        if best is None and entity is not None:
            best, provider = entity, resolver   # only Wikidata knows it: what it has is better than nothing
            print(f"   🌐 Only {resolver.name} knows it: '{best.title}'")
        if best is None:
            if judged:
                nearest, _, record = max(judged, key=lambda x: x[0].score)
                print(f"   🚫 Nothing close enough: the nearest, '{record.title}' ({record.source}), is not the title ({nearest.reason}, title {nearest.title}, author {nearest.author})")
            else:
                print("   ❌ No metadata found from any provider")
            return None
        if match is not None:
            print(f"   🏆 Best match: {best.source} (score={match.score}, title={match.title}, author={match.author}) - '{best.title}'")
        try:
            best = provider.enrich(best) if hasattr(provider, 'enrich') else best
        except Exception as e:
            print(f"   ⚠️ {provider.name} could not complete the answer: {e}")
        if best.provider_id in SERIES_PROVIDERS:
            self._volume(best, q, format)
        if resolver is not None:
            self._identify(best, q, resolver, entity)
        self._complete(best, format)
        return best

    def _first(self, format, role):
        """The first provider that is on and has the role (`resolver` or `completer`), or none."""
        return next((p for p in self._allowed(self._providers.get(format, self._providers['default'])) if getattr(p, role, False)), None)

    def _provider_of(self, record, format):
        if record is None:
            return None
        return next((p for p in self._providers.get(format, self._providers['default']) if getattr(p, 'id', None) == record.provider_id), None)

    def _entities(self, query, resolver):
        """The works the resolver says the file may be, the ones that are close to it first (judged, and only the accepted)."""
        try:
            records = resolver.lookup(query) or []
        except Exception as e:
            print(f"   ⚠️ {resolver.name} failed: {e}")
            return []
        out = []
        for record in records:
            match = judge(query, record)
            record.match = match.as_dict()
            record.provider_id = resolver.id
            if match.accepted:
                out.append((match, record))
        return [r for _, r in sorted(out, key=lambda x: -x[0].score)]

    def _translate(self, query, resolver, format):
        """When no book provider found the work by the title of the file, asks the resolver what the work is and tries the book providers with the
        other titles it has for it (the English one, the one in its own language). Returns the answer found, if any, and the work the resolver
        says it is. A title the file already has is not tried again; at most three others are."""
        entities = self._entities(query, resolver)
        tried = 0
        asked = {query.search_title}
        for entity in entities[:2]:
            for title in [entity.title] + list((entity.raw or {}).get('alt_titles') or [])[:4]:
                if not title or closeness(query.title, title) >= TITLE_MIN or tried == 3:
                    continue
                other = read_file_title(title, query.author, format)
                if other.search_title in asked:   # "Dune (1965)" is asked as "Dune": not twice
                    continue
                asked.add(other.search_title)
                tried += 1
                again = [x for x in self._judged(other, format) if x[0].accepted]
                if again:
                    match, _, best = max(again, key=lambda x: x[0].score)
                    best.raw = dict(best.raw or {}, translated_from=query.title)
                    print(f"   🌐 {resolver.name}: '{query.title}' is '{title}'")
                    return best, entity
        return None, (entities[0] if entities else None)

    @staticmethod
    def _merge(answer, entity):
        """What Wikidata adds to the answer of a book provider: the identifier of the work, the pages of Wikipedia, the genres (in Portuguese), the series
        when the answer has none. The source says both."""
        answer.raw = dict(answer.raw or {}, wikidata_id=entity.raw.get('wikidata_id'), sitelinks=entity.raw.get('sitelinks'))
        tags = list(answer.tags or [])
        known = {t.lower() for t in tags}
        answer.tags = tags + [t for t in entity.tags if t.lower() not in known]
        if not answer.series and entity.series:
            answer.series, answer.series_index = entity.series, entity.series_index
        answer.source = f'{answer.source} + Wikidata'
        return answer

    def _identify(self, best, query, resolver, entity=None):
        """Says what work the chosen answer is in Wikidata (after the provider completed it: its own completing would write over what is added).
        The work is the one already found when the title was translated, else the one Wikidata says the title of the file is."""
        if (best.raw or {}).get('wikidata_id'):
            return
        if entity is None:
            found = self._entities(query, resolver)
            entity = found[0] if found else None
        if entity is not None:
            self._merge(best, entity)

    def _complete(self, best, format):
        """Completes the answer with the text of the pages of the work, by the provider that does only that (Wikipedia)."""
        completer = self._first(format, 'completer')
        if completer is None or not (best.raw or {}).get('sitelinks'):
            return
        try:
            completer.complete(best)
        except Exception as e:
            print(f"   ⚠️ {completer.name} could not complete the answer: {e}")
        if (best.raw or {}).get('wikipedia'):
            best.source = f'{best.source} + {completer.name}'

    def _volume(self, series, query, format):
        """The answer of a series provider is about the series as a whole. The file is one volume of it: the volume number is put in, and when a
        book provider that is on has exactly that volume (not a deluxe edition, not a box), its ISBN, publisher, date and cover are added, the
        source then saying both."""
        if query.number is None:
            return
        series.series = series.series or series.title
        series.series_index = float(query.number)
        names = [series.title] + list((series.raw or {}).get('alt_titles') or [])
        for provider in self._allowed(self._providers.get(format, self._providers['default'])):
            if not hasattr(provider, 'volume'):
                continue
            try:
                volume = provider.volume([n for n in names if n], query.number)
            except Exception as e:
                print(f"   ⚠️ {provider.name} failed for the volume: {e}")
                continue
            if volume is None:
                continue
            series.isbn = volume.isbn or series.isbn
            series.publisher = volume.publisher or series.publisher
            series.publication_date = volume.publication_date or series.publication_date
            series.cover_url = volume.cover_url or series.cover_url
            series.source = f"{series.source} + {volume.source}"
            series.raw = dict(series.raw or {}, volume_work=(volume.raw or {}).get('openlibrary_work'), volume_title=volume.title)
            print(f"   📚 Volume {query.number} from {volume.source}: '{volume.title}'")
            return
        print(f"   ℹ️ No book that is exactly volume {query.number}: the suggestion is about the series")

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
