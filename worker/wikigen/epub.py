"""An EPUB as chapters and paragraphs, in the order of its spine: what the generator reads.

A chapter is a file of the spine with enough text; covers, tables of contents and almost empty pages are left out. The title and the part
come from the table of contents (toc.ncx or nav.xhtml). A short file the table does not mention is the epigraph of the chapter that
follows, and goes with it. Each chapter keeps the `href` of the file it starts in, which is how the backend ties it to the text it
extracted itself.
"""
import posixpath
import re
import zipfile
from urllib.parse import unquote

from lxml import etree, html

from textindex.epub import _encoding

MIN_CHARS = 400
BLOCKS = {'p', 'blockquote', 'li', 'h1', 'h2', 'h3', 'h4', 'div'}
MAX_PARAGRAPH_CHARS = 1800
SKIP_TITLE = re.compile(
    r'^(capa|cover|folha de rosto|title ?page|sum[aá]rio|contents|table of contents|cr[eé]ditos|credits|agradecimentos|acknowledg(e)?ments|'
    r'sobre o autor|about the author|obras do autor|also by|ficha t[eé]cnica|copyright|dedicat[oó]ria|dedication)\b', re.I)


def _parser():
    return etree.XMLParser(recover=True, resolve_entities=False, no_network=True)


def _opf(zf):
    container = etree.fromstring(zf.read('META-INF/container.xml'), parser=_parser())
    path = container.xpath('//*[local-name()="rootfile"]/@full-path')[0]
    return path, etree.fromstring(zf.read(path), parser=_parser())


def _join(base, href):
    return posixpath.normpath(posixpath.join(base, unquote(href.split('#')[0])))


def _spine(opf, base):
    manifest = {i.get('id'): i.get('href') for i in opf.xpath('//*[local-name()="manifest"]/*[local-name()="item"]')}
    title = ' '.join((opf.xpath('//*[local-name()="title"]/text()') or [''])[0].split())
    names = []
    for ref in opf.xpath('//*[local-name()="spine"]/*[local-name()="itemref"]'):
        href = manifest.get(ref.get('idref'))
        if href:
            names.append(_join(base, href))
    return title, names


def _toc(zf, opf, base):
    """{file: (chapter title, part title)} from toc.ncx (EPUB 2) or nav.xhtml (EPUB 3). A file with several items keeps the first."""
    titles = {}

    def label(el, xp):
        return ' '.join(''.join(el.xpath(xp)).split())

    ncx = opf.xpath('//*[local-name()="manifest"]/*[local-name()="item"][@media-type="application/x-dtbncx+xml"]/@href')
    if ncx:
        try:
            ncx_path = _join(base, ncx[0])
            root = etree.fromstring(zf.read(ncx_path), parser=_parser())
            ncx_base = posixpath.dirname(ncx_path)

            def walk(points, part):
                for p in points:
                    name = label(p, './*[local-name()="navLabel"]//text()')
                    src = (p.xpath('./*[local-name()="content"]/@src') or [''])[0]
                    kids = p.xpath('./*[local-name()="navPoint"]')
                    if src:
                        titles.setdefault(_join(ncx_base, src), (name, part))
                    walk(kids, name if kids else part)
            walk(root.xpath('/*/*[local-name()="navMap"]/*[local-name()="navPoint"]'), '')
            current = ''
            for p in root.xpath('/*/*[local-name()="navMap"]/*[local-name()="navPoint"]'):
                name = label(p, './*[local-name()="navLabel"]//text()')
                src = (p.xpath('./*[local-name()="content"]/@src') or [''])[0]
                if re.match(r'^(parte|part|livro|book)\s', name, re.I):
                    current = name
                elif src:
                    full = _join(ncx_base, src)
                    if full in titles and not titles[full][1]:
                        titles[full] = (titles[full][0], current)
        except (KeyError, etree.XMLSyntaxError):
            pass
    if not titles:
        nav = opf.xpath('//*[local-name()="manifest"]/*[local-name()="item"][contains(@properties, "nav")]/@href')
        if nav:
            try:
                nav_path = _join(base, nav[0])
                root = html.fromstring(zf.read(nav_path))
                nav_base = posixpath.dirname(nav_path)
                for a in root.xpath('//nav//a[@href]'):
                    titles.setdefault(_join(nav_base, a.get('href')), (' '.join(a.text_content().split()), ''))
            except (KeyError, etree.ParserError):
                pass
    return titles


def _paragraphs(markup):
    if not markup.strip():
        return '', []
    # left to itself the HTML parser guesses Latin-1 for a document that declares nothing; XHTML without a declaration is UTF-8
    parser = html.HTMLParser(recover=True, no_network=True, remove_comments=True, encoding=_encoding(markup))
    root = html.document_fromstring(markup, parser=parser)
    for bad in root.xpath('//script|//style'):
        bad.getparent().remove(bad)
    heading = ''
    for h in root.xpath('//h1|//h2|//h3'):
        heading = ' '.join(h.text_content().split())
        if heading:
            break
    out = []
    for el in root.iter():
        if not isinstance(el.tag, str) or el.tag.lower() not in BLOCKS:
            continue
        if any(isinstance(c.tag, str) and c.tag.lower() in BLOCKS for c in el):
            continue   # a block that holds other blocks is not a paragraph: the text is the children's
        # soft hyphens and zero-width characters: some EPUBs come with the hyphenation done, which breaks the search and the quotes
        text = ' '.join(el.text_content().replace('­', '').replace('​', '').replace('﻿', '').split())
        if text:
            out.append(text[:MAX_PARAGRAPH_CHARS])
    return heading, out


def read(path):
    """{'title', 'chapters': [{'index', 'id', 'href', 'title', 'part', 'epigraph_paragraphs', 'paragraphs'}]}, index from 1."""
    raw = []
    with zipfile.ZipFile(path) as zf:
        opf_path, opf = _opf(zf)
        base = posixpath.dirname(opf_path)
        book_title, names = _spine(opf, base)
        toc = _toc(zf, opf, base)
        for name in names:
            try:
                markup = zf.read(name)
            except KeyError:
                continue
            heading, paras = _paragraphs(markup)
            if sum(len(p) for p in paras) < MIN_CHARS:
                continue
            known = name in toc
            title, part = toc.get(name, (heading or posixpath.basename(name), ''))
            raw.append({'id': name, 'title': title or heading or posixpath.basename(name), 'part': part, 'known': known, 'paragraphs': paras})
    # a short file the table does not cite is the epigraph (or a loose page) of the chapter that comes next: it takes its name
    for i, ch in enumerate(raw):
        if not ch['known']:
            nxt = next((c for c in raw[i + 1:] if c['known']), None)
            if nxt:
                ch['epigraph_of'] = nxt['title']
                ch['part'] = nxt['part']
    chapters, pending = [], []
    for ch in raw:
        if ch.get('epigraph_of'):
            pending.append(ch)
            continue
        if SKIP_TITLE.match(ch['title']):
            pending = []
            continue
        epigraph = [p for e in pending for p in e['paragraphs']]
        first = pending[0] if pending else ch
        chapters.append({'index': len(chapters) + 1, 'id': ch['id'], 'href': first['id'], 'title': ch['title'], 'part': ch['part'],
                         'epigraph_paragraphs': len(epigraph), 'paragraphs': epigraph + ch['paragraphs']})
        pending = []
    return {'title': book_title, 'chapters': chapters}
