"""The text of the files, cut into segments that know where they are (RF-018, RF-019).

Each format has a reader that yields segments: a piece of text, its address in the file (a
locator, backend/internal/locator) and the section it is in, in reading order. Only native text
is read here; text recovered by OCR will come as segments of the same kind.

What is stored is the text as the file has it, with these changes and no others (version 1):
Unicode in its composed form, control characters and runs of whitespace turned into single spaces,
a word broken by a hyphen at the end of a line in a PDF joined again, and paragraphs kept apart by a
line break. Searching ignores case and accents in the database, so none of that is done to the text.
Change any of it, or a limit, and EXTRACTOR_VERSION goes up: every file is read again.
"""
from dataclasses import dataclass
from typing import Optional

EXTRACTOR_VERSION = 1

# The version of the locator contract (backend/internal/locator). Segments carry it, so whoever reads
# them knows how to read their addresses if the contract ever changes.
LOCATOR_VERSION = 1


@dataclass
class Segment:
    text: str
    locator: dict
    section: Optional[str] = None
    origin: str = 'native'


class Limits:
    """What a file may cost to be read (RNF-007): an archive that expands into gigabytes, a document of
    a hundred thousand pages or text with no end must fail, not take the worker down with them."""
    MAX_ARCHIVE_BYTES = 400 * 1024 * 1024   # everything an EPUB expands into
    MAX_ENTRY_BYTES = 40 * 1024 * 1024      # one chapter
    MAX_SPINE_ITEMS = 20000
    MAX_PDF_PAGES = 20000
    MAX_TEXT_FILE_BYTES = 60 * 1024 * 1024
    MAX_CHARS = 40_000_000                  # of text in one file (a long novel is a few million)
