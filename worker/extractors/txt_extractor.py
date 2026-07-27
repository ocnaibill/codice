"""TXT metadata extractor — filename-based.

For plain text files, metadata is derived from the filename
since there's no embedded metadata structure.
"""
from .base import BaseExtractor, ExtractedMetadata


class TxtExtractor(BaseExtractor):
    def can_extract(self, file_path: str) -> bool:
        return file_path.lower().endswith('.txt') or file_path.lower().endswith('.md')

    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        meta = ExtractedMetadata(
            format='txt' if file_path.lower().endswith('.txt') else 'md',
            title=self._fallback_title(file_path),
            author='Unknown Author',
            page_count=1,
        )

        # Count lines as rough page count
        try:
            with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                line_count = sum(1 for _ in f)
            meta.page_count = max(1, line_count // 40)  # ~40 lines per page
        except Exception:
            pass

        return meta