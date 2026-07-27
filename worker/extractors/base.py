from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Optional, List


@dataclass
class ExtractedMetadata:
    """Standard metadata dataclass returned by all extractors."""
    title: str = ""
    author: str = ""
    series: str = ""
    series_index: float = 0.0
    isbn: str = ""
    language: str = ""
    publisher: str = ""
    publication_date: str = ""
    description: str = ""
    tags: List[str] = field(default_factory=list)
    page_count: int = 0
    cover_path: str = ""  # Relative path to extracted cover image
    format: str = ""       # pdf, epub, cbz, txt, etc.
    raw: dict = field(default_factory=dict)  # Original metadata dict for debugging


class BaseExtractor(ABC):
    """Abstract base class for all file format extractors.

    Inspired by Calibre-Web's multi-format extraction approach:
    each format has a dedicated extractor that knows how to parse
    the file's internal metadata structure.
    """

    @abstractmethod
    def can_extract(self, file_path: str) -> bool:
        """Return True if this extractor can handle the given file."""
        pass

    @abstractmethod
    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        """Extract metadata from the file at file_path.

        Args:
            file_path: Absolute path to the file.
            covers_dir: Directory to save extracted cover images.

        Returns:
            ExtractedMetadata with all found fields filled.
        """
        pass

    def _fallback_title(self, file_path: str) -> str:
        """Derive a clean title from filename when no metadata is available."""
        import os
        filename = os.path.basename(file_path)
        name, _ = os.path.splitext(filename)
        # Remove common prefixes like timestamps
        import re
        name = re.sub(r'^\d+_', '', name)
        name = name.replace('_', ' ').replace('-', ' ')
        return name.strip()

    def _save_cover(self, image_data: bytes, file_path: str, covers_dir: str) -> str:
        """Save cover image bytes to covers directory and return relative path."""
        import os
        import hashlib
        base = os.path.basename(file_path)
        hash_digest = hashlib.md5(base.encode()).hexdigest()[:12]
        cover_filename = f"cover_{hash_digest}.jpg"
        cover_path = os.path.join(covers_dir, cover_filename)
        with open(cover_path, 'wb') as f:
            f.write(image_data)
        return f"/covers/{cover_filename}"