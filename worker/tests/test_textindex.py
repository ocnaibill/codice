"""The text of the files: cleaning, cutting, each reader, and the indexer that publishes it."""
import json
import os
import zipfile

import fitz
import pytest

from textindex import EXTRACTOR_VERSION, Limits, Segment
from textindex import normalize
from textindex.epub import epub_segments
from textindex.pdf import pdf_segments
from textindex.plain import plain_segments
from textindex import store as store_module
from textindex.store import TextIndexer, resolve

CORPUS = os.path.join(os.path.dirname(__file__), '..', '..', 'testdata', 'corpus')


# ── cleaning and cutting ─────────────────────────────────────────────

class TestClean:
    def test_composes_unicode_and_collapses_whitespace(self):
        assert normalize.clean('  Olá  mundo \t x ') == 'Olá mundo x'
        assert normalize.clean('á') == 'á'  # a decomposed accent becomes one character

    def test_removes_control_characters_including_the_search_markers(self):
        assert normalize.clean('a\x02b\x03c\x00d​e') == 'a b c d e'.replace(' ', ' ')

    def test_joins_words_broken_by_a_hyphen_only_when_the_line_goes_on_in_lower_case(self):
        assert normalize.dehyphenate('constan-\ntinopla') == 'constantinopla'
        assert normalize.dehyphenate('bio-\nGrafia') == 'bio-\nGrafia'   # a real hyphen before a name
        assert normalize.dehyphenate('a - b\nc') == 'a - b\nc'


class TestChunk:
    def test_groups_paragraphs_in_order_with_their_start(self):
        paras = ['a' * 500, 'b' * 500, 'c' * 500, 'd' * 10]
        chunks = list(normalize.chunk(paras))
        assert [len(t) for t, _ in chunks] == [1502, 10]   # three paragraphs reach the target, the rest is its own
        assert [s for _, s in chunks] == [0, 1503]
        assert chunks[0][0].split('\n') == paras[:3]

    def test_loses_no_text_and_never_exceeds_the_limit(self):
        paras = [('palavra ' * n).strip() for n in (5, 300, 900, 1, 450)]
        chunks = list(normalize.chunk(paras))
        assert all(len(t) <= normalize.HARD_MAX_CHARS for t, _ in chunks)
        assert ' '.join(t.replace('\n', ' ') for t, _ in chunks).split() == ' '.join(paras).split()

    def test_a_paragraph_longer_than_the_limit_is_cut_at_sentence_ends_first(self):
        sentence = 'Uma frase completa termina aqui. '
        parts = list(normalize.split_long((sentence * 200).strip(), 300))
        assert all(len(p) <= 300 for p in parts)
        assert all(p.endswith('.') for p in parts)

    def test_a_word_longer_than_the_limit_is_still_cut(self):
        parts = list(normalize.split_long('x' * 5000, 2400))
        assert all(len(p) <= 2400 for p in parts) and ''.join(parts) == 'x' * 5000


# ── EPUB ─────────────────────────────────────────────────────────────

def make_epub(path, chapters, spine=None, extra=None):
    """chapters: {href: xhtml body}; the spine lists them in this order (the manifest is in another)."""
    manifest = ''.join(f'<item id="i{i}" href="{href}" media-type="application/xhtml+xml"/>' for i, href in enumerate(sorted(chapters)))
    ids = {href: f'i{i}' for i, href in enumerate(sorted(chapters))}
    order = spine or list(chapters)
    itemrefs = ''.join(f'<itemref idref="{ids[h]}"/>' for h in order)
    opf = ('<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata/>'
           f'<manifest>{manifest}<item id="css" href="s.css" media-type="text/css"/></manifest><spine>{itemrefs}<itemref idref="css"/></spine></package>')
    with zipfile.ZipFile(path, 'w') as zf:
        zf.writestr('mimetype', 'application/epub+zip')
        zf.writestr('META-INF/container.xml',
                    '<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">'
                    '<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>')
        zf.writestr('OEBPS/content.opf', opf)
        for href, body in chapters.items():
            zf.writestr(f'OEBPS/{href}', f'<html xmlns="http://www.w3.org/1999/xhtml"><head><title>ignorado</title><style>p{{}}</style></head><body>{body}</body></html>')
        for name, data in (extra or {}).items():
            zf.writestr(name, data)


class TestEpub:
    def test_reads_chapters_in_the_order_of_the_spine_with_their_address_and_heading(self, tmp_path):
        path = tmp_path / 'b.epub'
        make_epub(path, {
            'text/a.xhtml': '<h1>Capítulo A</h1><p>Primeiro <b>parágrafo</b> do A.</p><script>var x = "nao";</script>',
            'text/b.xhtml': '<h2>Capítulo B</h2><p>Texto do B.</p><p>Outro do B.</p>',
        }, spine=['text/b.xhtml', 'text/a.xhtml'])
        segs = list(epub_segments(str(path)))
        assert [s.locator['href'] for s in segs] == ['text/b.xhtml', 'text/a.xhtml']
        assert [s.section for s in segs] == ['Capítulo B', 'Capítulo A']
        assert segs[1].text.split('\n') == ['Capítulo A', 'Primeiro parágrafo do A.']
        assert all(s.locator['type'] == 'epub' and s.origin == 'native' for s in segs)
        assert 'nao' not in ' '.join(s.text for s in segs) and 'ignorado' not in ' '.join(s.text for s in segs)

    def test_progression_says_how_far_into_the_chapter_a_segment_starts(self, tmp_path):
        path = tmp_path / 'b.epub'
        make_epub(path, {'c.xhtml': ''.join(f'<p>{"palavra " * 90}{i}</p>' for i in range(20))})
        segs = list(epub_segments(str(path)))
        progression = [s.locator['progression'] for s in segs]
        assert len(segs) > 3 and progression[0] == 0.0
        assert progression == sorted(progression) and 0 <= progression[-1] < 1

    def test_skips_what_is_not_a_chapter_and_a_chapter_the_archive_lacks(self, tmp_path):
        path = tmp_path / 'b.epub'
        make_epub(path, {'a.xhtml': '<p>Existe.</p>', 'gone.xhtml': '<p>Some.</p>'})
        with zipfile.ZipFile(path) as zf:
            members = {n: zf.read(n) for n in zf.namelist() if n != 'OEBPS/gone.xhtml'}
        with zipfile.ZipFile(path, 'w') as zf:
            for n, d in members.items():
                zf.writestr(n, d)
        assert [s.text for s in epub_segments(str(path))] == ['Existe.']

    def test_a_broken_archive_is_a_value_error(self, tmp_path):
        bad = tmp_path / 'b.epub'
        bad.write_bytes(b'PK\x03\x04 not really')
        with pytest.raises(ValueError):
            list(epub_segments(str(bad)))
        notzip = tmp_path / 'c.epub'
        notzip.write_text('plain text')
        with pytest.raises(ValueError):
            list(epub_segments(str(notzip)))

    def test_an_archive_that_expands_too_much_is_refused(self, tmp_path, monkeypatch):
        path = tmp_path / 'b.epub'
        make_epub(path, {'a.xhtml': '<p>' + 'x' * 5000 + '</p>'})
        monkeypatch.setattr(Limits, 'MAX_ARCHIVE_BYTES', 1000)
        with pytest.raises(ValueError, match='too much'):
            list(epub_segments(str(path)))

    def test_a_chapter_that_is_too_large_and_too_much_text_are_refused(self, tmp_path, monkeypatch):
        path = tmp_path / 'b.epub'
        make_epub(path, {'a.xhtml': '<p>' + 'palavra ' * 500 + '</p>'})
        monkeypatch.setattr(Limits, 'MAX_ENTRY_BYTES', 100)
        with pytest.raises(ValueError, match='too large'):
            list(epub_segments(str(path)))
        monkeypatch.setattr(Limits, 'MAX_ENTRY_BYTES', 10**8)
        monkeypatch.setattr(Limits, 'MAX_CHARS', 200)
        with pytest.raises(ValueError, match='too much text'):
            list(epub_segments(str(path)))

    def test_the_cancellation_check_is_called_between_chapters(self, tmp_path):
        path = tmp_path / 'b.epub'
        make_epub(path, {'a.xhtml': '<p>A</p>', 'b.xhtml': '<p>B</p>'})
        calls = []

        def checkpoint():
            calls.append(1)
            if len(calls) == 2:
                raise RuntimeError('cancelled')

        with pytest.raises(RuntimeError, match='cancelled'):
            list(epub_segments(str(path), checkpoint))

    @pytest.mark.parametrize('declaration,encoding', [
        ('<?xml version="1.0" encoding="ISO-8859-1"?>', 'iso-8859-1'),
        ('<?xml version="1.0" encoding="windows-1252"?>', 'cp1252'),
        ('<?xml version="1.0"?>', 'utf-8'),          # nothing declared: XML says UTF-8
        ('<!DOCTYPE html><meta charset="iso-8859-1">', 'iso-8859-1'),
        ('', 'utf-8'),
    ])
    def test_reads_the_encoding_the_chapter_declares_and_utf8_when_it_says_nothing(self, tmp_path, declaration, encoding):
        path = tmp_path / 'b.epub'
        make_epub(path, {'a.xhtml': '<p>coração e ação</p>'})
        with zipfile.ZipFile(path) as zf:
            members = {n: zf.read(n) for n in zf.namelist()}
        body = (declaration + '<html><body><p>coração e ação</p></body></html>').encode(encoding)
        members['OEBPS/a.xhtml'] = body
        with zipfile.ZipFile(path, 'w') as zf:
            for n, d in members.items():
                zf.writestr(n, d)
        assert [seg.text for seg in epub_segments(str(path))] == ['coração e ação']

    def test_the_corpus_epub_keeps_its_accents(self):
        segs = list(epub_segments(os.path.join(CORPUS, 'epub_acentos.epub')))
        text = ' '.join(s.text for s in segs)
        assert segs and 'Capítulo' in text and 'coração' in text


# ── PDF ──────────────────────────────────────────────────────────────

def make_pdf(path, pages, password=None, fontsize=11):
    doc = fitz.open()
    for text in pages:
        page = doc.new_page()
        if text:
            page.insert_textbox(fitz.Rect(50, 50, 550, 780), text, fontsize=fontsize)
    if password:
        doc.save(str(path), encryption=fitz.PDF_ENCRYPT_AES_256, owner_pw=password, user_pw=password)
    else:
        doc.save(str(path))
    doc.close()


class TestPdf:
    def test_one_segment_per_page_of_text_with_its_index_from_zero(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_pdf(path, ['Primeira página com texto suficiente.', '', 'Terceira página, também com texto de sobra.'])
        segs = list(pdf_segments(str(path)))
        assert [s.locator for s in segs] == [{'type': 'pdf', 'page': 0}, {'type': 'pdf', 'page': 2}]
        assert 'Primeira página' in segs[0].text

    def test_joins_a_word_broken_at_the_end_of_a_line(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_pdf(path, ['A cidade de Constan-\ntinopla foi tomada em 1453 pelos turcos.'])
        assert 'Constantinopla' in ' '.join(s.text for s in pdf_segments(str(path)))

    def test_an_image_on_the_page_is_not_text(self, tmp_path):
        path = tmp_path / 'a.pdf'
        doc = fitz.open()
        page = doc.new_page()
        page.insert_textbox(fitz.Rect(50, 300, 550, 780), 'Texto ao lado de uma figura, longo o bastante para contar.', fontsize=11)
        pix = fitz.Pixmap(fitz.csRGB, fitz.IRect(0, 0, 60, 60), False)
        pix.clear_with(200)
        page.insert_image(fitz.Rect(50, 50, 200, 200), pixmap=pix)
        doc.save(str(path))
        doc.close()
        text = ' '.join(s.text for s in pdf_segments(str(path)))
        assert 'Texto ao lado' in text and 'image' not in text.lower()

    def test_a_scan_has_no_text_to_read(self):
        assert list(pdf_segments(os.path.join(CORPUS, 'pdf_escaneado.pdf'))) == []

    def test_a_mixed_pdf_gives_only_the_page_with_text(self):
        segs = list(pdf_segments(os.path.join(CORPUS, 'pdf_misto.pdf')))
        assert [s.locator['page'] for s in segs] == [0]
        assert 'Acentuação' in segs[0].text

    def test_a_long_page_is_cut_but_keeps_its_address(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_pdf(path, [('Uma frase longa de exemplo que se repete. ' * 80)], fontsize=5)
        segs = list(pdf_segments(str(path)))
        assert len(segs) >= 1 and {s.locator['page'] for s in segs} == {0}
        assert all(len(s.text) <= normalize.HARD_MAX_CHARS for s in segs)

    def test_an_encrypted_or_broken_pdf_is_a_value_error(self, tmp_path):
        enc = tmp_path / 'e.pdf'
        make_pdf(enc, ['texto secreto'], password='segredo')
        with pytest.raises(ValueError, match='encrypted'):
            list(pdf_segments(str(enc)))
        bad = tmp_path / 'b.pdf'
        bad.write_bytes(b'%PDF-1.4 truncated')
        with pytest.raises(ValueError):
            list(pdf_segments(str(bad)))

    def test_too_many_pages_and_too_much_text_are_refused(self, tmp_path, monkeypatch):
        path = tmp_path / 'a.pdf'
        make_pdf(path, ['Texto de uma página com o bastante para contar.'] * 3)
        monkeypatch.setattr(Limits, 'MAX_PDF_PAGES', 2)
        with pytest.raises(ValueError, match='too many pages'):
            list(pdf_segments(str(path)))
        monkeypatch.setattr(Limits, 'MAX_PDF_PAGES', 100)
        monkeypatch.setattr(Limits, 'MAX_CHARS', 60)
        with pytest.raises(ValueError, match='too much text'):
            list(pdf_segments(str(path)))


# ── text and Markdown ────────────────────────────────────────────────

class TestPlain:
    def test_paragraphs_with_the_offset_where_each_starts_in_the_text(self, tmp_path):
        path = tmp_path / 'a.txt'
        body = 'Primeiro parágrafo,\nquebrado em linhas.\n\nSegundo parágrafo.\n\n\nTerceiro.'
        path.write_text(body, encoding='utf-8')
        segs = list(plain_segments(str(path)))
        assert len(segs) == 1 and segs[0].text == 'Primeiro parágrafo, quebrado em linhas.\nSegundo parágrafo.\nTerceiro.'
        assert segs[0].locator == {'type': 'text', 'offset': 0}

    def test_the_offset_points_into_the_original_text(self, tmp_path):
        path = tmp_path / 'a.txt'
        paras = [('Parágrafo %d: ' % i) + 'texto ' * 120 for i in range(12)]
        body = '\n\n'.join(paras)
        path.write_text(body, encoding='utf-8')
        segs = list(plain_segments(str(path)))
        assert len(segs) > 2
        for seg in segs:
            first_words = seg.text.split('\n')[0][:20]
            assert body[seg.locator['offset']:].startswith(first_words)

    def test_decodes_utf8_and_windows_1252_and_crlf(self, tmp_path):
        a = tmp_path / 'a.txt'
        a.write_bytes('ação\r\n\r\nção'.encode('utf-8'))
        assert [s.text for s in plain_segments(str(a))] == ['ação\nção']
        b = tmp_path / 'b.txt'
        b.write_bytes('coração'.encode('cp1252'))
        assert next(plain_segments(str(b))).text == 'coração'

    def test_markdown_knows_the_heading_it_is_under(self, tmp_path):
        path = tmp_path / 'a.md'
        path.write_text('# Título\n\nIntrodução.\n\n## Seção dois\n\n' + 'Texto da seção. ' * 200, encoding='utf-8')
        segs = list(plain_segments(str(path), 'md'))
        assert segs[0].section == 'Título'
        assert segs[-1].section == 'Seção dois'

    def test_an_empty_file_has_no_segments_and_a_huge_one_is_refused(self, tmp_path, monkeypatch):
        empty = tmp_path / 'e.txt'
        empty.write_text('  \n\n  ')
        assert list(plain_segments(str(empty))) == []
        big = tmp_path / 'b.txt'
        big.write_text('palavra ' * 100)
        monkeypatch.setattr(Limits, 'MAX_TEXT_FILE_BYTES', 50)
        with pytest.raises(ValueError, match='too large'):
            list(plain_segments(str(big)))


# ── the indexer ──────────────────────────────────────────────────────

class FakeDB:
    """Records what the indexer asks of the database and answers the few questions it asks."""

    def __init__(self, files, generation=1):
        self.files = files
        self.generation = generation
        self.calls = []   # (kind, sql-or-name, params)
        self.rows = []

    def fetchall(self, query, params=None):
        self.calls.append(('fetchall', 'files', params))
        return self.files

    def fetchone(self, query, params=None):
        name = query.split('(')[0].replace('SELECT', '').strip()
        self.calls.append(('fetchone', name, params))
        return (self.generation,) if name == 'text_extraction_begin' else (1,)

    def execute(self, query, params=None):
        self.calls.append(('execute', query.split('(')[0].replace('SELECT', '').strip(), params))

    def insert_many(self, query, rows, template=None):
        self.calls.append(('insert_many', 'insert_many', len(rows)))
        self.rows.extend(rows)

    def names(self):
        return [c[1] for c in self.calls if c[0] != 'fetchall']


def file_row(fid, fmt='pdf', sha='aa', language='pt', availability='available', mode='managed', root=None,
             path='a.pdf', version=None, source_sha=None, status=None):
    return (fid, fmt, sha, language, availability, mode, root, path, version, source_sha, status)


@pytest.fixture
def storage(tmp_path):
    make_pdf(tmp_path / 'a.pdf', ['Texto de uma página com o bastante para contar.'])
    make_epub(tmp_path / 'b.epub', {'a.xhtml': '<p>Capítulo um.</p>'})
    (tmp_path / 'scan.pdf').write_bytes(open(os.path.join(CORPUS, 'pdf_escaneado.pdf'), 'rb').read())
    (tmp_path / 'c.cbz').write_bytes(b'PK')
    return tmp_path


class TestIndexer:
    def test_begins_writes_and_publishes_a_generation(self, storage):
        db = FakeDB([file_row(7, 'pdf', 'aa', 'pt', path='a.pdf')], generation=3)
        out = TextIndexer(db, str(storage)).run(9)
        assert out == {7: 'ready'}
        assert db.names() == ['text_extraction_begin', 'insert_many', 'text_extraction_publish']
        file_id, generation, sequence, origin, section, text, locator, version, node = db.rows[0]
        assert (file_id, generation, sequence, origin, version, node) == (7, 3, 0, 'native', 1, None)
        assert 'Texto de uma página' in text and json.loads(locator) == {'type': 'pdf', 'page': 0}
        publish = [c for c in db.calls if c[1] == 'text_extraction_publish'][0][2]
        assert publish == (7, 3, EXTRACTOR_VERSION, 'aa', 'ready', 'pt', None)  # a PDF with no bookmarks has no structure

    def test_a_file_with_no_text_is_published_as_empty_without_segments(self, storage):
        db = FakeDB([file_row(7, 'pdf', path='scan.pdf')])
        assert TextIndexer(db, str(storage)).run(9) == {7: 'empty'}
        assert 'insert_many' not in db.names()
        assert [c for c in db.calls if c[1] == 'text_extraction_publish'][0][2][4] == 'empty'

    def test_a_kind_of_file_with_no_text_is_published_as_unsupported_without_being_opened(self, storage):
        db = FakeDB([file_row(7, 'cbz', path='does-not-matter.cbz')])
        assert TextIndexer(db, str(storage)).run(9) == {7: 'unsupported'}
        assert db.names() == ['text_extraction_begin', 'text_extraction_publish']

    def test_writes_in_batches_and_checks_for_cancellation_between_them(self, storage, monkeypatch):
        make_pdf(storage / 'big.pdf', ['Página %d com texto o bastante para contar.' % i for i in range(5)])
        monkeypatch.setattr(store_module, 'BATCH', 2)
        db = FakeDB([file_row(7, 'pdf', path='big.pdf')])
        checks = []
        TextIndexer(db, str(storage)).run(9, checkpoint=lambda: checks.append(1))
        assert [c[2] for c in db.calls if c[0] == 'insert_many'] == [2, 2, 1]
        assert len(checks) >= 3

    def test_a_file_that_cannot_be_read_is_recorded_and_the_others_still_are(self, storage):
        (storage / 'bad.pdf').write_bytes(b'%PDF truncated')
        db = FakeDB([file_row(7, 'pdf', 'aa', path='bad.pdf'), file_row(8, 'epub', 'bb', path='b.epub')])
        out = TextIndexer(db, str(storage), log=lambda *_: None).run(9)
        assert out == {7: 'failed', 8: 'ready'}
        fail = [c for c in db.calls if c[1] == 'text_extraction_fail'][0][2]
        assert fail[0] == 7 and fail[2] == 'aa' and 'PDF' in fail[3]

    def test_a_file_that_is_not_where_it_should_be_stops_the_job_so_it_can_be_tried_again(self, storage):
        db = FakeDB([file_row(7, 'pdf', path='vanished.pdf')])
        with pytest.raises(FileNotFoundError):
            TextIndexer(db, str(storage)).run(9)
        assert 'text_extraction_fail' not in db.names()   # it may be back next time: not a verdict on the file

    def test_reads_only_what_needs_it(self, storage):
        idx = TextIndexer(FakeDB([]), str(storage))
        current = file_row(1, version=EXTRACTOR_VERSION, source_sha='aa', status='ready')
        assert not idx.needs_reading(current, force=False)
        assert idx.needs_reading(current, force=True)
        assert idx.needs_reading(file_row(1), force=False)                                              # never read
        assert idx.needs_reading(file_row(1, version=EXTRACTOR_VERSION - 1, source_sha='aa', status='ready'), False)  # an older version
        assert idx.needs_reading(file_row(1, sha='bb', version=EXTRACTOR_VERSION, source_sha='aa', status='ready'), False)  # the file changed
        assert not idx.needs_reading(file_row(1, sha=None, version=EXTRACTOR_VERSION, source_sha=None, status='ready'), False)
        assert not idx.needs_reading(file_row(1, version=EXTRACTOR_VERSION, source_sha='aa', status='failed'), False)  # failed the same way

    def test_skips_files_that_are_not_available_and_reads_the_rest_of_the_work(self, storage):
        db = FakeDB([file_row(7, 'pdf', availability='missing'), file_row(8, 'epub', path='b.epub')])
        assert TextIndexer(db, str(storage)).run(9) == {8: 'ready'}

    def test_an_already_read_work_costs_nothing(self, storage):
        db = FakeDB([file_row(7, version=EXTRACTOR_VERSION, source_sha='aa', status='ready')])
        assert TextIndexer(db, str(storage)).run(9) == {}
        assert db.names() == []


class TestResolve:
    def test_managed_files_are_under_the_storage_and_referenced_ones_under_their_root(self):
        assert resolve('/data/lib', 'managed', None, 'Autor/Obra/a.epub') == '/data/lib/Autor/Obra/a.epub'
        assert resolve('/data/lib', 'referenced', '/mnt/livros', 'x/a.epub') == '/mnt/livros/x/a.epub'

    def test_a_path_that_climbs_out_of_its_root_is_not_a_place(self):
        assert resolve('/data/lib', 'managed', None, '../etc/passwd') is None
        assert resolve('/data/lib', 'managed', None, 'a/../../etc/passwd') is None
        assert resolve('/data/lib', 'referenced', '/mnt/livros', '../../etc/passwd') is None
        assert resolve('/data/lib', 'referenced', None, 'a.epub') is None
        assert resolve('/data/lib', 'managed', None, '') is None


# ── running headers and footers ──────────────────────────────────────

def make_book_pdf(path, pages, header=None, footer=True, height=842):
    """pages: the body of each page. header(n) is what page n carries at the top (None for nothing);
    the page number is at the bottom. Blocks are placed by coordinates, so what is margin is known."""
    doc = fitz.open()
    for n, body in enumerate(pages):
        page = doc.new_page(width=595, height=height)
        top = header(n) if header else None
        if top:
            page.insert_text((60, 0.05 * height), top, fontsize=9)
        page.insert_textbox(fitz.Rect(60, 0.14 * height, 540, 0.85 * height), body, fontsize=11)
        if footer:
            page.insert_text((290, 0.95 * height), str(n + 1), fontsize=9)
    doc.save(str(path))
    doc.close()


def body(n):
    return f'Texto único da página {n} com o bastante para contar como conteúdo do livro e mais nada.'


def all_text(path, **kw):
    return ' '.join(s.text for s in pdf_segments(str(path), **kw))


class TestRunningHeaders:
    def test_the_title_the_chapter_and_the_page_number_repeated_on_every_page_are_not_text(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_book_pdf(path, [body(n) for n in range(20)], header=lambda n: f'{n + 1} Frank Herbert')
        text = all_text(path)
        assert 'Frank Herbert' not in text
        assert all(f'página {n}' in text for n in range(20))
        rest = text
        for n in range(20):
            rest = rest.replace(body(n), '')
        assert rest.strip() == ''  # nothing left over: no page number, no header

    def test_a_header_that_alternates_between_two_things_is_removed_as_well(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_book_pdf(path, [body(n) for n in range(20)], header=lambda n: 'Dune' if n % 2 else f'{n} Frank Herbert')
        text = all_text(path)
        assert 'Dune' not in text and 'Frank Herbert' not in text

    def test_garbage_after_the_header_words_does_not_hide_it(self, tmp_path):
        path = tmp_path / 'a.pdf'
        # the words after the first two are different on every page
        make_book_pdf(path, [body(n) for n in range(20)], header=lambda n: f'Dune {n} ' + ' '.join(f'x{(n * 3 + i) % 11}' for i in range(5)))
        text = all_text(path)
        assert 'Dune' not in text and 'x1' not in text and 'x7' not in text

    def test_what_only_one_page_has_in_the_margin_is_text(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_book_pdf(path, [body(n) for n in range(20)], header=lambda n: 'CAPÍTULO SETE' if n == 6 else 'Dune')
        text = all_text(path)
        assert 'CAPÍTULO SETE' in text and 'Dune' not in text

    def test_something_on_a_few_pages_only_is_not_a_running_header(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_book_pdf(path, [body(n) for n in range(40)], header=lambda n: 'Interlúdio secreto' if n < 3 else None)
        assert all_text(path).count('Interlúdio secreto') == 3

    def test_two_headers_that_only_begin_alike_are_not_the_same_header(self, tmp_path):
        path = tmp_path / 'a.pdf'
        doc = fitz.open()
        for n in range(20):
            page = doc.new_page(width=595, height=842)
            page.insert_text((60, 40), f'Dune {n}', fontsize=9)
            if n in (3, 4, 5):
                page.insert_text((60, 75), 'Dune wanderers', fontsize=9)
            page.insert_textbox(fitz.Rect(60, 120, 540, 700), body(n), fontsize=11)
        doc.save(str(path))
        doc.close()
        text = all_text(path)
        assert text.count('Dune wanderers') == 3 and 'Dune 1' not in text

    def test_a_short_document_has_too_few_pages_to_say_what_repeats(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_book_pdf(path, [body(n) for n in range(5)], header=lambda n: 'Dune')
        assert all_text(path).count('Dune') == 5

    def test_a_block_that_starts_in_the_margin_but_is_body_is_kept(self, tmp_path):
        path = tmp_path / 'a.pdf'
        doc = fitz.open()
        for n in range(20):
            page = doc.new_page(width=595, height=842)
            # one block from the top of the page down into its middle, beginning with the same words on every page
            page.insert_textbox(fitz.Rect(60, 30, 540, 500), f'Dune amanhece {n}. ' + ' '.join(body(n) for _ in range(6)), fontsize=11)
        doc.save(str(path))
        doc.close()
        assert all_text(path).count('Dune amanhece') == 20

    def test_a_page_with_nothing_but_a_header_and_a_number_is_empty(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_book_pdf(path, [body(n) if n != 9 else '' for n in range(20)], header=lambda n: 'Dune')
        assert 9 not in {s.locator['page'] for s in pdf_segments(str(path))}

    def test_the_bookmarks_and_the_pages_are_not_disturbed(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_book_pdf(path, [body(n) for n in range(20)], header=lambda n: 'Dune')
        segs = list(pdf_segments(str(path)))
        assert [s.locator['page'] for s in segs] == list(range(20))
