"""Finds the pages of a PDF that have no text layer (RF-019).

A scanned page is an image: it can be seen but not searched or selected. This
only reports where that is so; running OCR on those pages is a separate step,
so a mixed PDF can be processed on just the pages that need it."""
import fitz  # PyMuPDF

# A page counts as having text when it holds at least this many characters that
# are not whitespace. A stray page number or running header is not a text layer.
MIN_TEXT_CHARS = 20


def detect_text_layer(file_path, min_chars=MIN_TEXT_CHARS):
    """Returns (page_count, pages_without_text), pages numbered from 1."""
    doc = fitz.open(file_path)
    try:
        missing = []
        for index in range(len(doc)):
            text = doc.load_page(index).get_text()
            if sum(1 for ch in text if not ch.isspace()) < min_chars:
                missing.append(index + 1)
        return len(doc), missing
    finally:
        doc.close()
