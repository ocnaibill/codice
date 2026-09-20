"""Turns extracted text into clean paragraphs and cuts them into segments."""
import re
import unicodedata

# Control characters (including the two the search uses to mark a match), the line and paragraph
# separators, the zero-width ones and the byte order mark.
_CONTROL = re.compile(r'[\x00-\x1f\x7f-\x9f  ​-‍⁠﻿]')
_SPACES = re.compile(r'[ \t  -   　]+')
_SENTENCE_END = re.compile(r'(?<=[.!?…])\s+')

TARGET_CHARS = 1200   # a segment is about this long...
HARD_MAX_CHARS = 2400  # ...and never longer


def clean(text: str) -> str:
    """One paragraph: composed Unicode, no control characters, single spaces, trimmed."""
    text = unicodedata.normalize('NFC', text)
    text = _CONTROL.sub(' ', text)
    return _SPACES.sub(' ', text).strip()


def dehyphenate(text: str) -> str:
    """Joins a word that a PDF broke with a hyphen at the end of a line: "constan-\\ntinopla". Only when
    the next line goes on in lower case, so a real dash or a list is left alone."""
    return re.sub(r'(?<=\w)-\n(?=[a-zà-ÿ])', '', text)


def split_long(paragraph: str, limit: int = HARD_MAX_CHARS):
    """A paragraph longer than the limit, at sentence ends and then at spaces."""
    if len(paragraph) <= limit:
        yield paragraph
        return
    piece = ''
    for sentence in _SENTENCE_END.split(paragraph):
        while len(sentence) > limit:  # no sentence end in reach: a word boundary
            cut = sentence.rfind(' ', 0, limit)
            cut = cut if cut > limit // 2 else limit
            if piece:
                yield piece
                piece = ''
            yield sentence[:cut].strip()
            sentence = sentence[cut:].strip()
        if piece and len(piece) + 1 + len(sentence) > limit:
            yield piece
            piece = ''
        piece = f'{piece} {sentence}'.strip()
    if piece:
        yield piece


def chunk(paragraphs, target: int = TARGET_CHARS, limit: int = HARD_MAX_CHARS):
    """Groups paragraphs, in order, into segments of about `target` characters that end where a
    paragraph does when they can. `paragraphs` is a list of clean strings; yields (text, start), where
    start is the offset of the segment's first character in the paragraphs joined by "\\n"."""
    offset = 0
    current, current_start = [], 0
    size = 0
    for paragraph in paragraphs:
        for part in split_long(paragraph, limit):
            if current and size + 1 + len(part) > limit:
                yield '\n'.join(current), current_start
                current, size = [], 0
            if not current:
                current_start = offset
            current.append(part)
            size += len(part) + (1 if len(current) > 1 else 0)
            offset += len(part) + 1
            if size >= target:
                yield '\n'.join(current), current_start
                current, size = [], 0
    if current:
        yield '\n'.join(current), current_start
