"""PDF metadata extractor using PyMuPDF.

Extracts Document Info dictionary (title, author, subject, etc.)
and generates cover from first page.
"""
import fitz  # PyMuPDF
from .base import BaseExtractor, ExtractedMetadata


class PdfExtractor(BaseExtractor):
    def can_extract(self, file_path: str) -> bool:
        return file_path.lower().endswith('.pdf')

    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        meta = ExtractedMetadata(format='pdf')
        doc = fitz.open(file_path)
        try:
            pdf_meta = doc.metadata or {}
            meta.title = pdf_meta.get('title', '') or self._fallback_title(file_path)
            meta.author = pdf_meta.get('author', '') or 'Unknown Author'
            meta.publisher = pdf_meta.get('publisher', '') or ''
            meta.language = pdf_meta.get('language', '') or ''
            meta.description = pdf_meta.get('subject', '') or ''
            meta.page_count = len(doc)

            if pdf_meta.get('keywords'):
                meta.tags = [t.strip() for t in pdf_meta['keywords'].split(',') if t.strip()]

            # Generate cover from first page
            if len(doc) > 0:
                page = doc.load_page(0)
                pix = page.get_pixmap(matrix=fitz.Matrix(2, 2))
                img_data = pix.tobytes('jpeg')
                meta.cover_path = self._save_cover(img_data, file_path, covers_dir)

        finally:
            doc.close()

        return meta