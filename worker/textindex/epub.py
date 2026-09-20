"""The text of an EPUB, chapter by chapter, in the order of the book (its spine)."""
import posixpath
import re
import zipfile
from urllib.parse import unquote

from lxml import etree, html

from . import Limits, Segment
from .normalize import chunk, clean

_HTML_TYPES = ('application/xhtml+xml', 'text/html', 'application/x-dtbook+xml')
_SKIP = {'script', 'style', 'head', 'title', 'nav', 'noscript', 'svg', 'math'}
_BLOCK = {
    'p', 'div', 'br', 'hr', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'li', 'ul', 'ol', 'tr', 'table', 'blockquote',
    'section', 'article', 'aside', 'pre', 'figure', 'figcaption', 'dt', 'dd', 'header', 'footer', 'body',
}
_HEADINGS = {'h1', 'h2', 'h3'}


def _opf_path(zf):
    try:
        root = etree.fromstring(zf.read('META-INF/container.xml'), parser=etree.XMLParser(recover=True, resolve_entities=False))
        rootfile = root.find('.//{*}rootfile')
        if rootfile is not None and rootfile.get('full-path'):
            return rootfile.get('full-path')
    except KeyError:
        pass
    for name in zf.namelist():
        if name.lower().endswith('.opf'):
            return name
    raise ValueError('not an EPUB: no package file')


_DECLARED = re.compile(rb'^\s*<\?xml[^>]*encoding=["\']([\w.-]+)["\']', re.I)
_META = re.compile(rb'<meta[^>]+charset=["\']?([\w.-]+)', re.I)


def _encoding(data: bytes) -> str:
    """The encoding an XHTML document declares (XML declaration, then <meta>), else UTF-8, which is
    what XML says it is when nothing is declared. Left to itself the HTML parser guesses Latin-1."""
    if data.startswith(b'\xef\xbb\xbf'):
        return 'utf-8'
    for pattern in (_DECLARED, _META):
        match = pattern.search(data[:2048])
        if match:
            name = match.group(1).decode('ascii', 'ignore')
            try:
                ''.encode(name)
                return name
            except LookupError:
                break
    return 'utf-8'


def _paragraphs(data: bytes):
    """The paragraphs of one XHTML document and its first heading."""
    parser = html.HTMLParser(recover=True, no_network=True, remove_comments=True, huge_tree=False, encoding=_encoding(data))
    root = html.document_fromstring(data, parser=parser) if data.strip() else None
    if root is None:
        return [], None
    paragraphs, current, heading = [], [], None

    def flush():
        text = clean(''.join(current))
        current.clear()
        if text:
            paragraphs.append(text)

    def walk(el):
        nonlocal heading
        tag = el.tag if isinstance(el.tag, str) else ''
        if tag in _SKIP:
            return
        block = tag in _BLOCK
        if block:
            flush()
        start = len(paragraphs)
        if el.text:
            current.append(el.text)
        for child in el:
            walk(child)
            if child.tail:
                current.append(child.tail)
        if block:
            flush()
        if tag in _HEADINGS and heading is None and len(paragraphs) > start:
            heading = paragraphs[start]

    walk(root)
    flush()
    return paragraphs, heading


def epub_segments(path, checkpoint=lambda: None):
    try:
        zf = zipfile.ZipFile(path)
    except zipfile.BadZipFile as err:
        raise ValueError(f'not a readable EPUB: {err}')
    with zf:
        infos = zf.infolist()
        if sum(i.file_size for i in infos) > Limits.MAX_ARCHIVE_BYTES:
            raise ValueError('the EPUB expands into too much data')
        opf = _opf_path(zf)
        opf_dir = posixpath.dirname(opf)
        try:
            package = etree.fromstring(zf.read(opf), parser=etree.XMLParser(recover=True, resolve_entities=False))
        except (KeyError, etree.XMLSyntaxError) as err:
            raise ValueError(f'the package file cannot be read: {err}')

        manifest = {}
        for item in package.iterfind('.//{*}manifest/{*}item'):
            if item.get('id') and item.get('href'):
                manifest[item.get('id')] = (item.get('href'), item.get('media-type', ''))
        spine = [ref.get('idref') for ref in package.iterfind('.//{*}spine/{*}itemref')]
        if len(spine) > Limits.MAX_SPINE_ITEMS:
            raise ValueError('the EPUB has too many chapters')

        total = 0
        for idref in spine:
            checkpoint()
            entry = manifest.get(idref)
            if not entry or entry[1] not in _HTML_TYPES:
                continue
            href = entry[0].split('#')[0]
            member = posixpath.normpath(posixpath.join(opf_dir, unquote(href)))
            try:
                info = zf.getinfo(member)
            except KeyError:
                continue  # a chapter the package lists and the archive does not have
            if info.file_size > Limits.MAX_ENTRY_BYTES:
                raise ValueError(f'a chapter is too large: {href}')
            paragraphs, heading = _paragraphs(zf.read(info))
            length = sum(len(p) + 1 for p in paragraphs) or 1
            for text, start in chunk(paragraphs):
                total += len(text)
                if total > Limits.MAX_CHARS:
                    raise ValueError('too much text in one file')
                yield Segment(
                    text=text,
                    section=heading or None,
                    locator={'type': 'epub', 'href': href, 'progression': round(min(1.0, start / length), 3)},
                )
