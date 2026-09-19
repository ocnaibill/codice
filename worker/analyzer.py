"""Media analysis pipeline with status lifecycle.

Inspired by Komga's media analysis pattern:
UNKNOWN → QUEUED → ANALYZING → READY | ERROR
Tracked per-work in the database.
"""
from enum import Enum
from typing import Optional
from dataclasses import dataclass
from datetime import datetime


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

    def update_status(self, work_id: int, status: MediaStatus, error: Optional[str] = None):
        """Update media status in database."""
        query = """
            UPDATE works
            SET media_status = %s, media_error = %s, updated_at = CURRENT_TIMESTAMP
            WHERE id = %s
        """
        self.db.execute(query, (status.value, error, work_id))
        print(f"   📊 Work {work_id} status → {status.value}")

    # Descriptive text fields stored on the work: field name -> column. Locks and
    # provenance are keyed by the field name; series_index follows series.
    TEXT_FIELDS = {
        'title': 'original_title',
        'series': 'series',
        'isbn': 'isbn',
        'language': 'language',
        'publisher': 'publisher',
        'publication_date': 'publication_date',
        'description': 'description',
    }

    def _load_state(self, work_id: int) -> dict:
        """Current values, locks and provenance of a work's descriptive fields."""
        row = self.db.fetchone(
            """SELECT w.original_title, COALESCE(p.name, ''), COALESCE(w.series, ''),
                      COALESCE(w.series_index, 0), COALESCE(w.isbn, ''), COALESCE(w.language, ''),
                      COALESCE(w.publisher, ''), COALESCE(w.publication_date, ''),
                      COALESCE(w.description, ''),
                      w.title_lock, w.author_lock, w.series_lock, w.cover_lock, w.isbn_lock,
                      w.language_lock, w.publisher_lock, w.publication_date_lock, w.description_lock
               FROM works w LEFT JOIN person p ON p.id = w.author_id
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
        written = []

        for field, column in self.TEXT_FIELDS.items():
            value = metadata.get(field)
            if value and self._may_fill(state, field) and value != state['values'].get(field):
                updates.append(f"{column} = %s")
                params.append(value)
                written.append(field)

        if metadata.get('series_index') and self._may_fill(state, 'series') \
                and metadata['series_index'] != state['values'].get('series_index'):
            updates.append("series_index = %s")
            params.append(metadata['series_index'])

        # Technical facts about the file itself: not editorial, never locked.
        if metadata.get('format'):
            updates.append("format = %s")
            params.append(metadata['format'])
        if metadata.get('page_count'):
            updates.append("page_count = %s")
            params.append(metadata['page_count'])

        if updates:
            updates.append("updated_at = CURRENT_TIMESTAMP")
            query = f"UPDATE works SET {', '.join(updates)} WHERE id = %s"
            params.append(work_id)
            self.db.execute(query, tuple(params))
        for field in written:
            self._record_source(work_id, field, 'file')

        # Author: same rule, resolved through the person table.
        author = metadata.get('author')
        if author and author != 'Unknown Author' and self._may_fill(state, 'author') \
                and author != state['values'].get('author'):
            author_id = self.db.fetchone(
                """INSERT INTO person (name) VALUES (%s)
                   ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
                   RETURNING id""",
                (author,))
            if author_id:
                self.db.execute("UPDATE works SET author_id = %s WHERE id = %s", (author_id[0], work_id))
                self._record_source(work_id, 'author', 'file')

        # Cover (with lock check), stored on the primary edition.
        if metadata.get('cover_path') and not state['locks'].get('cover'):
            self.save_cover(work_id, metadata['cover_path'], metadata.get('title', ''))

        # Tags found in the file (Many-to-Many)
        for tag_name in metadata.get('tags', []) or []:
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
            text = str(value)
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

        tags = {t for t in (record.get('tags') or []) if t}
        if tags:
            have = {r[0] for r in (self.db.fetchall(
                "SELECT t.name FROM work_tags wt JOIN tags t ON t.id = wt.tag_id WHERE wt.work_id = %s",
                (work_id,)) or [])}
            if not tags <= have:
                propose('tags', json.dumps(sorted(tags)))
        return stored

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
