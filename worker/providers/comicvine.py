"""ComicVine API metadata provider for comics (CBZ/CBR).

Inspired by Calibre-Web's ComicVine provider.
Requires COMICVINE_API_KEY environment variable.
"""
import os
import requests
from typing import Optional
from .base import BaseProvider, MetadataRecord


class ComicVineProvider(BaseProvider):
    def __init__(self):
        self.api_key = os.getenv('COMICVINE_API_KEY', '')
        self.base_url = 'https://comicvine.gamespot.com/api'

    @property
    def name(self) -> str:
        return 'ComicVine'

    def search(self, query: str) -> Optional[MetadataRecord]:
        if not self.api_key:
            print(f"   ⚠️ ComicVine: no API key set (COMICVINE_API_KEY env var is empty)")
            return None
        if not query:
            print(f"   ⚠️ ComicVine: empty query, skipping")
            return None

        print(f"   🔎 ComicVine: searching for '{query}'")
        print(f"   🔑 ComicVine: using API key {self.api_key[:8]}...{self.api_key[-4:]}")

        try:
            params = {
                'api_key': self.api_key,
                'format': 'json',
                'query': query,
                'resources': 'issue',
                'limit': 1,
            }
            headers = {'User-Agent': 'Codice/1.0'}
            resp = requests.get(f'{self.base_url}/search', params=params, headers=headers, timeout=10)
            print(f"   🌐 ComicVine: HTTP {resp.status_code}")
            if resp.status_code != 200:
                print(f"   ⚠️ ComicVine: non-200 response: {resp.text[:200]}")
                return None

            data = resp.json()
            results = data.get('results', [])
            if not results:
                return None

            issue = results[0]
            record = MetadataRecord(source=self.name)

            record.title = issue.get('name') or issue.get('issue_number', '')
            record.description = issue.get('description', '')

            # Volume/Series info
            volume = issue.get('volume', {})
            if volume:
                record.series = volume.get('name', '')
                record.series_index = float(issue.get('issue_number', 0) or 0)

            # Cover
            image = issue.get('image', {})
            record.cover_url = image.get('super_url') or image.get('original_url')

            # Date
            cover_date = issue.get('cover_date', '')
            if cover_date:
                record.publication_date = cover_date

            return record

        except Exception as e:
            print(f"   ⚠️ ComicVine API error: {e}")
            return None