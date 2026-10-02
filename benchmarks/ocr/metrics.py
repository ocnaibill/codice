"""How close the text OCR read is to the text that was on the page (#24, QA-007).

Pure functions, no engine: they take the text that is known to be on the page (the truth) and the text that came out.
Nothing here is a pass or a fail. The specification (15.3) approves no threshold, so these only measure.
"""
from __future__ import annotations

import re
import unicodedata

_WORD = re.compile(r'[^\W_]+', re.UNICODE)
MIN_WORD = 4  # shorter words say nothing about whether a page can be found by search


def collapse(text: str) -> str:
    """The text as one line, composed Unicode, spaces tidied: how the page is compared, because where the engine breaks
    its lines is not what is being measured."""
    return ' '.join(unicodedata.normalize('NFC', text or '').split())


def fold(text: str) -> str:
    """The text without case and without accents, the way the library's search sees it."""
    decomposed = unicodedata.normalize('NFD', text or '')
    return ''.join(c for c in decomposed if not unicodedata.combining(c)).casefold()


def words(text: str) -> list[str]:
    return _WORD.findall(unicodedata.normalize('NFC', text or ''))


def levenshtein(a: str, b: str) -> int:
    """The edit distance (insert, delete, replace, one each), by the bit-parallel method of Myers and Hyyro, which
    handles a page of text in milliseconds where the textbook table takes seconds."""
    if not a:
        return len(b)
    if not b:
        return len(a)
    m = len(a)
    mask = (1 << m) - 1
    last = 1 << (m - 1)
    peq: dict[str, int] = {}
    for i, ch in enumerate(a):
        peq[ch] = peq.get(ch, 0) | (1 << i)
    pv, mv, score = mask, 0, m
    for ch in b:
        eq = peq.get(ch, 0)
        xv = eq | mv
        xh = (((eq & pv) + pv) ^ pv) | eq
        ph = mv | (~(xh | pv) & mask)
        mh = pv & xh
        if ph & last:
            score += 1
        elif mh & last:
            score -= 1
        ph = ((ph << 1) | 1) & mask
        mh = (mh << 1) & mask
        pv = mh | (~(xv | ph) & mask)
        mv = ph & xv
    return score


def cer(truth: str, got: str) -> float:
    """Characters wrong over characters there should be (0 is perfect; it can pass 1 when the engine adds a lot)."""
    t, g = collapse(truth), collapse(got)
    if not t:
        return 0.0 if not g else 1.0
    return levenshtein(t, g) / len(t)


def wer(truth: str, got: str) -> float:
    """The same by words: a word with one wrong letter is one wrong word."""
    t, g = collapse(truth).split(' '), collapse(got).split(' ')
    t = [w for w in t if w]
    g = [w for w in g if w]
    if not t:
        return 0.0 if not g else 1.0
    index: dict[str, str] = {}
    for w in t + g:
        index.setdefault(w, chr(0x100 + len(index)))
    return levenshtein(''.join(index[w] for w in t), ''.join(index[w] for w in g)) / len(t)


def search_recall(truth: str, got: str, accents: bool = False) -> float | None:
    """Of the distinct words of the truth (four letters or more), the share that is in what was read: what decides
    whether a page can be found by a word in it. With `accents` False the word is found with or without them, as the
    library's search does; with True it has to be written as it is, which shows what the accents cost. None when the
    truth has no such word."""
    norm = (lambda s: s.casefold()) if accents else fold
    wanted = {norm(w) for w in words(truth) if len(w) >= MIN_WORD}
    if not wanted:
        return None
    have = {norm(w) for w in words(got)}
    return len(wanted & have) / len(wanted)


def reading_order(truth: str, got: str, span: int = 3, step: int = 15) -> tuple[float | None, float | None]:
    """(coverage, order) of the reading order. Takes runs of `span` words from along the truth that occur only once
    there, finds where each is in what was read, and says what share was found (coverage) and, of the pairs found, what
    share is in the same order as in the truth (order; 1 is right, a page of two columns read across scores about a
    half). None where there is nothing to say: no anchors, or fewer than two found."""
    t = [fold(w) for w in words(truth)]
    g = [fold(w) for w in words(got)]
    seen: dict[tuple, int] = {}
    for i in range(len(t) - span + 1):
        key = tuple(t[i:i + span])
        seen[key] = seen.get(key, 0) + 1
    where: dict[tuple, int] = {}
    for i in range(len(g) - span + 1):
        where.setdefault(tuple(g[i:i + span]), i)
    anchors = [tuple(t[i:i + span]) for i in range(0, len(t) - span + 1, step) if seen[tuple(t[i:i + span])] == 1]
    if not anchors:
        return None, None
    found = [where[a] for a in anchors if a in where]
    coverage = len(found) / len(anchors)
    if len(found) < 2:
        return coverage, None
    pairs = ordered = 0
    for i in range(len(found)):
        for j in range(i + 1, len(found)):
            pairs += 1
            ordered += found[i] < found[j]
    return coverage, ordered / pairs


def mean(values) -> float | None:
    values = [v for v in values if v is not None]
    return sum(values) / len(values) if values else None
