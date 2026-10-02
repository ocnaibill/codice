"""The text an audio file carries about itself: its chapters and its description (#25).

The chapters of an audiobook (a list of titles with the time each begins) and the description or comment it carries
are text, and they are read without any model. A chapter is a segment of its own, with its title as text and as
section, addressed at its minute (`audio`, track 0, milliseconds): "the chapter about the escape" is found, and opening
it opens the audio there. The description has no moment, so it is at the start. What the speech itself says is not
here: that needs recognition (issue #28).
"""
import re

import mutagen
import mutagen.mp4

from . import Limits, Segment
from .normalize import chunk, clean
from extractors.plain_text import plain_description

# Descriptive text in the tags of the formats, as (key, what it is called). A key is the name in the tags of MP4, the
# name of the frame in ID3 (COMM, USLT, and TXXX with one of the descriptions below) or the field of a Vorbis comment.
MP4_KEYS = [('desc', 'Descrição'), ('ldes', 'Descrição'), ('\xa9cmt', 'Comentário'), ('\xa9lyr', 'Letra')]
TXXX_NAMES = {'description': 'Descrição', 'synopsis': 'Descrição', 'summary': 'Descrição', 'comment': 'Comentário', 'lyrics': 'Letra'}
VORBIS_KEYS = [('description', 'Descrição'), ('synopsis', 'Descrição'), ('comment', 'Comentário'), ('lyrics', 'Letra')]

_VORBIS_CHAPTER = re.compile(r'^chapter(\d+)$', re.I)
_VORBIS_CHAPTER_NAME = re.compile(r'^chapter(\d+)name$', re.I)
_TIMESTAMP = re.compile(r'^(\d+):(\d{1,2}):(\d{1,2})(?:[.,](\d{1,3}))?$')


def _ms(stamp):
    """Milliseconds of an "HH:MM:SS.mmm", or None."""
    m = _TIMESTAMP.match(stamp.strip())
    if not m:
        return None
    hours, minutes, seconds, fraction = int(m.group(1)), int(m.group(2)), int(m.group(3)), m.group(4) or '0'
    return ((hours * 60 + minutes) * 60 + seconds) * 1000 + int(fraction.ljust(3, '0')[:3])


def _text(value):
    return ' '.join(str(item) for item in value) if isinstance(value, (list, tuple)) else str(value)


def _chapters(audio):
    """[(milliseconds, title)] of the chapters of the file, in order of time."""
    found = []
    tags = getattr(audio, 'tags', None)
    chapters = getattr(audio, 'chapters', None)  # MP4: the chapter list of the file
    if chapters:
        found = [(int(round(float(c.start) * 1000)), str(c.title)) for c in chapters]
    elif tags is not None and hasattr(tags, 'getall') and tags.getall('CHAP'):  # ID3: CHAP frames
        for frame in tags.getall('CHAP'):
            titles = frame.sub_frames.getall('TIT2') if hasattr(frame, 'sub_frames') else []
            title = _text(titles[0].text) if titles else ''
            found.append((int(frame.start_time), title))
    elif tags is not None and hasattr(tags, 'keys'):  # Vorbis comments: CHAPTER001=00:00:00.000, CHAPTER001NAME=...
        stamps, names = {}, {}
        for key in tags.keys():
            for pattern, target in ((_VORBIS_CHAPTER, stamps), (_VORBIS_CHAPTER_NAME, names)):
                m = pattern.match(str(key))
                if m:
                    target[int(m.group(1))] = _text(tags[key])
        for number, stamp in stamps.items():
            ms = _ms(stamp)
            if ms is not None:
                found.append((ms, names.get(number, '')))
    found.sort(key=lambda item: item[0])
    return found


def _descriptions(audio):
    """[(what it is called, text)] of the description, the comment and the lyrics in the tags, without repeats."""
    tags = getattr(audio, 'tags', None)
    if tags is None:
        return []
    found = []
    if hasattr(tags, 'getall'):  # ID3
        for frame in tags.getall('COMM'):
            found.append(('Comentário', _text(frame.text)))
        for frame in tags.getall('USLT'):
            found.append(('Letra', _text(frame.text)))
        for frame in tags.getall('TXXX'):
            label = TXXX_NAMES.get(str(frame.desc).strip().lower())
            if label:
                found.append((label, _text(frame.text)))
    elif isinstance(audio, mutagen.mp4.MP4):  # MP4
        for key, label in MP4_KEYS:
            if key in tags:
                found.append((label, _text(tags[key])))
    else:  # Vorbis comments (FLAC, OGG)
        for key, label in VORBIS_KEYS:
            for stored in tags.keys():
                if str(stored).lower() == key:
                    found.append((label, _text(tags[stored])))
    seen, out = set(), []
    for label, text in found:
        key = clean(text).lower()
        if key and key not in seen:
            seen.add(key)
            out.append((label, text))
    return out


def audio_segments(path, fmt='mp3', checkpoint=lambda: None):
    try:
        audio = mutagen.File(path)
    except mutagen.MutagenError as err:
        raise ValueError(f'not a readable audio file: {err}')
    except (OSError, ValueError, TypeError, AttributeError) as err:
        raise ValueError(f'not a readable audio file: {err}')
    if audio is None:  # a format mutagen does not know: nothing it can say
        return

    for label, text in _descriptions(audio):
        checkpoint()
        paragraphs = [p for p in (clean(line) for line in plain_description(text)[:Limits.MAX_METADATA_CHARS].split('\n')) if p]
        for segment, _start in chunk(paragraphs):
            yield Segment(text=segment, locator={'type': 'audio', 'track': 0, 'ms': 0}, section=label)

    count = 0
    for ms, title in _chapters(audio):
        checkpoint()
        title = clean(plain_description(title))[:Limits.MAX_METADATA_CHARS]
        if not title:
            continue
        count += 1
        if count > Limits.MAX_CHAPTERS:
            raise ValueError('the audio file has too many chapters')
        yield Segment(text=title, locator={'type': 'audio', 'track': 0, 'ms': max(ms, 0)}, section=title)
