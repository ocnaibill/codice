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


# --- what the file says it is (epub:type) ---

OPS = 'xmlns:epub="http://www.idpf.org/2007/ops"'


def epub3(path, docs, toc, landmarks=(), body_types=None, toc_types=None):
    """An EPUB 3. docs: {href: inner xhtml}; toc: [(title, href[#fragment])] in the navigation document;
    landmarks: [(epub:type, href)]; body_types: {href: epub:type of its <body>}."""
    body_types = body_types or {}
    manifest = ''.join(f'<item id="d{i}" href="{h}" media-type="application/xhtml+xml"/>' for i, h in enumerate(docs))
    spine = ''.join(f'<itemref idref="d{i}"/>' for i in range(len(docs)))
    opf = ('<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata/><manifest>'
           f'<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>{manifest}</manifest><spine>{spine}</spine></package>')
    toc_types = toc_types or {}
    toc_items = ''.join(f'<li><a href="{h}"' + (f' epub:type="{toc_types[h]}"' if h in toc_types else '') + f'>{t}</a></li>' for t, h in toc)
    marks = ''.join(f'<li><a epub:type="{k}" href="{h}">x</a></li>' for k, h in landmarks)
    nav = (f'<html xmlns="http://www.w3.org/1999/xhtml" {OPS}><body><nav epub:type="toc"><ol>{toc_items}</ol></nav>'
           f'<nav epub:type="landmarks"><ol>{marks}</ol></nav></body></html>')
    with zipfile.ZipFile(path, 'w') as zf:
        zf.writestr('mimetype', 'application/epub+zip')
        zf.writestr('META-INF/container.xml', '<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">'
                    '<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>')
        zf.writestr('OEBPS/content.opf', opf)
        zf.writestr('OEBPS/nav.xhtml', nav)
        for href, inner in docs.items():
            attr = f' epub:type="{body_types[href]}"' if href in body_types else ''
            zf.writestr(f'OEBPS/{href}', f'<html xmlns="http://www.w3.org/1999/xhtml" {OPS}><head><title>x</title></head><body{attr}>{inner}</body></html>')


def parts_of(path):
    out = {}
    list(epub_segments(str(path), out=out))
    return {n['title']: n['part'] for n in out['structure']}


class TestDeclaredPart:
    def test_the_tokens_say_which_end_of_the_book_it_is(self):
        assert structure.declared_part(['bodymatter']) == 'body'
        assert structure.declared_part(['frontmatter', 'preface']) == 'front'
        assert structure.declared_part(['backmatter']) == 'back'
        for kind in ('cover', 'titlepage', 'dedication', 'toc', 'foreword', 'copyright-page', 'epigraph'):
            assert structure.declared_part([kind]) == 'front', kind
        for kind in ('appendix', 'glossary', 'index', 'bibliography', 'colophon', 'afterword', 'endnotes', 'errata'):
            assert structure.declared_part([kind]) == 'back', kind
        assert structure.declared_part(['chapter']) == 'body' and structure.declared_part(['part']) == 'body'

    def test_what_puts_it_outside_the_story_wins_over_what_puts_it_in(self):
        assert structure.declared_part(['chapter', 'appendix']) == 'back'
        assert structure.declared_part(['bodymatter', 'preface']) == 'front'
        assert structure.declared_part(['preface', 'appendix']) == 'back'

    def test_a_token_that_does_not_say_where_it_is_says_nothing(self):
        for tokens in ([], None, ['introduction'], ['acknowledgments'], ['z3998:poem'], [''], ['  ']):
            assert structure.declared_part(tokens) is None, tokens
        assert structure.declared_part([' BodyMatter ']) == 'body'


class TestClassifyDeclared:
    def node(self, title, chars, declared=None, depth=0):
        return {'title': title, 'depth': depth, 'chars': chars, 'declared': declared}

    def test_a_node_that_says_what_it_is_is_not_guessed_from_its_name(self):
        nodes = [self.node('Capítulo I', 9000), self.node('Capítulo II', 9000), self.node('Quem é quem', 5000, 'back')]
        # Without the declaration the last node, as large as a chapter, is a chapter.
        undeclared = [{k: v for k, v in n.items() if k != 'declared'} for n in nodes]
        assert [n['part'] for n in structure.classify(undeclared)] == ['body', 'body', 'body']
        assert [n['part'] for n in structure.classify(nodes)] == ['body', 'body', 'back']

    def test_a_front_declared_node_in_the_middle_of_the_story_stays_front(self):
        nodes = [self.node('Capítulo I', 9000), self.node('Nota', 400, 'front'), self.node('Capítulo II', 9000)]
        assert [n['part'] for n in structure.classify(nodes)] == ['body', 'front', 'body']

    def test_a_short_node_that_declares_itself_the_story_is_the_story(self):
        nodes = [self.node('Capa', 30), self.node('Um', 9000), self.node('Dois', 9000), self.node('Epílogo', 300, 'body'), self.node('Notas', 800)]
        assert [n['part'] for n in structure.classify(nodes)] == ['front', 'body', 'body', 'body', 'back']

    def test_a_node_called_appendix_that_says_it_is_the_story_is_the_story(self):
        nodes = [self.node('Um', 9000), self.node('Apêndice', 9000, 'body'), self.node('Dois', 9000)]
        assert [n['part'] for n in structure.classify(nodes)] == ['body', 'body', 'body']

    def test_the_declaration_is_taken_out_of_the_node(self):
        nodes = structure.classify([self.node('Um', 9000, 'body'), self.node('Dois', 9000)])
        assert all(set(n) == {'title', 'depth', 'chars', 'part'} for n in nodes)

    def test_what_declares_nothing_is_decided_as_before(self):
        nodes = [self.node('Título', 40), self.node('Sumário', 300), self.node('Capítulo I', 9000), self.node('Capítulo II', 9000), self.node('Glossário', 3000)]
        assert [n['part'] for n in structure.classify(nodes)] == ['front', 'front', 'body', 'body', 'back']

    def test_short_nodes_that_declare_themselves_the_story_say_where_it_ends(self):
        # Too short to be taken for chapters by their size: what is declared is where the story is, and what follows is the end.
        nodes = [self.node('Um', 300, 'body'), self.node('Dois', 300, 'body'), self.node('Despedida', 200)]
        assert [n['part'] for n in structure.classify(nodes)] == ['body', 'body', 'back']

    def test_a_node_nested_under_one_that_is_not_the_story_is_not_the_story(self):
        nodes = [self.node('Um', 9000, 'body'), self.node('Anexos', 100, 'back'), self.node('Parte', 9000, 'body', depth=1)]
        assert [n['part'] for n in structure.classify(nodes)] == ['body', 'back', 'back']


class TestEpubDeclared:
    def text(self, tag, n=400):
        return f'<p>{words(n, tag)}</p>'

    def test_a_document_that_declares_itself_in_its_body_is_what_it_says(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub3(path, {'capa.xhtml': self.text('capa', 20), 'c1.xhtml': self.text('um'), 'c2.xhtml': self.text('dois'), 'quem.xhtml': self.text('quem')},
              [('Capa', 'capa.xhtml'), ('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml'), ('Quem é quem', 'quem.xhtml')],
              body_types={'capa.xhtml': 'cover', 'c1.xhtml': 'bodymatter chapter', 'c2.xhtml': 'bodymatter chapter', 'quem.xhtml': 'backmatter'})
        assert parts_of(path) == {'Capa': 'front', 'Um': 'body', 'Dois': 'body', 'Quem é quem': 'back'}

    def test_the_same_book_without_the_declaration_is_guessed_from_the_titles(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub3(path, {'c1.xhtml': self.text('um', 2000), 'c2.xhtml': self.text('dois', 2000), 'quem.xhtml': self.text('quem', 2000)},
              [('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml'), ('Quem é quem', 'quem.xhtml')])
        assert parts_of(path) == {'Um': 'body', 'Dois': 'body', 'Quem é quem': 'body'}

    def test_the_section_that_is_all_of_a_document_says_it_when_the_body_does_not(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub3(path, {'c1.xhtml': f'<section epub:type="chapter">{self.text("um", 2000)}</section>',
                     'c2.xhtml': f'<section epub:type="chapter">{self.text("dois", 2000)}</section>',
                     'quem.xhtml': f'<section epub:type="appendix">{self.text("quem", 2000)}</section>'},
              [('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml'), ('Quem é quem', 'quem.xhtml')])
        assert parts_of(path) == {'Um': 'body', 'Dois': 'body', 'Quem é quem': 'back'}

    def test_a_section_that_does_not_open_the_document_does_not_speak_for_it(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub3(path, {'c1.xhtml': self.text('um', 2000) + f'<section epub:type="appendix">{self.text("x", 100)}</section>',
                     'c2.xhtml': self.text('dois', 2000)},
              [('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml')])
        assert parts_of(path) == {'Um': 'body', 'Dois': 'body'}

    def test_a_node_that_points_into_a_document_is_what_the_section_it_points_to_says(self, tmp_path):
        path = tmp_path / 'a.epub'
        inner = (f'<section id="um" epub:type="chapter">{self.text("um", 2000)}</section>'
                 f'<section id="notas" epub:type="endnotes">{self.text("notas", 300)}</section>')
        epub3(path, {'tudo.xhtml': inner, 'dois.xhtml': self.text('dois', 2000)}, [('Um', 'tudo.xhtml#um'), ('Notas', 'tudo.xhtml#notas'), ('Dois', 'dois.xhtml')])
        assert parts_of(path) == {'Um': 'body', 'Notas': 'back', 'Dois': 'body'}

    def test_a_document_declared_back_gives_its_sections_the_same_when_they_say_nothing(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub3(path, {'c1.xhtml': self.text('um', 2000), 'c2.xhtml': self.text('dois', 2000),
                     'anexos.xhtml': f'<h2 id="a">A</h2>{self.text("a", 500)}<h2 id="b">B</h2>{self.text("b", 500)}'},
              [('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml'), ('A', 'anexos.xhtml#a'), ('B', 'anexos.xhtml#b')],
              body_types={'anexos.xhtml': 'backmatter'})
        assert parts_of(path) == {'Um': 'body', 'Dois': 'body', 'A': 'back', 'B': 'back'}

    def test_the_landmarks_say_where_the_story_starts(self, tmp_path):
        path = tmp_path / 'a.epub'
        # "Entrada" is long and neutral: by its name and size it would be the first chapter.
        epub3(path, {'entrada.xhtml': self.text('entrada', 2000), 'c1.xhtml': self.text('um', 2000), 'c2.xhtml': self.text('dois', 2000)},
              [('Entrada', 'entrada.xhtml'), ('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml')], landmarks=[('bodymatter', 'c1.xhtml')])
        assert parts_of(path) == {'Entrada': 'front', 'Um': 'body', 'Dois': 'body'}

    def test_and_where_it_ends(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub3(path, {'c1.xhtml': self.text('um', 2000), 'c2.xhtml': self.text('dois', 2000), 'fim.xhtml': self.text('fim', 2000)},
              [('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml'), ('Despedida', 'fim.xhtml')], landmarks=[('bodymatter', 'c1.xhtml'), ('backmatter', 'fim.xhtml')])
        assert parts_of(path) == {'Um': 'body', 'Dois': 'body', 'Despedida': 'back'}

    def test_what_a_document_says_of_itself_wins_over_the_landmarks(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub3(path, {'entrada.xhtml': self.text('entrada', 2000), 'c1.xhtml': self.text('um', 2000), 'c2.xhtml': self.text('dois', 2000)},
              [('Entrada', 'entrada.xhtml'), ('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml')], landmarks=[('bodymatter', 'c1.xhtml')],
              body_types={'entrada.xhtml': 'chapter'})
        assert parts_of(path)['Entrada'] == 'body'

    def test_an_epub_type_on_a_link_of_the_table_of_contents_is_not_a_landmark(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub3(path, {'c1.xhtml': self.text('um', 2000), 'c2.xhtml': self.text('dois', 2000)}, [('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml')],
              toc_types={'c2.xhtml': 'bodymatter'})
        assert parts_of(path) == {'Um': 'body', 'Dois': 'body'}

    def test_a_landmark_that_points_nowhere_in_the_book_says_nothing(self, tmp_path):
        path = tmp_path / 'a.epub'
        epub3(path, {'c1.xhtml': self.text('um', 2000), 'c2.xhtml': self.text('dois', 2000)}, [('Um', 'c1.xhtml'), ('Dois', 'c2.xhtml')],
              landmarks=[('bodymatter', 'outro.xhtml'), ('backmatter', 'sumiu.xhtml')])
        assert parts_of(path) == {'Um': 'body', 'Dois': 'body'}
