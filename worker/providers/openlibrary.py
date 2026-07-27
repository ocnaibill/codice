"""OpenLibrary API metadata provider.

Fallback provider when Google Books doesn't return results.
Uses OpenLibrary's search API.
"""
import requests
from typing import Optional
from .base import BaseProvider, MetadataRecord


class OpenLibraryProvider(BaseProvider):
    def __init__(self):
        self.search_url = 'https://openlibrary.org/search.json'
        self.book_url = 'https://openlibrary.org/books/'

    @property
    def name(self) -> str:
        return 'OpenLibrary'

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
            authors = doc.get('author_name', [])
            record.author = authors[0] if authors else None
            record.publisher = doc.get('publisher', [None])[0] if doc.get('publisher') else None
            record.language = doc.get('language', [None])[0] if doc.get('language') else None
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