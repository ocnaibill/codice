"""MangaDex metadata provider, for manga (DEC-141).

Like AniList it knows the manga as a whole: names in several languages, who made it, the genres, the demographic (seinen, shounen...),
the year, the synopsis in several languages and the cover. Public REST API, no key; it asks for a User-Agent that says who asks and does not
let its images be linked to (the cover is downloaded to the library, as it is for every provider), and its use policy is for non-commercial
use. Only the title is sent (DEC-097)."""
from typing import List

from .anilist import clean_description
from .base import BaseProvider, Credit, MetadataRecord
from .http import get_json

BASE = 'https://api.mangadex.org'
COVERS = 'https://uploads.mangadex.org/covers'
# MangaDex allows about 5 requests a second to an address: a little over one a second is far from it.
INTERVAL = 0.5
CANDIDATES = 10
MAX_TAGS = 12
# The languages of the synopsis, in the order the library wants them.
DESCRIPTION_LANGUAGES = ('pt-br', 'pt', 'en')


def first_text(by_language, order=()):
    """A text that comes by language: the first language wanted that is there, else any."""
    by_language = by_language if isinstance(by_language, dict) else {}
    for language in order:
        if by_language.get(language):
            return by_language[language]
    return next((v for v in by_language.values() if isinstance(v, str) and v), None)


class MangaDexProvider(BaseProvider):
    @property
    def name(self) -> str:
        return 'MangaDex'

    @property
    def id(self) -> str:
        return 'mangadex'

    def _record(self, manga: dict) -> MetadataRecord:
        attrs = manga.get('attributes') or {}
        record = MetadataRecord(source=self.name)
        record.title = first_text(attrs.get('title'), ('en', 'ja-ro'))
        record.series = record.title
        roles = {}
        cover_file = None
        for rel in manga.get('relationships') or []:
            data = rel.get('attributes') or {}
            if rel.get('type') in ('author', 'artist') and data.get('name'):
                kind = 'author' if rel['type'] == 'author' else 'illustrator'
                roles.setdefault(data['name'].strip(), []).append(kind)
            elif rel.get('type') == 'cover_art' and data.get('fileName'):
                cover_file = data['fileName']
        record.credits = [Credit(name, role=', '.join(dict.fromkeys(kinds))) for name, kinds in roles.items()][:4]
        authors = [c for c in record.credits if 'author' in (c.role or '')]
        first = authors[0] if authors else (record.credits[0] if record.credits else None)
        record.author = first.name if first else None
        record.description = clean_description(first_text(attrs.get('description'), DESCRIPTION_LANGUAGES)) or None
        tags = []
        demographic = attrs.get('publicationDemographic')
        if demographic:
            tags.append(demographic.capitalize())
        for tag in attrs.get('tags') or []:
            name = ((tag.get('attributes') or {}).get('name') or {}).get('en')
            if name and ((tag.get('attributes') or {}).get('group') in ('genre', 'theme')) and name not in tags:
                tags.append(name)
        record.tags = tags[:MAX_TAGS]
        if manga.get('id') and cover_file:
            record.cover_url = f"{COVERS}/{manga['id']}/{cover_file}.512.jpg"
        record.publication_date = str(attrs['year']) if attrs.get('year') else None
        alts = []
        for entry in attrs.get('altTitles') or []:
            alts.extend(v for v in (entry or {}).values() if isinstance(v, str) and v)
        for other in (attrs.get('title') or {}).values():
            if isinstance(other, str) and other and other != record.title:
                alts.append(other)
        # The title in the script of the language the series was written in: what MangaDex keeps for its original language, when it has one.
        original = attrs.get('originalLanguage')
        native = None
        for entry in attrs.get('altTitles') or []:
            value = (entry or {}).get(original) if isinstance(entry, dict) and original else None
            if isinstance(value, str) and value.strip():
                native = value.strip()
                break
        record.raw = {'mangadex_id': manga.get('id'), 'alt_titles': alts[:12], 'status': attrs.get('status'), 'demographic': demographic,
                      'language': original, 'native_title': native}
        return record

    def lookup(self, query) -> List[MetadataRecord]:
        title = query.search_title
        if not title:
            return []
        reply = get_json(self.name, f'{BASE}/manga', params={'title': title, 'limit': CANDIDATES, 'order[relevance]': 'desc',
                                                            'includes[]': ['author', 'artist', 'cover_art']}, interval=INTERVAL)
        return [self._record(m) for m in (reply.data or {}).get('data', [])] if reply.ok else []

    def search(self, query: str):
        from .query import read_file_title
        found = self.lookup(read_file_title(query, None, 'cbz'))
        return found[0] if found else None
