"""The text of a PDF, page by page. A page with no usable text yields nothing: it is a scan, and
waits for OCR (ocr_detect.py says which)."""
import fitz  # PyMuPDF

from . import Limits, Segment
from .normalize import chunk, clean, dehyphenate

# Non-space characters below which a page has no text layer worth reading (the same rule as ocr_detect).
MIN_TEXT_CHARS = 20


def pdf_segments(path, checkpoint=lambda: None):
    try:
        doc = fitz.open(path)
    except (RuntimeError, ValueError) as err:
        raise ValueError(f'not a readable PDF: {err}')
    try:
        if doc.needs_pass:
            raise ValueError('the PDF is encrypted')
        if len(doc) > Limits.MAX_PDF_PAGES:
            raise ValueError('the PDF has too many pages')
        total = 0
        for index in range(len(doc)):
            checkpoint()
            try:
                # Blocks in reading order (top to bottom, left to right): a two-column page is only as
                # good as this order, which the specification asks to be measured.
                blocks = doc.load_page(index).get_text('blocks', sort=True)
            except (RuntimeError, ValueError):
                continue  # a page that cannot be read does not stop the others
            paragraphs = []
            for block in blocks:
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
                yield Segment(text=text, locator={'type': 'pdf', 'page': index})
    finally:
        doc.close()
