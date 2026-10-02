"""The text of the files, cut into segments that know where they are (RF-018, RF-019).

Each format has a reader that yields segments: a piece of text, its address in the file (a
locator, backend/internal/locator) and the section it is in, in reading order. Only native text
is read here; text recovered by OCR will come as segments of the same kind.

What is stored is the text as the file has it, with these changes and no others (version 1):
Unicode in its composed form, control characters and runs of whitespace turned into single spaces,
a word broken by a hyphen at the end of a line in a PDF joined again, and paragraphs kept apart by a
line break. Searching ignores case and accents in the database, so none of that is done to the text.
Change any of it, or a limit, and EXTRACTOR_VERSION goes up: every file is read again.

Version 6 reads what an EPUB says about itself (structure.py, epub.py): the epub:type of a document, of a section and of
the landmarks of the navigation document tell the front matter, the story and the back matter, in place of a guess from the
titles. It concerns EPUBs only (FORMAT_VERSION).

Version 5 reads the text a comic or an audio file carries in its metadata (comic.py, audio.py): the ComicInfo.xml of a
comic, the chapters and the description of an audio file. It changes nothing for the other formats, so it is a
version of those formats only (FORMAT_VERSION): the files of the others are not read again for it.

Version 4 tells the language of a file from its text, when the file does not declare one (language.py), and
proposes it as a suggestion.

Version 3 takes the soft hyphens (and other invisible characters inside a word) out of the text.

Version 2 adds the shape of the book (structure.py): each segment knows the node of the file's
outline it is in, and the file's nodes are published with its text. A segment never spans two nodes.
"""
from dataclasses import dataclass
from typing import Optional

EXTRACTOR_VERSION = 6

# The version a file of each format has to have been read with to be up to date. A change that concerns one format
# raises only its number, so the others are not read again (a library of PDFs is not read again for a comic's
# ComicInfo.xml). Formats that are not here were last changed at BASE_VERSION.
BASE_VERSION = 4
FORMAT_VERSION = {fmt: 5 for fmt in ('cbz', 'cbr', 'mp3', 'm4a', 'm4b', 'ogg', 'wav', 'flac')} | {'epub': 6}


def required_version(fmt):
    """The version a file of this format has to have been read with (see FORMAT_VERSION)."""
    return FORMAT_VERSION.get((fmt or '').lower(), BASE_VERSION)

# The version of the locator contract (backend/internal/locator). Segments carry it, so whoever reads
# them knows how to read their addresses if the contract ever changes.
LOCATOR_VERSION = 1


@dataclass
class Segment:
    text: str
    locator: dict
    section: Optional[str] = None
    origin: str = 'native'
    node: Optional[int] = None  # the index, in the file's structure, of the outline node it is in


class Limits:
    """What a file may cost to be read (RNF-007): an archive that expands into gigabytes, a document of
    a hundred thousand pages or text with no end must fail, not take the worker down with them."""
    MAX_ARCHIVE_BYTES = 400 * 1024 * 1024   # everything an EPUB expands into
    MAX_ENTRY_BYTES = 40 * 1024 * 1024      # one chapter
    MAX_SPINE_ITEMS = 20000
    MAX_PDF_PAGES = 20000
    MAX_TEXT_FILE_BYTES = 60 * 1024 * 1024
    MAX_CHARS = 40_000_000                  # of text in one file (a long novel is a few million)
    MAX_METADATA_BYTES = 2 * 1024 * 1024    # a ComicInfo.xml (they are a few KB)
    MAX_METADATA_CHARS = 20_000             # of one description, one list of characters...
    MAX_CHAPTERS = 5000                     # of one audio file
