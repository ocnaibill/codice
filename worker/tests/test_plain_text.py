"""Descriptions come from the file as plain text, never as HTML (#58)."""
import os
import re
import zipfile

import pytest

from analyzer import Analyzer
from extractors.epub_extractor import EpubExtractor
from extractors.plain_text import MARKUP, plain_description
from maintenance import DESCRIPTIONS_FLAG, repair_descriptions_on
from pipeline import analyze_file
from tests.test_analyzer import FakeDB
from tests.test_pipeline import FakeExtractor, FakeProviders, meta, record


class TestPlainDescription:
    def test_the_description_that_was_seen_in_a_real_epub(self):
        raw = ('<b>Set on the desert planet Arrakis</b>, Dune is the story of the boy Paul Atreides &#8212; heir to a '
               'noble family.<br/><br/>&ldquo;A grand <i>and</i> shining epic.&rdquo; &mdash; <i>Chicago Tribune</i>'
               '<p>Caf&eacute; &amp; &aring;ngstr&ouml;m.</p>')
        assert plain_description(raw) == (
            'Set on the desert planet Arrakis, Dune is the story of the boy Paul Atreides — heir to a noble family.\n\n'
            '“A grand and shining epic.” — Chicago Tribune\n\nCafé & ångström.')

    def test_paragraphs_and_line_breaks_stay_as_breaks(self):
        assert plain_description('<p>One</p><p>Two</p>') == 'One\n\nTwo'
        assert plain_description('One<br>Two<br />Three<BR/>Four') == 'One\nTwo\nThree\nFour'
        assert plain_description('<ul><li>a</li><li>b</li></ul>') == 'a\nb'
        assert plain_description('<div>One</div><div>Two</div>') == 'One\n\nTwo'

    def test_entities_named_numeric_and_hexadecimal_are_read(self):
        assert plain_description('a&nbsp;b &#8212; &#x2014; &eacute; &aring; &quot;q&quot; &apos;') == 'a b — — é å "q" \''

    def test_what_has_no_markup_comes_back_the_same(self):
        for text in ('Uma sinopse simples.', 'Duas linhas\nsem marcação.', 'Primeiro.\n\nSegundo.', 'Q&A and AT&T and R&D',
                     '5 < 6 and 7 > 3', 'I <3 books', 'a<b', 'curly {braces} and [brackets]'):
            assert plain_description(text) == text

    def test_the_spaces_are_tidied(self):
        assert plain_description('  a   b\t c  \n\n\n\n d e \r\n f ') == 'a b c\n\nd e\nf'

    def test_what_is_not_text_goes_with_what_is_inside_it(self):
        assert plain_description('before<script>alert("x")</script>after') == 'before after'
        assert plain_description('a<style>p{color:red}</style>b<!-- hidden -->c') == 'a b c'
        assert plain_description('<SCRIPT type="text/javascript">x()</SCRIPT>ok') == 'ok'

    def test_tags_with_attributes_and_any_case_go(self):
        assert plain_description('<A HREF="http://x/?a=1&amp;b=2" onclick="evil()">link</A> <img src=x onerror=alert(1)>') == 'link'

    def test_escaped_markup_is_text_and_stays_text(self):
        # Written as "&lt;b&gt;" it was never a tag; it is shown as what it says, and never rendered.
        assert plain_description('use &lt;b&gt;bold&lt;/b&gt; here') == 'use <b>bold</b> here'
        assert plain_description('&lt;script&gt;alert(1)&lt;/script&gt;') == '<script>alert(1)</script>'

    def test_malformed_markup_and_entities_do_not_break_it(self):
        assert plain_description('open <b never closed') == 'open <b never closed'
        assert plain_description('a <b>bold <i>nested</b> text') == 'a bold nested text'
        assert plain_description('bad &bogus; and &#99999999; and &amp and &') == 'bad &bogus; and � and & and &'
        assert plain_description('<<b>>x') == '<>x'

    def test_control_characters_are_dropped(self):
        assert plain_description('a\x00b\x07c\x1fd\x7fe') == 'abcde'

    def test_nothing_in_nothing_out(self):
        for empty in (None, '', '   ', '<p></p>', '<br/>', '<!-- only a comment -->'):
            assert plain_description(empty) == ''

    def test_the_pattern_that_finds_markup_finds_what_the_cleaning_changes(self):
        for raw in ('<b>x</b>', 'a &eacute; b', 'a &#8212; b', 'a &#x2014; b', 'x<br/>y'):
            assert re.search(MARKUP, raw)
        for plain in ('Uma sinopse.', 'Q&A and AT&T', '5 < 6', 'I <3 books', 'a & b'):
            assert not re.search(MARKUP, plain)


def build_epub(tmp_path, description_xml):
    path = tmp_path / 'b.epub'
    with zipfile.ZipFile(path, 'w') as zf:
        zf.writestr('mimetype', 'application/epub+zip')
        zf.writestr('META-INF/container.xml',
                    '<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">'
                    '<rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>')
        zf.writestr('content.opf', f'''<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:xhtml="http://www.w3.org/1999/xhtml">
    <dc:title>Dune</dc:title><dc:creator>Frank Herbert</dc:creator>
    <dc:description>{description_xml}</dc:description>
  </metadata>
  <manifest><item id="c" href="c.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c"/></spine>
</package>''')
        zf.writestr('c.xhtml', '<html><body><p>x</p></body></html>')
    return str(path)


class TestFromARealEpub:
    def description_kept(self, tmp_path, description_xml):
        covers = tmp_path / 'covers'
        covers.mkdir()
        db = FakeDB()
        analyze_file(7, str(tmp_path / 'b.epub'), EpubExtractor(), Analyzer(db), FakeProviders(), str(covers))
        return db

    def stored_description(self, db):
        query, params = db.work_update()
        names = [c.split('=')[0].strip() for c in query.split('SET', 1)[1].split('WHERE')[0].split(',')]
        return params[names.index('description')]

    def test_html_escaped_inside_the_description_is_kept_as_plain_text(self, tmp_path):
        build_epub(tmp_path, '&lt;b&gt;Set on Arrakis&lt;/b&gt; &amp;#8212; the story.&lt;br/&gt;&amp;eacute;&lt;/i&gt;')
        db = self.description_kept(tmp_path, None)
        assert self.stored_description(db) == 'Set on Arrakis — the story.\né'

    def test_xhtml_written_inside_the_description_is_all_kept(self, tmp_path):
        build_epub(tmp_path, '<xhtml:p>First paragraph.</xhtml:p><xhtml:p>Second <xhtml:b>one</xhtml:b>.</xhtml:p>')
        db = self.description_kept(tmp_path, None)
        assert self.stored_description(db) == 'First paragraph.\n\nSecond one.'

    def test_a_plain_description_is_untouched(self, tmp_path):
        build_epub(tmp_path, 'Uma sinopse simples.')
        assert self.stored_description(self.description_kept(tmp_path, None)) == 'Uma sinopse simples.'


class TestPipeline:
    def run(self, description, provider_description=None):
        db = FakeDB()
        providers = FakeProviders(record(description=provider_description) if provider_description else None)
        analyze_file(7, '/f/b.epub', FakeExtractor(meta(description=description)), Analyzer(db), providers, '/covers')
        return db

    def test_the_description_of_the_file_is_kept_as_plain_text(self):
        db = self.run('<p>One</p><p>Two &amp; three</p>')
        _, params = db.work_update()
        assert 'One\n\nTwo & three' in params
        assert not any(isinstance(p, str) and '<p>' in p for p in params)

    def test_a_suggestion_from_a_provider_is_plain_text_too(self):
        db = self.run('', provider_description='<p>From the <b>provider</b> &#8212; ok</p>')
        stored = [p for q, p in db.matching('INSERT INTO metadata_candidates') if p[1] == 'description']
        assert stored and stored[0][2] == 'From the provider — ok'


URL = os.environ.get('TEST_DATABASE_URL')


@pytest.mark.skipif(not URL, reason='TEST_DATABASE_URL is not set')
class TestRepairForReal:
    """The repair run against a PostgreSQL, on temporary tables with only what it touches."""

    @pytest.fixture
    def conn(self):
        import psycopg2
        conn = psycopg2.connect(URL)
        cur = conn.cursor()
        cur.execute("""
            CREATE TEMP TABLE works (id INTEGER PRIMARY KEY, description TEXT, description_lock BOOLEAN NOT NULL DEFAULT FALSE);
            CREATE TEMP TABLE settings (key TEXT PRIMARY KEY, value JSONB NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
        """)
        yield conn
        conn.rollback()
        conn.close()

    def seed(self, conn, rows):
        cur = conn.cursor()
        for work_id, description, locked in rows:
            cur.execute('INSERT INTO works (id, description, description_lock) VALUES (%s, %s, %s)', (work_id, description, locked))

    def descriptions(self, conn):
        cur = conn.cursor()
        cur.execute('SELECT id, description FROM works ORDER BY id')
        return dict(cur.fetchall())

    def test_only_what_still_has_the_markup_of_the_file_is_changed(self, conn):
        self.seed(conn, [
            (1, '<b>Arrakis</b> &#8212; a story.<br/>Next', False),
            (2, 'Uma sinopse simples.', False),
            (3, '<b>edited by hand and locked</b>', True),
            (4, 'Q&A and 5 < 6', False),
            (5, None, False),
            (6, '<p></p>', False),
        ])
        assert repair_descriptions_on(conn) == 1
        assert self.descriptions(conn) == {
            1: 'Arrakis — a story.\nNext', 2: 'Uma sinopse simples.', 3: '<b>edited by hand and locked</b>',
            4: 'Q&A and 5 < 6', 5: None, 6: '<p></p>'}  # one that would become empty is left as it was

    def test_it_is_done_once_and_never_decodes_a_text_twice(self, conn):
        # Written by the person's own file as "&amp;lt;": it says "&lt;", and a second pass would turn it into "<".
        self.seed(conn, [(1, 'use &amp;lt; for the sign', False)])
        assert repair_descriptions_on(conn) == 1
        assert self.descriptions(conn)[1] == 'use &lt; for the sign'
        assert repair_descriptions_on(conn) is None
        assert self.descriptions(conn)[1] == 'use &lt; for the sign'
        cur = conn.cursor()
        cur.execute('SELECT value FROM settings WHERE key = %s', (DESCRIPTIONS_FLAG,))
        assert cur.fetchone()[0] == {'done': True, 'changed': 1}

    def test_it_is_recorded_even_when_there_was_nothing_to_change(self, conn):
        self.seed(conn, [(1, 'Uma sinopse simples.', False)])
        assert repair_descriptions_on(conn) == 0
        assert repair_descriptions_on(conn) is None
