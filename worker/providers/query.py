"""What the file says about itself, read to ask the providers and to judge what they answer.

Only the title is sent to a provider, and the ISBN the file carries to the two that find a book by it (DEC-097, DEC-142). The author and the number
are used here, to tell which of the answers is the work."""
import re
from dataclasses import dataclass
from typing import List, Optional, Tuple

from .text import closeness, normalize_isbn, tokens

# The formats whose files are one volume or issue of a series: their number is not part of the title.
SERIAL_FORMATS = ('cbz', 'cbr')

_NUMBER = re.compile(r'(?:^|\s)(?:v|vol\.?|volume|#|n[ºo]\.?|livro|book)?\s*0*(\d{1,4})$', re.I)
_NO_AUTHOR = ('', 'unknown author', 'autor desconhecido')


def _clean(title):
    """The title without what is in parentheses or brackets ("(2025) (Digital) [Group]") and with single spaces."""
    return re.sub(r'\s+', ' ', re.sub(r'\([^)]*\)|\[[^\]]*\]', ' ', title or '')).strip()


# What is written in parentheses after a title and says nothing about the book: the quality or the origin of the file.
_FILE_NOISE = frozenset({'digital', 'retail', 'scan', 'scanned', 'ocr', 'epub', 'pdf', 'mobi', 'azw3', 'ebook', 'hq', 'web', 'zlib', 'repack', 'completo', 'complete'})
_PARENTHESES = re.compile(r'\(([^()]*)\)')


_ARTICLES = frozenset({'the', 'of', 'and', 'a', 'o', 'de', 'do', 'da', 'e', 'em', 'in', 'on'})


def _looks_like_a_name(text):
    """Two or three capitalised words, none of the kind that goes in a title: "Neal Shusterman", not "The Chamber of Secrets"."""
    words = text.split()
    if not 2 <= len(words) <= 3:
        return False
    return all(w[0].isupper() and w.lower().strip('.') not in _ARTICLES and not any(ch.isdigit() for ch in w) for w in words)


def author_names(text):
    """The people a text of authors names, one by one ("Alan Moore; Dave Gibbons", "Moore, Alan & Dave Gibbons")."""
    parts = re.split(r'\s*(?:;|&|\band\b|\be\b|\by\b|\bet\b|\bund\b|/)\s*', text or '', flags=re.I)
    return [p.strip() for p in parts if p and p.strip()]


def _surnames(name):
    """The words of a name that stand for its surname: the last one, and the first when it is written "Surname, Name"."""
    words = tokens(name, keep_stop=True)
    out = set(words[-1:])
    if ',' in name:
        out.update(words[:1])
    return {w for w in out if len(w) > 1}


def author_closeness(wanted, credits):
    """How much the author the file says is one of the people a provider credits, from 0 to 1. Two names are the same person only if they
    share a word and one of the shared words is the surname of one of them: "Andrew Hunt" and "Andy Hunt" are, "Andrew Hunt" and
    "Andrew Smith" are not."""
    best = 0.0
    for want in author_names(wanted):
        for credit in credits:
            a, b = set(tokens(want, keep_stop=True)), set(tokens(credit, keep_stop=True))
            shared = a & b
            if not shared or not ((_surnames(want) | _surnames(credit)) & shared):
                continue
            best = max(best, 2 * len(shared) / (len(a) + len(b)))
    return best


def parenthetical(title, author=None):
    """The words in parentheses after a title that say something about the book ("A nuvem (Scythe)": the series, or the original title), which a
    search finds the book by. Not a year, not what is said of the file ("(Digital)"), and not a person: the author is never sent (DEC-097)."""
    for group in _PARENTHESES.findall(title or ''):
        text = re.sub(r'\s+', ' ', group).strip()
        words = text.split()
        if len(words) > 4 or not any(ch.isalpha() for ch in text) or any(ch.isdigit() for ch in text):
            continue
        if any(w.lower().strip('.,') in _FILE_NOISE for w in words) or _looks_like_a_name(text):
            continue
        if author and author_closeness(author, [text]) >= 0.5:
            continue
        return text
    return ''


@dataclass
class FileQuery:
    """The title of a file, read. `author` is what the file's own metadata says (it can veto an answer); `hint` is what the title seems to say
    after a dash, which only counts when it is right."""
    title: str
    author: Optional[str] = None
    number: Optional[int] = None
    hint: Optional[str] = None
    format: str = 'default'
    left: str = ''       # the title without the part after the dash
    stripped: str = ''   # ... and without the number
    extra: str = ''      # what the title says in parentheses about the book ("Scythe" in "A nuvem (Scythe)")
    isbn: Optional[str] = None   # the ISBN the file carries, as 13 digits (it is sent to the providers that search by it, DEC-142)

    @property
    def serial(self):
        return (self.format or '').lower() in SERIAL_FORMATS

    @property
    def search_title(self):
        """The one thing that is sent to a provider. The number of an issue or a volume is not part of the title of its series."""
        return (self.stripped if self.serial else self.left) or self.title

    @property
    def search_extra(self):
        """The title with what it says in parentheses, for a provider whose search finds the book by it; empty when there is nothing to add.
        Not for the files of a series: there the parentheses are the year, the group, the quality."""
        return f'{self.search_title} {self.extra}' if self.extra and not self.serial and self.search_title else ''

    def variants(self) -> List[Tuple[str, Optional[str]]]:
        """The ways the title can be read, each with the author an answer to it has to have (or None): the whole text as written; without the
        part after the dash (the author, if what is there is one); and without the number. An answer is the work if it is close to any of them."""
        out = [(self.title, self.author)]
        wanted = self.author or self.hint
        for text in (self.left, self.stripped):
            if text and text != out[-1][0] and text not in [t for t, _ in out]:
                out.append((text, wanted))
        return out


def read_file_title(title, author=None, format='default', isbn=None) -> FileQuery:
    """'A Nuvem 2 - Neal Shusterman' -> title as written, 'A Nuvem 2' without the author, 'A Nuvem' without the number, and the author as a hint."""
    clean = _clean(title)
    author = None if (author or '').strip().lower() in _NO_AUTHOR else author.strip()
    left, hint = clean, None
    if ' - ' in clean:
        before, after = clean.rsplit(' - ', 1)
        if before and (author_closeness(author, [after]) >= 0.5 if author else _looks_like_a_name(after)):
            left, hint = before.strip(), after.strip()
    number, stripped = None, left
    match = _NUMBER.search(left)
    rest = left[:match.start()].strip(' -:,') if match else ''
    if rest:   # what is left of the title once the number is gone: a title that is only a number ("1984") has none
        number, stripped = int(match.group(1)), rest
    return FileQuery(title=clean, author=author, number=number, hint=hint, format=format or 'default', left=left, stripped=stripped,
                     extra=parenthetical(title, author), isbn=normalize_isbn(isbn))
