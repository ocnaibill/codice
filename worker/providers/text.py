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


def main_title(title):
    """The title without its subtitle ("Sapiens: Uma breve história" -> "Sapiens")."""
    return re.split(r'\s*[:–—]\s*', title or '')[0]


def closeness(a, b):
    """How much two titles are the same, from 0 to 1: the share of words they have in common (the words that say nothing do not count)."""
    ta, tb = set(tokens(a)), set(tokens(b))
    total = len(ta) + len(tb)
    return 2 * len(ta & tb) / total if total else 0.0
