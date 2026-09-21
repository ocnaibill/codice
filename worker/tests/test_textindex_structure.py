"""The shape of a book: an outline's nodes, the part each is in, and the segments that know their node."""
import json
import os
import zipfile

import fitz
import pytest

from textindex import structure
from textindex.epub import epub_segments
from textindex.pdf import pdf_segments
from textindex.store import TextIndexer
from tests.test_textindex import FakeDB, file_row, make_epub, make_pdf

NCX = ('<?xml version="1.0"?><ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1"><navMap>{}</navMap></ncx>')


def point(n, title, src, children=''):
    return f'<navPoint id="n{n}"><navLabel><text>{title}</text></navLabel><content src="{src}"/>{children}</navPoint>'


def epub_with_outline(path, chapters, points, ncx_name='toc.ncx'):
    make_epub(path, chapters, extra={
        f'OEBPS/{ncx_name}': NCX.format(''.join(points)),
    })
    # the manifest of make_epub does not list the NCX: add it
    with zipfile.ZipFile(path) as zf:
        members = {n: zf.read(n) for n in zf.namelist()}
    members['OEBPS/content.opf'] = members['OEBPS/content.opf'].replace(
        b'<item id="css"', f'<item id="ncx" href="{ncx_name}" media-type="application/x-dtbncx+xml"/><item id="css"'.encode())
    with zipfile.ZipFile(path, 'w') as zf:
        for name, data in members.items():
            zf.writestr(name, data)


def words(n, tag):
    return ' '.join(f'{tag}{i}' for i in range(n))


class TestClassify:
    def nodes(self, *titles_and_chars):
        return [{'title': t, 'depth': d, 'chars': c} for t, d, c in titles_and_chars]

    def parts(self, nodes):
        return [n['part'] for n in structure.classify(nodes)]

    def test_front_body_and_back_by_words_and_by_place(self):
        nodes = self.nodes(('Título', 0, 40), ('Sumário', 0, 300), ('Prefácio', 0, 4000),
                           ('Capítulo I', 0, 9000), ('Capítulo II', 0, 9000),
                           ('Apêndice I', 0, 5000), ('Glossário', 0, 3000))
        assert self.parts(nodes) == ['front', 'front', 'front', 'body', 'body', 'back', 'back']

    def test_a_title_page_with_no_text_is_not_the_start_of_the_story(self):
        nodes = self.nodes(('DA TERRA Á LUA', 0, 30), ('BIBLIOTHECA ILLUSTRADA', 0, 20), ('CAPITULO I', 0, 8000), ('CAPITULO II', 0, 8000), ('Lista de erros corrigidos', 0, 200))
        assert self.parts(nodes) == ['front', 'front', 'body', 'body', 'back']

    def test_a_chapter_that_only_mentions_a_back_matter_word_is_a_chapter(self):
        nodes = self.nodes(('Capítulo I', 0, 8000), ('O fim do mundo', 0, 8000), ('Notes on the Text', 0, 8000),
                           ('End of the road', 0, 8000), ('Fim de jogo', 0, 8000), ('Capítulo VI', 0, 8000))
        assert self.parts(nodes) == ['body'] * 6

    def test_the_words_that_close_a_book_close_it_only_as_the_whole_title(self):
        nodes = self.nodes(('Capítulo I', 0, 8000), ('Capítulo II', 0, 8000), ('Notes', 0, 800), ('THE END', 0, 10))
        assert self.parts(nodes) == ['body', 'body', 'back', 'back']

    def test_acknowledgements_go_with_the_side_of_the_book_they_are_on(self):
        nodes = self.nodes(('Acknowledgments', 0, 500), ('Capítulo I', 0, 8000), ('Capítulo II', 0, 8000), ('Acknowledgments', 0, 500))
        assert self.parts(nodes) == ['front', 'body', 'body', 'back']

    def test_what_is_nested_under_back_matter_is_back_matter(self):
        nodes = self.nodes(('Livro Um', 0, 0), ('Capítulo I', 1, 9000), ('Livro Dois', 0, 0), ('Capítulo II', 1, 9000),
                           ('Appendix IV: The Almanak', 0, 400), ('Selected excerpts', 1, 9000))
        assert self.parts(nodes) == ['body', 'body', 'body', 'body', 'back', 'back']

    def test_a_closing_note_much_smaller_than_the_parts_of_the_book_is_not_part_of_the_story(self):
        nodes = self.nodes(('Prefácio', 0, 28000), ('Livro Primeiro', 0, 476000), ('Livro Segundo', 0, 385000), ('Livro Terceiro', 0, 304000),
                           ('Terminologia do Império', 0, 45000), ('Notas Cartográficas', 0, 1535))
        assert self.parts(nodes) == ['front', 'body', 'body', 'body', 'back', 'back']

    def test_a_short_last_chapter_of_a_book_of_short_chapters_is_still_the_story(self):
        nodes = self.nodes(('Capítulo I', 0, 9000), ('Capítulo II', 0, 9000), ('Capítulo III', 0, 9000), ('Epílogo', 0, 1600))
        assert self.parts(nodes) == ['body'] * 4

    def test_a_book_with_nothing_that_reads_as_a_story_is_all_around_it(self):
        assert set(self.parts(self.nodes(('Cover', 0, 10), ('Contents', 0, 100)))) == {'front'}

    def test_a_book_whose_titles_are_all_in_arabic_is_told_apart_as_well(self):
        nodes = self.nodes(('صفحة العنوان', 0, 0), ('صفحة حقوق الطبع والنشر', 0, 1832), ('إخلاص', 0, 186),
                           ('الكتاب الأول - الكثبان الرملية', 0, 356311), ('الكتاب الثاني - معاذديب', 0, 282662), ('الكتاب الثالث - النبي', 0, 224498),
                           ('الملاحق', 0, 38184), ('مصطلحات الإمبراطورية', 0, 34322), ('ملاحظات رسم الخرائط', 0, 1182), ('خاتمة بقلم بريان هربرت', 0, 20707))
        assert self.parts(nodes) == ['front'] * 3 + ['body'] * 3 + ['back'] * 4

    def test_the_writing_of_an_arabic_title_does_not_change_what_it_is(self):
        # with vowel marks, an alef with a hamza, or a stretched (tatweel) word
        for title in ('الْمَلَاحِق', 'الملاحـــق', 'ملاحق'):
            assert structure._kind(title) == 'back', title
        assert structure._kind('الفصل الأول') == 'neutral'

    def test_no_outline_is_no_nodes(self):
        assert structure.classify([]) == []


class TestEpubOutline:
    def test_chapters_that_share_a_file_are_told_apart_by_the_anchors_of_the_outline(self, tmp_path):
        path = tmp_path / 'a.epub'
        body = ''.join(f'<h2 id="c{n}">CAPITULO {n}</h2><p>{words(400, "c" + str(n) + "w")}</p>' for n in (1, 2, 3))
        epub_with_outline(path, {'all.xhtml': body}, [point(i, f'CAPITULO {i}', f'all.xhtml#c{i}') for i in (1, 2, 3)])
        out = {}
        segs = list(epub_segments(str(path), out=out))
        assert {s.node for s in segs} == {0, 1, 2}
        for s in segs:  # every segment holds one chapter's text only: the one it says it is in
            assert {w[0:2] for w in s.text.split() if w.startswith('c') and 'w' in w} <= {f'c{s.node + 1}'}
        assert [s.section for s in segs if s.node == 1][0] == 'CAPITULO 2'
        assert [n['title'] for n in out['structure']] == ['CAPITULO 1', 'CAPITULO 2', 'CAPITULO 3']
        assert all(n['part'] == 'body' for n in out['structure'])

    def test_a_chapter_goes_on_across_files_until_another_starts(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub_with_outline(path, {'a.xhtml': f'<h2 id="x">Um</h2><p>{words(300, "a")}</p>', 'b.xhtml': f'<p>{words(300, "b")}</p>', 'c.xhtml': f'<h2>Dois</h2><p>{words(300, "c")}</p>'},
                          [point(1, 'Um', 'a.xhtml#x'), point(2, 'Dois', 'c.xhtml')])
        segs = list(epub_segments(str(path)))
        nodes = {s.locator['href']: s.node for s in segs}
        assert nodes == {'a.xhtml': 0, 'b.xhtml': 0, 'c.xhtml': 1}

    def test_nested_outlines_keep_their_depth(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub_with_outline(path, {'a.xhtml': f'<h1 id="l">Livro</h1><p>{words(200, "a")}</p><h2 id="c">Cap</h2><p>{words(400, "b")}</p>'},
                          [point(1, 'Livro Um', 'a.xhtml#l', point(2, 'Capítulo I', 'a.xhtml#c'))])
        out = {}
        list(epub_segments(str(path), out=out))
        assert [(n['title'], n['depth']) for n in out['structure']] == [('Livro Um', 0), ('Capítulo I', 1)]

    def test_no_table_of_contents_is_no_structure_and_the_heading_is_the_section_as_before(self, tmp_path):
        path = tmp_path / 'a.epub'
        make_epub(path, {'a.xhtml': '<h1>Capítulo A</h1><p>Texto.</p>'})
        out = {}
        segs = list(epub_segments(str(path), out=out))
        assert 'structure' not in out and segs[0].node is None and segs[0].section == 'Capítulo A'

    def test_an_anchor_that_is_not_there_puts_the_node_at_the_start_of_its_file(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub_with_outline(path, {'a.xhtml': f'<p>{words(300, "a")}</p>'}, [point(1, 'Um', 'a.xhtml#missing')])
        assert {s.node for s in epub_segments(str(path))} == {0}

    def test_the_progression_still_says_how_far_into_the_file(self, tmp_path):
        path = tmp_path / 'a.epub'
        body = ''.join(f'<h2 id="c{n}">C{n}</h2><p>{words(400, "c" + str(n))}</p>' for n in (1, 2))
        epub_with_outline(path, {'all.xhtml': body}, [point(i, f'C{i}', f'all.xhtml#c{i}') for i in (1, 2)])
        progression = [s.locator['progression'] for s in epub_segments(str(path))]
        assert progression == sorted(progression) and progression[0] == 0.0 and 0.3 < [s for s in progression if s > 0.2][0] < 0.7

    def test_an_outline_the_archive_cannot_read_is_no_outline(self, tmp_path):
        path = tmp_path / 'a.epub'
        make_epub(path, {'a.xhtml': '<p>Texto.</p>'}, extra={'OEBPS/toc.ncx': '<<< not xml'})
        assert [s.node for s in epub_segments(str(path))] == [None]


class TestPdfOutline:
    def pdf_with_bookmarks(self, path, pages, toc):
        doc = fitz.open()
        for text in pages:
            page = doc.new_page()
            page.insert_text((72, 100), text, fontsize=11)
        doc.set_toc(toc)
        doc.save(str(path))
        doc.close()

    def test_pages_are_in_the_node_of_the_last_bookmark_at_or_before_them(self, tmp_path):
        path = tmp_path / 'a.pdf'
        self.pdf_with_bookmarks(path, [f'Texto da página {i} com o bastante para contar.' for i in range(5)],
                                [[1, 'Prefácio', 1], [1, 'Capítulo 1', 2], [2, 'Seção', 3], [1, 'Capítulo 2', 5]])
        out = {}
        segs = list(pdf_segments(str(path), out=out))
        assert [(s.locator['page'], s.node) for s in segs] == [(0, 0), (1, 1), (2, 2), (3, 2), (4, 3)]
        assert [(n['title'], n['depth']) for n in out['structure']] == [('Prefácio', 0), ('Capítulo 1', 0), ('Seção', 1), ('Capítulo 2', 0)]

    def test_a_page_before_the_first_bookmark_is_in_no_node(self, tmp_path):
        path = tmp_path / 'a.pdf'
        self.pdf_with_bookmarks(path, ['Capa com texto suficiente para contar.', 'Página com o bastante para contar também.'], [[1, 'Capítulo 1', 2]])
        assert [s.node for s in pdf_segments(str(path))] == [None, 0]

    def test_bookmarks_listed_out_of_order_cannot_send_a_page_back(self, tmp_path):
        path = tmp_path / 'a.pdf'
        self.pdf_with_bookmarks(path, [f'Texto da página {i} com o bastante para contar.' for i in range(4)], [[1, 'B', 3], [1, 'A', 2]])
        assert [s.node for s in pdf_segments(str(path))] == [None, None, 1, 1]

    def test_no_bookmarks_is_no_structure(self, tmp_path):
        path = tmp_path / 'a.pdf'
        make_pdf(path, ['Texto com o bastante para contar.'])
        out = {}
        assert [s.node for s in pdf_segments(str(path), out=out)] == [None] and 'structure' not in out


class TestIndexerStructure:
    def test_the_structure_is_published_with_the_text_and_the_segments_carry_their_node(self, tmp_path):
        epub_with_outline(tmp_path / 'b.epub', {'a.xhtml': f'<h2 id="a">A</h2><p>{words(300, "a")}</p><h2 id="b">B</h2><p>{words(300, "b")}</p>'},
                          [point(1, 'A', 'a.xhtml#a'), point(2, 'B', 'a.xhtml#b')])
        db = FakeDB([file_row(7, 'epub', 'aa', 'pt', path='b.epub')])
        assert TextIndexer(db, str(tmp_path)).run(9) == {7: 'ready'}
        assert {row[-1] for row in db.rows} == {0, 1}
        publish = [c for c in db.calls if c[1] == 'text_extraction_publish'][0][2]
        structure = json.loads(publish[-1])
        assert [n['title'] for n in structure] == ['A', 'B']


class TestEpubOutlineTies:
    def test_when_a_chapter_and_its_section_open_at_the_same_place_the_deeper_one_holds_the_text(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub_with_outline(path, {'a.xhtml': f'<h1 id="x">Livro</h1><p>{words(300, "a")}</p>'},
                          [point(1, 'Livro', 'a.xhtml#x', point(2, 'Capítulo I', 'a.xhtml#x'))])
        assert {s.node for s in epub_segments(str(path))} == {1}
