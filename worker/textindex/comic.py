"""The text a comic carries about itself: the ComicInfo.xml inside its archive (#25).

The pictures are the comic and their text is for OCR; what is here is what the file says of itself, as text: the
synopsis, the characters, the teams, the places, the story arc, the notes. It lets "the comic with that character" be
found. None of it belongs to a page, so every segment has the same address, the first picture, marked with the name of
the file it was read from (`item`), and says in its section which field it is: opening it opens the comic at the
start, and the result says it is from the file's description, not from a page.
"""
import zipfile

from lxml import etree

from . import Limits, Segment
from .normalize import chunk, clean
from extractors.plain_text import plain_description

ITEM = 'ComicInfo.xml'

# The fields of ComicInfo that are text about the story, in the order they are read, and what each is called.
FIELDS = [
    ('Summary', 'Sinopse'),
    ('StoryArc', 'Arco'),
    ('Characters', 'Personagens'),
    ('Teams', 'Equipes'),
    ('Locations', 'Locais'),
    ('Notes', 'Notas'),
]


def _read_comicinfo(path, fmt):
    """The bytes of the ComicInfo.xml of a CBZ or CBR, or None when it has none. Too large is read as none."""
    if fmt == 'cbr':
        try:
            import rarfile
        except ImportError:
            raise ValueError('CBR files cannot be read here (rarfile is not installed)')
        try:
            with rarfile.RarFile(path) as archive:
                for info in archive.infolist():
                    if info.filename.lower() == ITEM.lower() and info.file_size <= Limits.MAX_METADATA_BYTES:
                        return archive.read(info)
        except (rarfile.Error, OSError) as err:
            raise ValueError(f'not a readable CBR: {err}')
        return None
    try:
        with zipfile.ZipFile(path) as archive:
            for info in archive.infolist():
                if info.filename.lower() == ITEM.lower() and info.file_size <= Limits.MAX_METADATA_BYTES:
                    return archive.read(info)
    except zipfile.BadZipFile as err:
        raise ValueError(f'not a readable CBZ: {err}')
    return None


def _fields(data):
    """{tag: text} of the fields that have any. A ComicInfo that does not parse says nothing: the comic is fine."""
    try:
        parser = etree.XMLParser(resolve_entities=False, no_network=True, huge_tree=False, recover=False)
        root = etree.fromstring(data, parser=parser)
    except etree.XMLSyntaxError:
        return {}
    out = {}
    for tag, _label in FIELDS:
        element = root.find(tag)
        if element is not None:
            text = ''.join(element.itertext()).strip()
            if text:
                out[tag] = text
    return out


def comic_segments(path, fmt='cbz', checkpoint=lambda: None):
    data = _read_comicinfo(path, fmt)
    if not data:
        return
    fields = _fields(data)
    locator = {'type': 'image', 'index': 0, 'item': ITEM}
    for tag, label in FIELDS:
        checkpoint()
        text = fields.get(tag)
        if not text:
            continue
        # The field may carry HTML (a summary often does): it is text here, never markup.
        paragraphs = [p for p in (clean(line) for line in plain_description(text)[:Limits.MAX_METADATA_CHARS].split('\n')) if p]
        for segment, _start in chunk(paragraphs):
            yield Segment(text=segment, locator=dict(locator), section=label)
