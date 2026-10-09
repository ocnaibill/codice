"""Google Books API metadata provider.

Google Books answers without a key only from a daily quota that every anonymous client in the world shares, and answers 429 when it is gone;
with a key of the owner's it has a quota of its own. So the provider does not ask without one (the owner is told in the administration, and
turning it on needs it). The search is by the text of the title, and the right book is rarely the first answer: it asks for the most Google
gives (20) and lets the registry judge them (providers.match), against the author the file says, which does not leave the server (DEC-097).
What the title says in parentheses ("A nuvem (Scythe)") is also a title of the book and finds it where the title alone does not.
"""
import os
from typing import List, Optional

from .base import BaseProvider, Credit, MetadataRecord
from .http import get_json
from .query import read_file_title

BASE = 'https://www.googleapis.com/books/v1/volumes'
CANDIDATES = 20   # what Google gives for one search, at most
# What is asked for: every field the record is made of, and no more, so that the answer is small.
FIELDS = ('items(id,volumeInfo(title,subtitle,authors,publisher,publishedDate,description,industryIdentifiers,categories,imageLinks,'
          'ratingsCount))')


def _isbn(identifiers):
    """An ISBN-13 when the volume has one, else an ISBN-10."""
    isbns = {i.get('type'): i.get('identifier') for i in identifiers or [] if isinstance(i, dict)}
    return isbns.get('ISBN_13') or isbns.get('ISBN_10')


class GoogleBooksProvider(BaseProvider):
    def __init__(self):
        self.api_key = os.getenv('GOOGLE_BOOKS_API_KEY', '')
        self.base_url = BASE

    @property
    def name(self) -> str:
        return 'Google Books'

    @property
    def id(self) -> str:
        return 'google_books'

    def _record(self, item: dict) -> Optional[MetadataRecord]:
        volume = item.get('volumeInfo') or {}
        if not volume.get('title'):
            return None
        record = MetadataRecord(source=self.name)
        record.title = volume['title']
        record.credits = [Credit(a.strip()) for a in (volume.get('authors') or []) if isinstance(a, str) and a.strip()]
        record.author = record.credits[0].name if record.credits else None
        record.publisher = volume.get('publisher')
        # No language: it is the language of the volume the title matched, which may be another edition's.
        # The language of a file is read from the file (DEC-096).
        record.description = volume.get('description')
        record.isbn = _isbn(volume.get('industryIdentifiers'))
        record.publication_date = volume.get('publishedDate')
        record.tags = [c.strip() for c in volume.get('categories') or [] if isinstance(c, str) and c.strip()]
        links = volume.get('imageLinks') or {}
        cover = links.get('thumbnail') or links.get('smallThumbnail')
        record.cover_url = cover.replace('http://', 'https://') if cover else None
        # Many volumes are equally close to a title (the editions of one book): the one that says more about itself is the one to take.
        record.prior = (min(volume.get('ratingsCount') or 0, 20) / 4 + (1 if record.description else 0) + (1 if record.isbn else 0)
                        + (0.5 if record.cover_url else 0))
        record.raw = {'google_id': item.get('id'), 'subtitle': volume.get('subtitle')}
        return record

    def _ask(self, text) -> List[dict]:
        params = {'q': text, 'maxResults': CANDIDATES, 'printType': 'books', 'fields': FIELDS, 'key': self.api_key}
        reply = get_json(self.name, self.base_url, params=params, secrets=(self.api_key,))
        data = reply.data if isinstance(reply.data, dict) else {}
        return data.get('items') or []

    def lookup(self, query) -> List[MetadataRecord]:
        """The volumes the title finds, by the title and, when the file's title says more in parentheses, by that too (each volume once)."""
        if not self.api_key:
            print("   ⚠️ Google Books: no API key (GOOGLE_BOOKS_API_KEY is empty), so it is not asked: without one it shares a quota with everybody")
            return []
        texts = [t for t in (query.search_title, query.search_extra) if t]
        items, seen = [], set()
        for text in texts:
            for item in self._ask(text):
                if item.get('id') not in seen:
                    seen.add(item.get('id'))
                    items.append(item)
        return [r for r in (self._record(i) for i in items) if r is not None]

    def search(self, query: str) -> Optional[MetadataRecord]:
        """The first volume for a text (the manual search of the registry)."""
        found = self.lookup(read_file_title(query, None, 'default'))
        return found[0] if found else None
