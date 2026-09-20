"""The text of a .txt or .md file, by paragraph. The place of a segment is its offset, in characters,
in the text as it was decoded."""
import re

from . import Limits, Segment
from .normalize import chunk, clean

_BLANK_LINE = re.compile(r'\n[ \t]*\n')
_HEADING = re.compile(r'^\s{0,3}#{1,6}\s+(.*?)\s*#*\s*$')


def decode(data: bytes) -> str:
    for encoding in ('utf-8-sig', 'cp1252'):
        try:
            return data.decode(encoding)
        except UnicodeDecodeError:
            continue
    return data.decode('latin-1')


def plain_segments(path, fmt='txt', checkpoint=lambda: None):
    with open(path, 'rb') as f:
        data = f.read(Limits.MAX_TEXT_FILE_BYTES + 1)
    if len(data) > Limits.MAX_TEXT_FILE_BYTES:
        raise ValueError('the text file is too large')
    text = decode(data).replace('\r\n', '\n').replace('\r', '\n')

    # Paragraphs with the offset in the original text of each, and the heading each is under.
    paragraphs, offsets, sections = [], [], []
    position, section = 0, None
    for raw in _BLANK_LINE.split(text):
        start = text.find(raw, position) if raw else position
        position = start + len(raw)
        cleaned = clean(raw)
        if fmt == 'md':
            first = raw.strip().split('\n', 1)[0]
            match = _HEADING.match(first)
            if match and match.group(1):
                section = clean(match.group(1)) or section
        if cleaned:
            paragraphs.append(cleaned)
            offsets.append(start)
            sections.append(section)
    if not paragraphs:
        return

    # chunk() reports offsets in the joined clean text; map each back to the paragraph it starts in.
    joined_starts, running = [], 0
    for p in paragraphs:
        joined_starts.append(running)
        running += len(p) + 1
    total = 0
    for segment, start in chunk(paragraphs):
        checkpoint()
        total += len(segment)
        if total > Limits.MAX_CHARS:
            raise ValueError('too much text in one file')
        index = max(i for i, s in enumerate(joined_starts) if s <= start)
        yield Segment(text=segment, section=sections[index], locator={'type': 'text', 'offset': offsets[index]})
