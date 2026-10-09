"""Media analysis pipeline with status lifecycle.

Inspired by Komga's media analysis pattern:
UNKNOWN → QUEUED → ANALYZING → READY | ERROR
Tracked per-work in the database.
"""
from enum import Enum
from typing import Optional
from dataclasses import dataclass
from datetime import datetime

from people import library_roles, name_key, parse_name


# The cover is stored on the work's primary edition. Since the data model allows
# several editions per work, the conflict target is the "one primary edition per
# work" index (migration 00002); `ON CONFLICT (work_id)` alone no longer matches
# any constraint and would fail.
UPSERT_PRIMARY_EDITION_COVER = """
    INSERT INTO editions (work_id, title, cover_url)
    VALUES (%s, %s, %s)
    ON CONFLICT (work_id) WHERE is_primary
    DO UPDATE SET cover_url = EXCLUDED.cover_url
"""


MAX_TAG = 50  # tags.name is VARCHAR(50)


# The most each column holds, in characters: works.original_title and editions.title VARCHAR(255), works.series
# VARCHAR(512), editions.publisher VARCHAR(256), person.name, family_name and given_name VARCHAR(255), person_alias.alias
# VARCHAR(255). A longer text is shortened (a title that is a whole blurb is still a title).
SHORTENED_FIELDS = {'title': 255, 'series': 512, 'publisher': 256, 'author': 255}
# editions.isbn VARCHAR(64), language VARCHAR(16), publication_date VARCHAR(32): shortened, they would be another value,
# and one that long is not one of these at all, so it is left out.
EXACT_FIELDS = {'isbn': 64, 'language': 16, 'publication_date': 32}


def shorten(text, limit):
    """A text as a column of `limit` characters can hold it. A book's fields are free text (an EPUB has put a whole
    blurb in dc:title), and a value the column cannot hold must not fail the analysis of the file: it is cut at a word
    when it can be, never ends on punctuation, and ends in an ellipsis to say it was cut."""
    text = str(text)
    if len(text) <= limit:
        return text
    cut = text[:limit - 1]
    space = cut.rfind(' ')
    if space >= limit // 2:
        cut = cut[:space]
    return cut.rstrip(' ,;:.-–—/') + '…'


def fit_field(field, value):
    """The value of a descriptive field as its column holds it, or None when it cannot be (see EXACT_FIELDS)."""
    if value is None or isinstance(value, (int, float)) or field not in SHORTENED_FIELDS and field not in EXACT_FIELDS:
        return value
    if field in EXACT_FIELDS:
        return value if len(str(value)) <= EXACT_FIELDS[field] else None
    return shorten(value, SHORTENED_FIELDS[field])


def clean_tag(name):
    """A tag as the database can hold it. A book's subjects are free text (one EPUB lists "Translated by
    Ebook Translator: https://translator.bookfere.com" as one), and a value the column cannot hold must
    not fail the whole analysis of the file: it is shortened, at a word when it can be, and never ends
    on punctuation."""
    name = ' '.join(str(name or '').split())
    if len(name) <= MAX_TAG:
        return name
    cut = name[:MAX_TAG]
    space = cut.rfind(' ')
    if space >= MAX_TAG // 2:
        cut = cut[:space]
    return cut.rstrip(' ,;:.-–—/')


class MediaStatus(str, Enum):
    UNKNOWN = 'UNKNOWN'
    QUEUED = 'QUEUED'
    ANALYZING = 'ANALYZING'
    READY = 'READY'
    ERROR = 'ERROR'
    OUTDATED = 'OUTDATED'


@dataclass
class AnalysisResult:
    """Result of media analysis pipeline."""
    status: MediaStatus
    metadata: Optional[dict] = None
    error: Optional[str] = None
    started_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None


class Analyzer:
    """Orchestrates the media analysis pipeline for a work.

    Steps:
    1. Extract metadata from file (format-specific extractor)
    2. Enrich via external providers
    3. Save results to database
    4. Update status lifecycle
    """

    def __init__(self, db):
        self.db = db

    def mark_protected(self, work_id: int):
        """The primary file of the work asks for a password to open: the sheet and the notices say so."""
        self.db.execute(
            "UPDATE files SET protected = TRUE WHERE id = (SELECT file_id FROM work_primary WHERE work_id = %s)", (work_id,))
        print(f"   🔒 Work {work_id}: the file asks for a password")

    def update_status(self, work_id: int, status: MediaStatus, error: Optional[str] = None):
        """Update media status in database."""
        query = """
            UPDATE works
            SET media_status = %s, media_error = %s, updated_at = CURRENT_TIMESTAMP
            WHERE id = %s
        """
        self.db.execute(query, (status.value, error, work_id))
        print(f"   📊 Work {work_id} status → {status.value}")

    # Descriptive text fields: field name -> column. Locks and provenance are
    # keyed by the field name; series_index follows series. Title, series and
    # description belong to the work; the rest describe its primary edition.
    WORK_FIELDS = {
        'title': 'original_title',
        'series': 'series',
        'description': 'description',
    }
    EDITION_FIELDS = {
        'isbn': 'isbn',
        'language': 'language',
        'publisher': 'publisher',
        'publication_date': 'publication_date',
    }

    def _load_state(self, work_id: int) -> dict:
        """Current values, locks and provenance of a work's descriptive fields."""
        row = self.db.fetchone(
            """SELECT w.original_title,
                      COALESCE((SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id
                                WHERE c.work_id = w.id AND c.role = 'author'
                                ORDER BY c.position, p.name LIMIT 1), ''),
                      COALESCE(w.series, ''), COALESCE(w.series_index, 0),
                      COALESCE(e.isbn, ''), COALESCE(e.language, ''),
                      COALESCE(e.publisher, ''), COALESCE(e.publication_date, ''),
                      COALESCE(w.description, ''),
                      w.title_lock, w.author_lock, w.series_lock, w.cover_lock, w.isbn_lock,
                      w.language_lock, w.publisher_lock, w.publication_date_lock, w.description_lock
               FROM works w LEFT JOIN editions e ON e.work_id = w.id AND e.is_primary
               WHERE w.id = %s""",
            (work_id,))
        if not row:
            return {'values': {}, 'locks': {}, 'sources': {}}
        names = ['title', 'author', 'series', 'series_index', 'isbn', 'language', 'publisher',
                 'publication_date', 'description']
        lock_names = ['title', 'author', 'series', 'cover', 'isbn', 'language', 'publisher',
                      'publication_date', 'description']
        values = dict(zip(names, row[:9]))
        if values['author'] == 'Unknown Author':
            values['author'] = ''
        locks = dict(zip(lock_names, row[9:]))
        sources = dict(self.db.fetchall(
            "SELECT field, source FROM work_field_sources WHERE work_id = %s", (work_id,)) or [])
        return {'values': values, 'locks': locks, 'sources': sources}

    FILE_EXTENSIONS = ('.epub', '.pdf', '.cbz', '.cbr', '.txt', '.md', '.mobi', '.azw', '.azw3',
                       '.mp3', '.m4a', '.m4b', '.flac', '.ogg', '.wav')

    @classmethod
    def _is_placeholder(cls, field: str, value) -> bool:
        """A freshly uploaded work is titled with its file name. That is a
        stand-in, not something a person wrote, so the title in the file may
        replace it."""
        return field == 'title' and isinstance(value, str) and value.lower().endswith(cls.FILE_EXTENSIONS)

    @classmethod
    def _may_fill(cls, state: dict, field: str) -> bool:
        """Automatic extraction may write a field only if a person has not
        confirmed it and it is either empty (or a placeholder) or was itself
        read from the file. A value with no recorded origin predates provenance
        and is left alone."""
        if state['locks'].get(field):
            return False
        current = state['values'].get(field)
        if not current or cls._is_placeholder(field, current):
            return True
        return state['sources'].get(field) == 'file'

    def _record_source(self, work_id: int, field: str, source: str):
        self.db.execute(
            """INSERT INTO work_field_sources (work_id, field, source) VALUES (%s, %s, %s)
               ON CONFLICT (work_id, field) DO UPDATE SET source = EXCLUDED.source, updated_at = now()""",
            (work_id, field, source))

    def save_metadata(self, work_id: int, metadata: dict):
        """Save metadata read from the file itself.

        Native metadata fills empty fields and refreshes fields that were
        themselves read from the file. It never overwrites a locked field or a
        value someone confirmed, and it records where each value came from.
        External providers do not write here: their data goes through
        save_candidates and waits for an admin."""
        state = self._load_state(work_id)

        updates = []
        params = []
        edition_updates = []
        edition_params = []
        written = []

        for fields, sets, values in ((self.WORK_FIELDS, updates, params),
                                     (self.EDITION_FIELDS, edition_updates, edition_params)):
            for field, column in fields.items():
                value = fit_field(field, metadata.get(field))
                if value and self._may_fill(state, field) and value != state['values'].get(field):
                    sets.append(f"{column} = %s")
                    values.append(value)
                    written.append(field)

        if metadata.get('series_index') and self._may_fill(state, 'series') \
                and metadata['series_index'] != state['values'].get('series_index'):
            updates.append("series_index = %s")
            params.append(metadata['series_index'])

        # Technical facts about the file itself: not editorial, never locked.
        if metadata.get('format'):
            self.db.execute(
                "UPDATE files SET format = %s WHERE id = (SELECT file_id FROM work_primary WHERE work_id = %s)",
                (metadata['format'], work_id))
        if metadata.get('page_count'):
            updates.append("page_count = %s")
            params.append(metadata['page_count'])

        if updates:
            updates.append("updated_at = CURRENT_TIMESTAMP")
            query = f"UPDATE works SET {', '.join(updates)} WHERE id = %s"
            params.append(work_id)
            self.db.execute(query, tuple(params))
        if edition_updates:
            self.db.execute(
                f"UPDATE editions SET {', '.join(edition_updates)} WHERE work_id = %s AND is_primary",
                tuple(edition_params) + (work_id,))
        for field in written:
            self._record_source(work_id, field, 'file')

        # Author: same rule, resolved through the person table.
        # A catalogue's way of writing a name ("Herbert, Frank, author") is stored as the name people say,
        # and what the file wrote is kept as an alias (#36).
        written = metadata.get('author')
        author, family, given, renamed = parse_name(written) if written and written != 'Unknown Author' else ('', None, None, False)
        author, family, given = (fit_field('author', n) if n else n for n in (author, family, given))
        if author and self._may_fill(state, 'author') and author != state['values'].get('author'):
            # What is already known about the person is not replaced: the surname is learned once.
            author_id = self.db.fetchone(
                """INSERT INTO person (name, family_name, given_name) VALUES (%s, %s, %s)
                   ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name,
                       family_name = COALESCE(person.family_name, EXCLUDED.family_name),
                       given_name = CASE WHEN person.family_name IS NULL THEN EXCLUDED.given_name ELSE person.given_name END
                   RETURNING id""",
                (author, family, given))
            if author_id and renamed:
                self.db.execute(
                    "INSERT INTO person_alias (person_id, alias) VALUES (%s, %s) ON CONFLICT DO NOTHING",
                    (author_id[0], shorten(' '.join(written.split()), SHORTENED_FIELDS['author'])))
            if author_id:
                self.db.execute(
                    "DELETE FROM work_contributors WHERE work_id = %s AND role = 'author' AND position = 0",
                    (work_id,))
                self.db.execute(
                    """INSERT INTO work_contributors (work_id, person_id, role, position)
                       VALUES (%s, %s, 'author', 0) ON CONFLICT DO NOTHING""",
                    (work_id, author_id[0]))
                self._record_source(work_id, 'author', 'file')

        # Cover (with lock check), stored on the primary edition.
        if metadata.get('cover_path') and not state['locks'].get('cover'):
            self.save_cover(work_id, metadata['cover_path'], metadata.get('title', ''))

        # Tags found in the file (Many-to-Many)
        for tag_name in dict.fromkeys(clean_tag(t) for t in metadata.get('tags', []) or []):
            if not tag_name:
                continue
            self.db.execute(
                "INSERT INTO tags (name) VALUES (%s) ON CONFLICT (name) DO NOTHING",
                (tag_name,)
            )
            tag_row = self.db.fetchone("SELECT id FROM tags WHERE name = %s", (tag_name,))
            if tag_row:
                self.db.execute(
                    "INSERT INTO work_tags (work_id, tag_id) VALUES (%s, %s) ON CONFLICT DO NOTHING",
                    (work_id, tag_row[0])
                )

    def save_cover(self, work_id: int, cover_path: str, title: str = ''):
        """Store a cover on the work's primary edition."""
        self.db.execute(UPSERT_PRIMARY_EDITION_COVER, (work_id, title, cover_path))

    # Fields an external provider may propose, and how each is compared.
    CANDIDATE_FIELDS = ['title', 'author', 'series', 'series_index', 'isbn', 'language',
                        'publisher', 'publication_date', 'description']

    def save_candidates(self, work_id: int, record: dict, source: str, evidence: Optional[dict] = None) -> int:
        """Store a provider's suggestions for an admin to accept or reject.

        Nothing is applied to the work. A field is skipped when it is locked,
        when the provider has no value, or when it equals the current value. The
        (work, field, source, value) key makes a repeated or previously rejected
        suggestion a no-op. Returns how many suggestions were stored."""
        import json
        state = self._load_state(work_id)
        evidence_json = json.dumps(evidence or {})
        stored = 0

        def propose(field: str, value: str):
            nonlocal stored
            self.db.execute(
                """INSERT INTO metadata_candidates (work_id, field, value, source, evidence)
                   VALUES (%s, %s, %s, %s, %s::jsonb)
                   ON CONFLICT (work_id, field, source, value) DO NOTHING""",
                (work_id, field, value, source, evidence_json))
            stored += 1

        for field in self.CANDIDATE_FIELDS:
            value = record.get(field)
            if value in (None, '', 0):
                continue
            lock_name = 'series' if field == 'series_index' else field
            if state['locks'].get(lock_name):
                continue
            text = fit_field(field, str(value)) if field != 'series_index' else str(value)
            if text is None:
                continue
            current = state['values'].get(field)
            if field == 'series_index':
                try:
                    if float(value) == float(current or 0):
                        continue
                except (TypeError, ValueError):
                    continue
            elif text == str(current or ''):
                continue
            propose(field, text)

        extra = self._missing_contributors(work_id, record, state)
        if extra:
            propose('contributors', json.dumps(extra, ensure_ascii=False))

        tags = {clean_tag(t) for t in (record.get('tags') or []) if clean_tag(t)}
        if tags:
            have = {r[0] for r in (self.db.fetchall(
                "SELECT t.name FROM work_tags wt JOIN tags t ON t.id = wt.tag_id WHERE wt.work_id = %s",
                (work_id,)) or [])}
            if not tags <= have:
                propose('tags', json.dumps(sorted(tags)))
        return stored

    def _missing_contributors(self, work_id: int, record: dict, state: dict) -> list:
        """The people the provider credits besides the author it suggests (a co-author, an illustrator, a
        translator), with the role of each, leaving out whoever the work already has in that role. The first
        author is the `author` suggestion's business, not this one's. A locked author is not given co-authors."""
        credits = record.get('credits') or []
        if len(credits) == 0:
            return []
        have = {(name_key(n), r) for n, r in (self.db.fetchall(
            """SELECT p.name, c.role FROM work_contributors c JOIN person p ON p.id = c.person_id
               WHERE c.work_id = %s""", (work_id,)) or [])}
        first = name_key(record.get('author') or '')
        author_locked = bool(state['locks'].get('author'))
        out, seen = [], set()
        for credit in credits:
            name = ' '.join(str(credit.get('name') or '').split())
            key = name_key(name)
            if not key:
                continue
            for role in library_roles(credit.get('role')):
                if role == 'author' and (key == first or author_locked):
                    continue
                if (key, role) in have or (key, role) in seen:
                    continue
                seen.add((key, role))
                out.append({'name': name, 'role': role})
        return out

    def save_identifiers(self, work_id: int, metadata: dict):
        """Save identifiers (ISBN, provider IDs) to work_identifiers table."""
        identifiers = []

        # ISBN from extractor
        isbn = metadata.get('isbn')
        if isbn:
            identifiers.append(('isbn', isbn))

        # Provider identifiers from enriched metadata
        enriched_source = metadata.get('enriched_source')
        raw = metadata.get('raw', {})
        if enriched_source == 'google_books' and raw.get('google_id'):
            identifiers.append(('google_books', raw['google_id']))
        if enriched_source == 'openlibrary' and raw.get('openlibrary_id'):
            identifiers.append(('openlibrary', raw['openlibrary_id']))
        if enriched_source == 'comicvine' and raw.get('comicvine_id'):
            identifiers.append(('comicvine', raw['comicvine_id']))

        for id_type, id_value in identifiers:
            self.db.execute("""
                INSERT INTO work_identifiers (work_id, identifier_type, identifier_value)
                VALUES (%s, %s, %s)
                ON CONFLICT (work_id, identifier_type, identifier_value) DO NOTHING;
            """, (work_id, id_type, id_value))

    def save_text_layer(self, work_id: int, page_count: int, pages_without_text: list):
        """Record which pages of the work's file have no text (RF-019). The file is
        the work's primary one; a PDF is the only format this applies to."""
        self.db.execute(
            """INSERT INTO text_layers (file_id, page_count, pages_without_text, needs_ocr)
               SELECT file_id, %s, %s, %s FROM work_primary WHERE work_id = %s AND file_id IS NOT NULL
               ON CONFLICT (file_id) DO UPDATE SET page_count = EXCLUDED.page_count,
                   pages_without_text = EXCLUDED.pages_without_text,
                   needs_ocr = EXCLUDED.needs_ocr, detected_at = now()""",
            (page_count, list(pages_without_text), bool(pages_without_text), work_id))

    def save_declared_mode(self, work_id: int, mode):
        """Record how the work's file says it is read ('rtl', 'webtoon'), or None when it says nothing (#19).
        Analysing the file again replaces what was recorded, so a file that stops declaring one is not stuck."""
        self.db.execute(
            """UPDATE files SET declared_mode = %s
               WHERE id = (SELECT file_id FROM work_primary WHERE work_id = %s)""",
            (mode, work_id))

    def save_media_pages(self, work_id: int, metadata: dict):
        """Save per-page metadata to media_pages table."""
        pages = metadata.get('raw', {}).get('pages', [])
        if not pages:
            return

        for page in pages:
            self.db.execute("""
                INSERT INTO media_pages (work_id, page_number, file_name)
                VALUES (%s, %s, %s)
                ON CONFLICT (work_id, page_number) DO NOTHING;
            """, (work_id, page['page_number'], page['file_name']))
