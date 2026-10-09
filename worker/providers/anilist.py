"""AniList metadata provider, for manga (DEC-141).

AniList knows a manga as a whole (the series): its names, who made it, genres, demographic tags, year, synopsis and cover. It does not know
the volumes one by one, which is what the book providers are for. Public GraphQL API, no key; free for non-commercial use, and it may not be
used as a store of data or to collect in bulk, so only what an administrator accepts is kept. Only the title is sent (DEC-097)."""
import re
from typing import List

from .base import BaseProvider, Credit, MetadataRecord
from .http import get_json

URL = 'https://graphql.anilist.co'
QUERY = '''query($q: String) { Page(perPage: 8) { media(search: $q, type: MANGA, sort: SEARCH_MATCH) {
  id format status countryOfOrigin volumes chapters popularity isAdult
  startDate { year month day }
  title { romaji english native } synonyms description(asHtml: false) genres
  tags { name rank isMediaSpoiler isAdult }
  coverImage { extraLarge large }
  staff(perPage: 8, sort: RELEVANCE) { edges { role node { name { full } } } }
} } }'''
# AniList allows 90 requests a minute (30 when it is degraded): one every second leaves room.
INTERVAL = 1.0
MAX_TAGS = 12
MIN_TAG_RANK = 60


def clean_description(text):
    """The synopsis without the markup AniList leaves in it (line breaks and a few tags)."""
    text = re.sub(r'<br\s*/?>', '\n', text or '', flags=re.I)
    text = re.sub(r'<[^>]+>', '', text)
    return re.sub(r'\n{3,}', '\n\n', text).strip()


def staff_role(role):
    """What a person of the staff is to the library: the one who wrote it ("Story", "Original Creator"), the one who drew it ("Art",
    "Illustration"), both ("Story & Art"), or nothing (an assistant, an editor of the magazine...), which is left out."""
    words = (role or '').lower()
    roles = []
    if 'story' in words or 'original' in words:
        roles.append('author')
    if 'art' in words or 'illustration' in words:
        roles.append('illustrator')
    return ', '.join(roles)


def pick_tags(genres, tags):
    """The genres and the tags that matter (a good rank, not a spoiler, not adult), each once."""
    out, seen = [], set()
    names = list(genres or []) + [t['name'] for t in tags or [] if t.get('rank', 0) >= MIN_TAG_RANK and not t.get('isMediaSpoiler') and not t.get('isAdult')]
    for name in names:
        if isinstance(name, str) and name.strip() and name.strip().lower() not in seen:
            seen.add(name.strip().lower())
            out.append(name.strip())
        if len(out) == MAX_TAGS:
            break
    return out


class AniListProvider(BaseProvider):
    @property
    def name(self) -> str:
        return 'AniList'

    @property
    def id(self) -> str:
        return 'anilist'

    def _record(self, media: dict) -> MetadataRecord:
        titles = media.get('title') or {}
        record = MetadataRecord(source=self.name)
        record.title = titles.get('romaji') or titles.get('english') or titles.get('native')
        record.series = record.title
        record.credits = []
        for edge in (media.get('staff') or {}).get('edges') or []:
            role = staff_role(edge.get('role'))
            name = ((edge.get('node') or {}).get('name') or {}).get('full')
            if role and name and len(record.credits) < 4:
                record.credits.append(Credit(name.strip(), role=role))
        first = next((c for c in record.credits if 'author' in (c.role or '')), record.credits[0] if record.credits else None)
        record.author = first.name if first else None
        record.description = clean_description(media.get('description')) or None
        record.tags = pick_tags(media.get('genres'), media.get('tags'))
        cover = media.get('coverImage') or {}
        record.cover_url = cover.get('extraLarge') or cover.get('large')
        start = media.get('startDate') or {}
        if start.get('year'):
            record.publication_date = (f"{start['year']}-{start['month']:02d}-{start['day']:02d}"
                                       if start.get('month') and start.get('day') else str(start['year']))
        record.prior = min((media.get('popularity') or 0) / 10000, 10)
        alts = [t for t in (titles.get('english'), titles.get('native'), *(media.get('synonyms') or [])[:6]) if isinstance(t, str) and t]
        record.raw = {'anilist_id': media.get('id'), 'alt_titles': alts, 'format': media.get('format'), 'volumes': media.get('volumes'),
                      'chapters': media.get('chapters'), 'status': media.get('status'), 'country': media.get('countryOfOrigin')}
        return record

    def lookup(self, query) -> List[MetadataRecord]:
        title = query.search_title
        if not title:
            return []
        reply = get_json(self.name, URL, post={'query': QUERY, 'variables': {'q': title}}, interval=INTERVAL)
        page = (((reply.data or {}).get('data') or {}).get('Page') or {}) if reply.ok else {}
        return [self._record(m) for m in page.get('media') or [] if not m.get('isAdult')]

    def search(self, query: str):
        from .query import read_file_title
        found = self.lookup(read_file_title(query, None, 'cbz'))
        return found[0] if found else None
