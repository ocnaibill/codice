"""OpenLibrary API metadata provider.

Open Library's search answers only the fields it is asked for: without `fields` it gives neither ISBN, nor publisher, nor subjects. The
description of a work is not in the search at all, it is in the work (`/works/{id}`), which is asked for only for the answer that was chosen.
Only the title leaves the server (DEC-097), and the ISBN of the file, which finds the work if Open Library has the edition (DEC-142); the author the
file says is compared here (providers.match).
"""
import re
from typing import List, Optional

from .base import BaseProvider, Credit, MetadataRecord
from .http import get_json
from .match import TITLE_MIN
from .text import closeness, is_volume, main_title, tokens

BASE = 'https://openlibrary.org'
# What the search is asked for: every field the record is made of (and no more, so that the answer is small).
FIELDS = ('key,title,subtitle,author_name,author_key,first_publish_year,isbn,publisher,subject,cover_i,edition_count,'
          'series_name,series_position')
CANDIDATES = 20
MAX_TAGS = 8
MAX_ISBNS = 300

# Subjects that are not about the book: where it can be borrowed, awards and lists written as `nyt:…=date`, and the like.
_JUNK_SUBJECTS = frozenset({
    'accessible book', 'protected daisy', 'in library', 'lending library', 'overdrive', 'large type books', 'internet archive wishlist',
    'open library staff picks', 'new york times reviewed', 'new york times bestseller', 'long now manual for civilization',
})
_NAMESPACED = re.compile(r'^[a-z_]+:', re.I)
_TRANSLATION = re.compile(r'^translations? (into|from) ', re.I)


def clean_subjects(subjects, limit=MAX_TAGS):
    """The subjects that tell what a book is about, each once (without regard to case, accents or hyphens), the first `limit`."""
    out, seen = [], set()
    for raw in subjects or []:
        if not isinstance(raw, str):
            continue
        subject = raw.strip()
        key = ' '.join(tokens(subject, keep_stop=True))
        if not subject or key in seen or key in _JUNK_SUBJECTS or _NAMESPACED.match(subject) or _TRANSLATION.match(subject):
            continue
        seen.add(key)
        out.append(subject)
        if len(out) == limit:
            break
    return out


def _isbn(isbns):
    """An ISBN-13 when the work has one (the first of the list belongs to any edition, in any language)."""
    isbns = [i for i in isbns or [] if isinstance(i, str)]
    return next((i for i in isbns if len(i.replace('-', '')) == 13), isbns[0] if isbns else None)


class OpenLibraryProvider(BaseProvider):
    def __init__(self):
        self.search_url = f'{BASE}/search.json'
        self.book_url = f'{BASE}/books/'

    @property
    def name(self) -> str:
        return 'OpenLibrary'

    @property
    def id(self) -> str:
        return 'openlibrary'

    @staticmethod
    def _credits(doc: dict) -> list:
        """Every author of the work. `author_key` names each one, in the same order as `author_name`;
        when the two lists do not line up, which key belongs to whom is not known, so none is used."""
        names = [n.strip() for n in (doc.get('author_name') or []) if isinstance(n, str) and n.strip()]
        keys = doc.get('author_key') or []
        paired = len(keys) == len(doc.get('author_name') or []) and len(names) == len(keys)
        return [Credit(name, ids={'openlibrary': str(keys[i])} if paired and keys[i] else {})
                for i, name in enumerate(names)]

    def _record(self, doc: dict) -> MetadataRecord:
        record = MetadataRecord(source=self.name)
        record.title = doc.get('title')
        record.credits = self._credits(doc)
        record.author = record.credits[0].name if record.credits else None
        record.publisher = doc.get('publisher', [None])[0] if doc.get('publisher') else None
        # No language: `language` lists the languages of the editions of the work, in codes of three letters, and
        # the first is not the file's. The language of a file is read from the file (DEC-096).
        # The first year the work was published is the work's, not the date of any edition (DEC-156).
        year = doc.get('first_publish_year')
        record.original_year = str(year) if isinstance(year, int) and year else None
        record.isbn = _isbn(doc.get('isbn'))
        if doc.get('cover_i'):
            record.cover_url = f"https://covers.openlibrary.org/b/id/{doc['cover_i']}-L.jpg"
        record.tags = clean_subjects(doc.get('subject'))
        series = (doc.get('series_name') or [None])[0]
        if series:
            record.series = series
            try:
                record.series_index = float((doc.get('series_position') or [None])[0])
            except (TypeError, ValueError):
                pass
        # A work with many editions is the one people mean when two answers are equally close to the title.
        record.prior = min(doc.get('edition_count') or 0, 40) / 4
        record.raw = {'openlibrary_work': doc.get('key'), 'openlibrary_id': (doc.get('key') or '').rsplit('/', 1)[-1] or None,
                      'subtitle': doc.get('subtitle'), 'edition_count': doc.get('edition_count'),
                      'isbns': [i for i in (doc.get('isbn') or []) if isinstance(i, str)][:MAX_ISBNS]}  # every edition of the work: the file's is one of them
        return record

    def _ask(self, **params) -> List[dict]:
        reply = get_json(self.name, self.search_url, params={**params, 'limit': CANDIDATES, 'fields': FIELDS})
        return (reply.data or {}).get('docs', []) if reply.ok else []

    def lookup(self, query) -> List[MetadataRecord]:
        """The works the title is. Asked by the title of the work first; when none is close to what the file says it is asked again by the
        whole text, which also finds a work by the title of one of its editions (a translation)."""
        title = query.search_title
        if query.isbn:   # the ISBN of the file finds the work if Open Library has the edition (that is the book)
            by_isbn = self._ask(isbn=query.isbn)
            if by_isbn:
                return [self._record(d) for d in by_isbn]
        if not title:
            return []
        docs = self._ask(title=title)
        wanted = [text for text, _ in query.variants()]
        if not any(closeness(w, d.get('title', '')) >= TITLE_MIN or closeness(main_title(w), main_title(d.get('title', ''))) >= TITLE_MIN
                   for d in docs for w in wanted):
            known = {d.get('key') for d in docs}
            docs += [d for d in self._ask(q=title) if d.get('key') not in known]
        return [self._record(d) for d in docs]

    def volume(self, series_titles, number) -> Optional[MetadataRecord]:
        """The book that is volume `number` of a series (asked by one of its names and the number: only the title leaves, DEC-097), if there is
        one that is exactly that volume (providers.text.is_volume) and not a deluxe edition or a box. The one with most editions is the volume."""
        if not series_titles or number is None:
            return None
        docs = [d for d in self._ask(q=f'{series_titles[0]} volume {number}') if is_volume(d.get('title', ''), series_titles, number)]
        docs.sort(key=lambda d: -(d.get('edition_count') or 0))
        return self._record(docs[0]) if docs else None

    def search(self, query: str) -> Optional[MetadataRecord]:
        """The first work for a text, as it always was (the registry's `search`)."""
        if not query:
            return None
        docs = self._ask(q=query)
        return self._record(docs[0]) if docs else None

    def enrich(self, record: MetadataRecord) -> MetadataRecord:
        """The description of the work that was chosen, and the subjects it is filed under (the search gives only some)."""
        key = (record.raw or {}).get('openlibrary_work')
        if not key:
            return record
        reply = get_json(self.name, f'{BASE}{key}.json')
        if not reply.ok:
            return record
        description = reply.data.get('description')
        if isinstance(description, dict):
            description = description.get('value')
        if isinstance(description, str) and description.strip():
            record.description = description.strip()
        record.tags = clean_subjects(list(record.tags) + list(reply.data.get('subjects') or []))
        return record
