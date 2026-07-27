"""Google Books API metadata provider.

Inspired by Calibre-Web's GoogleBooks provider pattern.
Uses the Google Books v1 API to search for book metadata by title/ISBN.
"""
import os
import requests
from typing import Optional
from .base import BaseProvider, MetadataRecord


class GoogleBooksProvider(BaseProvider):
    def __init__(self):
        self.api_key = os.getenv('GOOGLE_BOOKS_API_KEY', '')
        self.base_url = 'https://www.googleapis.com/books/v1/volumes'

    @property
    def name(self) -> str:
        return 'Google Books'

    def search(self, query: str) -> Optional[MetadataRecord]:
        if not query:
            return None

        params = {'q': query, 'maxResults': 1}
        if self.api_key:
            params['key'] = self.api_key

        try:
            resp = requests.get(self.base_url, params=params, timeout=5)
            if resp.status_code != 200:
                return None

            data = resp.json()
            if 'items' not in data or not data['items']:
                return None

            volume = data['items'][0]['volumeInfo']
            record = MetadataRecord(source=self.name)

            record.title = volume.get('title')
            authors = volume.get('authors', [])
            record.author = authors[0] if authors else None
            record.publisher = volume.get('publisher')
            record.language = volume.get('language')
            record.description = volume.get('description')

            # ISBN
            for identifier in volume.get('industryIdentifiers', []):
                if identifier.get('type') in ('ISBN_13', 'ISBN_10'):
                    record.isbn = identifier.get('identifier')
                    break

            # Publication date
            record.publication_date = volume.get('publishedDate')

            # Categories as tags
            record.tags = [c.strip() for c in volume.get('categories', []) if c.strip()]

            # Cover image
            image_links = volume.get('imageLinks', {})
            record.cover_url = image_links.get('thumbnail') or image_links.get('smallThumbnail')
            if record.cover_url:
                record.cover_url = record.cover_url.replace('http://', 'https://')

            return record

        except Exception as e:
            print(f"   ⚠️ Google Books API error: {e}")
            return None