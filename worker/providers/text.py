"""Small helpers to compare titles and names the way the providers' answers need: without accents, case or punctuation."""
import re
import unicodedata

# Words that say nothing about which work a title is, in the languages of the library (Portuguese, English, Spanish, French).
STOP = frozenset({
    'the', 'a', 'an', 'of', 'and', 'in', 'on', 'to', 'for', 'vol', 'volume', 'v',
    'o', 'os', 'as', 'um', 'uma', 'e', 'de', 'do', 'da', 'dos', 'das', 'em', 'no', 'na',
    'el', 'la', 'los', 'las', 'y', 'del', 'le', 'les', 'et', 'des', 'du', 'un', 'une',
})


def fold(text):
    """Lower case, no accents."""
    decomposed = unicodedata.normalize('NFKD', text or '')
    return ''.join(c for c in decomposed if not unicodedata.combining(c)).lower()


def tokens(text, keep_stop=False):
    """The words and numbers of a text, folded; the words that say nothing are left out unless asked."""
    words = [w for w in re.split(r'[^a-z0-9]+', fold(text)) if w]
    return words if keep_stop else [w for w in words if w not in STOP]


def normalize_isbn(text):
    """An ISBN as 13 digits, or None when the text is not one: ISBN-10 is converted, hyphens, spaces and a "urn:isbn:" in front are taken off,
    and a number whose check digit is wrong is not an ISBN (a file's identifier can be anything)."""
    chars = re.sub(r'(?i)^\s*(urn:)?isbn[:\s-]*', '', str(text or ''))
    chars = re.sub(r'[\s-]', '', chars).upper()
    if re.fullmatch(r'\d{9}[\dX]', chars):
        total = sum((10 - i) * (10 if c == 'X' else int(c)) for i, c in enumerate(chars))
        if total % 11:
            return None
        chars = '978' + chars[:9]
        chars += str((10 - sum(int(c) * (3 if i % 2 else 1) for i, c in enumerate(chars)) % 10) % 10)
        return chars
    if re.fullmatch(r'97[89]\d{10}', chars):
        total = sum(int(c) * (3 if i % 2 else 1) for i, c in enumerate(chars))
        return chars if total % 10 == 0 else None
    return None


def main_title(title):
    """The title without its subtitle ("Sapiens: Uma breve história" -> "Sapiens")."""
    return re.split(r'\s*[:–—]\s*', title or '')[0]


def closeness(a, b):
    """How much two titles are the same, from 0 to 1: the share of words they have in common (the words that say nothing do not count)."""
    ta, tb = set(tokens(a)), set(tokens(b))
    total = len(ta) + len(tb)
    return 2 * len(ta & tb) / total if total else 0.0


_VOLUME_WORDS = frozenset({'vol', 'volume', 'tome', 'v', 'book', 'manga', 'livro'})


def is_volume(title, series_titles, number):
    """Whether the title of a book is volume `number` of the series (one of its names): the name of the series, the number, and nothing else.
    "Naruto 01", "ONE PIECE 1" and "Vagabond, Volume 1" are; a "Deluxe Volume 1", a box of volumes or another volume are not (any other word in
    the title, or a number other than ours, makes it another book)."""
    words = [w for w in tokens(title, keep_stop=True) if w not in STOP or w in _VOLUME_WORDS]
    numbers = {str(int(w)) for w in words if w.isdigit()}
    text = {w for w in words if not w.isdigit() and w not in _VOLUME_WORDS}
    # Without a word in Latin letters there is nothing to compare (the words of a title in Japanese are none): that is no volume of anything.
    if numbers != {str(number)} or not text:
        return False
    return any(text == {w for w in tokens(name) if w not in _VOLUME_WORDS} for name in series_titles)
