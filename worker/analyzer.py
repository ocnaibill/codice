"""Media analysis pipeline with status lifecycle.

Inspired by Komga's media analysis pattern:
UNKNOWN → QUEUED → ANALYZING → READY | ERROR
Tracked per-work in the database.
"""
from enum import Enum
from typing import Optional
from dataclasses import dataclass
from datetime import datetime


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

    def save_metadata(self, work_id: int, metadata: dict):
        """Save extracted metadata to database, respecting lock columns."""

        # 1. Check which fields are locked (user manually edited → don't overwrite)
        locks = self.db.fetchone(
            "SELECT title_lock, author_lock, series_lock, cover_lock FROM works WHERE id = %s",
            (work_id,)
        )
        title_lock = locks[0] if locks else False
        author_lock = locks[1] if locks else False
        series_lock = locks[2] if locks else False
        cover_lock = locks[3] if locks else False

        # 2. Build dynamic UPDATE (skip locked fields)
        updates = []
        params = []

        if not title_lock and metadata.get('title'):
            updates.append("original_title = %s")
            params.append(metadata['title'])

        if metadata.get('format'):
            updates.append("format = %s")
            params.append(metadata['format'])

        if metadata.get('page_count'):
            updates.append("page_count = %s")
            params.append(metadata['page_count'])

        if not series_lock and metadata.get('series'):
            updates.append("series = %s")
            params.append(metadata['series'])

        if not series_lock and metadata.get('series_index'):
            updates.append("series_index = %s")
            params.append(metadata['series_index'])

        if metadata.get('isbn'):
            updates.append("isbn = %s")
            params.append(metadata['isbn'])

        if metadata.get('language'):
            updates.append("language = %s")
            params.append(metadata['language'])

        if metadata.get('publisher'):
            updates.append("publisher = %s")
            params.append(metadata['publisher'])

        if metadata.get('publication_date'):
            updates.append("publication_date = %s")
            params.append(metadata['publication_date'])

        if metadata.get('description'):
            updates.append("description = %s")
            params.append(metadata['description'])

        if updates:
            updates.append("updated_at = CURRENT_TIMESTAMP")
            query = f"UPDATE works SET {', '.join(updates)} WHERE id = %s"
            params.append(work_id)
            self.db.execute(query, tuple(params))

        # 3. Update author (with lock check)
        author = metadata.get('author')
        if author and author != 'Unknown Author' and not author_lock:
            author_query = """
                INSERT INTO person (name) VALUES (%s)
                ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
                RETURNING id
            """
            author_id = self.db.fetchone(author_query, (author,))
            if author_id:
                self.db.execute(
                    "UPDATE works SET author_id = %s WHERE id = %s",
                    (author_id[0], work_id)
                )

        # 4. Update cover (with lock check)
        cover_path = metadata.get('cover_path')
        if cover_path and not cover_lock:
            self.db.execute(
                """INSERT INTO editions (work_id, title, cover_url)
                   VALUES (%s, %s, %s)
                   ON CONFLICT (work_id)
                   DO UPDATE SET cover_url = EXCLUDED.cover_url""",
                (work_id, metadata.get('title', ''), cover_path)
            )

        # 5. Save tags (Many-to-Many)
        tags = metadata.get('tags', [])
        if tags:
            for tag_name in tags:
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
