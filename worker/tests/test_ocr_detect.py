"""Detection of pages without a text layer, on the shared corpus."""
import os

import pytest

from analyzer import Analyzer
from ocr_detect import detect_text_layer
from tests.test_analyzer import FakeDB

CORPUS = os.path.join(os.path.dirname(__file__), '..', '..', 'testdata', 'corpus')


def corpus(name):
    return os.path.join(CORPUS, name)


class TestDetect:
    def test_a_digital_pdf_has_text_on_every_page(self):
        pages, missing = detect_text_layer(corpus('pdf_digital.pdf'))
        assert pages > 0 and missing == []

    def test_a_scanned_pdf_has_none(self):
        pages, missing = detect_text_layer(corpus('pdf_escaneado.pdf'))
        assert pages > 0 and missing == list(range(1, pages + 1))

    def test_a_mixed_pdf_reports_only_the_pages_that_need_it(self):
        pages, missing = detect_text_layer(corpus('pdf_misto.pdf'))
        assert 0 < len(missing) < pages
        assert all(1 <= n <= pages for n in missing)

    def test_the_threshold_decides_what_counts_as_text(self):
        pages, missing = detect_text_layer(corpus('pdf_digital.pdf'), min_chars=10**9)
        assert missing == list(range(1, pages + 1))


class TestStored:
    def test_records_the_pages_and_whether_ocr_is_needed(self):
        db = FakeDB()
        Analyzer(db).save_text_layer(7, 10, [3, 4])
        (query, params), = db.matching("INSERT INTO text_layers")
        assert params == (10, [3, 4], True, 7)
        assert "work_primary" in query and "ON CONFLICT (file_id)" in query

    def test_a_complete_text_layer_needs_no_ocr(self):
        db = FakeDB()
        Analyzer(db).save_text_layer(7, 10, [])
        assert db.matching("INSERT INTO text_layers")[0][1] == (10, [], False, 7)
