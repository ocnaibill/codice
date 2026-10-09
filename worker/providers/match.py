"""Whether what a provider answered is the work the file is.

A provider always answers something, so the answer has to be judged. The title has to be close to the file's, and the author, when the file's
own metadata says one, has to be one of the people credited (an answer by someone else is not the book, whatever its title). What is not
close enough is not a suggestion: nothing is better than the wrong work."""
from dataclasses import dataclass, asdict

from .query import FileQuery, author_closeness
from .text import closeness, main_title, normalize_isbn

# How close a title has to be, from 0 to 1, to be taken for the file's.
TITLE_MIN = 0.85
# How close the author has to be, when the file says one and the provider credits someone.
AUTHOR_MIN = 0.5
# An answer that has the ISBN of the file is the book if something else agrees: the title is at least this close, or the author is the one the file says.
# An ISBN alone is not enough: the ISBN a file carries can be a wrong one.
ISBN_TITLE_MIN = 0.5


@dataclass
class Match:
    score: float = 0.0     # for ordering the answers; it only means something next to the others
    title: float = 0.0     # how close the title is, from 0 to 1
    author: float = -1.0   # how close the author is; -1 when there was nothing to compare
    accepted: bool = False
    reason: str = ''       # why it was not accepted: 'title', 'author' or 'number'
    isbn: bool = False     # the answer has the ISBN of the file: that is the book, whatever the title says

    def as_dict(self):
        out = asdict(self)
        if not self.isbn:
            del out['isbn']   # told only when it is what decided
        return out


def _titles(record):
    """The names an answer goes by: its title (with the subtitle, if it has one) and the series it is of."""
    titles = [record.title or '']
    subtitle = (record.raw or {}).get('subtitle')
    if subtitle and record.title:
        titles.append(f'{record.title} {subtitle}')
    if record.series:
        titles.append(record.series)
    titles.extend((record.raw or {}).get('alt_titles') or [])   # the other names of a work (the English and the native title of a manga)
    return [t for t in titles if t]


def _credits(record):
    names = [c.name for c in record.credits if c.name]
    return names or ([record.author] if record.author else [])


def _isbns(record):
    """Every ISBN an answer says it has, as 13 digits (a provider may list the ISBN-10 and the ISBN-13 of a volume)."""
    found = [record.isbn] + list((record.raw or {}).get('isbns') or [])
    return {i for i in (normalize_isbn(x) for x in found) if i}


def judge(query: FileQuery, record) -> Match:
    """Judges an answer against every way the file's title can be read; the best reading is the one that counts. An answer that has the ISBN of
    the file is the book when the title is somewhat close or the author is the file's (a translated title does not make it another book); an ISBN
    that nothing else agrees with is left to the title, as a wrong ISBN in a file is possible."""
    names, titles = _credits(record), _titles(record)
    if query.isbn and query.isbn in _isbns(record):
        readings = [text for text, _ in query.variants()]
        title = max([closeness(r, t) for r in readings for t in titles] + [closeness(main_title(r), main_title(t)) for r in readings for t in titles] or [0.0])
        author = max([author_closeness(wanted, names) for _, wanted in query.variants() if wanted and names] or [-1.0])
        if title >= ISBN_TITLE_MIN or author >= AUTHOR_MIN:
            return Match(score=300.0 + min(max(getattr(record, 'prior', 0) or 0, 0), 10), title=round(title, 2), author=round(author, 2), accepted=True, isbn=True)
    best = Match()
    for text, wanted in query.variants():
        title = max([closeness(text, t) for t in titles] + [closeness(main_title(text), main_title(t)) for t in titles] or [0.0])
        author = author_closeness(wanted, names) if wanted and names else -1.0
        reason = ''
        if title < TITLE_MIN:
            reason = 'title'
        elif 0 <= author < AUTHOR_MIN:
            reason = 'author'
        elif query.number is not None and record.series_index not in (None, 0) and int(record.series_index) != query.number:
            reason = 'number'
        score = title * 100 + max(author, 0) * 60 + min(max(getattr(record, 'prior', 0) or 0, 0), 10)
        if query.number is not None and record.series_index not in (None, 0) and int(record.series_index) == query.number:
            score += 20
        candidate = Match(score=round(score, 1), title=round(title, 2), author=round(author, 2), accepted=not reason, reason=reason)
        if (candidate.accepted, candidate.score) > (best.accepted, best.score) or not best.score:
            best = candidate
    return best
