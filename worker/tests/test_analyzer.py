"""Tests for the analyzer's database writes, using a fake database that models a
work's current values, locks and field provenance."""
import json
from analyzer import Analyzer, UPSERT_PRIMARY_EDITION_COVER

VALUE_NAMES = ['title', 'author', 'series', 'series_index', 'isbn', 'language', 'publisher',
               'publication_date', 'description']
LOCK_NAMES = ['title', 'author', 'series', 'cover', 'isbn', 'language', 'publisher',
              'publication_date', 'description']


class FakeDB:
    """Records statements and answers the analyzer's reads from a given state."""

    def __init__(self, values=None, locks=None, sources=None, tags=()):
        self.values = {n: '' for n in VALUE_NAMES}
        self.values['series_index'] = 0
        self.values['title'] = 'upload.epub'
        self.values.update(values or {})
        self.locks = {n: False for n in LOCK_NAMES}
        self.locks.update(locks or {})
        self.sources = sources or {}
        self.tags = list(tags)
        self.statements = []

    def execute(self, query, params=()):
        self.statements.append((" ".join(query.split()), params))

    def fetchone(self, query, params=()):
        if "w.title_lock" in query:
            return tuple(self.values[n] for n in VALUE_NAMES) + tuple(self.locks[n] for n in LOCK_NAMES)
        if "INSERT INTO person" in query:
            return (99,)
        if "SELECT id FROM tags" in query:
            return (1,)
        return None

    def fetchall(self, query, params=()):
        if "work_field_sources" in query:
            return list(self.sources.items())
        if "FROM work_tags" in query:
            return [(t,) for t in self.tags]
        return []

    # helpers for assertions
    def matching(self, text):
        return [(q, p) for q, p in self.statements if text in q]

    def work_update(self):
        ups = self.matching("UPDATE works SET")
        return ups[0] if ups else None

    def edition_update(self):
        ups = self.matching("UPDATE editions SET")
        return ups[0] if ups else None

    def recorded_sources(self):
        return {p[1]: p[2] for q, p in self.matching("INSERT INTO work_field_sources")}


NATIVE = {'title': 'Duna', 'author': 'Frank Herbert', 'format': 'epub', 'page_count': 500,
          'language': 'pt', 'publisher': 'Aleph', 'isbn': '111', 'description': 'texto do arquivo'}


class TestNativeMetadata:
    def test_fills_empty_fields_and_records_that_they_came_from_the_file(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, dict(NATIVE))

        query, params = db.work_update()
        for column in ("original_title = %s", "description = %s", "page_count = %s"):
            assert column in query
        # What describes the edition is written to the primary edition, and the format to its file.
        edition_query, _ = db.edition_update()
        for column in ("language = %s", "publisher = %s", "isbn = %s"):
            assert column in edition_query
        assert "is_primary" in edition_query
        assert db.matching("UPDATE files SET format")[0][1] == ('epub', 7)
        assert db.recorded_sources() == {
            'title': 'file', 'language': 'file', 'publisher': 'file', 'isbn': 'file',
            'description': 'file', 'author': 'file'}

    def test_does_not_overwrite_a_value_of_unknown_origin(self):
        # A value that predates provenance may have been typed by a person.
        db = FakeDB(values={'title': 'Título antigo', 'publisher': 'Editora antiga'}, sources={})
        Analyzer(db).save_metadata(7, dict(NATIVE))
        query, _ = db.work_update()
        edition_query, _ = db.edition_update()
        assert "original_title" not in query and "publisher" not in edition_query
        assert "language = %s" in edition_query  # an empty field is still filled

    def test_does_not_overwrite_a_confirmed_value(self):
        db = FakeDB(values={'title': 'Corrigido', 'isbn': '999'},
                    sources={'title': 'manual', 'isbn': 'openlibrary'})
        Analyzer(db).save_metadata(7, dict(NATIVE))
        query, _ = db.work_update()
        edition_query, _ = db.edition_update()
        assert "original_title" not in query and "isbn" not in edition_query

    def test_does_not_overwrite_a_locked_field_even_if_empty(self):
        db = FakeDB(locks={'title': True, 'author': True, 'publisher': True})
        Analyzer(db).save_metadata(7, dict(NATIVE))
        query, _ = db.work_update()
        edition_query, _ = db.edition_update()
        assert "original_title" not in query and "publisher" not in edition_query
        assert db.matching("INSERT INTO person") == []
        assert 'title' not in db.recorded_sources() and 'author' not in db.recorded_sources()

    def test_refreshes_a_value_that_was_itself_read_from_the_file(self):
        db = FakeDB(values={'title': 'Leitura antiga'}, sources={'title': 'file'})
        Analyzer(db).save_metadata(7, dict(NATIVE))
        query, params = db.work_update()
        assert "original_title = %s" in query and 'Duna' in params

    def test_an_unchanged_value_causes_no_write_or_provenance_entry(self):
        db = FakeDB(values={'title': 'Duna', 'author': 'Frank Herbert'}, sources={'title': 'file', 'author': 'file'})
        Analyzer(db).save_metadata(7, {'title': 'Duna', 'author': 'Frank Herbert'})
        assert db.work_update() is None and db.edition_update() is None
        assert db.recorded_sources() == {}


class TestCover:
    def test_targets_the_primary_edition_index(self):
        # `ON CONFLICT (work_id)` without the predicate matches no constraint now
        # that a work can have several editions, and PostgreSQL rejects it.
        sql = " ".join(UPSERT_PRIMARY_EDITION_COVER.split())
        assert "ON CONFLICT (work_id) WHERE is_primary" in sql

    def test_save_metadata_stores_the_cover_with_that_statement(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, {"title": "Duna", "cover_path": "/covers/duna.jpg"})
        writes = db.matching("INSERT INTO editions")
        assert len(writes) == 1
        assert writes[0][0] == " ".join(UPSERT_PRIMARY_EDITION_COVER.split())
        assert writes[0][1] == (7, "Duna", "/covers/duna.jpg")

    def test_a_locked_cover_is_left_alone(self):
        db = FakeDB(locks={'cover': True})
        Analyzer(db).save_metadata(7, {"title": "Duna", "cover_path": "/covers/duna.jpg"})
        assert db.matching("INSERT INTO editions") == []


def candidates(db):
    return {(p[1], p[2], p[3]): p for q, p in db.matching("INSERT INTO metadata_candidates")}


class TestCandidates:
    def test_stores_differing_fields_and_applies_nothing(self):
        db = FakeDB(values={'title': 'upload.epub'})
        stored = Analyzer(db).save_candidates(
            7, {'title': 'Duna', 'author': 'Frank Herbert', 'isbn': '9788576573135', 'series_index': 1.0},
            'openlibrary', {'openlibrary_id': 'OL1'})
        assert stored == 4
        assert set(k[0] for k in candidates(db)) == {'title', 'author', 'isbn', 'series_index'}
        # The work itself is untouched.
        assert db.matching("UPDATE works") == [] and db.matching("INSERT INTO person") == []
        params = next(iter(candidates(db).values()))
        assert params[3] == 'openlibrary' and json.loads(params[4]) == {'openlibrary_id': 'OL1'}

    def test_skips_locked_empty_and_identical_values(self):
        db = FakeDB(values={'title': 'Duna', 'publisher': 'Aleph'}, locks={'isbn': True})
        Analyzer(db).save_candidates(
            7, {'title': 'Duna',            # identical
                'isbn': '111',              # locked
                'publisher': 'Aleph',       # identical
                'language': '',             # nothing to propose
                'description': None},       # nothing to propose
            'google_books')
        assert candidates(db) == {}

    def test_series_index_is_compared_as_a_number(self):
        db = FakeDB(values={'series_index': 1})
        Analyzer(db).save_candidates(7, {'series_index': 1.0}, 'comicvine')
        assert candidates(db) == {}

    def test_the_series_lock_also_covers_the_series_number(self):
        db = FakeDB(locks={'series': True})
        Analyzer(db).save_candidates(7, {'series': 'Watchmen', 'series_index': 2}, 'comicvine')
        assert candidates(db) == {}

    def test_tags_are_proposed_only_when_something_is_new(self):
        db = FakeDB(tags=['Sci-Fi'])
        Analyzer(db).save_candidates(7, {'tags': ['Sci-Fi']}, 'openlibrary')
        assert candidates(db) == {}
        Analyzer(db).save_candidates(7, {'tags': ['Sci-Fi', 'Clássico']}, 'openlibrary')
        (params,) = candidates(db).values()
        assert params[1] == 'tags' and json.loads(params[2]) == ['Clássico', 'Sci-Fi']

    def test_repeating_a_suggestion_is_a_noop_in_the_database(self):
        db = FakeDB()
        Analyzer(db).save_candidates(7, {'title': 'Duna'}, 'openlibrary')
        query, _ = db.matching("INSERT INTO metadata_candidates")[0]
        assert "ON CONFLICT (work_id, field, source, value) DO NOTHING" in query


class TestAuthor:
    def test_becomes_the_first_author_contributor(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, dict(NATIVE))
        assert db.matching("UPDATE works SET author_id") == []
        assert db.matching("DELETE FROM work_contributors")[0][1] == (7,)
        insert = db.matching("INSERT INTO work_contributors")[0]
        assert insert[1] == (7, 99) and "'author', 0" in insert[0]
