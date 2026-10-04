"""What a comic file says about how it is read (#19): the direction in its ComicInfo.xml, or pages that are strips."""
import os
import struct
import sys
import zipfile
import zlib

import pytest

from analyzer import Analyzer
from extractors import comic_layout
from extractors.cbr_extractor import CbrExtractor
from extractors.cbz_extractor import CbzExtractor
from extractors.comic_layout import declared_mode, image_size, is_strip, sample
from pipeline import analyze_file
from tests.test_analyzer import FakeDB
from tests.test_pipeline import FakeExtractor, FakeProviders, meta

CORPUS = os.path.join(os.path.dirname(__file__), '..', '..', 'testdata', 'corpus')


def png(width, height):
    """A PNG with a real header and a real (blank) image, so the size is read from where it really is."""
    def chunk(kind, data):
        body = kind + data
        return struct.pack('>I', len(data)) + body + struct.pack('>I', zlib.crc32(body) & 0xFFFFFFFF)
    row = b'\x00' + b'\xff' * width
    return (b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 0, 0, 0, 0))
            + chunk(b'IDAT', zlib.compress(row * height)) + chunk(b'IEND', b''))


def jpeg(width, height, before_frame=b''):
    """The segments of a JPEG up to its frame header; `before_frame` stands for a large EXIF block."""
    soi = b'\xff\xd8'
    app0 = b'\xff\xe0' + struct.pack('>H', 16) + b'JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00'
    frame = b'\xff\xc0' + struct.pack('>HBHHB', 11, 8, height, width, 1) + b'\x01\x11\x00'
    return soi + app0 + before_frame + frame + b'\xff\xd9'


def exif(size):
    return b'\xff\xe1' + struct.pack('>H', size + 2) + b'x' * size


def webp_lossy(width, height):
    payload = b'\x00\x00\x00' + b'\x9d\x01\x2a' + struct.pack('<HH', width, height)
    return b'RIFF' + struct.pack('<I', 4 + 8 + len(payload)) + b'WEBP' + b'VP8 ' + struct.pack('<I', len(payload)) + payload


def webp_lossless(width, height):
    bits = (width - 1) | ((height - 1) << 14)
    payload = b'\x2f' + struct.pack('<I', bits)
    return b'RIFF' + struct.pack('<I', 4 + 8 + len(payload)) + b'WEBP' + b'VP8L' + struct.pack('<I', len(payload)) + payload


def webp_extended(width, height):
    payload = b'\x00\x00\x00\x00' + (width - 1).to_bytes(3, 'little') + (height - 1).to_bytes(3, 'little')
    return b'RIFF' + struct.pack('<I', 4 + 8 + len(payload)) + b'WEBP' + b'VP8X' + struct.pack('<I', len(payload)) + payload


class TestImageSize:
    def test_reads_each_format_from_its_header(self):
        assert image_size(png(400, 6000)) == (400, 6000)
        assert image_size(jpeg(320, 480)) == (320, 480)
        assert image_size(webp_lossy(300, 4500)) == (300, 4500)
        assert image_size(webp_lossless(301, 4501)) == (301, 4501)
        assert image_size(webp_extended(302, 4502)) == (302, 4502)

    def test_a_jpeg_with_a_large_exif_block_before_its_size_is_still_read(self):
        assert image_size(jpeg(500, 900, before_frame=exif(60000))) == (500, 900)

    def test_what_is_not_an_image_or_is_cut_short_has_no_size(self):
        assert image_size(b'') is None
        assert image_size(b'not an image at all') is None
        assert image_size(png(10, 10)[:20]) is None
        assert image_size(jpeg(10, 10)[:12]) is None
        assert image_size(webp_lossy(10, 10)[:20]) is None

    def test_a_jpeg_with_a_table_before_its_frame_header_is_still_read(self):
        dht = b'\xff\xc4' + struct.pack('>H', 20) + b'\x00' * 18  # a table, not a frame: it has no size
        assert image_size(jpeg(320, 480, before_frame=dht)) == (320, 480)

    def test_a_lossy_webp_ignores_the_scaling_bits_of_its_size(self):
        assert image_size(webp_lossy(300 | 0x4000, 4500 | 0x8000)) == (300, 4500)

    def test_the_corpus_pages_are_measured(self):
        with zipfile.ZipFile(os.path.join(CORPUS, 'cbz_webtoon_imagem_longa.cbz')) as zf:
            assert image_size(zf.read('001.png')) == (400, 6000)


class TestDeclaredMode:
    def test_pages_several_times_taller_than_wide_make_a_strip(self):
        assert is_strip([(400, 6000)])
        assert is_strip([(800, 3200), (800, 3200), (800, 1200)])
        assert is_strip([(800, 2800)] * 3)               # 3.5 is
        assert is_strip([(800, 2400)] * 3)               # and 3.0 exactly is
        assert not is_strip([(800, 1200)] * 5)           # an ordinary page is 1.5 times
        assert not is_strip([(1600, 1200)] * 5)          # a spread is wider than tall
        assert not is_strip([(800, 2300)] * 4)           # 2.9 is still a tall page, not a strip

    def test_it_takes_most_of_the_pages_not_one(self):
        assert not is_strip([(400, 6000)] + [(800, 1200)] * 4)  # one long page does not turn a book into a strip
        assert not is_strip([(400, 6000)] * 2 + [(800, 1200)] * 2)  # half is not most
        assert is_strip([(400, 6000)] * 3 + [(800, 1200)] * 2)

    def test_pages_that_could_not_be_measured_count_for_nothing(self):
        assert not is_strip([])
        assert not is_strip([(0, 0), (0, 0)])
        assert is_strip([(0, 0), (400, 6000), (400, 6000)])

    def test_comicinfo_right_to_left(self):
        page = [(800, 1200)]
        assert declared_mode('YesAndRightToLeft', page) == 'rtl'
        assert declared_mode(' yesandrighttoleft ', page) == 'rtl'

    def test_manga_that_is_not_right_to_left_says_nothing(self):
        page = [(800, 1200)]
        for value in ('Yes', 'No', 'Unknown', '', None):
            assert declared_mode(value, page) is None

    def test_a_strip_is_read_top_to_bottom_whatever_comicinfo_says(self):
        assert declared_mode('YesAndRightToLeft', [(400, 6000)]) == 'webtoon'
        assert declared_mode('', [(400, 6000)]) == 'webtoon'


class TestSample:
    def test_a_short_file_is_measured_whole(self):
        names = [f'{i}.png' for i in range(5)]
        assert sample(names) == names
        assert sample([]) == []

    def test_a_long_one_is_measured_in_a_few_places_spread_over_it(self):
        names = [f'{i:03}.png' for i in range(200)]
        picked = sample(names)
        assert len(picked) == comic_layout.SAMPLE_PAGES
        assert picked[0] == '000.png' and len(set(picked)) == len(picked)
        assert picked[-1] > '150.png'  # not just the first few


def build_cbz(tmp_path, pages, comicinfo=None):
    path = tmp_path / 'c.cbz'
    with zipfile.ZipFile(path, 'w') as zf:
        if comicinfo is not None:
            zf.writestr('ComicInfo.xml', comicinfo)
        for i, data in enumerate(pages):
            zf.writestr(f'{i:03}.png', data)
    return str(path)


def extract_cbz(path, tmp_path):
    covers = tmp_path / 'covers'
    covers.mkdir(exist_ok=True)
    return CbzExtractor().extract(path, str(covers))


class TestCbzExtractor:
    @pytest.mark.parametrize('name,expected', [('cbz_rtl.cbz', 'rtl'), ('cbz_webtoon_imagem_longa.cbz', 'webtoon'), ('cbz_ltr.cbz', None)])
    def test_the_corpus(self, tmp_path, name, expected):
        meta_ = extract_cbz(os.path.join(CORPUS, name), tmp_path)
        assert meta_.raw.get('declared_mode') == expected

    def test_a_book_with_one_long_page_among_ordinary_ones_is_not_a_strip(self, tmp_path):
        pages = [png(400, 6000)] + [png(80, 120) for _ in range(9)]
        assert 'declared_mode' not in extract_cbz(build_cbz(tmp_path, pages), tmp_path).raw

    def test_a_long_file_is_measured_in_the_middle_too(self, tmp_path):
        # An ordinary cover and an opening page, then strips: the sample spreads over the file.
        pages = [png(80, 120)] * 2 + [png(40, 600)] * 38
        assert extract_cbz(build_cbz(tmp_path, pages), tmp_path).raw.get('declared_mode') == 'webtoon'

    def test_a_tall_jpeg_with_a_large_exif_block_is_measured_by_the_extractor(self, tmp_path):
        page = jpeg(400, 6000, before_frame=exif(60000))
        path = tmp_path / 'c.cbz'
        with zipfile.ZipFile(path, 'w') as zf:
            zf.writestr('001.jpg', page)
        assert extract_cbz(str(path), tmp_path).raw.get('declared_mode') == 'webtoon'

    def test_pages_that_are_not_images_do_not_stop_the_analysis(self, tmp_path):
        meta_ = extract_cbz(build_cbz(tmp_path, [b'fake-image-data', b'fake-image-data'],
                                      '<ComicInfo><Title>T</Title><Manga>YesAndRightToLeft</Manga></ComicInfo>'), tmp_path)
        assert meta_.title == 'T' and meta_.page_count == 2
        assert meta_.raw['declared_mode'] == 'rtl'  # the direction is declared, the sizes are just unknown

    def test_the_manga_value_is_kept_as_the_file_wrote_it(self, tmp_path):
        info = '<ComicInfo><Manga>Yes</Manga></ComicInfo>'
        meta_ = extract_cbz(build_cbz(tmp_path, [png(80, 120)], info), tmp_path)
        assert meta_.raw['manga'] == 'Yes' and 'declared_mode' not in meta_.raw


class FakeRar:
    """What the CBR extractor asks of a RAR archive, from memory (a RAR cannot be made without its own tool)."""
    files = {}

    def __init__(self, path):
        pass

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def namelist(self):
        return list(self.files)

    def getinfo(self, name):
        from types import SimpleNamespace
        return SimpleNamespace(file_size=len(self.files[name]))

    def read(self, name):
        return self.files[name]

    def open(self, name):
        import io
        return io.BytesIO(self.files[name])


class TestCbrExtractor:
    def extract(self, tmp_path, monkeypatch, files):
        import rarfile
        monkeypatch.setattr(rarfile, 'RarFile', type('R', (FakeRar,), {'files': files}))
        covers = tmp_path / 'covers'
        covers.mkdir(exist_ok=True)
        return CbrExtractor().extract('/x/c.cbr', str(covers))

    def test_right_to_left(self, tmp_path, monkeypatch):
        files = {'ComicInfo.xml': b'<ComicInfo><Manga>YesAndRightToLeft</Manga></ComicInfo>', '001.png': png(80, 120)}
        assert self.extract(tmp_path, monkeypatch, files).raw['declared_mode'] == 'rtl'

    def test_a_strip(self, tmp_path, monkeypatch):
        assert self.extract(tmp_path, monkeypatch, {'001.png': png(40, 600)}).raw['declared_mode'] == 'webtoon'

    def test_a_long_file_is_measured_in_the_middle_too(self, tmp_path, monkeypatch):
        files = {f'{i:03}.png': png(80, 120) if i < 2 else png(40, 600) for i in range(40)}
        assert self.extract(tmp_path, monkeypatch, files).raw['declared_mode'] == 'webtoon'

    def test_nothing_declared(self, tmp_path, monkeypatch):
        meta_ = self.extract(tmp_path, monkeypatch, {'001.png': png(80, 120), '002.png': png(80, 120)})
        assert 'declared_mode' not in meta_.raw


class TestPipeline:
    def run(self, fmt, raw):
        db = FakeDB()
        analyze_file(7, '/f/c.' + fmt, FakeExtractor(meta(format=fmt, raw=raw)), Analyzer(db), FakeProviders(), '/covers')
        return [(q, p) for q, p in db.statements if 'declared_mode' in q]

    def test_the_mode_a_comic_declares_is_recorded(self):
        assert [p for _, p in self.run('cbz', {'declared_mode': 'rtl'})] == [('rtl', 7)]
        assert [p for _, p in self.run('cbr', {'declared_mode': 'webtoon'})] == [('webtoon', 7)]

    def test_a_comic_that_declares_nothing_clears_what_was_recorded(self):
        # Analysed again after the file changed: it must not keep a mode it no longer declares.
        assert [p for _, p in self.run('cbz', {})] == [(None, 7)]

    def test_other_formats_are_not_asked(self):
        assert self.run('epub', {'declared_mode': 'rtl'}) == []


URL = os.environ.get('TEST_DATABASE_URL')


@pytest.mark.skipif(not URL, reason='TEST_DATABASE_URL is not set')
class TestSavedForReal:
    """The statement run against a PostgreSQL, on temporary tables with only what it touches."""

    @pytest.fixture
    def db(self):
        import psycopg2

        class Db:
            def __init__(self, conn):
                self.cur = conn.cursor()

            def execute(self, query, params=()):
                self.cur.execute(query, params)

        conn = psycopg2.connect(URL)
        cur = conn.cursor()
        cur.execute("""
            CREATE TEMP TABLE files (id INTEGER PRIMARY KEY, declared_mode VARCHAR(8)
                CHECK (declared_mode IN ('rtl', 'webtoon')));
            CREATE TEMP TABLE work_primary (work_id INTEGER, file_id INTEGER);
            INSERT INTO files (id) VALUES (100), (200);
            INSERT INTO work_primary VALUES (1, 100), (2, 200), (3, NULL);
        """)
        yield Db(conn)
        conn.rollback()
        conn.close()

    def modes(self, db):
        db.execute('SELECT id, declared_mode FROM files ORDER BY id')
        return db.cur.fetchall()

    def test_it_is_recorded_on_the_file_of_that_work_only(self, db):
        Analyzer(db).save_declared_mode(1, 'rtl')
        assert self.modes(db) == [(100, 'rtl'), (200, None)]

    def test_analysing_again_replaces_it_and_can_clear_it(self, db):
        Analyzer(db).save_declared_mode(1, 'rtl')
        Analyzer(db).save_declared_mode(1, 'webtoon')
        assert self.modes(db) == [(100, 'webtoon'), (200, None)]
        Analyzer(db).save_declared_mode(1, None)
        assert self.modes(db) == [(100, None), (200, None)]

    def test_a_work_without_a_file_changes_nothing(self, db):
        Analyzer(db).save_declared_mode(3, 'rtl')
        Analyzer(db).save_declared_mode(99, 'rtl')
        assert self.modes(db) == [(100, None), (200, None)]
