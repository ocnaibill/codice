"""Tests for the analyzer's database writes, using a recording fake database."""
from analyzer import Analyzer, UPSERT_PRIMARY_EDITION_COVER


class RecordingDB:
    """Records every statement instead of talking to PostgreSQL."""

    def __init__(self, locks=(False, False, False, False)):
        self.statements = []
        self._locks = locks

    def execute(self, query, params=()):
        self.statements.append((" ".join(query.split()), params))

    def fetchone(self, query, params=()):
        if "title_lock" in query:
            return self._locks
        return None


def statements_containing(db, text):
    return [(q, p) for q, p in db.statements if text in q]


class TestCoverUpsert:
    def test_targets_the_primary_edition_index(self):
        # `ON CONFLICT (work_id)` without the predicate matches no constraint now
        # that a work can have several editions, and PostgreSQL rejects it.
        sql = " ".join(UPSERT_PRIMARY_EDITION_COVER.split())
        assert "ON CONFLICT (work_id) WHERE is_primary" in sql
        assert "DO UPDATE SET cover_url" in sql

    def test_save_metadata_stores_the_cover_with_that_statement(self):
        db = RecordingDB()
        Analyzer(db).save_metadata(7, {"title": "Duna", "cover_path": "/covers/duna.jpg"})

        writes = statements_containing(db, "INSERT INTO editions")
        assert len(writes) == 1
        query, params = writes[0]
        assert query == " ".join(UPSERT_PRIMARY_EDITION_COVER.split())
        assert params == (7, "Duna", "/covers/duna.jpg")

    def test_a_locked_cover_is_left_alone(self):
        db = RecordingDB(locks=(False, False, False, True))
        Analyzer(db).save_metadata(7, {"title": "Duna", "cover_path": "/covers/duna.jpg"})
        assert statements_containing(db, "INSERT INTO editions") == []


class TestLocks:
    def test_locked_title_and_author_are_not_overwritten(self):
        db = RecordingDB(locks=(True, True, False, False))
        Analyzer(db).save_metadata(7, {"title": "Outro", "author": "Alguém", "language": "pt"})

        updates = statements_containing(db, "UPDATE works SET")
        assert all("original_title" not in q for q, _ in updates)
        assert statements_containing(db, "INSERT INTO person") == []
        # Unlocked fields still update.
        assert any("language = %s" in q for q, _ in updates)
