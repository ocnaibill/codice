"""The text of an EPUB, chapter by chapter, in the order of the book (its spine)."""
import posixpath
import re
import zipfile
from urllib.parse import unquote

from lxml import etree, html

from . import Limits, Segment
from .normalize import chunk, clean
from .structure import classify

_HTML_TYPES = ('application/xhtml+xml', 'text/html', 'application/x-dtbook+xml')
_SKIP = {'script', 'style', 'head', 'title', 'nav', 'noscript', 'svg', 'math'}
_BLOCK = {
    'p', 'div', 'br', 'hr', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'li', 'ul', 'ol', 'tr', 'table', 'blockquote',
    'section', 'article', 'aside', 'pre', 'figure', 'figcaption', 'dt', 'dd', 'header', 'footer', 'body',
}
_HEADINGS = {'h1', 'h2', 'h3'}
_OPS = 'http://www.idpf.org/2007/ops'
MAX_TITLE = 200


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


def _paragraphs(data: bytes, wanted=()):
    """The paragraphs of one XHTML document, its first heading, and where each wanted anchor is: the
    index of the paragraph that follows it (or holds it)."""
    parser = html.HTMLParser(recover=True, no_network=True, remove_comments=True, huge_tree=False, encoding=_encoding(data))
    root = html.document_fromstring(data, parser=parser) if data.strip() else None
    if root is None:
        return [], None, {}
    paragraphs, current, heading, anchors = [], [], None, {}

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
        if wanted:
            ident = el.get('id') or (el.get('name') if tag == 'a' else None)
            if ident and ident in wanted:
                anchors.setdefault(ident, len(paragraphs))
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
    return paragraphs, heading, anchors


def _target(base_dir, src):
    """(the archive member a table-of-contents link points at, its fragment)."""
    path, _, fragment = (src or '').partition('#')
    member = posixpath.normpath(posixpath.join(base_dir, unquote(path))) if path else None
    return member, unquote(fragment)


def _ncx_entries(data, base_dir):
    root = etree.fromstring(data, parser=etree.XMLParser(recover=True, resolve_entities=False))
    entries = []

    def walk(points, depth):
        for point in points:
            label, content = point.find('{*}navLabel/{*}text'), point.find('{*}content')
            if label is not None and content is not None and content.get('src'):
                member, fragment = _target(base_dir, content.get('src'))
                entries.append((clean(''.join(label.itertext()))[:MAX_TITLE], depth, member, fragment))
            walk(point.findall('{*}navPoint'), depth + 1)

    nav_map = root.find('.//{*}navMap') if root is not None else None
    walk(nav_map.findall('{*}navPoint') if nav_map is not None else [], 0)
    return entries


def _nav_entries(data, base_dir):
    root = etree.fromstring(data, parser=etree.XMLParser(recover=True, resolve_entities=False))
    entries = []
    if root is None:
        return entries

    def walk(ol, depth):
        for li in ol.findall('{*}li'):
            link = li.find('{*}a')
            if link is not None and link.get('href'):
                member, fragment = _target(base_dir, link.get('href'))
                entries.append((clean(''.join(link.itertext()))[:MAX_TITLE], depth, member, fragment))
            for child in li.findall('{*}ol'):
                walk(child, depth + 1)

    for nav in root.iter('{*}nav'):
        if 'toc' in (nav.get('{%s}type' % _OPS) or '').split():
            ol = nav.find('{*}ol')
            if ol is not None:
                walk(ol, 0)
            break
    return entries


def _table_of_contents(zf, package, opf_dir):
    """The book's own outline, in reading order: (title, depth, archive member, fragment). An EPUB 3
    navigation document or an EPUB 2 NCX, whichever lists more; none if the book has neither."""
    candidates = []
    for item in package.iterfind('.//{*}manifest/{*}item'):
        href, kind = item.get('href'), item.get('media-type', '')
        if not href:
            continue
        member = posixpath.normpath(posixpath.join(opf_dir, unquote(href)))
        try:
            if 'nav' in (item.get('properties') or '').split():
                candidates.append(_nav_entries(zf.read(member), posixpath.dirname(member)))
            elif kind == 'application/x-dtbncx+xml':
                candidates.append(_ncx_entries(zf.read(member), posixpath.dirname(member)))
        except (KeyError, etree.XMLSyntaxError):
            continue  # an outline that is not there or cannot be read is no outline
    return max(candidates, key=len, default=[])


def epub_segments(path, checkpoint=lambda: None, out=None):
    """Yields the segments of the book. When `out` is a dict, the outline's nodes are put in it under
    'structure' once every segment has been yielded (a segment carries the index of its node)."""
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

        outline = _table_of_contents(zf, package, opf_dir)
        by_member = {}
        for index, (_title, _depth, member, fragment) in enumerate(outline):
            if member:
                by_member.setdefault(member, []).append((index, fragment))
        chars = [0] * len(outline)
        current = None  # the node the text is in: it goes on across files until another starts

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
            here = by_member.get(member, [])
            paragraphs, heading, anchors = _paragraphs(zf.read(info), {f for _i, f in here if f})
            length = sum(len(p) + 1 for p in paragraphs) or 1

            # Where each node of the outline starts in this file; the last one listed at a place wins,
            # which is the deeper one when a chapter and its section open together.
            starts = {}
            for index, fragment in here:
                starts[min(anchors.get(fragment, 0) if fragment else 0, len(paragraphs))] = index
            bounds = sorted(starts)
            regions = []  # (first paragraph, end, node)
            cursor = 0
            for i, at in enumerate(bounds):
                if at > cursor:
                    regions.append((cursor, at, current))
                current = starts[at]
                cursor = at
                end = bounds[i + 1] if i + 1 < len(bounds) else len(paragraphs)
                regions.append((at, end, current))
                cursor = end
            if not regions:
                regions.append((0, len(paragraphs), current))
            elif cursor < len(paragraphs):
                regions.append((cursor, len(paragraphs), current))

            for first, end, node in regions:
                run = paragraphs[first:end]
                before = sum(len(p) + 1 for p in paragraphs[:first])
                for text, start in chunk(run):
                    total += len(text)
                    if total > Limits.MAX_CHARS:
                        raise ValueError('too much text in one file')
                    if node is not None:
                        chars[node] += len(text)
                    yield Segment(
                        text=text,
                        section=(outline[node][0] if node is not None and outline[node][0] else heading) or None,
                        locator={'type': 'epub', 'href': href, 'progression': round(min(1.0, (before + start) / length), 3)},
                        node=node,
                    )

        if out is not None and outline:
            out['structure'] = classify([{'title': t, 'depth': d, 'chars': chars[i]} for i, (t, d, _m, _f) in enumerate(outline)])
