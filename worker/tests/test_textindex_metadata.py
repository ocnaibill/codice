"""The text a comic or an audio file carries about itself: ComicInfo.xml, chapters, description (#25)."""
import json
import os
import shutil
import zipfile

import pytest

from textindex import BASE_VERSION, EXTRACTOR_VERSION, Limits, required_version
from textindex import store as store_module
from textindex.audio import audio_segments
from textindex.comic import comic_segments
from textindex.store import TextIndexer
from tests.test_textindex import FakeDB, file_row

FIXTURES = os.path.join(os.path.dirname(__file__), 'fixtures')
IMAGE = {'type': 'image', 'index': 0, 'item': 'ComicInfo.xml'}


def cbz(tmp_path, comicinfo=None, name='c.cbz', info_name='ComicInfo.xml'):
    path = tmp_path / name
    with zipfile.ZipFile(path, 'w') as zf:
        if comicinfo is not None:
            zf.writestr(info_name, comicinfo)
        zf.writestr('001.png', b'not really an image')
    return str(path)


def info(**fields):
    body = ''.join(f'<{tag}>{value}</{tag}>' for tag, value in fields.items())
    return f'<?xml version="1.0"?><ComicInfo><Title>Volume</Title><Series>Série</Series>{body}</ComicInfo>'


def sections(segments):
    return [(s.section, s.text) for s in segments]


class TestComicInfo:
    def test_what_the_file_says_of_the_story_is_text_in_the_order_of_the_fields(self, tmp_path):
        path = cbz(tmp_path, info(Notes='Edição de colecionador.', Characters='Ana, Bruno', Summary='Uma corrida longa.',
                                  StoryArc='A Fuga', Teams='Os Corredores', Locations='São Paulo, Rio'))
        got = list(comic_segments(path))
        assert sections(got) == [('Sinopse', 'Uma corrida longa.'), ('Arco', 'A Fuga'), ('Personagens', 'Ana, Bruno'),
                                 ('Equipes', 'Os Corredores'), ('Locais', 'São Paulo, Rio'), ('Notas', 'Edição de colecionador.')]

    def test_none_of_it_belongs_to_a_page_so_all_of_it_opens_the_start_and_says_where_it_came_from(self, tmp_path):
        got = list(comic_segments(cbz(tmp_path, info(Summary='x', Characters='y'))))
        assert [s.locator for s in got] == [IMAGE, IMAGE]
        assert all(s.origin == 'native' for s in got)

    def test_the_title_and_the_series_are_not_repeated_here(self, tmp_path):
        assert list(comic_segments(cbz(tmp_path, info()))) == []  # they are the catalogue's own metadata

    def test_a_comic_without_ComicInfo_has_none(self, tmp_path):
        assert list(comic_segments(cbz(tmp_path))) == []

    def test_fields_that_are_empty_say_nothing(self, tmp_path):
        got = list(comic_segments(cbz(tmp_path, info(Summary='   ', Characters='', Teams='Equipe'))))
        assert sections(got) == [('Equipes', 'Equipe')]

    def test_markup_in_a_field_is_taken_out_and_never_kept(self, tmp_path):
        summary = '&lt;p&gt;Uma &lt;b&gt;história&lt;/b&gt; &amp;amp; mais.&lt;/p&gt;&lt;p&gt;Segundo parágrafo.&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;'
        got = list(comic_segments(cbz(tmp_path, info(Summary=summary))))
        assert [s.text for s in got] == ['Uma história & mais.\nSegundo parágrafo.']
        assert '<' not in got[0].text

    def test_the_name_of_the_file_is_found_whatever_its_case(self, tmp_path):
        assert sections(comic_segments(cbz(tmp_path, info(Summary='ok'), info_name='comicinfo.XML'))) == [('Sinopse', 'ok')]

    def test_a_ComicInfo_that_does_not_parse_says_nothing_and_the_comic_is_fine(self, tmp_path):
        assert list(comic_segments(cbz(tmp_path, '<ComicInfo><Summary>never closed'))) == []

    def test_an_external_entity_is_not_read(self, tmp_path):
        secret = tmp_path / 'secret.txt'
        secret.write_text('TOP-SECRET-CONTENT')
        xml = f'<?xml version="1.0"?><!DOCTYPE x [<!ENTITY leak SYSTEM "file://{secret}">]><ComicInfo><Summary>&leak;</Summary></ComicInfo>'
        got = list(comic_segments(cbz(tmp_path, xml)))
        assert all('TOP-SECRET' not in s.text for s in got)

    def test_an_entity_that_expands_into_a_flood_is_not_expanded(self, tmp_path):
        bomb = ('<?xml version="1.0"?><!DOCTYPE b [<!ENTITY a "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa">'
                '<!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;"><!ENTITY c "&b;&b;&b;&b;&b;&b;&b;&b;&b;&b;">'
                '<!ENTITY d "&c;&c;&c;&c;&c;&c;&c;&c;&c;&c;">]><ComicInfo><Summary>&d;</Summary></ComicInfo>')
        got = list(comic_segments(cbz(tmp_path, bomb)))
        assert sum(len(s.text) for s in got) < 1000

    def test_a_ComicInfo_far_larger_than_any_is_not_read(self, tmp_path):
        big = info(Summary='x' * (Limits.MAX_METADATA_BYTES + 10))
        assert list(comic_segments(cbz(tmp_path, big))) == []

    def test_a_long_description_is_cut_into_segments_and_capped(self, tmp_path):
        paragraphs = ' '.join(f'Frase número {i} da sinopse.' for i in range(2000))
        got = list(comic_segments(cbz(tmp_path, info(Summary=paragraphs))))
        assert len(got) > 1 and all(len(s.text) <= 2400 for s in got)
        assert sum(len(s.text) for s in got) <= Limits.MAX_METADATA_CHARS + len(got)

    def test_a_file_that_is_not_an_archive_cannot_be_read(self, tmp_path):
        (tmp_path / 'bad.cbz').write_bytes(b'not a zip')
        with pytest.raises(ValueError):
            list(comic_segments(str(tmp_path / 'bad.cbz')))


class FakeRar:
    """What the CBR reader asks of a RAR archive, from memory (a RAR cannot be made without its own tool)."""
    files = {}

    def __init__(self, path):
        pass

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def infolist(self):
        from types import SimpleNamespace
        return [SimpleNamespace(filename=n, file_size=len(d)) for n, d in self.files.items()]

    def read(self, info):
        return self.files[info.filename]


class TestCbr:
    def test_a_cbr_is_read_the_same_way(self, tmp_path, monkeypatch):
        import rarfile
        monkeypatch.setattr(rarfile, 'RarFile', type('R', (FakeRar,), {'files': {'ComicInfo.xml': info(Summary='Em RAR.').encode(), '001.png': b'x'}}))
        assert sections(comic_segments('/x/c.cbr', 'cbr')) == [('Sinopse', 'Em RAR.')]

    def test_the_name_is_found_whatever_its_case_and_a_huge_one_is_not_read(self, tmp_path, monkeypatch):
        import rarfile
        monkeypatch.setattr(rarfile, 'RarFile', type('R', (FakeRar,), {'files': {'comicinfo.xml': info(Summary='Minúsculas.').encode()}}))
        assert sections(comic_segments('/x/c.cbr', 'cbr')) == [('Sinopse', 'Minúsculas.')]
        huge = info(Summary='x' * (Limits.MAX_METADATA_BYTES + 10)).encode()
        monkeypatch.setattr(rarfile, 'RarFile', type('R', (FakeRar,), {'files': {'ComicInfo.xml': huge}}))
        assert list(comic_segments('/x/c.cbr', 'cbr')) == []

    def test_without_ComicInfo_it_has_none(self, tmp_path, monkeypatch):
        import rarfile
        monkeypatch.setattr(rarfile, 'RarFile', type('R', (FakeRar,), {'files': {'001.png': b'x'}}))
        assert list(comic_segments('/x/c.cbr', 'cbr')) == []


def at(ms, title):
    return {'type': 'audio', 'track': 0, 'ms': ms}, title


class TestAudio:
    @pytest.mark.parametrize('name', ['capitulos.m4b', 'capitulos.mp3'])
    def test_the_chapters_of_a_real_file_are_found_at_their_minute(self, name):
        got = [s for s in audio_segments(os.path.join(FIXTURES, name)) if s.section not in ('Comentário', 'Descrição')]
        assert [(s.locator['ms'], s.text, s.section) for s in got] == [
            (0, 'Abertura', 'Abertura'), (1000, 'Capítulo 1: A corrida de ontem', 'Capítulo 1: A corrida de ontem'),
            (2500, 'Capítulo 2: Informações importantes', 'Capítulo 2: Informações importantes')]
        assert all(s.locator['type'] == 'audio' and s.locator['track'] == 0 and s.origin == 'native' for s in got)

    @pytest.mark.parametrize('name', ['capitulos.m4b', 'capitulos.mp3'])
    def test_the_description_and_the_comment_have_no_moment_so_they_are_at_the_start(self, name):
        got = [s for s in audio_segments(os.path.join(FIXTURES, name)) if s.section in ('Comentário', 'Descrição')]
        assert sorted(sections(got)) == [('Comentário', 'Um audiolivro sintético sobre corridas e informações.'),
                                         ('Descrição', 'Narrado por uma voz de teste.')]
        assert all(s.locator == {'type': 'audio', 'track': 0, 'ms': 0} for s in got)

    def test_what_describes_the_file_comes_before_its_chapters(self):
        got = [s.section for s in audio_segments(os.path.join(FIXTURES, 'capitulos.m4b'))]
        assert set(got[:2]) == {'Comentário', 'Descrição'}
        assert got[2:] == ['Abertura', 'Capítulo 1: A corrida de ontem', 'Capítulo 2: Informações importantes']

    def test_chapters_given_out_of_order_are_read_in_order_of_time(self, tmp_path):
        from mutagen.id3 import ID3, CHAP, TIT2
        path = tmp_path / 'b.mp3'
        shutil.copy(os.path.join(FIXTURES, 'capitulos.mp3'), path)
        tags = ID3(path)
        tags.delall('CHAP')
        tags.delall('CTOC')
        for element, start, title in (('c', 9000, 'Fim'), ('a', 0, 'Começo'), ('b', 4000, 'Meio')):
            tags.add(CHAP(element_id=element, start_time=start, end_time=start + 1, start_offset=0xFFFFFFFF, end_offset=0xFFFFFFFF, sub_frames=[TIT2(encoding=3, text=[title])]))
        tags.save(path)
        chapters = [(s.locator['ms'], s.text) for s in audio_segments(str(path)) if s.section not in ('Comentário', 'Descrição')]
        assert chapters == [(0, 'Começo'), (4000, 'Meio'), (9000, 'Fim')]

    def test_a_file_with_no_tags_has_none(self, tmp_path):
        from mutagen.id3 import ID3
        path = tmp_path / 'plain.mp3'
        shutil.copy(os.path.join(FIXTURES, 'capitulos.mp3'), path)
        ID3(path).delete(path)
        assert list(audio_segments(str(path))) == []

    def test_chapters_without_a_title_are_left_out(self, tmp_path):
        from mutagen.id3 import ID3, CHAP, TIT2
        path = tmp_path / 'b.mp3'
        shutil.copy(os.path.join(FIXTURES, 'capitulos.mp3'), path)
        tags = ID3(path)
        tags.delall('CHAP')
        tags.add(CHAP(element_id='a', start_time=0, end_time=1, start_offset=0xFFFFFFFF, end_offset=0xFFFFFFFF, sub_frames=[TIT2(encoding=3, text=['  '])]))
        tags.add(CHAP(element_id='b', start_time=500, end_time=1, start_offset=0xFFFFFFFF, end_offset=0xFFFFFFFF, sub_frames=[TIT2(encoding=3, text=['Só este'])]))
        tags.save(path)
        assert [(s.locator['ms'], s.text) for s in audio_segments(str(path)) if s.section not in ('Comentário', 'Descrição')] == [(500, 'Só este')]

    def test_markup_in_the_title_of_a_chapter_is_taken_out(self, tmp_path):
        from mutagen.id3 import ID3, CHAP, TIT2
        path = tmp_path / 'b.mp3'
        shutil.copy(os.path.join(FIXTURES, 'capitulos.mp3'), path)
        tags = ID3(path)
        tags.delall('CHAP')
        tags.add(CHAP(element_id='a', start_time=0, end_time=1, start_offset=0xFFFFFFFF, end_offset=0xFFFFFFFF, sub_frames=[TIT2(encoding=3, text=['<b>Capítulo 1</b> &amp; o fim<script>x()</script>'])]))
        tags.save(path)
        assert [s.text for s in audio_segments(str(path)) if s.section not in ('Comentário', 'Descrição')] == ['Capítulo 1 & o fim']

    def test_the_same_text_in_two_tags_is_kept_once(self, tmp_path):
        from mutagen.id3 import ID3, COMM, TXXX
        path = tmp_path / 'b.mp3'
        shutil.copy(os.path.join(FIXTURES, 'capitulos.mp3'), path)
        tags = ID3(path)
        tags.delall('TXXX')
        tags.add(COMM(encoding=3, lang='por', desc='', text=['Mesmo texto.']))
        tags.add(TXXX(encoding=3, desc='description', text=['Mesmo texto.']))
        tags.save(path)
        assert [s.text for s in audio_segments(str(path)) if s.section in ('Comentário', 'Descrição')] == ['Mesmo texto.']

    def test_markup_in_a_description_is_taken_out(self, tmp_path):
        from mutagen.id3 import ID3, COMM
        path = tmp_path / 'b.mp3'
        shutil.copy(os.path.join(FIXTURES, 'capitulos.mp3'), path)
        tags = ID3(path)
        tags.delall('TXXX')
        tags.add(COMM(encoding=3, lang='por', desc='', text=['<p>Uma <b>voz</b> &amp; um livro.</p><script>x()</script>']))
        tags.save(path)
        got = [s.text for s in audio_segments(str(path)) if s.section == 'Comentário']
        assert got == ['Uma voz & um livro.']

    def test_the_chapters_of_a_vorbis_file_are_read(self, tmp_path):
        from mutagen.flac import FLAC
        path = tmp_path / 'b.flac'
        shutil.copy(os.path.join(FIXTURES, 'silencio.flac'), path)
        audio = FLAC(path)
        audio['CHAPTER002'] = '00:01:05.250'
        audio['CHAPTER002NAME'] = 'Segundo'
        audio['CHAPTER001'] = '00:00:00.000'
        audio['CHAPTER001NAME'] = 'Primeiro'
        audio['DESCRIPTION'] = 'Uma descrição em Vorbis.'
        audio.save()
        got = list(audio_segments(str(path)))
        assert [(s.section, s.locator['ms']) for s in got] == [('Descrição', 0), ('Primeiro', 0), ('Segundo', 65250)]

    def test_a_file_that_is_not_audio_cannot_be_read(self, tmp_path):
        (tmp_path / 'bad.mp3').write_bytes(b'not an audio file at all' * 10)
        with pytest.raises(ValueError):
            list(audio_segments(str(tmp_path / 'bad.mp3')))

    def test_a_file_with_far_too_many_chapters_is_refused(self, monkeypatch):
        monkeypatch.setattr(Limits, 'MAX_CHAPTERS', 2)
        with pytest.raises(ValueError):
            list(audio_segments(os.path.join(FIXTURES, 'capitulos.mp3')))


class TestVersions:
    def test_a_change_for_one_kind_of_format_does_not_read_the_others_again(self):
        for fmt in ('cbz', 'cbr', 'mp3', 'm4a', 'm4b', 'ogg', 'wav', 'flac', 'CBZ'):
            assert required_version(fmt) == 5  # the version that reads their metadata
        for fmt in ('epub', 'EPUB'):
            assert required_version(fmt) == EXTRACTOR_VERSION == 6  # the version that reads what an EPUB declares
        for fmt in ('pdf', 'txt', 'md', 'mobi', '', None):
            assert required_version(fmt) == BASE_VERSION

    def test_a_comic_read_before_is_read_again_and_a_pdf_is_not(self, tmp_path):
        idx = TextIndexer(FakeDB([]), str(tmp_path))
        old = BASE_VERSION
        assert idx.needs_reading(file_row(1, 'cbz', version=old, source_sha='aa', status='unsupported'), False)
        assert idx.needs_reading(file_row(1, 'm4b', version=old, source_sha='aa', status='unsupported'), False)
        assert not idx.needs_reading(file_row(1, 'pdf', version=old, source_sha='aa', status='ready'), False)
        assert not idx.needs_reading(file_row(1, 'cbz', version=5, source_sha='aa', status='ready'), False)  # up to date for a comic
        assert idx.needs_reading(file_row(1, 'pdf', version=old - 1, source_sha='aa', status='ready'), False)

    def test_an_epub_read_before_is_read_again_and_the_others_are_not(self, tmp_path):
        idx = TextIndexer(FakeDB([]), str(tmp_path))
        assert idx.needs_reading(file_row(1, 'epub', version=5, source_sha='aa', status='ready'), False)
        assert idx.needs_reading(file_row(1, 'epub', version=BASE_VERSION, source_sha='aa', status='ready'), False)
        assert not idx.needs_reading(file_row(1, 'epub', version=EXTRACTOR_VERSION, source_sha='aa', status='ready'), False)
        assert not idx.needs_reading(file_row(1, 'pdf', version=5, source_sha='aa', status='ready'), False)
        assert not idx.needs_reading(file_row(1, 'txt', version=BASE_VERSION, source_sha='aa', status='ready'), False)


class TestIndexer:
    def run(self, tmp_path, rows):
        db = FakeDB(rows)
        outcome = TextIndexer(db, str(tmp_path)).run(9)
        return db, outcome

    def published(self, db):
        return [c for c in db.calls if c[1] == 'text_extraction_publish'][0][2]

    def test_a_comic_with_a_description_is_ready_and_its_segments_are_stored(self, tmp_path):
        cbz(tmp_path, info(Summary='Uma corrida longa.', Characters='Ana'))
        db, outcome = self.run(tmp_path, [file_row(7, 'cbz', path='c.cbz', language='')])
        assert outcome == {7: 'ready'}
        assert [(r[3], r[4], r[5]) for r in db.rows] == [('native', 'Sinopse', 'Uma corrida longa.'), ('native', 'Personagens', 'Ana')]
        assert json.loads(db.rows[0][6]) == IMAGE

    def test_a_comic_with_nothing_to_say_is_unsupported_not_an_empty_scan(self, tmp_path):
        cbz(tmp_path, info())
        db, outcome = self.run(tmp_path, [file_row(7, 'cbz', path='c.cbz')])
        assert outcome == {7: 'unsupported'}
        assert db.rows == []

    def test_the_language_of_a_comic_is_not_guessed_from_its_description(self, tmp_path):
        cbz(tmp_path, info(Summary='A corrida de ontem foi longa e o corredor chegou cansado ao fim da rua. ' * 20))
        db, outcome = self.run(tmp_path, [file_row(7, 'cbz', path='c.cbz', language='')])
        assert outcome == {7: 'ready'}
        assert not [c for c in db.calls if 'metadata_candidates' in str(c)]
        assert self.published(db)[6] is None  # no language published

    def test_an_audio_file_with_chapters_is_ready(self, tmp_path):
        shutil.copy(os.path.join(FIXTURES, 'capitulos.m4b'), tmp_path / 'b.m4b')
        db, outcome = self.run(tmp_path, [file_row(7, 'm4b', path='b.m4b', language='')])
        assert outcome == {7: 'ready'}
        assert len(db.rows) == 5  # two descriptions and three chapters
        assert {json.loads(r[6])['type'] for r in db.rows} == {'audio'}

    def test_an_audio_file_without_tags_is_unsupported(self, tmp_path):
        from mutagen.id3 import ID3
        shutil.copy(os.path.join(FIXTURES, 'capitulos.mp3'), tmp_path / 'p.mp3')
        ID3(tmp_path / 'p.mp3').delete(tmp_path / 'p.mp3')
        db, outcome = self.run(tmp_path, [file_row(7, 'mp3', path='p.mp3')])
        assert outcome == {7: 'unsupported'}

    def test_a_comic_that_cannot_be_read_is_recorded_and_the_others_still_are(self, tmp_path):
        (tmp_path / 'bad.cbz').write_bytes(b'not a zip')
        shutil.copy(os.path.join(FIXTURES, 'capitulos.mp3'), tmp_path / 'ok.mp3')
        db, outcome = self.run(tmp_path, [file_row(7, 'cbz', path='bad.cbz'), file_row(8, 'mp3', path='ok.mp3')])
        assert outcome == {7: 'failed', 8: 'ready'}

    def test_a_pdf_read_with_the_last_version_of_its_format_is_left_alone(self, tmp_path):
        db, outcome = self.run(tmp_path, [file_row(7, 'pdf', version=BASE_VERSION, source_sha='aa', status='ready')])
        assert outcome == {} and db.calls == [('fetchall', 'files', (9,))]

    def test_an_epub_read_before_the_version_that_reads_what_it_declares_is_read_again(self, tmp_path):
        from tests.test_textindex import make_epub
        make_epub(tmp_path / 'e.epub', {'a.xhtml': '<p>Um texto de capítulo que o EPUB traz, com o bastante para ser lido.</p>'})
        db, outcome = self.run(tmp_path, [file_row(7, 'epub', path='e.epub', version=5, source_sha='aa', status='ready')])
        assert outcome == {7: 'ready'}
        db, outcome = self.run(tmp_path, [file_row(7, 'epub', version=EXTRACTOR_VERSION, source_sha='aa', status='ready')])
        assert outcome == {} and db.calls == [('fetchall', 'files', (9,))]
