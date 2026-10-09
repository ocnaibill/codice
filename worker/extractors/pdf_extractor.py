"""PDF metadata extractor using PyMuPDF.

Extracts Document Info dictionary (title, author, subject, etc.)
and generates cover from first page.
"""
import fitz  # PyMuPDF
from .base import BaseExtractor, ExtractedMetadata

# The cover is the first page at twice its size, but never more than this many pixels on the longer side: an A4 page
# (842 pt, 1684 px) is as it was, and a poster or a page of the largest size the format has (14,400 pt) does not ask the
# renderer for a picture it refuses (MuPDF: "Overly large image").
COVER_MAX_SIDE = 2000


class PdfExtractor(BaseExtractor):
    def can_extract(self, file_path: str) -> bool:
        return file_path.lower().endswith('.pdf')

    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        meta = ExtractedMetadata(format='pdf')
        doc = fitz.open(file_path)
        try:
            if doc.needs_pass:
                # A password to open it: the metadata and the first page are closed to us. The file is kept, its title is its
                # name, and the reader asks the person for the password; the work is not an error.
                meta.title = self._fallback_title(file_path)
                meta.author = 'Unknown Author'
                meta.protected = True
                return meta
            pdf_meta = doc.metadata or {}
            meta.title = pdf_meta.get('title', '') or self._fallback_title(file_path)
            meta.author = pdf_meta.get('author', '') or 'Unknown Author'
            meta.publisher = pdf_meta.get('publisher', '') or ''
            meta.language = pdf_meta.get('language', '') or ''
            meta.description = pdf_meta.get('subject', '') or ''
            meta.page_count = len(doc)

            if pdf_meta.get('keywords'):
                meta.tags = [t.strip() for t in pdf_meta['keywords'].split(',') if t.strip()]

            # Generate cover from first page. It is not what makes a file readable: a page that cannot be drawn
            # leaves the work without a cover, and the rest of what was read stays.
            if len(doc) > 0:
                try:
                    page = doc.load_page(0)
                    side = max(page.rect.width, page.rect.height) or 1
                    scale = min(2.0, COVER_MAX_SIDE / side)
                    pix = page.get_pixmap(matrix=fitz.Matrix(scale, scale))
                    img_data = pix.tobytes('jpeg')
                    meta.cover_path = self._save_cover(img_data, file_path, covers_dir)
                except (RuntimeError, ValueError, OSError) as error:  # fitz raises RuntimeError and its subclasses
                    print(f"   ⚠️  No cover for {file_path}: {error}")

        finally:
            doc.close()

        return meta