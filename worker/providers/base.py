from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Optional, List


@dataclass
class MetadataRecord:
    """Standard metadata record returned by all providers.

    Inspired by Calibre-Web's metadata provider pattern:
    each provider returns structured data that gets merged
    into the work's metadata with priority ordering.
    """
    title: Optional[str] = None
    author: Optional[str] = None
    series: Optional[str] = None
    series_index: Optional[float] = None
    isbn: Optional[str] = None
    language: Optional[str] = None
    publisher: Optional[str] = None
    publication_date: Optional[str] = None
    description: Optional[str] = None
    tags: List[str] = field(default_factory=list)
    cover_url: Optional[str] = None
    source: str = ""
    raw: dict = field(default_factory=dict)


class BaseProvider(ABC):
    """Abstract base class for all metadata providers.

    Each provider queries an external API (Google Books, OpenLibrary,
    ComicVine, etc.) and returns structured MetadataRecord.
    """

    @abstractmethod
    def search(self, query: str) -> Optional[MetadataRecord]:
        """Search for metadata using a query string (title, ISBN, etc.)."""
        pass

    @property
    @abstractmethod
    def name(self) -> str:
        """Human-readable provider name."""
        pass

    def download_cover(self, cover_url: str, file_path: str, covers_dir: str) -> Optional[str]:
        """Download cover from provider and save locally. Returns local path or None."""
        import os
        import hashlib
        import requests

        if not cover_url:
            return None

        try:
            resp = requests.get(cover_url, timeout=10)
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