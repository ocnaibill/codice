"""OpenLibrary API metadata provider.

Fallback provider when Google Books doesn't return results.
Uses OpenLibrary's search API.
"""
import requests
from typing import Optional
from .base import BaseProvider, Credit, MetadataRecord


class OpenLibraryProvider(BaseProvider):
    def __init__(self):
        self.search_url = 'https://openlibrary.org/search.json'
        self.book_url = 'https://openlibrary.org/books/'

    @property
    def name(self) -> str:
        return 'OpenLibrary'

    @staticmethod
    def _credits(doc: dict) -> list:
        """Every author of the work. `author_key` names each one, in the same order as `author_name`;
        when the two lists do not line up, which key belongs to whom is not known, so none is used."""
        names = [n.strip() for n in (doc.get('author_name') or []) if isinstance(n, str) and n.strip()]
        keys = doc.get('author_key') or []
        paired = len(keys) == len(doc.get('author_name') or []) and len(names) == len(keys)
        return [Credit(name, ids={'openlibrary': str(keys[i])} if paired and keys[i] else {})
                for i, name in enumerate(names)]

    def search(self, query: str) -> Optional[MetadataRecord]:
        if not query:
            return None

        try:
            resp = requests.get(self.search_url, params={'q': query, 'limit': 1}, timeout=5)
            if resp.status_code != 200:
                return None

            data = resp.json()
            docs = data.get('docs', [])
            if not docs:
                return None

            doc = docs[0]
            record = MetadataRecord(source=self.name)

            record.title = doc.get('title')
            record.credits = self._credits(doc)
            record.author = record.credits[0].name if record.credits else None
            record.publisher = doc.get('publisher', [None])[0] if doc.get('publisher') else None
            # No language: `language` lists the languages of the editions of the work, in codes of three letters, and
            # the first is not the file's. The language of a file is read from the file (DEC-096).
            record.publication_date = doc.get('first_publish_year')

            # ISBN
            isbns = doc.get('isbn', [])
            record.isbn = isbns[0] if isbns else None

            # Cover
            cover_i = doc.get('cover_i')
            if cover_i:
                record.cover_url = f'https://covers.openlibrary.org/b/id/{cover_i}-L.jpg'

            # Subjects as tags
            subjects = doc.get('subject', [])
            record.tags = [s.strip() for s in subjects[:5] if s.strip()]

            return record

        except Exception as e:
            print(f"   ⚠️ OpenLibrary API error: {e}")
            return None