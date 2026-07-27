"""Tests for format-specific extractors.

Creates synthetic files to test each extractor's ability
to parse metadata from different formats.
"""
import os
import tempfile
import zipfile
import xml.etree.ElementTree as ET
import pytest
from extractors.base import ExtractedMetadata
from extractors.epub_extractor import EpubExtractor
from extractors.pdf_extractor import PdfExtractor
from extractors.cbz_extractor import CbzExtractor
from extractors.txt_extractor import TxtExtractor


def create_epub_with_opf(tmpdir: str, title: str, author: str) -> str:
    """Create a minimal EPUB file with OPF metadata."""
    epub_path = os.path.join(tmpdir, "test.epub")
    with zipfile.ZipFile(epub_path, 'w') as zf:
        # Mimetype
        zf.writestr("mimetype", "application/epub+zip")
        # Container
        container = (
            '<?xml version="1.0"?>'
            '<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">'
            '<rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>'
            '</rootfiles></container>'
        )
        zf.writestr("META-INF/container.xml", container)
        # OPF
        opf = f'''<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>{title}</dc:title>
    <dc:creator>{author}</dc:creator>
    <dc:publisher>TestPublisher</dc:publisher>
    <dc:language>en</dc:language>
    <dc:identifier>urn:isbn:978-3-16-148410-0</dc:identifier>
    <dc:description>A test book.</dc:description>
    <dc:subject>Fiction</dc:subject>
    <dc:subject>Fantasy</dc:subject>
  </metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="content"/>
  </spine>
</package>'''
        zf.writestr("content.opf", opf)
        zf.writestr("content.xhtml", "<html><body><p>Hello</p></body></html>")
    return epub_path


def create_cbz_with_comicinfo(tmpdir: str) -> str:
    """Create a minimal CBZ with ComicInfo.xml."""
    cbz_path = os.path.join(tmpdir, "test.cbz")
    with zipfile.ZipFile(cbz_path, 'w') as zf:
        comicinfo = '''<?xml version="1.0"?>
<ComicInfo>
  <Title>Test Comic</Title>
  <Series>Test Series</Series>
  <Number>3</Number>
  <Writer>John Doe</Writer>
  <Publisher>TestPub</Publisher>
  <Summary>A test comic.</Summary>
  <Genre>Action, Adventure</Genre>
  <LanguageISO>en</LanguageISO>
  <Pages>24</Pages>
</ComicInfo>'''
        zf.writestr("ComicInfo.xml", comicinfo)
        zf.writestr("page001.jpg", b"fake-image-data")
        zf.writestr("page002.jpg", b"fake-image-data")
    return cbz_path


def create_pdf_with_metadata(tmpdir: str) -> str:
    """Create a minimal PDF with custom metadata."""
    # Minimal valid PDF (reference: PDF spec example)
    pdf_content = '''%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>
endobj
xref
0 4
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000115 00000 n 
trailer
<< /Size 4 /Root 1 0 R /Info 4 0 R >>
startxref
267
%%EOF'''
    pdf_path = os.path.join(tmpdir, "test.pdf")
    with open(pdf_path, 'wb') as f:
        f.write(pdf_content.encode('latin-1'))
    return pdf_path


class TestEpubExtractor:
    def setup_method(self):
        self.extractor = EpubExtractor()

    def test_can_extract_epub(self):
        assert self.extractor.can_extract("/path/book.epub")
        assert not self.extractor.can_extract("/path/book.pdf")

    def test_extract_opf_metadata(self):
        with tempfile.TemporaryDirectory() as tmp:
            epub_path = create_epub_with_opf(tmp, "Test Book", "Jane Austen")
            covers_dir = os.path.join(tmp, "covers")
            os.makedirs(covers_dir, exist_ok=True)
            meta = self.extractor.extract(epub_path, covers_dir)
            assert meta.title == "Test Book"
            assert meta.author == "Jane Austen"
            assert meta.publisher == "TestPublisher"
            assert meta.language == "en"
            assert "Fiction" in meta.tags
            assert "Fantasy" in meta.tags


class TestCbzExtractor:
    def setup_method(self):
        self.extractor = CbzExtractor()

    def test_can_extract_cbz(self):
        assert self.extractor.can_extract("/path/comic.cbz")

    def test_extract_comicinfo(self):
        with tempfile.TemporaryDirectory() as tmp:
            cbz_path = create_cbz_with_comicinfo(tmp)
            covers_dir = os.path.join(tmp, "covers")
            os.makedirs(covers_dir, exist_ok=True)
            meta = self.extractor.extract(cbz_path, covers_dir)
            # ComicInfo.xml is parsed; title comes from it
            # _fallback_title only runs if parse didn't set a title
            # But the CBZ filename is 'test.cbz', _fallback_title gives 'test',
            # and the check `if not meta.title:` triggers because meta.title was
            # set by _parse_comicinfo to 'Test Comic'. So this should pass.
            assert meta.title == "Test Comic", f"got '{meta.title}'"
            assert meta.series == "Test Series"
            assert meta.series_index == 3.0
            assert meta.author == "John Doe"
            assert meta.publisher == "TestPub"
            assert "Action" in meta.tags

    def test_page_count(self):
        with tempfile.TemporaryDirectory() as tmp:
            cbz_path = create_cbz_with_comicinfo(tmp)
            covers_dir = os.path.join(tmp, "covers")
            os.makedirs(covers_dir, exist_ok=True)
            meta = self.extractor.extract(cbz_path, covers_dir)
            assert meta.page_count == 2  # Only 2 images


class TestTxtExtractor:
    def setup_method(self):
        self.extractor = TxtExtractor()

    def test_can_extract_txt(self):
        assert self.extractor.can_extract("/path/notes.txt")
        assert self.extractor.can_extract("/path/readme.md")

    def test_extract_filename_title(self):
        with tempfile.TemporaryDirectory() as tmp:
            txt_path = os.path.join(tmp, "my_book.txt")
            with open(txt_path, 'w') as f:
                f.write("Hello\n" * 100)
            covers_dir = os.path.join(tmp, "covers")
            os.makedirs(covers_dir, exist_ok=True)
            meta = self.extractor.extract(txt_path, covers_dir)
            # _fallback_title replaces _ with space
            assert meta.title == "my book"


class TestPdfExtractor:
    def setup_method(self):
        self.extractor = PdfExtractor()

    def test_can_extract_pdf(self):
        assert self.extractor.can_extract("/path/doc.pdf")

    def test_extract_invalid_pdf(self):
        with tempfile.TemporaryDirectory() as tmp:
            pdf_path = create_pdf_with_metadata(tmp)
            covers_dir = os.path.join(tmp, "covers")
            os.makedirs(covers_dir, exist_ok=True)
            # Should handle gracefully (fallback to filename)
            meta = self.extractor.extract(pdf_path, covers_dir)
            assert meta.title == "test"  # Fallback from filename
            assert meta.format == "pdf"