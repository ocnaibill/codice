"""What a comic file itself says about how it is meant to be read (#19).

Two things are taken, and only when the file is clear about them:
  * ComicInfo.xml says `<Manga>YesAndRightToLeft</Manga>`: it reads right to left;
  * most of its pages are much taller than wide: it is a strip to scroll (webtoon).

Anything else says nothing, and the reader keeps what the person chose. The size of a page is read from the
header of the image, so no page is decoded and nothing here needs an imaging library.
"""
import struct
from typing import Optional

# How many pages are looked at, and how tall a page has to be, against its width, to count as part of a strip.
# An ordinary page is about 1.4 times taller than wide; a strip is several times.
SAMPLE_PAGES = 8
STRIP_RATIO = 3.0
# Enough of a file for the header of any image, even a JPEG with a large EXIF block before its size.
HEADER_BYTES = 256 * 1024


def image_size(data: bytes) -> Optional[tuple]:
    """(width, height) of a PNG, JPEG or WebP, from its header; None when it cannot be read."""
    try:
        if data[:8] == b'\x89PNG\r\n\x1a\n':
            return struct.unpack('>II', data[16:24])
        if data[:2] == b'\xff\xd8':
            return _jpeg_size(data)
        if data[:4] == b'RIFF' and data[8:12] == b'WEBP':
            return _webp_size(data)
    except (struct.error, IndexError):
        pass
    return None


def _jpeg_size(data: bytes) -> Optional[tuple]:
    i = 2
    while i + 9 < len(data):
        if data[i] != 0xFF:
            i += 1
            continue
        marker = data[i + 1]
        if marker == 0xFF:  # padding
            i += 1
            continue
        if marker in (0xD8, 0x01) or 0xD0 <= marker <= 0xD7:  # markers without a length
            i += 2
            continue
        length = struct.unpack('>H', data[i + 2:i + 4])[0]
        # The start-of-frame markers carry the size (not DHT, JPG and DAC, which share the range).
        if 0xC0 <= marker <= 0xCF and marker not in (0xC4, 0xC8, 0xCC):
            height, width = struct.unpack('>HH', data[i + 5:i + 9])
            return width, height
        i += 2 + length
    return None


def _webp_size(data: bytes) -> Optional[tuple]:
    kind = data[12:16]
    if kind == b'VP8 ':
        width, height = struct.unpack('<HH', data[26:30])
        return width & 0x3FFF, height & 0x3FFF
    if kind == b'VP8L':
        bits = struct.unpack('<I', data[21:25])[0]
        return (bits & 0x3FFF) + 1, ((bits >> 14) & 0x3FFF) + 1
    if kind == b'VP8X':
        return int.from_bytes(data[24:27], 'little') + 1, int.from_bytes(data[27:30], 'little') + 1
    return None


def is_strip(sizes: list) -> bool:
    """True when most of the pages looked at are several times taller than wide."""
    known = [(w, h) for w, h in sizes if w and h]
    if not known:
        return False
    tall = sum(1 for w, h in known if h / w >= STRIP_RATIO)
    return tall * 2 > len(known)


def declared_mode(manga: str, sizes: list) -> Optional[str]:
    """'rtl', 'webtoon' or None (the file says nothing). A strip is read top to bottom whatever the
    direction of the text, so it wins over a ComicInfo that says right to left."""
    if is_strip(sizes):
        return 'webtoon'
    if (manga or '').strip().lower() == 'yesandrighttoleft':
        return 'rtl'
    return None


def sample(names: list) -> list:
    """The pages to measure: a few spread over the whole file, so a long cover or a title page does not decide."""
    if len(names) <= SAMPLE_PAGES:
        return list(names)
    step = len(names) / SAMPLE_PAGES
    return [names[int(i * step)] for i in range(SAMPLE_PAGES)]
