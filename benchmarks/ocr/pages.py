"""Pages made to be read by OCR, with the text that is on them known exactly (#24, QA-007).

The text comes from books that anyone may redistribute and the pages are drawn here, so the truth is not what some
engine read but what was written on the page. What is drawn is a picture and nothing else: like a scan, a page has no
text layer, and it is made as the worker meets one (an image in a PDF, at the resolution it was scanned at).

What a drawn page cannot be is dirty the way a real scan is (a bent spine, a shadow, a faded ink). The looks below
add what can be added without leaving the standard library and PyMuPDF; pages of real scans are measured by giving the
benchmark a folder of them, which is what the maintainer is asked for.
"""
from __future__ import annotations

import random
import re
from dataclasses import dataclass, replace

import fitz

PAGE_W, PAGE_H = 612.0, 792.0  # US Letter, in points
FONTS = {'serif': 'tiro', 'sans': 'helv', 'italic': 'tiit'}
# What replaces the characters the page fonts do not have, so the truth is exactly what is drawn.
SUBSTITUTES = {
    '\u2018': "'", '\u2019': "'", '\u201c': '"', '\u201d': '"', '\u2013': '-', '\u2014': '-', '\u2026': '...',
    '\u00a0': ' ', '\u2009': ' ', '\u200b': '', '\ufeff': '', '\u2212': '-',
}


@dataclass(frozen=True)
class Look:
    """How a page is drawn and scanned."""
    name: str
    columns: int = 1
    font: str = 'serif'
    size: float = 12.0
    dpi: int = 200               # the resolution of the scan
    rotate: int = 0              # the page scanned turned by 0, 90, 180 or 270 degrees
    skew: float = 0.0            # and crooked by this many degrees
    speckles: int = 0            # specks of dirt
    ink: float = 0.0             # 0 is black
    paper: float = 1.0           # 1 is white
    jpeg: int | None = None      # quality of a JPEG scan; None for a lossless one
    header: bool = False         # a running head and a page number

    def but(self, **changes) -> 'Look':
        return replace(self, **changes)


def clean_source(text: str) -> str:
    """The text of a Project Gutenberg book without the licence around it."""
    start = re.search(r'\*\*\* ?START OF (?:THE|THIS) PROJECT GUTENBERG EBOOK[^\n]*\n', text)
    end = re.search(r'\*\*\* ?END OF (?:THE|THIS) PROJECT GUTENBERG EBOOK', text)
    if start:
        text = text[start.end():]
    if end:
        text = text[:text.find(end.group(0))] if end.group(0) in text else text
    return text.replace('\r\n', '\n')


def paragraphs(text: str, minimum: int = 160) -> list[str]:
    """The paragraphs of a book, each on one line, those too short to be prose (titles, verses, lists) left out."""
    out = []
    for block in re.split(r'\n\s*\n', text):
        line = ' '.join(block.split())
        if len(line) >= minimum and not line.isupper():
            out.append(line)
    return out


def drawable(text: str, font: fitz.Font) -> str:
    """The text with what the font cannot draw replaced or taken out."""
    out = []
    for ch in text:
        ch = SUBSTITUTES.get(ch, ch)
        out.extend(c for c in ch if c == ' ' or font.has_glyph(ord(c)))
    return ' '.join(''.join(out).split())


def excerpt(paras: list[str], rng: random.Random, words: int = 900) -> str:
    """Consecutive paragraphs from somewhere in the book, about `words` words: more than a page holds."""
    start = rng.randrange(0, max(1, len(paras) - 40))
    out, count = [], 0
    for p in paras[start:] or paras:
        out.append(p)
        count += len(p.split())
        if count >= words:
            break
    return ' '.join(out)


def alternate(first: list[str], second: list[str], rng: random.Random, words: int = 900) -> str:
    """A text that changes language by the paragraph: what a bilingual edition or a book with long quotations is."""
    a, b = excerpt(first, rng, words).split('. '), excerpt(second, rng, words).split('. ')
    out, count = [], 0
    for i in range(max(len(a), len(b))):
        for side in (a, b):
            if i < len(side):
                out.append(side[i].rstrip('.') + '.')
                count += len(side[i].split())
        if count >= words * 1.2:
            break
    return ' '.join(out)


def _rects(look: Look) -> list[fitz.Rect]:
    # A page that will be turned has a shorter block of text, so that it still fits inside the sheet once turned.
    top, bottom = (150, 640) if look.rotate in (90, 270) else (90, 720)
    left, right = (110, 502) if look.rotate in (90, 270) else (72, 540)
    if look.columns == 1:
        return [fitz.Rect(left, top, right, bottom)]
    gutter = 20
    middle = (left + right) / 2
    return [fitz.Rect(left, top, middle - gutter / 2, bottom), fitz.Rect(middle + gutter / 2, top, right, bottom)]


def draw(look: Look, text: str, rng: random.Random, title: str = '', number: int = 1) -> tuple[bytes, str]:
    """(the picture of the page, the text that is on it). The picture is a PNG, or a JPEG when the look says so."""
    font = fitz.Font(FONTS[look.font])
    text = drawable(text, font)
    doc = fitz.open()
    page = doc.new_page(width=PAGE_W, height=PAGE_H)
    if look.paper < 1:
        page.draw_rect(page.rect, color=None, fill=(look.paper,) * 3)
    angle = (look.rotate + look.skew) % 360
    morph = (fitz.Point(PAGE_W / 2, PAGE_H / 2), fitz.Matrix(angle)) if angle else None
    ink = (look.ink,) * 3

    rest = text.split()
    placed: list[str] = []
    writer = fitz.TextWriter(page.rect)
    for rect in _rects(look):
        if not rest:
            break
        left_over = writer.fill_textbox(rect, ' '.join(rest), font=font, fontsize=look.size, lineheight=1.25)
        # What did not fit comes back as the lines (each with its width) that were left, not as words.
        left_over = [word for line in left_over for word in (line if isinstance(line, str) else line[0]).split()]
        placed.extend(rest[:len(rest) - len(left_over)])
        rest = left_over
    writer.write_text(page, color=ink, morph=morph)
    body = ' '.join(placed)

    truth = body
    if look.header:
        head, foot = drawable((title or 'Capitulo primeiro').upper(), font), str(number)
        small = fitz.TextWriter(page.rect)
        width = font.text_length(head, fontsize=10)
        small.append(fitz.Point((PAGE_W - width) / 2, 60 if look.rotate not in (90, 270) else 138), head, font=font, fontsize=10)
        small.append(fitz.Point(PAGE_W / 2 - 4, 760 if look.rotate not in (90, 270) else 656), foot, font=font, fontsize=11)
        small.write_text(page, color=ink, morph=morph)
        truth = f'{head} {body} {foot}'

    for _ in range(look.speckles):
        x, y, r = rng.uniform(20, PAGE_W - 20), rng.uniform(20, PAGE_H - 20), rng.uniform(0.4, 1.3)
        page.draw_circle(fitz.Point(x, y), r, color=None, fill=ink)

    pix = page.get_pixmap(dpi=look.dpi, colorspace=fitz.csGRAY, alpha=False)
    data = pix.tobytes('jpeg', jpg_quality=look.jpeg) if look.jpeg else pix.tobytes('png')
    doc.close()
    return data, truth


def scan(pictures: list[bytes]) -> bytes:
    """A PDF with each picture as a page and nothing else on it: no text layer, as a scan comes."""
    doc = fitz.open()
    for data in pictures:
        page = doc.new_page(width=PAGE_W, height=PAGE_H)
        page.insert_image(page.rect, stream=data)
    out = doc.tobytes()
    doc.close()
    return out
