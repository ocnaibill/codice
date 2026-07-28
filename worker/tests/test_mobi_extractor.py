"""Tests for MobiExtractor — EXTH binary header parser.

Creates synthetic MOBI-like files and edge cases to test
the PalmDB + MOBI + EXTH parsing logic.
"""
import os
import struct
import tempfile
import pytest
from extractors.base import ExtractedMetadata
from extractors.mobi_extractor import MobiExtractor


def create_minimal_mobi_with_exth(tmpdir: str, title: str = "Test Book", author: str = "Jane Austen",
                                   publisher: str = "TestPub", isbn: str = "978-3-16-148410-0",
                                   description: str = "A test book.", language: str = "en",
                                   tags: list = None) -> str:
    """Create a synthetic MOBI file with a minimal valid PalmDB + MOBI + EXTH header."""
    if tags is None:
        tags = ["Fiction", "Fantasy"]

    filepath = os.path.join(tmpdir, "test.mobi")

    # We'll build the binary content in sections.
    # Layout:
    #   [0:78)      PalmDB header
    #   [78:78+N*8) Record offsets (N records)
    #   Record 0: PalmDOC header (16 bytes) + MOBI header + EXTH header + title string

    # First, compute sizes so we can lay out offsets correctly.
    # MOBI header: 132 bytes minimum (we'll write 132)
    mobi_header_len = 132

    # Build EXTH header records
    exth_records = b""
    # Title (type 503)
    title_bytes = title.encode('utf-8')
    exth_records += struct.pack('>II', 503, 8 + len(title_bytes)) + title_bytes
    # Author (type 100)
    author_bytes = author.encode('utf-8')
    exth_records += struct.pack('>II', 100, 8 + len(author_bytes)) + author_bytes
    # Publisher (type 101)
    pub_bytes = publisher.encode('utf-8')
    exth_records += struct.pack('>II', 101, 8 + len(pub_bytes)) + pub_bytes
    # ISBN (type 104)
    isbn_bytes = isbn.encode('utf-8')
    exth_records += struct.pack('>II', 104, 8 + len(isbn_bytes)) + isbn_bytes
    # Description (type 103)
    desc_bytes = description.encode('utf-8')
    exth_records += struct.pack('>II', 103, 8 + len(desc_bytes)) + desc_bytes
    # Language (type 524)
    lang_bytes = language.encode('utf-8')
    exth_records += struct.pack('>II', 524, 8 + len(lang_bytes)) + lang_bytes
    # Tags (type 105) — one per tag
    for tag in tags:
        tag_bytes = tag.encode('utf-8')
        exth_records += struct.pack('>II', 105, 8 + len(tag_bytes)) + tag_bytes

    # EXTH header: magic(4) + header_length(4) + record_count(4) + records
    exth_header_len = 12 + len(exth_records)
    exth_header = b'EXTH' + struct.pack('>II', exth_header_len, len(tags) + 6)

    # Full first record content: PalmDOC(16) + MOBI header + EXTH header + title_text
    # Title text at offset relative to first record start
    palmdoc_len = 16
    mobi_content_offset = palmdoc_len  # start of MOBI header within record
    title_text_offset = mobi_content_offset + mobi_header_len + exth_header_len + len(exth_records)
    title_text = title.encode('utf-8')

    # MOBI header fields we need:
    mobi_header = b'MOBI'                           # magic
    mobi_header += struct.pack('>I', mobi_header_len)  # header_length
    mobi_header += struct.pack('>I', 65001)            # encoding (utf-8)
    mobi_header += b'\x00' * 72                        # padding (up to offset 84)
    # full_title_offset at mobi_offset + 84 (relative to first record start)
    mobi_header += struct.pack('>I', title_text_offset)
    # full_title_length at mobi_offset + 88
    mobi_header += struct.pack('>I', len(title_text))
    mobi_header += b'\x00' * 16                        # padding up to offset 104
    # This is actually at mobi_offset+104 for first_image_index
    mobi_header += struct.pack('>I', 0xFFFFFFFF)       # no image
    mobi_header += b'\x00' * 16                        # padding up to offset 120
    # EXTH flags at mobi_offset + 128
    mobi_header += struct.pack('>I', 0x40)             # has_exth = True
    mobi_header = mobi_header[:mobi_header_len].ljust(mobi_header_len, b'\x00')

    first_record_content = (
        b'\x00' * palmdoc_len +
        mobi_header +
        exth_header +
        exth_records +
        title_text
    )

    # Record 0 offset: right after PalmDB header
    first_record_offset = 78 + (1 * 8)  # 1 record * 8 bytes per record entry
    record_count = 1

    # PalmDB header (78 bytes)
    palm_header = b'\x00' * 78
    # Number of records at bytes 76-77
    palm_header = palm_header[:76] + struct.pack('>H', record_count)

    # Record offset list
    record_offsets = struct.pack('>I', first_record_offset)
    # Unique ID (4 bytes, unique for each record)
    record_offsets += struct.pack('>I', 0)

    # Assemble everything
    content = palm_header + record_offsets + first_record_content

    with open(filepath, 'wb') as f:
        f.write(content)

    return filepath


class TestMobiExtractor:
    def setup_method(self):
        self.extractor = MobiExtractor()

    def test_can_extract_mobi(self):
        assert self.extractor.can_extract("/path/book.mobi")
        assert self.extractor.can_extract("/path/book.azw")
        assert self.extractor.can_extract("/path/book.azw3")
        assert self.extractor.can_extract("/path/book.prc")

    def test_cannot_extract_other_formats(self):
        assert not self.extractor.can_extract("/path/book.pdf")
        assert not self.extractor.can_extract("/path/book.epub")
        assert not self.extractor.can_extract("/path/book.cbz")
        assert not self.extractor.can_extract("/path/book.txt")

    def test_extract_invalid_file_fallback(self):
        """Invalid MOBI file should fall back to filename title and Unknown Author."""
        with tempfile.TemporaryDirectory() as tmp:
            filepath = os.path.join(tmp, "my_book.mobi")
            with open(filepath, 'wb') as f:
                f.write(b"not a valid mobi file")
            covers_dir = os.path.join(tmp, "covers")
            os.makedirs(covers_dir, exist_ok=True)
            meta = self.extractor.extract(filepath, covers_dir)
            # fallback title from filename (underscores → spaces)
            assert meta.title == "my book", f"got '{meta.title}'"
            assert meta.author == "Unknown Author"
            assert meta.format == "mobi"

    def test_extract_invalid_empty_file(self):
        """Empty file should fall back gracefully."""
        with tempfile.TemporaryDirectory() as tmp:
            filepath = os.path.join(tmp, "empty.mobi")
            with open(filepath, 'wb') as f:
                f.write(b"")
            covers_dir = os.path.join(tmp, "covers")
            os.makedirs(covers_dir, exist_ok=True)
            meta = self.extractor.extract(filepath, covers_dir)
            assert meta.title == "empty"
            assert meta.author == "Unknown Author"
            assert meta.format == "mobi"

    def test_extract_exth_metadata(self):
        """Extract metadata from a synthetic MOBI with EXTH records."""
        with tempfile.TemporaryDirectory() as tmp:
            mobi_path = create_minimal_mobi_with_exth(
                tmp,
                title="Test Book",
                author="Jane Austen",
                publisher="TestPub",
                isbn="978-3-16-148410-0",
                description="A test book.",
                language="en",
                tags=["Fiction", "Fantasy"],
            )
            covers_dir = os.path.join(tmp, "covers")
            os.makedirs(covers_dir, exist_ok=True)
            meta = self.extractor.extract(mobi_path, covers_dir)
            assert meta.title == "Test Book", f"got '{meta.title}'"
            assert meta.author == "Jane Austen", f"got '{meta.author}'"
            assert meta.publisher == "TestPub", f"got '{meta.publisher}'"
            assert meta.isbn == "978-3-16-148410-0", f"got '{meta.isbn}'"
            assert meta.description == "A test book.", f"got '{meta.description}'"
            assert meta.language == "en", f"got '{meta.language}'"
            assert "Fiction" in meta.tags, f"tags: {meta.tags}"
            assert "Fantasy" in meta.tags, f"tags: {meta.tags}"
            assert meta.format == "mobi"