"""The text OCR finds becomes segments of the file like any other text (#24): of origin 'ocr', for the pages that have no
text layer and only for them, kept when the native text is read again."""
import os

import fitz
import pytest

import ocr
from ocr import OcrIndexer, Tesseract
from textindex import Limits
from textindex.pdf import pdf_segments
from textindex.store import TextIndexer
from tests.test_ocr import OcrDB, FakeEngine, scan_page
from tests.test_textindex import FakeDB, file_row

NATIVE = 'Texto de uma página nativa, com o bastante para contar como texto do arquivo.'
RECOGNISED = 'Primeiro parágrafo reconhecido pelo motor de OCR, longo o bastante.\n\nSegundo parágrafo com a palavra constan-\ntinopla quebrada no fim da linha.'


def mixed_pdf(path, pages):
    """A PDF whose pages are 'text' (a text layer) or 'scan' (a picture only)."""
    doc = fitz.open()
    for kind in pages:
        page = doc.new_page(width=300, height=300)
        if kind == 'text':
            page.insert_text((20, 60), NATIVE, fontsize=9)
        else:
            pix = fitz.Pixmap(fitz.csGRAY, fitz.IRect(0, 0, 80, 80), False)
            pix.set_rect(pix.irect, (255,))
            page.insert_image(page.rect, pixmap=pix)
    doc.save(str(path))
    doc.close()
    return str(path)


class TestPdfReaderWithRecognisedPages:
    def segments(self, path, ocr_text=None):
        return list(pdf_segments(path, ocr=ocr_text))

    def test_a_scan_with_no_recognised_text_yields_nothing_as_before(self, tmp_path):
        got = self.segments(mixed_pdf(tmp_path / 'a.pdf', ['text', 'scan']))
        assert [(s.locator['page'], s.origin) for s in got] == [(0, 'native')]

    def test_the_pages_without_a_text_layer_are_read_from_what_OCR_found(self, tmp_path):
        got = self.segments(mixed_pdf(tmp_path / 'a.pdf', ['text', 'scan', 'text']), {1: RECOGNISED})
        assert [(s.locator, s.origin) for s in got] == [
            ({'type': 'pdf', 'page': 0}, 'native'), ({'type': 'pdf', 'page': 1}, 'ocr'), ({'type': 'pdf', 'page': 2}, 'native')]
        assert 'Primeiro parágrafo reconhecido' in got[1].text
        assert 'constantinopla' in got[1].text and 'constan-' not in got[1].text  # the hyphen at the end of a line is joined
        assert '\n' in got[1].text  # and the paragraphs stay apart

    def test_a_page_with_a_text_layer_is_always_read_from_it(self, tmp_path):
        got = self.segments(mixed_pdf(tmp_path / 'a.pdf', ['text']), {0: RECOGNISED})
        assert [s.origin for s in got] == ['native'] and 'nativa' in got[0].text

    def test_what_OCR_found_too_little_of_is_not_text(self, tmp_path):
        got = self.segments(mixed_pdf(tmp_path / 'a.pdf', ['scan', 'scan']), {0: ' 12 ', 1: '\n\n  \n'})
        assert got == []

    def test_a_scanned_book_read_only_by_OCR_is_all_of_origin_ocr_in_page_order(self, tmp_path):
        got = self.segments(mixed_pdf(tmp_path / 'a.pdf', ['scan', 'scan', 'scan']), {2: RECOGNISED, 0: RECOGNISED})
        assert [(s.locator['page'], s.origin) for s in got] == [(0, 'ocr'), (2, 'ocr')]

    def test_what_the_engine_wrote_is_cleaned_like_any_text(self, tmp_path):
        dirty = 'Um   parágrafo\x07 com espaços    estranhos e caracteres de controle,\ne uma linha quebrada.\u0000'
        got = self.segments(mixed_pdf(tmp_path / 'a.pdf', ['scan']), {0: dirty})
        assert got[0].text == 'Um parágrafo com espaços estranhos e caracteres de controle, e uma linha quebrada.'

    def test_too_much_recognised_text_in_one_file_is_refused_like_native_text(self, tmp_path, monkeypatch):
        monkeypatch.setattr(Limits, 'MAX_CHARS', 100)
        with pytest.raises(ValueError, match='too much text'):
            self.segments(mixed_pdf(tmp_path / 'a.pdf', ['scan']), {0: RECOGNISED})


class RecognisedDB(OcrDB):
    """The OCR database, answering the question of the indexer about the pages that were read from what was saved."""

    def fetchall(self, query, params=None):
        if 'SELECT page, text FROM ocr_pages' in query:
            return [(page, row['text']) for page, row in self.saved.items() if row['state'] == 'done']
        return super().fetchall(query, params)


class TestPublishing:
    def publish(self, tmp_path, pages, ocr_rows):
        mixed_pdf(tmp_path / 'a.pdf', pages)
        db = FakeDB([file_row(7, 'pdf', 'aa', 'pt', path='a.pdf')], ocr=ocr_rows)
        out = TextIndexer(db, str(tmp_path)).run(9)
        call = [c for c in db.calls if c[1] == 'text_extraction_publish'][0][2]
        return out, call, db

    def test_text_that_is_only_native_is_published_as_native(self, tmp_path):
        out, call, db = self.publish(tmp_path, ['text'], [])
        assert out == {7: 'ready'} and call[5] == 'native'

    def test_native_pages_and_recognised_ones_make_a_mixed_text(self, tmp_path):
        out, call, db = self.publish(tmp_path, ['text', 'scan'], [(1, RECOGNISED)])
        assert out == {7: 'ready'} and call[5] == 'mixed'
        assert [r[3] for r in db.rows] == ['native', 'ocr']  # each segment says where it comes from

    def test_a_book_that_is_all_scan_is_published_as_ocr(self, tmp_path):
        out, call, db = self.publish(tmp_path, ['scan', 'scan'], [(0, RECOGNISED), (1, RECOGNISED)])
        assert out == {7: 'ready'} and call[5] == 'ocr'

    def test_a_scan_nobody_has_read_yet_is_empty_and_native(self, tmp_path):
        out, call, db = self.publish(tmp_path, ['scan'], [])
        assert out == {7: 'empty'} and call[5] == 'native' and db.rows == []

    def test_the_pages_asked_for_are_those_of_this_file_as_it_is(self, tmp_path):
        out, call, db = self.publish(tmp_path, ['scan'], [(0, RECOGNISED)])
        assert [c[2] for c in db.calls if c[1] == 'ocr_pages'] == [(7, 'aa')]

    def test_only_a_pdf_asks_for_recognised_pages(self, tmp_path):
        (tmp_path / 'n.txt').write_text('Um texto simples, com o bastante para contar como texto do arquivo.')
        db = FakeDB([file_row(7, 'txt', path='n.txt')])
        TextIndexer(db, str(tmp_path)).run(9)
        assert [c for c in db.calls if c[1] == 'ocr_pages'] == []


@pytest.mark.skipif(not Tesseract().installed(), reason='the OCR engine (tesseract) is not installed')
class TestWithTheRealEngine:
    """The engine itself, on pages made here: text drawn, turned into a picture, and read back."""
    TEXT = 'A corrida de ontem foi longa e as informações importantes chegaram pelo correio naquela semana.'

    @staticmethod
    def picture_of_text(text, size=14):
        doc = fitz.open()
        page = doc.new_page(width=612, height=200)
        page.insert_textbox(fitz.Rect(30, 30, 580, 180), text, fontsize=size, fontname='helv')
        pix = page.get_pixmap(dpi=200, colorspace=fitz.csGRAY)
        doc.close()
        scan = fitz.open()
        scan_page_ = scan.new_page(width=612, height=200)
        scan_page_.insert_image(scan_page_.rect, pixmap=pix)
        return scan

    def engine(self):
        engine = Tesseract()
        if not {'por', 'eng'} & set(engine.languages()):
            pytest.skip('the engine has neither Portuguese nor English')
        return engine

    def test_a_page_that_is_a_picture_of_text_is_read(self):
        engine = self.engine()
        language = 'por' if 'por' in engine.languages() else 'eng'
        scan = self.picture_of_text(self.TEXT if language == 'por' else 'The race of yesterday was long and the important news came by mail that week.')
        indexer = OcrIndexer(OcrDB([]), '/nowhere', engine)
        state, text, error, dpi = indexer.read_page(scan, 0, language)
        assert state == 'done' and error is None and dpi in range(ocr.MIN_DPI, ocr.MAX_DPI + 1)
        words = text.lower().split()
        wanted = ['corrida', 'ontem', 'longa', 'importantes', 'correio', 'semana'] if language == 'por' else ['race', 'yesterday', 'long', 'important', 'mail', 'week']
        assert sum(1 for w in wanted if any(w in word for word in words)) >= len(wanted) - 1, text

    def test_a_blank_page_is_blank(self):
        engine = self.engine()
        scan = fitz.open()
        scan.new_page(width=300, height=300)
        state, text, error, dpi = OcrIndexer(OcrDB([]), '/nowhere', engine).read_page(scan, 0, engine.languages()[0])
        assert (state, text, error) == ('blank', '', None)

    def test_a_language_the_engine_has_not_got_fails_the_page_not_the_job(self):
        engine = self.engine()
        scan = self.picture_of_text(self.TEXT)
        state, text, error, dpi = OcrIndexer(OcrDB([]), '/nowhere', engine).read_page(scan, 0, 'xyzzy')
        assert state == 'failed' and text == '' and error

    def test_a_whole_scanned_file_goes_from_pictures_to_searchable_segments(self, tmp_path):
        engine = self.engine()
        language = 'por' if 'por' in engine.languages() else 'eng'
        text = self.TEXT if language == 'por' else 'The race of yesterday was long and the important news came by mail that week.'
        scan = self.picture_of_text(text)
        scan.save(str(tmp_path / 'scan.pdf'))
        db = RecognisedDB([], without=[1])
        db.files = [file_row(7, 'pdf', 'aa', language[:2] if language == 'por' else 'en', path='scan.pdf')]
        out = OcrIndexer(db, str(tmp_path), engine, log=lambda *a: None).run(9)
        assert out == {7: {'read': 1, 'failed': 0}}
        publish = [c for c in db.calls if c[1] == 'text_extraction_publish'][0][2]
        assert publish[4] == 'ready' and publish[5] == 'ocr'
        assert [r[3] for r in db.rows] == ['ocr'] and ('corrida' if language == 'por' else 'race') in db.rows[0][5].lower()
