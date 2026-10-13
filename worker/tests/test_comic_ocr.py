"""Reading comics and manga by OCR (DEC-176): the choice, the model and its download, the job and what it keeps."""
import json
import zipfile

import pytest

import comic_ocr
from comic_ocr import (ENGINE_NAME, MODELS, Archive, ComicOcrIndexer, download_progress, lines_of, model_bytes, natural_names)


class DB:
    """Records what is written; answers what the indexer asks."""

    def __init__(self, choice=None, files=(), known=(), pages_read=0):
        self.choice, self.files, self.known, self.pages = choice, list(files), list(known), pages_read
        self.executed = []

    def fetchone(self, query, params=()):
        if 'FROM settings' in query:
            return (self.choice,) if self.choice is not None else None
        if 'count(*) FROM comic_ocr_pages' in query:
            return (self.pages,)
        return None

    def fetchall(self, query, params=()):
        if 'lower(COALESCE(f.format' in query:
            return self.files
        if 'SELECT page FROM comic_ocr_pages' in query:
            return [(p,) for p in self.known]
        return []

    def execute(self, query, params=()):
        self.executed.append((' '.join(query.split()), params))

    def beats(self):
        return [json.loads(p[1]) for q, p in self.executed if p and p[0] == 'comic_ocr.worker']

    def saved(self):
        return [p for q, p in self.executed if q.startswith('INSERT INTO comic_ocr_pages')]


class Engine:
    """An engine with nothing to download: it says what each page holds from the bytes of the page."""
    name = ENGINE_NAME

    def __init__(self, model, pages=None, fails=()):
        self.model, self.loaded, self.pages, self.fails, self.read_calls = model, False, pages or {}, set(fails), []
        self.on_read = None  # called with how many pages were read, after each one

    def load(self):
        self.loaded = True

    def version(self):
        return '3.7.0'

    def read(self, data):
        self.read_calls.append(data)
        if data in self.fails:
            raise RuntimeError('o motor falhou')
        if self.on_read:
            self.on_read(len(self.read_calls))
        return self.pages.get(data, []), (1000, 1500)


def line(text, x=100, y=100, w=300, h=30, conf=98.0):
    return {'text': text, 'x': x, 'y': y, 'w': w, 'h': h, 'conf': conf}


def comic(tmp_path, pages, name='c.cbz'):
    path = tmp_path / name
    with zipfile.ZipFile(path, 'w') as zf:
        for page_name, data in pages:
            zf.writestr(page_name, data)
    return str(path)


def indexer(db, tmp_path, engines=None, **kw):
    made = engines if engines is not None else {}

    def factory(model):
        made.setdefault(model, Engine(model))
        return made[model]

    ix = ComicOcrIndexer(db, str(tmp_path), engine_factory=factory, log=lambda *a: None, watch_db=DB(), root=str(tmp_path / 'models'), **kw)
    return ix, made


class TestTheChoice:
    @pytest.mark.parametrize('value,expected', [
        ({'model': 'mobile'}, 'mobile'), ({'model': 'server'}, 'server'), ({'model': 'none'}, 'none'),
        ({'model': 'gigante'}, 'none'), ({}, 'none'), (None, 'none'), ('{"model": "server"}', 'server'), ('isto não é JSON', 'none'),
    ])
    def test_what_is_not_one_of_the_models_is_none(self, tmp_path, value, expected):
        assert indexer(DB(choice=value), tmp_path)[0].choice() == expected

    def test_nothing_chosen_is_none(self, tmp_path):
        assert indexer(DB(choice=None), tmp_path)[0].choice() == 'none'


class TestWhatItSaysOfItself:
    def test_the_heartbeat_says_the_model_the_state_and_how_many_pages_it_read(self, tmp_path):
        db = DB(pages_read=42)
        ix, _ = indexer(db, tmp_path)
        ix.model = 'mobile'
        ix.heartbeat('ready')
        assert db.beats()[-1] == {'model': 'mobile', 'state': 'ready', 'error': '', 'engine': 'paddleocr', 'pages': 42}

    def test_the_percentage_is_there_only_while_the_model_is_being_prepared(self, tmp_path):
        db = DB()
        ix, _ = indexer(db, tmp_path)
        ix.model, ix.progress = 'server', 37
        ix.heartbeat('preparing')
        ix.heartbeat('ready')
        first, second = db.beats()
        assert first['progress'] == 37 and 'progress' not in second

    def test_with_no_model_the_page_count_is_not_asked_for(self, tmp_path):
        db = DB(pages_read=9)
        ix, _ = indexer(db, tmp_path)
        ix.heartbeat('idle')
        assert db.beats()[-1]['pages'] == 0 and db.beats()[-1]['model'] == 'none'


class TestTheModel:
    def test_it_is_loaded_once_and_the_download_is_told_before_and_after(self, tmp_path):
        db = DB()
        ix, made = indexer(db, tmp_path)
        engine = ix.prepare('mobile')
        assert engine.loaded
        states = [b['state'] for b in db.beats()]
        assert states == ['preparing', 'ready'] and db.beats()[0]['progress'] == 0
        ix.prepare('mobile')
        assert len(db.beats()) == 2 and list(made) == ['mobile']

    def test_a_model_that_cannot_be_loaded_is_said_with_why_and_is_tried_again_later(self, tmp_path):
        db = DB()

        class Broken(Engine):
            def load(self):
                raise OSError('sem espaço em disco')

        ix = ComicOcrIndexer(db, str(tmp_path), engine_factory=lambda m: Broken(m), log=lambda *a: None, watch_db=DB(), root=str(tmp_path))
        with pytest.raises(OSError):
            ix.prepare('server')
        assert db.beats()[-1]['state'] == 'error' and 'sem espaço' in db.beats()[-1]['error']
        assert 'server' not in ix.engines

    def test_the_percentage_is_what_is_on_disk_of_the_two_models_over_what_they_weigh(self, tmp_path):
        base = tmp_path / 'official_models'
        for name in (MODELS['mobile']['det'], MODELS['mobile']['rec']):
            (base / name).mkdir(parents=True)
        (base / MODELS['mobile']['det'] / 'a.bin').write_bytes(b'x' * 1024 * 1024)
        (base / MODELS['mobile']['rec'] / 'b.bin').write_bytes(b'x' * 1024 * 1024)
        assert model_bytes('mobile', str(tmp_path)) == 2 * 1024 * 1024
        assert download_progress('mobile', str(tmp_path)) == int(100 * 2 / MODELS['mobile']['mb'])
        assert download_progress('server', str(tmp_path)) == 0
        (base / MODELS['mobile']['rec'] / 'c.bin').write_bytes(b'x' * 100 * 1024 * 1024)
        assert download_progress('mobile', str(tmp_path)) == 99


class TestTheJob:
    def file_row(self, path, file_id=7):
        return [(file_id, 'sha', 'cbz', 'managed', None, path)]

    def run_job(self, tmp_path, pages, engine, known=(), choice=None, during=None, more_files=()):
        path = comic(tmp_path, pages)
        db = DB(choice=choice or {'model': 'mobile'}, files=self.file_row('c.cbz') + [self.file_row('c.cbz', file_id=i)[0] for i in more_files], known=known)
        if during:
            engine.on_read = lambda n: during(db, n)
        ix, made = indexer(db, tmp_path, engines={'mobile': engine})
        made_text = []
        original = comic_ocr.TextIndexer

        class Text:
            def __init__(self, *a):
                pass

            def run(self, work_id, force=False, checkpoint=None, only=None):
                made_text.append((work_id, force, only))
                return {}

        comic_ocr.TextIndexer = Text
        try:
            outcome = ix.run(5, checkpoint=lambda: None)
        finally:
            comic_ocr.TextIndexer = original
        return db, outcome, made_text, path

    def test_the_pages_are_read_in_page_order_and_kept_once_each_with_the_model_that_read_them(self, tmp_path):
        engine = Engine('mobile', pages={b'p2': [line('SEGUNDA PÁGINA AQUI')], b'p10': [], b'p1': [line('PRIMEIRA PÁGINA AQUI')]})
        db, outcome, text, _ = self.run_job(tmp_path, [('10.png', b'p10'), ('2.png', b'p2'), ('1.png', b'p1')], engine)
        # Page names are ordered as the rest of the system counts them: as text, "1", "10", "2".
        assert engine.read_calls == [b'p1', b'p10', b'p2']
        assert outcome == {7: {'read': 3, 'failed': 0}}
        rows = db.saved()
        assert [r[1] for r in rows] == [0, 1, 2]
        assert [r[3] for r in rows] == ['done', 'blank', 'done']
        assert rows[0][7] == 'mobile' and rows[0][8] == 'paddleocr' and rows[0][9] == '3.7.0'
        assert json.loads(rows[0][4])[0]['text'] == 'PRIMEIRA PÁGINA AQUI'
        assert json.loads(rows[1][4]) == []

    def test_the_text_of_the_file_is_made_again_from_what_was_read(self, tmp_path):
        engine = Engine('mobile', pages={b'a': [line('UMA FALA QUALQUER')]})
        _, _, text, _ = self.run_job(tmp_path, [('1.png', b'a')], engine)
        assert text == [(5, True, {7})]

    def test_a_page_that_was_already_read_is_not_read_again(self, tmp_path):
        engine = Engine('mobile', pages={b'b': [line('SEGUNDA FALA AQUI')]})
        db, outcome, _, _ = self.run_job(tmp_path, [('1.png', b'a'), ('2.png', b'b')], engine, known=[0])
        assert engine.read_calls == [b'b']
        assert outcome == {7: {'read': 1, 'failed': 0}}

    def test_a_page_that_fails_is_kept_as_failed_with_why_and_the_file_goes_on(self, tmp_path):
        engine = Engine('mobile', pages={b'b': [line('SEGUNDA FALA AQUI')]}, fails=[b'a'])
        db, outcome, _, _ = self.run_job(tmp_path, [('1.png', b'a'), ('2.png', b'b')], engine)
        assert outcome == {7: {'read': 1, 'failed': 1}}
        failed = db.saved()[0]
        assert failed[3] == 'failed' and 'o motor falhou' in failed[11] and failed[4] == '[]'

    def test_many_failures_in_a_row_stop_the_job_and_say_so(self, tmp_path):
        pages = [(f'{i}.png', bytes([i])) for i in range(1, 8)]
        engine = Engine('mobile', fails=[bytes([i]) for i in range(1, 8)])
        with pytest.raises(RuntimeError, match='in a row'):
            self.run_job(tmp_path, pages, engine)

    def test_with_no_model_chosen_nothing_is_read(self, tmp_path):
        engine = Engine('mobile')
        db, outcome, text, _ = self.run_job(tmp_path, [('1.png', b'a')], engine, choice={'model': 'none'})
        assert outcome == {} and engine.read_calls == [] and text == [] and db.saved() == []

    def test_a_model_chosen_while_it_runs_stops_the_job_at_the_page_that_is_next_and_the_text_is_made_of_what_was_read(self, tmp_path):
        engine = Engine('mobile', pages={b'a': [line('PRIMEIRA FALA AQUI')], b'b': [line('SEGUNDA FALA AQUI')]})

        def choose_another(db, n):
            if n == 1:
                db.choice = {'model': 'server'}

        db, outcome, text, _ = self.run_job(tmp_path, [('1.png', b'a'), ('2.png', b'b'), ('3.png', b'c')], engine, during=choose_another)
        assert engine.read_calls == [b'a']  # the model of the job is no longer the chosen: the pages that are left are not read with it
        assert outcome == {7: {'read': 1, 'failed': 0}}
        assert [r[1] for r in db.saved()] == [0]
        assert text == [(5, True, {7})]

    def test_the_files_that_are_left_are_not_started_with_a_model_that_is_no_longer_the_chosen(self, tmp_path):
        engine = Engine('mobile', pages={b'a': [line('PRIMEIRA FALA AQUI')]})

        def choose_another(db, n):
            db.choice = {'model': 'server'}

        _, outcome, text, _ = self.run_job(tmp_path, [('1.png', b'a')], engine, during=choose_another, more_files=[8, 9])
        assert list(outcome) == [7]  # the other two were not opened, and no text was made of them
        assert text == [(5, True, {7})]

    def test_a_model_taken_away_while_it_runs_stops_it_too(self, tmp_path):
        engine = Engine('mobile', pages={b'a': [line('PRIMEIRA FALA AQUI')]})

        def turn_off(db, n):
            db.choice = {'model': 'none'}

        _, outcome, _, _ = self.run_job(tmp_path, [('1.png', b'a'), ('2.png', b'b')], engine, during=turn_off)
        assert engine.read_calls == [b'a'] and outcome == {7: {'read': 1, 'failed': 0}}

    def test_what_was_read_of_another_version_of_the_file_is_thrown_away(self, tmp_path):
        engine = Engine('mobile')
        db, _, _, _ = self.run_job(tmp_path, [('1.png', b'a')], engine)
        assert any(q.startswith('DELETE FROM comic_ocr_pages') and 'IS DISTINCT FROM' in q for q, _ in db.executed)

    def test_a_file_that_is_not_at_its_place_is_an_error_and_not_a_blank_comic(self, tmp_path):
        db = DB(choice={'model': 'mobile'}, files=self.file_row('nao-existe.cbz'))
        ix, _ = indexer(db, tmp_path)
        with pytest.raises(FileNotFoundError):
            ix.run(5)
        assert db.beats()[-1]['state'] == 'error'


class TestWhatIsQueued:
    def test_with_no_model_it_says_it_is_idle_and_queues_nothing(self, tmp_path):
        db = DB(choice={'model': 'none'})
        ix, made = indexer(db, tmp_path)
        ix.enqueue_missing()
        assert made == {} and db.beats()[-1]['state'] == 'idle'
        assert not any(q.startswith('INSERT INTO jobs') for q, _ in db.executed)

    def test_with_a_model_it_prepares_it_when_it_is_chosen_and_queues_the_comics_that_were_not_read(self, tmp_path):
        db = DB(choice={'model': 'server'})
        ix, made = indexer(db, tmp_path)
        ix.enqueue_missing()
        assert made['server'].loaded
        query = next(q for q, _ in db.executed if q.startswith('INSERT INTO jobs'))
        assert "'comic_ocr'" in query and "IN ('cbz', 'cbr')" in query and 'w.page_count' in query
        assert "j.state = 'failed'" in query  # a work whose job failed is left alone for a while

    def test_a_model_that_cannot_be_prepared_is_logged_and_does_not_stop_the_service(self, tmp_path):
        db = DB(choice={'model': 'mobile'})

        class Broken(Engine):
            def load(self):
                raise OSError('rede fora do ar')

        said = []
        ix = ComicOcrIndexer(db, str(tmp_path), engine_factory=lambda m: Broken(m), log=said.append, watch_db=DB(), root=str(tmp_path))
        ix.enqueue_missing()
        assert said and 'rede fora do ar' in said[0]
        assert not any(q.startswith('INSERT INTO jobs') for q, _ in db.executed)


class TestThePages:
    def test_the_pictures_are_the_ones_the_extractors_count_and_in_their_order(self):
        names = ['b.png', 'ComicInfo.xml', 'a.JPG', 'dir/', 'c.webp', 'notes.txt', 'a2.jpeg']
        assert natural_names(names) == ['a.JPG', 'a2.jpeg', 'b.png', 'c.webp']

    def test_a_page_that_is_too_large_is_not_read(self, tmp_path, monkeypatch):
        path = comic(tmp_path, [('1.png', b'x' * 100)])
        monkeypatch.setattr(comic_ocr.Limits, 'MAX_ENTRY_BYTES', 10)
        with Archive(path, 'cbz') as archive:
            with pytest.raises(ValueError, match='too large'):
                archive.read(0)

    def test_a_file_that_is_not_an_archive_is_a_value_error(self, tmp_path):
        bad = tmp_path / 'x.cbz'
        bad.write_bytes(b'nada')
        with pytest.raises(ValueError, match='not a readable CBZ'):
            Archive(str(bad), 'cbz').__enter__()


class TestWhatTheEngineFinds:
    def test_the_lines_are_in_the_pixels_of_the_page_not_of_the_shrunk_picture(self):
        result = [{'rec_texts': ['OLÁ MUNDO', ' '], 'rec_scores': [0.987, 0.9],
                   'rec_polys': [[[10, 20], [110, 20], [110, 50], [10, 50]], [[0, 0], [1, 0], [1, 1], [0, 1]]]}]
        assert lines_of(result, scale=0.5) == [{'x': 20, 'y': 40, 'w': 200, 'h': 60, 'text': 'OLÁ MUNDO', 'conf': pytest.approx(98.7)}]

    def test_the_result_may_come_wrapped_as_the_library_gives_it(self):
        class Item:
            json = {'res': {'rec_texts': ['TEXTO'], 'rec_scores': [0.9], 'dt_polys': [[[0, 0], [10, 0], [10, 5], [0, 5]]]}}

        assert lines_of([Item()])[0]['text'] == 'TEXTO'
