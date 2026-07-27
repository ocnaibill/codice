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
            SET media_status = $1, media_error = $2, updated_at = CURRENT_TIMESTAMP
            WHERE id = $3
        """
        self.db.execute(query, (status.value, error, work_id))
        print(f"   📊 Work {work_id} status → {status.value}")

    def save_metadata(self, work_id: int, metadata: dict):
        """Save extracted metadata to database."""
        query = """
            UPDATE works
            SET original_title = COALESCE($1, original_title),
                format = COALESCE($2, format),
                page_count = COALESCE($3, page_count),
                updated_at = CURRENT_TIMESTAMP
            WHERE id = $4
        """
        self.db.execute(
            query,
            (
                metadata.get('title'),
                metadata.get('format'),
                metadata.get('page_count'),
                work_id,
            )
        )

        # Update author
        author = metadata.get('author')
        if author and author != 'Unknown Author':
            author_query = """
                INSERT INTO person (name) VALUES ($1)
                ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
                RETURNING id
            """
            author_id = self.db.fetchone(author_query, (author,))
            if author_id:
                self.db.execute(
                    "UPDATE works SET author_id = $1 WHERE id = $2",
                    (author_id[0], work_id)
                )

        # Update cover
        cover_path = metadata.get('cover_path')
        if cover_path:
            self.db.execute(
                "UPDATE editions SET cover_url = $1 WHERE work_id = $2",
                (cover_path, work_id)
            )