"""The text of a PDF, page by page. A page with no usable text yields nothing: it is a scan, and
waits for OCR (ocr_detect.py says which)."""
import re
from collections import Counter
from bisect import bisect_right

import fitz  # PyMuPDF

from . import Limits, Segment
from .normalize import chunk, clean, dehyphenate
from .structure import classify

# Non-space characters below which a page has no text layer worth reading (the same rule as ocr_detect).
MIN_TEXT_CHARS = 20


MAX_TITLE = 200

# The running headers and footers of a printed page (the title of the book, of the chapter, the page
# number) are not the text of the book: on every page they would be the words that every page has in
# common. A block is one of them when it sits entirely in the top or bottom margin and looks like
# what many other pages carry there (its first two words, with digits taken as one). What only one
# page has there (a chapter's title) is text.
MARGIN = 0.12          # of the page's height
MIN_PAGES = 8          # a shorter document has too few pages to say what repeats
MIN_SHARE = 0.15       # of the pages, at least this many carry it
MIN_REPEATS = 4


def _key(text):
    tokens = ['#' if t.isdigit() else t for t in re.findall(r'\w+', text.lower())]
    return ' '.join(tokens[:2])


def _margins(page, blocks):
    """(block index, key) of the text blocks that lie wholly in the top or the bottom margin."""
    height = page.rect.height or 1
    out = []
    for i, block in enumerate(blocks):
        if len(block) > 6 and block[6] != 0:
            continue
        if block[3] / height <= MARGIN or block[1] / height >= 1 - MARGIN:
            key = _key(block[4])
            if key:
                out.append((i, key))
    return out


def _running_keys(doc, checkpoint):
    """What repeats in the margins of the pages: the keys that at least MIN_SHARE of them carry."""
    if len(doc) < MIN_PAGES:
        return set()
    seen = Counter()
    for index in range(len(doc)):
        checkpoint()
        try:
            page = doc.load_page(index)
            blocks = page.get_text('blocks', sort=True)
        except (RuntimeError, ValueError):
            continue
        seen.update({key for _i, key in _margins(page, blocks)})
    need = max(MIN_REPEATS, MIN_SHARE * len(doc))
    return {key for key, pages in seen.items() if pages >= need}


def _outline(doc):
    """The PDF's bookmarks as (title, depth, first page index), in the order they are listed. A bookmark
    that points nowhere (no page) says nothing about the text and is left out."""
    try:
        entries = doc.get_toc(simple=True)
    except (RuntimeError, ValueError):
        return []
    return [(clean(str(title))[:MAX_TITLE], max(int(level) - 1, 0), int(page) - 1)
            for level, title, page in entries if isinstance(page, int) and page >= 1]


def pdf_segments(path, checkpoint=lambda: None, out=None):
    """Yields the segments of the PDF, page by page. When `out` is a dict, the bookmarks' nodes are put
    in it under 'structure' once every segment has been yielded."""
    try:
        doc = fitz.open(path)
    except (RuntimeError, ValueError) as err:
        raise ValueError(f'not a readable PDF: {err}')
    try:
        if doc.needs_pass:
            raise ValueError('the PDF is encrypted')
        if len(doc) > Limits.MAX_PDF_PAGES:
            raise ValueError('the PDF has too many pages')
        running = _running_keys(doc, checkpoint)
        outline = _outline(doc)
        # A bookmark opens at its page and holds the text up to the next one: pages are looked up by the
        # greatest start not after them (a running maximum, so a bookmark listed out of order cannot
        # send a page back).
        starts, high = [], -1
        for _title, _depth, page in outline:
            high = max(high, page)
            starts.append(high)
        chars = [0] * len(outline)
        total = 0
        for index in range(len(doc)):
            checkpoint()
            try:
                # Blocks in reading order (top to bottom, left to right): a two-column page is only as
                # good as this order, which the specification asks to be measured.
                page = doc.load_page(index)
                blocks = page.get_text('blocks', sort=True)
            except (RuntimeError, ValueError):
                continue  # a page that cannot be read does not stop the others
            headers = {i for i, key in _margins(page, blocks) if key in running}
            paragraphs = []
            for number, block in enumerate(blocks):
                if number in headers:
                    continue
                if len(block) > 6 and block[6] != 0:  # 0 is text; the rest are images
                    continue
                text = clean(dehyphenate(block[4]).replace('\n', ' '))
                if text:
                    paragraphs.append(text)
            if sum(1 for p in paragraphs for ch in p if not ch.isspace()) < MIN_TEXT_CHARS:
                continue
            for text, _start in chunk(paragraphs):
                total += len(text)
                if total > Limits.MAX_CHARS:
                    raise ValueError('too much text in one file')
                node = bisect_right(starts, index) - 1 if starts else -1
                node = node if node >= 0 else None
                if node is not None:
                    chars[node] += len(text)
                yield Segment(text=text, locator={'type': 'pdf', 'page': index}, section=outline[node][0] if node is not None else None, node=node)
        if out is not None and outline:
            out['structure'] = classify([{'title': t, 'depth': d, 'chars': chars[i]} for i, (t, d, _p) in enumerate(outline)])
    finally:
        doc.close()
