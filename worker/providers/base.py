from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Optional, List


@dataclass
class Credit:
    """One person a provider credits on a record: who, in which role, and the identifiers the provider
    knows for them ({'openlibrary': 'OL113611A'}). Both the role and the identifiers are only what the
    provider says; a provider that does not say leaves them empty, and nothing is made up."""
    name: str
    role: Optional[str] = None
    ids: dict = field(default_factory=dict)

    def as_dict(self) -> dict:
        out = {'name': self.name}
        if self.role:
            out['role'] = self.role
        if self.ids:
            out['ids'] = dict(self.ids)
        return out


@dataclass
class MetadataRecord:
    """Standard metadata record returned by all providers.

    Inspired by Calibre-Web's metadata provider pattern:
    each provider returns structured data that gets merged
    into the work's metadata with priority ordering.
    """
    title: Optional[str] = None
    author: Optional[str] = None
    credits: List[Credit] = field(default_factory=list)  # every person credited; `author` is the first
    series: Optional[str] = None
    series_index: Optional[float] = None
    isbn: Optional[str] = None
    language: Optional[str] = None
    publisher: Optional[str] = None
    publication_date: Optional[str] = None   # of the edition in hand
    original_year: Optional[str] = None      # the year the work was first published (DEC-156), as a whole number in text
    description: Optional[str] = None
    tags: List[str] = field(default_factory=list)
    cover_url: Optional[str] = None
    source: str = ""
    raw: dict = field(default_factory=dict)
    provider_id: str = ''   # the id of the provider that answered (what the owner chooses by), set by the registry
    prior: float = 0.0   # how well known the work is, from 0 to 10, for breaking a tie between answers that are equally close
    match: dict = field(default_factory=dict)   # how close the answer is to the file (providers.match), set when it is judged


class BaseProvider(ABC):
    """Abstract base class for all metadata providers.

    Each provider queries an external API (Google Books, OpenLibrary,
    ComicVine, etc.) and returns structured MetadataRecord.
    """

    @abstractmethod
    def search(self, query: str) -> Optional[MetadataRecord]:
        """Search for metadata using a query string (title, ISBN, etc.)."""
        pass

    def lookup(self, query) -> List[MetadataRecord]:
        """The answers to a file (a providers.query.FileQuery), several if the provider has them. Only the title leaves the server (DEC-097), and the ISBN
        of the file for a provider that finds a book by it (DEC-142): `query.search_title` is what to ask; the rest
        of the query is for judging the answers. A provider that only knows how to `search` gives its one answer."""
        record = self.search(query.search_title)
        return [record] if record is not None else []

    def enrich(self, record: MetadataRecord) -> MetadataRecord:
        """Completes the answer that was chosen with what costs another request (the description of a work, for one): only the one chosen is
        asked about, not every candidate. A provider with nothing to add returns it as it is."""
        return record

    @property
    @abstractmethod
    def name(self) -> str:
        """Human-readable provider name."""
        pass

    @property
    @abstractmethod
    def id(self) -> str:
        """The name the owner's choice is kept under (see gate.py): it is also what the administration lists."""
        pass

    def download_cover(self, cover_url: str, file_path: str, covers_dir: str) -> Optional[str]:
        """Download cover from provider and save locally. Returns local path or None."""
        import os
        import hashlib
        import requests

        if not cover_url:
            return None

        try:
            from .http import user_agent   # MangaDex and Wikimedia refuse a request that does not say who asks
            resp = requests.get(cover_url, headers={'User-Agent': user_agent()}, timeout=10)
            if resp.status_code != 200:
                return None

            base = os.path.basename(file_path)
            hash_digest = hashlib.md5(base.encode()).hexdigest()[:12]
            safe_name = hashlib.md5(cover_url.encode()).hexdigest()[:8]
            cover_filename = f"provider_{hash_digest}_{safe_name}.jpg"
            cover_path = os.path.join(covers_dir, cover_filename)

            with open(cover_path, 'wb') as f:
                f.write(resp.content)

            return f"/covers/{cover_filename}"
        except Exception as e:
            print(f"   ⚠️ Cover download failed: {e}")
            return None