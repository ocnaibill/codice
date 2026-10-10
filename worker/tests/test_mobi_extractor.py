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


def build_mobi(tmpdir: str, title: str = "Test Book", author: str = "Jane Austen", publisher: str = "TestPub",
               isbn: str = "978-3-16-148410-0", description: str = "A test book.", language: str = "en", tags: list = None,
               date: str = None, encoding: int = 65001, header_length: int = 232, exth: bool = True, full_title: str = None,
               images=(), cover_offset: int = None, filename: str = "test.mobi") -> str:
    """A MOBI file laid out as the real ones are (the fields where the format puts them, measured on a real book).

    PalmDB header (78 bytes) and the table of records; record 0 is the PalmDOC header (16 bytes), the MOBI header (`header_length`
    bytes, which begins with 'MOBI'), the EXTH header and the full title; then one record per image, in order. In the MOBI header, from
    the magic: +4 length, +8 type, +12 text encoding, +20 version, +68 offset of the full title (from the start of record 0) and +72 its
    length, +92 the index of the first image record, +112 the EXTH flags (0x40: there is an EXTH)."""
    tags = ["Fiction", "Fantasy"] if tags is None else tags
    codec = 'utf-8' if encoding == 65001 else 'cp1252'

    def record(kind, value):
        return struct.pack('>II', kind, 8 + len(value)) + value

    items = []
    if exth:
        items += [record(100, author.encode(codec)), record(101, publisher.encode(codec)), record(104, isbn.encode(codec)),
                  record(103, description.encode(codec)), record(524, language.encode(codec))]
        if date:
            items.append(record(106, date.encode(codec)))
        items += [record(105, tag.encode(codec)) for tag in tags]
        if cover_offset is not None:
            items.append(record(201, struct.pack('>I', cover_offset)))
        items.append(record(503, title.encode(codec)))
    exth_body = b''.join(items)
    exth_header = b'EXTH' + struct.pack('>II', 12 + len(exth_body), len(items)) + exth_body if exth else b''
    exth_header += b'\x00' * ((4 - len(exth_header) % 4) % 4)

    shown_title = (full_title if full_title is not None else title).encode(codec)
    title_at = 16 + header_length + len(exth_header)
    mobi = bytearray(header_length)
    mobi[0:4] = b'MOBI'
    struct.pack_into('>I', mobi, 4, header_length)
    struct.pack_into('>I', mobi, 8, 2)            # a book
    struct.pack_into('>I', mobi, 12, encoding)
    if header_length >= 24:
        struct.pack_into('>I', mobi, 20, 6)
    if header_length >= 76:
        struct.pack_into('>I', mobi, 68, title_at)
        struct.pack_into('>I', mobi, 72, len(shown_title))
    if header_length >= 96:
        # the first image is the record after record 0 (if the book has images)
        struct.pack_into('>I', mobi, 92, 1 if images else 0xFFFFFFFF)
    if header_length >= 116:
        struct.pack_into('>I', mobi, 112, 0x40 if exth else 0)

    palmdoc = bytearray(16)
    struct.pack_into('>H', palmdoc, 0, 2)          # PalmDOC compression
    record0 = bytes(palmdoc) + bytes(mobi) + exth_header + shown_title + b'\x00\x00'
    records = [record0] + [bytes(image) for image in images]

    table_end = 78 + 8 * len(records) + 2
    offsets, at = [], table_end
    for rec in records:
        offsets.append(at)
        at += len(rec)
    palm = bytearray(78)
    palm[0:8] = b'Test_Boo'
    palm[60:68] = b'BOOKMOBI'
    struct.pack_into('>H', palm, 76, len(records))
    table = b''.join(struct.pack('>I', off) + struct.pack('>I', 2 * i) for i, off in enumerate(offsets)) + b'\x00\x00'
    path = os.path.join(tmpdir, filename)
    with open(path, 'wb') as handle:
        handle.write(bytes(palm) + table + b''.join(records))
    return path


JPEG = b'\xff\xd8\xff\xe0' + b'JFIF-' + b'\x01' * 40
PNG = b'\x89PNG\r\n\x1a\n' + b'\x02' * 40
TINY = b'\xff\xd8\xff\xe0' + b'tiny'


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
            mobi_path = build_mobi(
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
    def extract(self, **kw):
        with tempfile.TemporaryDirectory() as tmp:
            path = build_mobi(tmp, **kw)
            covers = os.path.join(tmp, 'covers')
            os.makedirs(covers)
            meta = self.extractor.extract(path, covers)
            kept = sorted(os.listdir(covers))
            return meta, kept, [open(os.path.join(covers, n), 'rb').read() for n in kept]

    def test_reads_a_book_laid_out_as_the_real_ones_are(self):
        """The case of the report: a UTF-8 MOBI 6 whose title, author, language, publisher and date were never read."""
        meta, _, _ = self.extract(title='Quarta Asa', author='Rebecca Yarros', publisher='Planeta Minotauro', language='pt',
                                  date='2024-03-15 03:00:00+00:00', tags=[], isbn='')
        assert meta.title == 'Quarta Asa'
        assert meta.author == 'Rebecca Yarros'
        assert meta.publisher == 'Planeta Minotauro'
        assert meta.language == 'pt'
        assert meta.publication_date == '2024-03-15'   # the date, not the timestamp of the file

    def test_the_date_is_kept_as_far_as_it_is_one(self):
        for raw, kept in (('2024-03-15T00:00:00Z', '2024-03-15'), ('2024-03', '2024-03'), ('2024', '2024'), ('março de 2024', 'março de 2024')):
            meta, _, _ = self.extract(date=raw)
            assert meta.publication_date == kept, raw

    def test_the_title_of_the_header_is_the_one_when_there_is_no_better(self):
        # EXTH 503 absent in a book that has only the full title of the MOBI header
        with tempfile.TemporaryDirectory() as tmp:
            path = build_mobi(tmp, title='Do Cabeçalho', exth=False)
            meta = self.extractor.extract(path, os.path.join(tmp, 'covers'))
            assert meta.title == 'Do Cabeçalho'
            assert meta.author == 'Unknown Author'   # no EXTH: nobody said

    def test_the_encoding_is_the_one_of_the_header_and_not_the_type_of_the_book(self):
        meta, _, _ = self.extract(title='Ação e Reação', author='José d\u2019Ávila'.replace('\u2019', "'"), encoding=1252)
        assert meta.title == 'Ação e Reação'
        assert meta.author == "José d'Ávila"

    def test_the_cover_is_the_one_the_exth_says_counted_from_the_first_image(self):
        meta, kept, data = self.extract(images=[TINY, JPEG, PNG], cover_offset=1)
        assert len(kept) == 1 and data[0] == JPEG   # not the first image, which is the little one
        assert meta.cover_path

    def test_without_a_cover_offset_the_first_image_is_the_cover(self):
        meta, kept, data = self.extract(images=[PNG, JPEG])
        assert len(kept) == 1 and data[0] == PNG
        assert meta.cover_path

    def test_a_cover_offset_that_is_not_an_image_falls_back_to_the_first(self):
        meta, kept, data = self.extract(images=[JPEG, b'not an image at all'], cover_offset=1)
        assert len(kept) == 1 and data[0] == JPEG

    def test_a_cover_offset_that_points_nowhere_does_not_break_the_reading(self):
        meta, kept, _ = self.extract(images=[JPEG], cover_offset=99)
        assert meta.title == 'Test Book' and len(kept) == 1

    def test_no_image_is_no_cover(self):
        meta, kept, _ = self.extract(images=[])
        assert kept == [] and not meta.cover_path

    def test_an_old_short_header_has_no_exth_to_read_and_does_not_fail(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = build_mobi(tmp, header_length=24, title='Antigo')
            # What follows a short header is not a field of it: even where the flags would be, the bytes say "there is an EXTH".
            data = bytearray(open(path, 'rb').read())
            record0 = struct.unpack('>I', data[78:82])[0]
            struct.pack_into('>I', data, record0 + 16 + 112, 0x40)
            open(path, 'wb').write(bytes(data))
            meta = self.extractor.extract(path, os.path.join(tmp, 'covers'))
            assert meta.format == 'mobi'
            assert meta.author == 'Unknown Author'

    def test_a_file_with_the_header_cut_short_falls_back_to_the_name(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = build_mobi(tmp, filename='cortado.mobi')
            data = open(path, 'rb').read()
            open(path, 'wb').write(data[:150])
            meta = self.extractor.extract(path, os.path.join(tmp, 'covers'))
            assert meta.title == 'cortado'
