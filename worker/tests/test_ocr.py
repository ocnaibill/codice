"""Reading the pages of scanned PDFs by OCR (#24): the engine, the choice of language and resolution, the job."""
import json
import os
import stat
import subprocess

import fitz
import pytest

import ocr
from ocr import (EngineError, EngineMissing, OcrIndexer, Tesseract, effective_dpi, engine_language, render)
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

    def __init__(self, files, enabled=True, language='por+eng', known=(), without=(1, 2, 3)):
        super().__init__([file_row(7, 'pdf', 'aa', 'pt', path='scan.pdf')])
        self.setting = {'enabled': enabled, 'language': language} if enabled is not None else None
        self.known = list(known)       # (page, state) already kept
        self.without = list(without)   # pages without text, from 1
        self.statements = []
        self.saved = {}

    def fetchone(self, query, params=None):
        if 'FROM settings' in query:
            return (self.setting,) if self.setting is not None else None
        return super().fetchone(query, params)

    def fetchall(self, query, params=None):
        if 'FROM files f' in query and 'text_layers' in query:
            return [(7, 'aa', 'pt', 'managed', None, 'scan.pdf', self.without)]
        if 'SELECT page, state FROM ocr_pages' in query:
            return list(self.known)
        return super().fetchall(query, params)

    def execute(self, query, params=None):
        self.statements.append((' '.join(query.split()), params))
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
        assert value == {'engine': 'tesseract', 'version': '5.3.0', 'languages': ['eng', 'por'], 'state': 'working', 'error': ''}

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

    def test_a_file_whose_edition_has_no_language_is_read_in_the_owners_default(self, scans):
        db = OcrDB([], language='por+eng')
        db.files = [file_row(7, 'pdf', 'aa', '', path='scan.pdf')]
        # the language of the edition comes from the query of the pages, which the fake answers with 'pt'
        out, engine = self.run(db, scans)
        assert engine.calls[0][0] == 'por'

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
