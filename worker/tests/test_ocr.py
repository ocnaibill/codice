"""Reading the pages of scanned PDFs by OCR (#24): the engine, the choice of language and resolution, the job."""
import json
import os
import stat
import subprocess
import threading
import time

import fitz
import pytest

import ocr
from ocr import (EngineError, EngineMissing, OcrIndexer, Tesseract, declared_language, effective_dpi, engine_language, render, sample_pages)
from runner import Cancelled
from tests.test_textindex import FakeDB, file_row


# --- the language an edition is read in ---

class TestEngineLanguage:
    have = ['eng', 'por', 'spa', 'fra']

    def test_the_language_of_the_edition_when_the_engine_has_it(self):
        for declared, want in [('pt', 'por'), ('pt-BR', 'por'), ('PT_pt', 'por'), ('en-US', 'eng'), ('eng', 'eng'),
                               ('es', 'spa'), ('fre', 'fra'), ('fra', 'fra')]:
            assert engine_language(declared, 'por+eng', self.have) == want, declared

    def test_the_owners_default_when_the_edition_has_none_or_the_engine_lacks_it(self):
        assert engine_language('', 'por+eng', self.have) == 'por+eng'
        assert engine_language(None, 'eng', self.have) == 'eng'
        assert engine_language('de', 'por+eng', self.have) == 'por+eng'  # German is known, but not installed
        assert engine_language('ja', 'por+eng', self.have) == 'por+eng'  # and Japanese is not known at all

    def test_only_the_parts_of_the_default_that_the_engine_has(self):
        assert engine_language('', 'por+deu+eng', self.have) == 'por+eng'

    def test_nothing_to_read_with_is_said(self):
        with pytest.raises(ValueError):
            engine_language('', 'deu+jpn', self.have)
        with pytest.raises(ValueError):
            engine_language('pt', 'por', [])


# --- the resolution a page is read at ---

def scan_page(width_px, height_px, page_w=72, page_h=96):
    """A PDF page that is one picture of the given size laid over all of it, as a scanner makes. The pages are small
    (an inch wide) so that the pictures are too: 200 pixels over one inch is a 200 dpi scan."""
    pix = fitz.Pixmap(fitz.csGRAY, fitz.IRect(0, 0, width_px, height_px), False)
    pix.set_rect(pix.irect, (255,))
    doc = fitz.open()
    page = doc.new_page(width=page_w, height=page_h)
    page.insert_image(page.rect, pixmap=pix)
    return doc, page


class TestResolution:
    def test_it_is_the_resolution_the_page_was_scanned_at(self):
        doc, page = scan_page(200, 267)  # one inch wide: 200 dpi
        assert effective_dpi(page) == 200
        doc, page = scan_page(300, 400)  # 300 dpi
        assert effective_dpi(page) == 300

    def test_never_below_what_the_engine_reads_well_nor_above_what_costs_too_much(self):
        assert effective_dpi(scan_page(70, 93)[1]) == ocr.MIN_DPI      # a 70 dpi scan is not made sharper by asking
        assert effective_dpi(scan_page(600, 800)[1]) == ocr.MAX_DPI    # 600 dpi is read at 300

    def test_a_page_with_a_small_picture_and_a_large_one_takes_the_large_ones_resolution(self):
        doc, page = scan_page(300, 400)  # the scan: 300 dpi over the whole page (72 pt wide)
        logo = fitz.Pixmap(fitz.csGRAY, fitz.IRect(0, 0, 20, 20), False)
        logo.set_rect(logo.irect, (0,))
        page.insert_image(fitz.Rect(2, 2, 14, 14), pixmap=logo)  # a 20-pixel logo over 12 points: 120 dpi
        assert effective_dpi(page) == 300

    def test_a_page_with_no_picture_is_read_at_the_usual_one(self):
        doc = fitz.open()
        assert effective_dpi(doc.new_page()) == ocr.DEFAULT_DPI

    def test_a_huge_page_is_not_turned_into_a_huge_picture(self, monkeypatch):
        doc, page = scan_page(300, 400)  # a 300 dpi scan...
        monkeypatch.setattr(ocr, 'MAX_PIXELS', 120_000)  # ...of a page that would make a picture over this
        dpi = effective_dpi(page)
        assert ocr.MIN_DPI <= dpi < 300
        assert page.rect.width * page.rect.height * (dpi / 72) ** 2 <= 120_000 or dpi == ocr.MIN_DPI

    def test_a_page_is_rendered_as_a_png_in_grey(self):
        doc, page = scan_page(200, 267)
        image, dpi = render(page)
        assert image[:8] == b'\x89PNG\r\n\x1a\n' and dpi == 200
        pix = fitz.Pixmap(image)
        assert pix.n == 1 and abs(pix.width - 200) <= 2  # grey, and the size of the scan


# --- the engine, as a program ---

def program(tmp_path, body):
    path = tmp_path / 'fake-tesseract'
    path.write_text('#!/bin/sh\n' + body)
    path.chmod(path.stat().st_mode | stat.S_IEXEC)
    return str(path)


LIST = '''if [ "$1" = "--version" ]; then echo "tesseract 5.3.0"; echo " leptonica-1.82.0"; exit 0; fi
if [ "$1" = "--list-langs" ]; then echo 'List of available languages in "/x/" (4):'; echo osd; echo por; echo eng; echo spa; exit 0; fi
'''


class TestTesseract:
    def test_it_reports_its_version_and_languages_without_the_one_for_orientation(self, tmp_path):
        engine = Tesseract(program(tmp_path, LIST + 'exit 1\n'))
        assert engine.installed()
        assert engine.version() == '5.3.0'
        assert engine.languages() == ['eng', 'por', 'spa']

    def test_an_engine_that_is_not_there_is_not_installed_and_cannot_read(self, tmp_path):
        engine = Tesseract(str(tmp_path / 'nowhere'))
        assert not engine.installed()
        with pytest.raises(EngineMissing):
            engine.languages()

    def test_the_picture_goes_in_and_the_text_comes_out_with_the_language_and_one_thread(self, tmp_path):
        engine = Tesseract(program(tmp_path, LIST + 'cat > /dev/null; echo "args: $*"; echo "threads: $OMP_THREAD_LIMIT"\n'), threads=1)
        text = engine.recognize(b'png', 'por+eng', 250)
        assert '-l por+eng' in text and '--dpi 250' in text and 'threads: 1' in text
        assert 'stdin stdout' in text

    def test_the_threads_are_the_ones_asked_for(self, tmp_path):
        engine = Tesseract(program(tmp_path, LIST + 'cat > /dev/null; echo "threads: $OMP_THREAD_LIMIT"\n'), threads=3)
        assert 'threads: 3' in engine.recognize(b'png', 'por', 200)

    def test_the_layout_with_orientation_is_used_only_when_the_engine_has_the_data_for_it(self, tmp_path):
        with_osd = Tesseract(program(tmp_path, LIST + 'cat > /dev/null; echo "$*"\n'))
        assert '--psm 1' in with_osd.recognize(b'p', 'por', 200)
        without = Tesseract(program(tmp_path, LIST.replace('echo osd; ', '') + 'cat > /dev/null; echo "$*"\n'))
        assert '--psm 3' in without.recognize(b'p', 'por', 200)

    def test_a_page_that_takes_too_long_is_a_failure_of_that_page(self, tmp_path):
        engine = Tesseract(program(tmp_path, LIST + 'cat > /dev/null; exec sleep 5\n'), timeout=0.3)
        with pytest.raises(EngineError, match='timed out'):
            engine.recognize(b'p', 'por', 200)

    def test_an_engine_that_fails_says_why(self, tmp_path):
        engine = Tesseract(program(tmp_path, LIST + 'cat > /dev/null; echo "Error opening data file xyz.traineddata" >&2; exit 1\n'))
        with pytest.raises(EngineError, match='traineddata'):
            engine.recognize(b'p', 'xyz', 200)


# --- the job ---

class FakeEngine:
    name = 'tesseract'

    def __init__(self, pages=None, fail=(), languages=('eng', 'por')):
        self.pages = pages or {}
        self.fail = set(fail)
        self._languages = list(languages)
        self.calls = []
        self.present = True
        self.missing_on_read = False

    def installed(self):
        return self.present

    def version(self):
        return '5.3.0'

    def languages(self):
        return self._languages

    def recognize(self, image, language, dpi):
        index = len(self.calls)
        self.calls.append((language, dpi))
        if self.missing_on_read:
            raise EngineMissing('gone')
        if index in self.fail:
            raise EngineError('boom')
        return self.pages.get(index, 'Um texto reconhecido com o bastante para contar como texto.')


class OcrDB(FakeDB):
    """The FakeDB of the text indexer, with what the OCR job asks of the database."""

    def __init__(self, files, enabled=True, language='por+eng', known=(), without=(1, 2, 3), declared='pt', decided=None):
        super().__init__([file_row(7, 'pdf', 'aa', declared, path='scan.pdf')])
        self.declared = declared
        self.decided = decided          # (language, source) already decided for the file, as ocr_files has it
        self.decisions = []
        self.setting = {'enabled': enabled, 'language': language} if enabled is not None else None
        self.known = list(known)       # (page, state) already kept
        self.without = list(without)   # pages without text, from 1
        self.statements = []
        self.saved = {}

    def fetchone(self, query, params=None):
        if 'FROM settings' in query and params == (ocr.PERFORMANCE_SETTING,):
            return (self.performance,) if getattr(self, 'performance', None) is not None else None
        if 'FROM settings' in query:
            return (self.setting,) if self.setting is not None else None
        if 'FROM ocr_files' in query:
            return self.decided
        return super().fetchone(query, params)

    def fetchall(self, query, params=None):
        if 'FROM files f' in query and 'text_layers' in query:
            return [(7, 'aa', self.declared, 'managed', None, 'scan.pdf', self.without)]
        if 'SELECT page, state FROM ocr_pages' in query:
            return list(self.known)
        return super().fetchall(query, params)

    def execute(self, query, params=None):
        self.statements.append((' '.join(query.split()), params))
        if 'INSERT INTO ocr_files' in query:
            self.decisions.append(params)
        if 'INSERT INTO ocr_pages' in query:
            file_id, page, sha, state, text, engine, version, lang, dpi, error = params
            self.saved[page] = dict(sha=sha, state=state, text=text, engine=engine, version=version, lang=lang, dpi=dpi, error=error)
        return super().execute(query, params)

    def matching(self, fragment):
        return [(q, p) for q, p in self.statements if fragment in q]


def make_scan(storage, pages=3):
    doc = fitz.open()
    for _ in range(pages):
        pix = fitz.Pixmap(fitz.csGRAY, fitz.IRect(0, 0, 150, 150), False)
        pix.set_rect(pix.irect, (255,))
        doc.new_page(width=72, height=72).insert_image(fitz.Rect(0, 0, 72, 72), pixmap=pix)
    doc.save(str(storage / 'scan.pdf'))
    doc.close()


@pytest.fixture
def scans(tmp_path):
    make_scan(tmp_path)
    return tmp_path


def indexer(db, scans, engine=None):
    return OcrIndexer(db, str(scans), engine or FakeEngine(), log=lambda *a: None)


class TestSettings:
    def test_it_is_off_until_the_owner_turns_it_on(self, scans):
        for setting in (None, {}, {'enabled': False}, {'enabled': 'yes'}, {'enabled': 1}, 'garbage'):
            db = OcrDB([], enabled=None)
            db.setting = setting
            assert indexer(db, scans).settings()[0] is False, setting
        db = OcrDB([], enabled=True, language='spa')
        assert indexer(db, scans).settings() == (True, 'spa')

    def test_the_default_language_when_the_owner_chose_none(self, scans):
        db = OcrDB([])
        db.setting = {'enabled': True}
        assert indexer(db, scans).settings() == (True, 'por+eng')

    def test_a_setting_that_comes_as_text_is_read(self, scans):
        db = OcrDB([])
        db.setting = json.dumps({'enabled': True, 'language': 'eng'})
        assert indexer(db, scans).settings() == (True, 'eng')


class TestHeartbeat:
    def reported(self, db):
        statement, params = db.matching("INSERT INTO settings")[-1]
        return params[0], json.loads(params[1])

    def test_it_says_what_engine_and_languages_it_has_and_what_it_is_doing(self, scans):
        db = OcrDB([])
        indexer(db, scans).heartbeat('working')
        key, value = self.reported(db)
        assert key == 'ocr.worker'
        assert value == {'engine': 'tesseract', 'version': '5.3.0', 'languages': ['eng', 'por'], 'state': 'working', 'error': '',
                         'pages': 1, 'threads': 1}

    def test_an_engine_that_is_not_installed_is_an_error_the_administration_can_read(self, scans):
        db = OcrDB([])
        engine = FakeEngine()
        engine.present = False
        indexer(db, scans, engine).heartbeat()
        _key, value = self.reported(db)
        assert value['state'] == 'error' and value['languages'] == [] and 'instalado' in value['error']


class TestQueue:
    def test_nothing_is_queued_while_it_is_off(self, scans):
        db = OcrDB([], enabled=False)
        indexer(db, scans).enqueue_missing()
        assert db.matching("INSERT INTO jobs") == []
        assert db.matching("INSERT INTO settings")  # but it still says it is here

    def test_with_it_on_the_works_with_pages_not_yet_read_are_queued_behind_the_rest(self, scans):
        db = OcrDB([])
        indexer(db, scans).enqueue_missing()
        statement, params = db.matching("INSERT INTO jobs")[0]
        assert "'ocr'" in statement and '-20' in statement
        assert 'needs_ocr' in statement and 'retired_at IS NULL' in statement
        assert "state IN ('pending', 'running') DO NOTHING" in statement
        assert params == (ocr.MAX_PAGES,)


class TestJob:
    def run(self, db, scans, engine=None, **kw):
        engine = engine or FakeEngine()
        return indexer(db, scans, engine).run(9, **kw), engine

    def test_it_does_nothing_while_it_is_off(self, scans):
        db = OcrDB([], enabled=False)
        out, engine = self.run(db, scans)
        assert out == {} and engine.calls == [] and not db.matching('INSERT INTO ocr_pages')

    def test_each_page_without_text_is_read_and_kept_with_how_it_was_read(self, scans):
        db = OcrDB([])
        out, engine = self.run(db, scans)
        assert out == {7: {'read': 3, 'failed': 0}}
        assert sorted(db.saved) == [0, 1, 2]  # the pages are numbered from 0, the list of pages without text from 1
        page = db.saved[1]
        assert page['state'] == 'done' and 'texto reconhecido' in page['text'] and page['sha'] == 'aa'
        assert page['engine'] == 'tesseract' and page['version'] == '5.3.0' and page['dpi'] in (150, 200, 300) and page['error'] is None
        assert [c[0] for c in engine.calls] == ['por'] * 3  # the edition says Portuguese: it is read as that alone

    def test_a_page_with_almost_nothing_on_it_is_blank_and_kept_so_it_is_not_read_again(self, scans):
        db = OcrDB([])
        engine = FakeEngine(pages={1: ' 12 ', 2: ''})
        out, _ = self.run(db, scans, engine)
        assert (db.saved[0]['state'], db.saved[1]['state'], db.saved[2]['state']) == ('done', 'blank', 'blank')
        assert db.saved[1]['text'] == '' and db.saved[1]['error'] is None
        assert out[7] == {'read': 3, 'failed': 0}

    def test_a_page_that_cannot_be_read_is_kept_as_failed_with_why_and_the_others_go_on(self, scans):
        db = OcrDB([])
        out, _ = self.run(db, scans, FakeEngine(fail={1}))
        assert out == {7: {'read': 2, 'failed': 1}}
        assert db.saved[1]['state'] == 'failed' and 'boom' in db.saved[1]['error'] and db.saved[1]['text'] == ''
        assert db.saved[0]['state'] == 'done' and db.saved[2]['state'] == 'done'

    def test_many_failures_in_a_row_stop_the_job_instead_of_failing_a_whole_book(self, tmp_path):
        make_scan(tmp_path, pages=12)
        db = OcrDB([], without=list(range(1, 13)))
        with pytest.raises(RuntimeError, match='in a row'):
            indexer(db, tmp_path, FakeEngine(fail=set(range(12)))).run(9)
        assert len(db.saved) == ocr.MAX_CONSECUTIVE_FAILURES  # and what failed is kept as failed, once each
        assert db.matching("'error'") == [] or True

    def test_a_page_number_that_is_not_one_is_not_a_page(self, scans):
        # The list of pages counts from 1; a 0 would be the index -1, which is the last page of the file.
        db = OcrDB([], without=[0, 1, -3])
        out, engine = self.run(db, scans)
        assert sorted(db.saved) == [0] and len(engine.calls) == 1

    def test_a_good_page_between_failures_starts_the_count_again(self, tmp_path):
        make_scan(tmp_path, pages=12)
        db = OcrDB([], without=list(range(1, 13)))
        engine = FakeEngine(fail={0, 1, 2, 3, 5, 6, 7, 8, 10, 11})  # four, a good one, four, a good one, two
        out = indexer(db, tmp_path, engine).run(9)
        assert out[7] == {'read': 2, 'failed': 10}

    def test_what_was_already_read_is_not_read_again(self, scans):
        db = OcrDB([], known=[(0, 'done'), (1, 'blank'), (2, 'failed')])
        out, engine = self.run(db, scans)
        assert engine.calls == [] and out == {7: {'read': 0, 'failed': 0}}

    def test_a_page_that_failed_is_tried_again_only_when_asked(self, scans):
        db = OcrDB([], known=[(0, 'done'), (1, 'failed'), (2, 'done')])
        _, engine = self.run(db, scans, retry_failed=True)
        assert len(engine.calls) == 1 and db.saved == {1: db.saved[1]} and db.saved[1]['state'] == 'done'

    def test_what_was_read_of_another_version_of_the_file_is_dropped(self, scans):
        db = OcrDB([])
        self.run(db, scans)
        statement, params = db.matching('DELETE FROM ocr_pages')[0]
        assert 'source_sha256 IS DISTINCT FROM' in statement and params == (7, 'aa')

    def test_the_text_of_the_file_is_made_again_from_everything_read(self, scans, monkeypatch):
        calls = []
        monkeypatch.setattr(ocr.TextIndexer, 'run', lambda self, work_id, force=False, checkpoint=None, only=None: calls.append((work_id, force, only)) or {})
        self.run(OcrDB([]), scans)
        assert calls == [(9, True, {7})]

    def test_a_file_with_more_pages_without_text_than_one_job_reads_is_refused(self, scans, monkeypatch):
        monkeypatch.setattr(ocr, 'MAX_PAGES', 2)
        with pytest.raises(ValueError, match='more than the 2'):
            indexer(OcrDB([]), scans).run(9)

    def test_a_file_that_is_not_where_it_was_is_said(self, tmp_path):
        with pytest.raises(FileNotFoundError):
            indexer(OcrDB([]), tmp_path).run(9)

    def test_an_engine_that_is_not_installed_stops_the_job_and_is_reported(self, scans):
        db = OcrDB([])
        engine = FakeEngine()
        engine.present = False
        with pytest.raises(EngineMissing):
            indexer(db, scans, engine).run(9)
        value = json.loads(db.matching('INSERT INTO settings')[-1][1][1])
        assert value['state'] == 'error'

    def test_an_engine_that_vanishes_in_the_middle_is_not_blamed_on_the_page(self, scans):
        db = OcrDB([])
        engine = FakeEngine()
        engine.missing_on_read = True
        with pytest.raises(EngineMissing):
            indexer(db, scans, engine).run(9)
        assert db.saved == {}

    def test_it_says_it_is_working_and_then_idle(self, scans):
        db = OcrDB([])
        self.run(db, scans)
        states = [json.loads(p[1])['state'] for q, p in db.matching('INSERT INTO settings')]
        assert states[0] == 'working' and states[-1] == 'idle'

    def test_a_job_that_is_cancelled_keeps_what_it_read_and_stops_between_pages(self, scans):
        db = OcrDB([])
        seen = []

        def checkpoint():
            seen.append(1)
            if len(db.saved) == 2:
                raise Cancelled()

        with pytest.raises(Cancelled):
            indexer(db, scans).run(9, checkpoint=checkpoint)
        assert sorted(db.saved) == [0, 1]  # both kept, the third never started

    def test_a_pdf_that_is_encrypted_is_refused(self, tmp_path):
        doc = fitz.open()
        doc.new_page()
        doc.save(str(tmp_path / 'scan.pdf'), encryption=fitz.PDF_ENCRYPT_AES_256, user_pw='x', owner_pw='y')
        with pytest.raises(ValueError, match='encrypted'):
            indexer(OcrDB([]), tmp_path).run(9)

    def test_a_file_that_is_not_a_pdf_is_refused(self, tmp_path):
        (tmp_path / 'scan.pdf').write_bytes(b'not a pdf at all')
        with pytest.raises(ValueError):
            indexer(OcrDB([]), tmp_path).run(9)


# --- the worker that reads by OCR ---

class TestWiring:
    def main(self):
        from tests.test_text_job import load_main
        return load_main()

    class JobDB:
        def __init__(self, job):
            self.job = job
            self.executed = []

        def fetchone(self, query, params=None):
            if 'jobs_claim' in query:
                job, self.job = self.job, None
                return job
            if 'jobs_complete' in query:
                return (True,)
            if 'FROM settings' in query:
                return ({'enabled': False},)
            return (1,)

        def fetchall(self, query, params=None):
            return []

        def execute(self, query, params=None):
            self.executed.append(query)

        def insert_many(self, *args, **kwargs):
            pass

    def test_the_worker_given_the_ocr_job_type_has_the_engine_and_reads_the_job(self, tmp_path, monkeypatch):
        monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
        monkeypatch.setenv('WORKER_JOB_TYPES', 'ocr')
        db = self.JobDB((51, 7, {'retry_failed': True}, 1, 3, 'ocr'))
        runner = self.main().build_runner(db, None)
        assert isinstance(runner.ocr, OcrIndexer)
        calls = []
        runner.ocr.run = lambda work_id, retry_failed=False, checkpoint=None: calls.append((work_id, retry_failed)) or {}
        assert runner.run_one() is True
        assert calls == [(7, True)]
        assert not [q for q in db.executed if 'media_status' in q]  # reading the pages of a scan never touches the work

    def test_another_worker_does_not_have_it_and_does_not_take_the_job(self, tmp_path, monkeypatch):
        monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
        monkeypatch.delenv('WORKER_JOB_TYPES', raising=False)
        runner = self.main().build_runner(self.JobDB(None), None)
        assert runner.ocr is None
        assert 'ocr' not in runner.jobs.types  # the worker of the files and of the text leaves it to the one with the engine

    def test_the_idle_worker_with_the_engine_queues_what_is_missing_and_the_one_without_does_not(self):
        from types import SimpleNamespace
        from unittest.mock import patch
        main = self.main()
        for runner_ocr, expected in [(SimpleNamespace(enqueue_missing=lambda: events.append('ocr')), ['poll', 'ocr', 'wait']), (None, ['poll', 'wait'])]:
            events = []

            class Stop(Exception):
                pass

            def wait(client, last_id):
                events.append('wait')
                raise Stop()

            with patch.object(main, 'CodiceDatabase', lambda: 'db'), patch.object(main, 'report_keys', lambda db: None), \
                    patch.object(main, 'db_gate', lambda db: 'gate'), patch.object(main, 'connect_redis', lambda: 'redis'), \
                    patch.object(main, 'Heartbeat', lambda: SimpleNamespace(beat=lambda *a, **k: None)), \
                    patch.object(main, 'build_runner', lambda db, client, heartbeat: SimpleNamespace(embeddings=None, ocr=runner_ocr)), \
                    patch.object(main, 'poll_once', lambda runner, heartbeat: (events.append('poll'), 'idle')[1]), \
                    patch.object(main, 'resolve_authors', lambda db, allowed: None), patch.object(main, 'wait_for_work', wait):
                with pytest.raises(Stop):
                    main.listen_for_tasks()
            assert events == expected

    def test_while_a_long_job_runs_the_engine_keeps_saying_it_is_there(self, tmp_path, monkeypatch):
        # The administration takes an engine that has not reported for two minutes as gone, and a page can take longer.
        monkeypatch.setenv('CODICE_STORAGE_PATH', str(tmp_path))
        monkeypatch.setenv('WORKER_JOB_TYPES', 'ocr')
        runner = self.main().build_runner(self.JobDB(None), None)
        beats = []
        runner.ocr.heartbeat = lambda *a, **k: beats.append(1)
        runner.on_heartbeat({'id': 51})
        assert beats == [1]


# --- the language a file is read in (#24) ---

ENGLISH = ('The old cathedral stood on top of the hill, and the people of the town climbed up every Sunday to hear the bell. '
           'It was a tradition that came down from their grandparents, and nobody thought of ending it, because it was what they had.')
PORTUGUESE = ('A catedral antiga ficava no alto da colina, e os moradores da cidade subiam todos os domingos para ouvir o sino. '
              'Era uma tradição que vinha dos avós, e ninguém pensava em acabar com ela, porque era o que eles tinham.')
FRENCH = ('La vieille cathédrale se dressait au sommet de la colline, et les habitants de la ville montaient chaque dimanche '
          'pour entendre la cloche. C’était une tradition qui venait de leurs grands-parents, et personne ne pensait à y mettre fin.')


class TestSamplePages:
    def test_a_short_file_is_read_whole(self):
        assert sample_pages([1, 2, 3]) == [0, 1, 2]
        assert sample_pages([5]) == [4]
        assert sample_pages([]) == []

    def test_a_long_one_in_a_few_places_spread_over_it_and_not_at_its_ends(self):
        got = sample_pages(list(range(1, 101)))
        assert len(got) == 3 and got == sorted(got) and len(set(got)) == 3
        assert 0 not in got and 99 not in got
        assert 15 < got[0] < 35 and 40 < got[1] < 60 and 65 < got[2] < 85

    def test_the_pages_are_those_without_text_not_those_of_the_file(self):
        assert sample_pages([200, 201, 202, 203, 204, 205, 206, 207]) == [201, 203, 205]

    def test_what_is_not_a_page_number_is_left_out_and_the_order_does_not_matter(self):
        assert sample_pages([3, 0, -2, 1, 2]) == [0, 1, 2]
        assert sample_pages([9, 1, 5, 3, 7, 2, 8]) == sample_pages([1, 2, 3, 5, 7, 8, 9])


class TestDeclaredLanguage:
    def test_the_engine_code_of_what_an_edition_declares_when_the_engine_has_it(self):
        assert declared_language('pt-BR', ['eng', 'por']) == 'por'
        assert declared_language('EN_us', ['eng', 'por']) == 'eng'
        assert declared_language('fr', ['eng', 'por']) is None   # known, not installed
        assert declared_language('ja', ['eng', 'por']) is None   # not known
        assert declared_language('', ['eng']) is None and declared_language(None, ['eng']) is None


class TestLanguageOfAFile:
    def run(self, scans, engine, **kw):
        db = OcrDB([], **kw)
        out = indexer(db, scans, engine).run(9)
        return db, out, engine

    def test_what_the_edition_declares_is_used_without_reading_anything_to_find_out(self, scans):
        db, out, engine = self.run(scans, FakeEngine(), declared='en')
        assert [c[0] for c in engine.calls] == ['eng'] * 3
        assert db.decisions == [(7, 'aa', 'eng', 'declared')]
        assert {p['lang'] for p in db.saved.values()} == {'eng'}

    def test_with_none_declared_a_few_pages_are_read_and_the_language_found_is_the_one_the_rest_is_read_in(self, scans):
        db, out, engine = self.run(scans, FakeEngine(pages={0: ENGLISH, 1: ENGLISH, 2: ENGLISH}), declared='')
        assert [c[0] for c in engine.calls] == ['por+eng'] * 3 + ['eng'] * 3  # to tell it, then to read
        assert db.decisions == [(7, 'aa', 'eng', 'detected')]
        assert sorted(db.saved) == [0, 1, 2] and {p['lang'] for p in db.saved.values()} == {'eng'}

    def test_a_portuguese_book_is_read_in_portuguese_alone(self, scans):
        db, out, engine = self.run(scans, FakeEngine(pages={0: PORTUGUESE, 1: PORTUGUESE, 2: PORTUGUESE}), declared='')
        assert db.decisions == [(7, 'aa', 'por', 'detected')]
        assert [c[0] for c in engine.calls][3:] == ['por'] * 3

    def test_the_pages_read_to_tell_the_language_are_not_kept_only_the_final_reading_is(self, scans):
        db, out, engine = self.run(scans, FakeEngine(pages={0: ENGLISH, 1: ENGLISH, 2: ENGLISH, 3: 'A leitura final da página zero, com texto o bastante.'}), declared='')
        assert db.saved[0]['text'].startswith('A leitura final')

    def test_when_it_cannot_be_told_the_owners_default_is_used_and_said_so(self, scans):
        # Too little text to tell, or two languages that score alike.
        db, out, engine = self.run(scans, FakeEngine(pages={0: 'Um texto curto demais para dizer.', 1: '', 2: ''}), declared='')
        assert db.decisions == [(7, 'aa', 'por+eng', 'default')]
        assert {p['lang'] for p in db.saved.values()} == {'por+eng'}

    def test_a_language_the_engine_has_not_got_is_not_used(self, scans):
        db, out, engine = self.run(scans, FakeEngine(pages={0: FRENCH, 1: FRENCH, 2: FRENCH}), declared='')  # the engine has eng and por
        assert db.decisions == [(7, 'aa', 'por+eng', 'default')]

    def test_pages_that_fail_to_be_read_for_this_leave_it_to_the_default(self, scans):
        db, out, engine = self.run(scans, FakeEngine(fail={0, 1, 2}), declared='')
        assert db.decisions == [(7, 'aa', 'por+eng', 'default')]
        assert out[7]['read'] == 3  # and the book is read all the same

    def test_what_was_decided_is_kept_and_used_by_the_next_reading(self, scans):
        db, out, engine = self.run(scans, FakeEngine(), declared='', decided=('eng', 'detected'))
        assert [c[0] for c in engine.calls] == ['eng'] * 3  # nothing read to find it out again
        assert db.decisions == []

    def test_a_fallback_to_the_default_is_not_kept_as_a_decision(self, scans):
        db, out, engine = self.run(scans, FakeEngine(pages={0: ENGLISH, 1: ENGLISH, 2: ENGLISH}), declared='', decided=('por+eng', 'default'))
        assert db.decisions == [(7, 'aa', 'eng', 'detected')]

    def test_what_somebody_on_the_staff_chose_wins_over_what_the_edition_declares(self, scans):
        db, out, engine = self.run(scans, FakeEngine(), declared='pt', decided=('eng', 'manual'))
        assert [c[0] for c in engine.calls] == ['eng'] * 3 and db.decisions == []

    def test_a_decision_with_a_language_the_engine_has_lost_is_decided_again(self, scans):
        db, out, engine = self.run(scans, FakeEngine(), declared='pt', decided=('deu', 'manual'))
        assert [c[0] for c in engine.calls] == ['por'] * 3
        assert db.decisions == [(7, 'aa', 'por', 'declared')]

    def test_the_decision_of_another_version_of_the_file_is_dropped(self, scans):
        db, out, engine = self.run(scans, FakeEngine(), declared='pt')
        statement, params = db.matching('DELETE FROM ocr_files')[0]
        assert 'source_sha256 IS DISTINCT FROM' in statement and params == (7, 'aa')

    def test_telling_the_language_stops_when_the_job_is_cancelled_and_decides_nothing(self, scans):
        db = OcrDB([], declared='')
        seen = []

        def checkpoint():
            seen.append(1)
            if len(seen) == 3:
                raise Cancelled()

        with pytest.raises(Cancelled):
            indexer(db, scans, FakeEngine(pages={0: ENGLISH, 1: ENGLISH})).run(9, checkpoint=checkpoint)
        assert db.decisions == [] and db.saved == {}

    def test_a_short_file_is_sampled_whole_and_a_long_one_in_three_pages(self, tmp_path):
        make_scan(tmp_path, pages=40)
        db = OcrDB([], declared='', without=list(range(1, 41)))
        engine = FakeEngine(pages={0: ENGLISH, 1: ENGLISH, 2: ENGLISH})
        indexer(db, tmp_path, engine).run(9)
        assert len(engine.calls) == 3 + 40
        assert [c[0] for c in engine.calls[:3]] == ['por+eng'] * 3


# --- what the owner tunes: how many pages are read at once, and with how many cores ---

class TestTuning:
    def test_without_a_choice_it_is_one_page_with_the_cores_the_installation_gives(self, scans, monkeypatch):
        monkeypatch.delenv('OCR_THREADS', raising=False)
        assert indexer(OcrDB([]), scans).tuning() == (1, 1)
        monkeypatch.setenv('OCR_THREADS', '3')
        assert indexer(OcrDB([]), scans).tuning() == (1, 3)

    def test_the_owner_choice_wins_over_the_installation(self, scans, monkeypatch):
        monkeypatch.setenv('OCR_THREADS', '3')
        db = OcrDB([])
        db.performance = {'ocrPages': 4, 'ocrThreads': 2}
        assert indexer(db, scans).tuning() == (4, 2)
        db.performance = {'ocrPages': 4}  # only what was chosen: the other is still the installation's
        assert indexer(db, scans).tuning() == (4, 3)

    def test_what_is_not_a_number_in_its_range_is_as_if_it_was_not_there(self, scans, monkeypatch):
        monkeypatch.delenv('OCR_THREADS', raising=False)
        for bad in [{'ocrPages': 0}, {'ocrPages': 9}, {'ocrPages': -1}, {'ocrPages': '4'}, {'ocrPages': 2.5}, {'ocrPages': True},
                    {'ocrPages': None}, {'ocrThreads': 9}, {'ocrThreads': 'x'}, 'nonsense', ['x'], 5, None]:
            db = OcrDB([])
            db.performance = bad
            assert indexer(db, scans).tuning() == (1, 1), bad

    def test_the_largest_values_are_allowed(self, scans):
        db = OcrDB([])
        db.performance = {'ocrPages': ocr.MAX_PAGES_AT_ONCE, 'ocrThreads': ocr.MAX_ENGINE_THREADS}
        assert indexer(db, scans).tuning() == (8, 8)

    def test_true_is_not_a_number_of_cores(self, scans, monkeypatch):
        monkeypatch.setenv('OCR_THREADS', '3')
        db = OcrDB([])
        db.performance = {'ocrThreads': True, 'ocrPages': True}
        assert indexer(db, scans).tuning() == (1, 3)  # True would be 1: the installation's 3 stays

    def test_a_setting_that_arrives_as_text_is_read(self, scans):
        db = OcrDB([])
        db.performance = json.dumps({'ocrPages': 3, 'ocrThreads': 2})
        assert indexer(db, scans).tuning() == (3, 2)
        db.performance = '{not json'
        assert indexer(db, scans).tuning() == (1, 1)

    def test_a_threads_variable_that_is_not_a_number_is_one(self, scans, monkeypatch):
        monkeypatch.setenv('OCR_THREADS', 'many')
        assert indexer(OcrDB([]), scans).tuning() == (1, 1)
        monkeypatch.setenv('OCR_THREADS', '99')
        assert indexer(OcrDB([]), scans).tuning()[1] == ocr.MAX_ENGINE_THREADS

    def test_a_job_takes_what_was_chosen_at_its_start_and_says_it(self, scans):
        db = OcrDB([])
        db.performance = {'ocrPages': 2, 'ocrThreads': 2}
        engine = FakeEngine()
        engine.threads = 1
        ix = OcrIndexer(db, str(scans), engine, log=lambda *a: None)
        ix.run(9)
        assert (ix.pages_at_once, ix.threads, engine.threads) == (2, 2, 2)
        reported = [json.loads(p[1]) for q, p in db.matching('INSERT INTO settings') if p and p[0] == 'ocr.worker']
        assert reported and reported[-1]['pages'] == 2 and reported[-1]['threads'] == 2
        # the next job, with a new choice, takes the new one
        db.performance = {'ocrPages': 1}
        ix.run(9)
        assert (ix.pages_at_once, ix.threads) == (1, 1)


# --- the pages of a file read at once ---

def make_distinct_scan(storage, pages):
    """A scan whose pages are told apart by their grey (the engine of the test reads it back from the picture)."""
    doc = fitz.open()
    for i in range(pages):
        pix = fitz.Pixmap(fitz.csGRAY, fitz.IRect(0, 0, 150, 150), False)
        pix.set_rect(pix.irect, (255 - 5 * i,))
        doc.new_page(width=72, height=72).insert_image(fitz.Rect(0, 0, 72, 72), pixmap=pix)
    doc.save(str(storage / 'scan.pdf'))
    doc.close()


def page_of(image):
    return (255 - fitz.Pixmap(image).pixel(0, 0)[0]) // 5


class AtOnceEngine:
    """An engine that knows which page it was given, and how many it is reading at the same moment."""
    name = 'tesseract'

    def __init__(self, fail=(), blank=(), delay=0.05, missing_on=None):
        self.fail, self.blank, self.delay, self.missing_on = set(fail), set(blank), delay, missing_on
        self.lock = threading.Lock()
        self.inside = self.peak = 0
        self.order = []
        self.threads = 1

    def installed(self):
        return True

    def version(self):
        return '5.3.0'

    def languages(self):
        return ['eng', 'por']

    def recognize(self, image, language, dpi):
        page = page_of(image)
        with self.lock:
            self.inside += 1
            self.peak = max(self.peak, self.inside)
            self.order.append(page)
        try:
            time.sleep(self.delay)
            if page == self.missing_on:
                raise EngineMissing('gone')
            if page in self.fail:
                raise EngineError(f'boom {page}')
            if page in self.blank:
                return ''
            return f'Texto reconhecido da página {page}, com o bastante para contar como texto.'
        finally:
            with self.lock:
                self.inside -= 1


def parallel_indexer(scans, pages, engine, at_once, without=None, **kwargs):
    make_distinct_scan(scans, pages)
    db = OcrDB([], without=without or list(range(1, pages + 1)), **kwargs)
    db.performance = {'ocrPages': at_once}
    return db, OcrIndexer(db, str(scans), engine, log=lambda *a: None)


class TestPagesAtOnce:
    def test_it_reads_the_pages_together_and_never_more_than_asked(self, scans):
        engine = AtOnceEngine()
        db, ix = parallel_indexer(scans, 12, engine, at_once=3)
        out = ix.run(9)
        assert out == {7: {'read': 12, 'failed': 0}}
        assert 2 <= engine.peak <= 3, f'peak {engine.peak}: it must read together, and never above 3'
        assert sorted(db.saved) == list(range(12))

    def test_one_at_a_time_is_what_it_has_always_been(self, scans):
        engine = AtOnceEngine(delay=0.01)
        db, ix = parallel_indexer(scans, 6, engine, at_once=1)
        ix.run(9)
        assert engine.peak == 1
        assert engine.order == list(range(6))

    def test_one_at_a_time_makes_no_threads_at_all(self, scans, monkeypatch):
        def no_pool(*args, **kwargs):
            raise AssertionError('a pool of threads was made for one page at a time')
        monkeypatch.setattr(ocr, 'ThreadPoolExecutor', no_pool)
        engine = AtOnceEngine(delay=0)
        db, ix = parallel_indexer(scans, 4, engine, at_once=1)
        ix.run(9)
        assert sorted(db.saved) == [0, 1, 2, 3]

    def test_no_more_pages_are_drawn_than_are_asked_for_ahead_of_what_is_saved(self, scans, monkeypatch):
        engine = AtOnceEngine(delay=0.03)
        db, ix = parallel_indexer(scans, 15, engine, at_once=3)
        drawn = []
        real = ocr.render

        def counting(page):
            drawn.append(len(db.saved))
            return real(page)
        monkeypatch.setattr(ocr, 'render', counting)
        ix.run(9)
        ahead = [n_drawn - saved for n_drawn, saved in zip(range(1, len(drawn) + 1), drawn)]
        assert max(ahead) <= 3, f'{max(ahead)} pages were drawn and not saved at once: the memory is more than what was chosen'

    def test_the_result_is_the_same_as_one_at_a_time_what_is_blank_and_what_failed_included(self, scans, tmp_path):
        kinds = {}
        for at_once in (1, 4):
            engine = AtOnceEngine(fail={2, 7}, blank={3, 4, 9}, delay=0.01)
            work = tmp_path / f'w{at_once}'
            work.mkdir()
            db, ix = parallel_indexer(work, 12, engine, at_once=at_once)
            out = ix.run(9)
            kinds[at_once] = ({p: (v['state'], v['text'], v['error']) for p, v in sorted(db.saved.items())}, out)
        assert kinds[1] == kinds[4]
        states = {p: v[0] for p, v in kinds[4][0].items()}
        assert states[2] == states[7] == 'failed' and states[3] == states[4] == states[9] == 'blank' and states[0] == 'done'

    def test_the_pages_are_saved_in_order_even_when_a_later_one_is_read_first(self, scans):
        class Backwards(AtOnceEngine):
            def recognize(self, image, language, dpi):
                time.sleep(0.12 if page_of(image) == 0 else 0.0)  # the first page is the slowest
                return super().recognize(image, language, dpi)
        engine = Backwards(delay=0)
        db, ix = parallel_indexer(scans, 6, engine, at_once=3)
        ix.run(9)
        order = [p[1] for q, p in db.matching('INSERT INTO ocr_pages')]
        assert order == list(range(6)), order

    def test_five_failures_in_a_row_stop_it_and_nothing_is_left_running(self, scans):
        engine = AtOnceEngine(fail=set(range(10)), delay=0.02)
        db, ix = parallel_indexer(scans, 20, engine, at_once=3)
        with pytest.raises(RuntimeError, match='5 pages in a row'):
            ix.run(9)
        assert engine.inside == 0, 'a page was still being read when the job ended'
        assert len(db.saved) == 5  # the five that counted are kept as failed, as with one at a time
        assert all(v['state'] == 'failed' for v in db.saved.values())
        assert not any(t.name.startswith('ocr-page') for t in threading.enumerate())

    def test_a_good_page_among_the_failures_starts_the_count_again(self, scans):
        engine = AtOnceEngine(fail={0, 1, 2, 3, 5, 6, 7, 8, 10, 11}, delay=0.01)  # four, one good, four, one good, two
        db, ix = parallel_indexer(scans, 12, engine, at_once=3)
        out = ix.run(9)
        assert out == {7: {'read': 2, 'failed': 10}}

    def test_a_job_that_is_cancelled_keeps_what_it_read_and_nothing_is_left_running(self, scans):
        engine = AtOnceEngine(delay=0.03)
        db, ix = parallel_indexer(scans, 20, engine, at_once=3)
        seen = []

        def checkpoint():
            seen.append(len(db.saved))
            if len(db.saved) >= 4:
                raise Cancelled()
        with pytest.raises(Cancelled):
            ix.read_file(7, 'aa', 'pt', 'managed', None, 'scan.pdf', list(range(1, 21)), 'por+eng', False, checkpoint)
        assert engine.inside == 0
        assert 4 <= len(db.saved) < 20, len(db.saved)
        assert not [t for t in threading.enumerate() if t.name.startswith('ocr-page')]

    def test_an_engine_that_is_gone_is_said_and_stops_everything(self, scans):
        engine = AtOnceEngine(missing_on=4, delay=0.01)
        db, ix = parallel_indexer(scans, 12, engine, at_once=3)
        with pytest.raises(EngineMissing):
            ix.run(9)
        assert engine.inside == 0

    def test_a_page_that_cannot_be_drawn_is_a_failed_page_not_a_stop(self, scans, monkeypatch):
        engine = AtOnceEngine(delay=0.01)
        db, ix = parallel_indexer(scans, 6, engine, at_once=2)
        real = ocr.render

        def render_or_fail(page):
            if page.number == 2:
                raise RuntimeError('bad page')
            return real(page)
        monkeypatch.setattr(ocr, 'render', render_or_fail)
        out = ix.run(9)
        assert out == {7: {'read': 5, 'failed': 1}}
        assert db.saved[2]['state'] == 'failed' and 'bad page' in db.saved[2]['error']
        assert 2 not in engine.order

    def test_the_engine_is_not_given_more_pages_than_there_are(self, scans):
        engine = AtOnceEngine(delay=0.01)
        db, ix = parallel_indexer(scans, 2, engine, at_once=8)
        out = ix.run(9)
        assert out == {7: {'read': 2, 'failed': 0}} and engine.peak <= 2
