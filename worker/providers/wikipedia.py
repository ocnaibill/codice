"""Wikipedia, for the text about a work (DEC-141).

It does not look anything up by itself: a title in the search of Wikipedia finds pages of every kind. It completes an answer that Wikidata
identified, taking the summary of the page the work has in Portuguese (or, without it, in English). The text is under CC BY-SA 4.0 and says so,
with the link, where it is kept. Only the title of that page leaves the server (DEC-097), and it is the one Wikidata gave."""
from typing import List
from urllib.parse import quote

from .base import BaseProvider, MetadataRecord
from .http import get_json

LANGUAGES = ('pt', 'en')
MAX_EXTRACT = 1500


def attribution(language, url):
    return f'Fonte: Wikipédia ({language}), CC BY-SA 4.0 — {url}'


def cut(text, limit=MAX_EXTRACT):
    """The text, at most `limit` characters, ending at the end of a sentence when there is one to end at."""
    text = (text or '').strip()
    if len(text) <= limit:
        return text
    head = text[:limit]
    end = max(head.rfind('. '), head.rfind('.\n'))
    return head[:end + 1] if end > limit // 2 else head.rstrip() + '…'


class WikipediaProvider(BaseProvider):
    # It only completes the answers that have the pages of a work (providers.wikidata): it is not a place to look for the work.
    completer = True

    @property
    def name(self) -> str:
        return 'Wikipedia'

    @property
    def id(self) -> str:
        return 'wikipedia'

    def lookup(self, query) -> List[MetadataRecord]:
        return []

    def search(self, query: str):
        return None

    def complete(self, record: MetadataRecord) -> MetadataRecord:
        """The summary of the page of the work, as the description when the answer has none; the page is told in `raw['wikipedia']`."""
        pages = (record.raw or {}).get('sitelinks') or {}
        for language in LANGUAGES:
            title = pages.get(language)
            if not title:
                continue
            reply = get_json(self.name, f'https://{language}.wikipedia.org/api/rest_v1/page/summary/{quote(title, safe="")}')
            data = reply.data if reply.ok and isinstance(reply.data, dict) else {}
            extract = (data.get('extract') or '').strip()
            if data.get('type') != 'standard' or not extract:
                continue
            url = ((data.get('content_urls') or {}).get('desktop') or {}).get('page') or f'https://{language}.wikipedia.org/wiki/{quote(title.replace(" ", "_"))}'
            record.raw = dict(record.raw or {}, wikipedia={'language': language, 'title': title, 'url': url, 'license': 'CC BY-SA 4.0'})
            if not record.description:
                record.description = f'{cut(extract)}\n\n{attribution(language, url)}'
            return record
        return record
