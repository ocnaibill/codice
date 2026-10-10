"""Tests for the analyzer's database writes, using a fake database that models a
work's current values, locks and field provenance."""
import json
from analyzer import Analyzer, UPSERT_PRIMARY_EDITION_COVER, clean_tag, MAX_TAG

VALUE_NAMES = ['title', 'author', 'series', 'series_index', 'isbn', 'language', 'publisher',
               'publication_date', 'description', 'original_year']
LOCK_NAMES = ['title', 'author', 'series', 'cover', 'isbn', 'language', 'publisher',
              'publication_date', 'description', 'original_year']


class FakeDB:
    """Records statements and answers the analyzer's reads from a given state."""

    def __init__(self, values=None, locks=None, sources=None, tags=(), contributors=()):
        self.values = {n: '' for n in VALUE_NAMES}
        self.values['series_index'] = 0
        self.values['title'] = 'upload.epub'
        self.values.update(values or {})
        self.locks = {n: False for n in LOCK_NAMES}
        self.locks.update(locks or {})
        self.sources = sources or {}
        self.tags = list(tags)
        self.contributors = list(contributors)  # [(name, role)]
        self.statements = []

    def execute(self, query, params=()):
        self.statements.append((" ".join(query.split()), params))

    def fetchone(self, query, params=()):
        if "w.title_lock" in query:
            return tuple(self.values[n] for n in VALUE_NAMES) + tuple(self.locks[n] for n in LOCK_NAMES)
        if "INSERT INTO person" in query:
            self.statements.append((" ".join(query.split()), params))
            return (99,)
        if "SELECT id FROM tags" in query:
            return (1,)
        return None

    def fetchall(self, query, params=()):
        if "work_field_sources" in query:
            return list(self.sources.items())
        if "FROM work_tags" in query:
            return [(t,) for t in self.tags]
        if "FROM work_contributors c JOIN person" in query:
            return list(self.contributors)
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


class TestLongTags:
    LONG = 'Translated by Ebook Translator: https://translator.bookfere.com'

    def test_a_subject_the_column_cannot_hold_is_shortened_not_fatal(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': 'Duna', 'tags': [self.LONG, 'Sci-Fi']})
        written = [p[0] for q, p in db.matching("INSERT INTO tags")]
        assert written and all(len(t) <= MAX_TAG for t in written) and 'Sci-Fi' in written

    def test_shortened_at_a_word_and_never_ending_on_punctuation(self):
        assert clean_tag(self.LONG) == 'Translated by Ebook Translator'
        assert clean_tag('palavra ' * 30) == 'palavra palavra palavra palavra palavra palavra'
        assert not clean_tag('x' * 49 + ',,,,').endswith(',') and len(clean_tag('x' * 80)) == MAX_TAG

    def test_a_tag_that_fits_is_left_alone_but_for_its_spaces(self):
        assert clean_tag('  Ficção   científica ') == 'Ficção científica'
        assert clean_tag('') == '' and clean_tag(None) == ''

    def test_two_subjects_that_become_the_same_tag_are_written_once(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': 'Duna', 'tags': [self.LONG, self.LONG + ' (2)']})
        assert len(db.matching("INSERT INTO tags")) == 1

    def test_a_long_tag_suggested_by_a_provider_is_proposed_as_it_would_be_stored(self):
        db = FakeDB()
        Analyzer(db).save_candidates(7, {'tags': [self.LONG]}, 'openlibrary')
        (params,) = candidates(db).values()
        assert json.loads(params[2]) == ['Translated by Ebook Translator']


class TestAuthorNames:
    def author_inserts(self, db):
        return [p for q, p in db.matching("INSERT INTO person (name")]

    def aliases(self, db):
        return [p for q, p in db.matching("INSERT INTO person_alias")]

    def test_a_catalogues_way_is_stored_as_the_name_people_say_and_what_was_written_is_an_alias(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, dict(NATIVE, author='Herbert, Frank, author'))
        assert self.author_inserts(db) == [('Frank Herbert', 'Herbert', 'Frank')]   # the surname is known: the writing says so
        assert self.aliases(db) == [(99, 'Herbert, Frank, author')]
        assert 'author' in db.recorded_sources()

    def test_a_name_that_needs_no_fixing_gets_no_alias(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, dict(NATIVE, author='Frank Herbert'))
        assert self.author_inserts(db) == [('Frank Herbert', None, None)] and self.aliases(db) == []

    def test_a_name_with_no_role_is_left_as_written(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, dict(NATIVE, author='Herbert, Frank'))
        assert self.author_inserts(db) == [('Herbert, Frank', None, None)] and self.aliases(db) == []

    def test_reading_the_same_file_again_changes_nothing(self):
        db = FakeDB(values={'author': 'Frank Herbert'}, sources={'author': 'file'})
        Analyzer(db).save_metadata(7, dict(NATIVE, author='Herbert, Frank, author'))
        assert self.author_inserts(db) == [] and self.aliases(db) == []

    def test_the_surname_already_known_is_not_replaced(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, dict(NATIVE, author='Herbert, Frank, author'))
        (query, _), = [(q, p) for q, p in db.matching("INSERT INTO person (name")]
        assert "COALESCE(person.family_name, EXCLUDED.family_name)" in query
        assert "CASE WHEN person.family_name IS NULL THEN EXCLUDED.given_name ELSE person.given_name END" in query

    def test_an_unknown_author_is_not_a_person(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, dict(NATIVE, author='Unknown Author'))
        assert self.author_inserts(db) == []


class TestContributorSuggestions:
    def propose(self, record, **db_kw):
        import json
        db = FakeDB(values={'author': 'F. Herbert'}, **db_kw)
        Analyzer(db).save_candidates(7, dict(record), 'Open Library', {})
        found = [json.loads(p[2]) for q, p in db.matching("INSERT INTO metadata_candidates") if p[1] == 'contributors']
        return found[0] if found else None

    def test_the_other_people_a_provider_credits_are_suggested_with_their_roles_but_not_the_first_author(self):
        got = self.propose({'author': 'Terry Pratchett', 'credits': [
            {'name': 'Terry Pratchett', 'ids': {'openlibrary': 'OL1A'}},
            {'name': 'Neil Gaiman', 'ids': {'openlibrary': 'OL2A'}},
            {'name': 'John Schoenherr', 'role': 'illustrator'}]})
        assert got == [{'name': 'Neil Gaiman', 'role': 'author'}, {'name': 'John Schoenherr', 'role': 'illustrator'}]

    def test_nothing_is_suggested_when_only_the_first_author_is_credited_or_nothing_is_credited(self):
        assert self.propose({'author': 'Frank Herbert', 'credits': [{'name': 'Frank Herbert'}]}) is None
        assert self.propose({'author': 'Frank Herbert', 'credits': []}) is None
        assert self.propose({'author': 'Frank Herbert'}) is None

    def test_who_the_work_already_has_in_that_role_is_left_out_whichever_way_the_name_is_written(self):
        got = self.propose({'author': 'Terry Pratchett', 'credits': [
            {'name': 'Terry Pratchett'}, {'name': 'Neil Gaiman'}, {'name': 'John Schoenherr', 'role': 'illustrator'}]},
            contributors=[('Gaiman, Neil', 'author'), ('John Schoenherr', 'translator')])
        # Schoenherr is there as a translator, not as the illustrator he is credited as.
        assert got == [{'name': 'John Schoenherr', 'role': 'illustrator'}]

    def test_a_locked_author_is_given_no_co_authors_but_the_others_are_still_suggested(self):
        got = self.propose({'author': 'Terry Pratchett', 'credits': [
            {'name': 'Neil Gaiman'}, {'name': 'John Schoenherr', 'role': 'illustrator'}]}, locks={'author': True})
        assert got == [{'name': 'John Schoenherr', 'role': 'illustrator'}]

    def test_a_role_the_library_has_no_word_for_is_not_made_up_and_a_person_credited_twice_is_kept_once(self):
        got = self.propose({'author': 'A One', 'credits': [
            {'name': 'B Two', 'role': 'colorist'}, {'name': 'C Three', 'role': 'writer, inker'},
            {'name': 'Three, C', 'role': 'writer'}, {'name': '  ', 'role': 'writer'}]})
        assert got == [{'name': 'C Three', 'role': 'author'}, {'name': 'C Three', 'role': 'illustrator'}]

    def test_a_repeated_suggestion_is_a_no_op_for_the_database(self):
        db = FakeDB(values={'author': 'A One'})
        Analyzer(db).save_candidates(7, {'author': 'A One', 'credits': [{'name': 'B Two'}]}, 'Open Library', {})
        query = [q for q, p in db.matching("INSERT INTO metadata_candidates") if p[1] == 'contributors'][0]
        assert "ON CONFLICT (work_id, field, source, value) DO NOTHING" in query


class TestLongFields:
    """A file's metadata is free text. What a column cannot hold must not fail the analysis (#169)."""
    BLURB = ('Uma história longa sobre um planeta de areia, três famílias e um segredo guardado por séculos, ' * 60).strip()

    def work_values(self, db):
        query, params = db.work_update()
        return dict(zip([c.split(' = ')[0] for c in query.split('SET ')[1].split(', ')], params))

    def edition_values(self, db):
        update = db.edition_update()
        if update is None:
            return {}
        query, params = update
        return dict(zip([c.split(' = ')[0] for c in query.split('SET ')[1].split(' WHERE')[0].split(', ')], params))

    def test_a_title_that_is_a_whole_blurb_is_shortened_at_a_word_and_says_so(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': self.BLURB})
        title = self.work_values(db)['original_title']
        assert len(title) <= 255 and title.endswith('…') and ' …' not in title and self.BLURB.startswith(title[:-1])

    def test_the_limit_is_in_characters_not_bytes(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': 'ç' * 255})
        assert self.work_values(db)['original_title'] == 'ç' * 255   # 510 bytes, 255 characters: it fits
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': 'ç' * 256})
        assert len(self.work_values(db)['original_title']) == 255

    def test_series_and_publisher_have_their_own_limits(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': 'Duna', 'series': 'S' * 600, 'publisher': 'P' * 300})
        assert len(self.work_values(db)['series']) == 512
        assert len(self.edition_values(db)['publisher']) == 256

    def test_a_value_that_would_be_another_one_if_cut_is_left_out(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': 'Duna', 'isbn': '9' * 80, 'language': 'portuguese-brazilian-x', 'publication_date': '2020-05-01T00:00:00Z-and-more-text'})
        assert self.edition_values(db) == {}
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': 'Duna', 'isbn': '9788576573135', 'language': 'pt-BR', 'publication_date': '2020-05-01'})
        assert self.edition_values(db) == {'isbn': '9788576573135', 'language': 'pt-BR', 'publication_date': '2020-05-01'}

    def test_an_author_name_that_is_too_long_and_its_alias_are_shortened(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': 'Duna', 'author': ('H' * 300) + ', ' + ('F' * 300) + ', author'})
        (name, family, given), = [p for q, p in db.matching("INSERT INTO person (name")]
        assert all(v is not None and len(v) <= 255 for v in (name, family, given))
        (alias,) = [p[1] for q, p in db.matching("INSERT INTO person_alias")]
        assert len(alias) == 255 and alias.endswith('…')

    def test_the_limits_are_exact_at_the_edge(self):
        for field, limit in (('author', 255),):
            db = FakeDB()
            Analyzer(db).save_metadata(7, {'title': 'Duna', field: 'a' * limit})
            assert [p[0] for q, p in db.matching("INSERT INTO person (name")] == ['a' * limit]
            db = FakeDB()
            Analyzer(db).save_metadata(7, {'title': 'Duna', field: 'a' * (limit + 1)})
            assert len([p[0] for q, p in db.matching("INSERT INTO person (name")][0]) == limit
        for field, limit in (('isbn', 64), ('language', 16), ('publication_date', 32)):
            db = FakeDB()
            Analyzer(db).save_metadata(7, {'title': 'Duna', field: '1' * limit})
            assert self.edition_values(db) == {field: '1' * limit}
            db = FakeDB()
            Analyzer(db).save_metadata(7, {'title': 'Duna', field: '1' * (limit + 1)})
            assert self.edition_values(db) == {}

    def test_what_fits_and_what_has_no_limit_is_left_alone(self):
        db = FakeDB()
        description = 'texto ' * 5000
        Analyzer(db).save_metadata(7, {'title': 'Duna', 'description': description, 'series': 'Duna', 'series_index': 2.0})
        values = self.work_values(db)
        assert values['description'] == description and values['original_title'] == 'Duna' and values['series_index'] == 2.0

    def test_reading_the_same_long_title_again_changes_nothing(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, {'title': self.BLURB})
        shortened = self.work_values(db)['original_title']
        again = FakeDB(values={'title': shortened}, sources={'title': 'file'})
        Analyzer(again).save_metadata(7, {'title': self.BLURB})
        assert again.work_update() is None

    def test_a_suggestion_is_shortened_too_so_that_accepting_it_cannot_fail(self):
        db = FakeDB()
        Analyzer(db).save_candidates(7, {'title': self.BLURB, 'isbn': '9' * 80, 'publisher': 'Aleph'}, 'openlibrary')
        proposed = {k[0]: v for k, v in candidates(db).items()}
        assert len(proposed['title'][2]) <= 255 and 'isbn' not in proposed and 'publisher' in proposed


class TestShorten:
    def test_a_text_that_fits_is_the_same_text(self):
        from analyzer import shorten
        assert shorten('Duna', 255) == 'Duna' and shorten('x' * 255, 255) == 'x' * 255

    def test_it_cuts_at_a_word_when_the_word_is_not_too_far_back(self):
        from analyzer import shorten
        assert shorten('um dois três quatro cinco', 16) == 'um dois três…'

    def test_it_cuts_anywhere_when_there_is_no_word_to_cut_at(self):
        from analyzer import shorten
        assert shorten('x' * 100, 20) == 'x' * 19 + '…'
        assert shorten('ab ' + 'x' * 100, 20) == 'ab ' + 'x' * 16 + '…'   # a space too far back is not worth cutting at

    def test_it_never_ends_on_punctuation_before_the_ellipsis(self):
        from analyzer import shorten
        assert shorten('palavra, outra, e mais coisas', 17) == 'palavra, outra…'


class TestOriginalYearCandidates:
    """The year a work was first published (DEC-156): suggested like the other descriptive fields, never applied by itself."""

    def propose(self, db, **record):
        Analyzer(db).save_candidates(7, record, 'OpenLibrary')
        return [p for q, p in db.matching('INSERT INTO metadata_candidates')]

    def test_a_year_is_suggested(self):
        db = FakeDB()
        rows = self.propose(db, original_year='1965')
        assert [(p[1], p[2]) for p in rows] == [('original_year', '1965')]

    def test_the_year_the_work_has_is_not_suggested_again(self):
        db = FakeDB(values={'original_year': '1965'})
        assert self.propose(db, original_year='1965') == []

    def test_a_year_that_is_locked_is_not_suggested(self):
        db = FakeDB(values={'original_year': '1970'}, locks={'original_year': True})
        assert self.propose(db, original_year='1965') == []

    def test_what_is_not_a_year_of_a_work_is_not_suggested(self):
        for bad in ('1965-05-01', 'sem data', '0', '10000', '-3001', '19 65', '01965'):
            assert self.propose(FakeDB(), original_year=bad) == [], bad

    def test_a_year_before_the_common_era_is_one(self):
        rows = self.propose(FakeDB(), original_year='-384')
        assert [p[2] for p in rows] == ['-384']

    def test_the_year_is_never_written_to_the_work_by_the_native_metadata(self):
        db = FakeDB()
        Analyzer(db).save_metadata(7, dict(NATIVE, original_year='1965'))
        query, _ = db.work_update()
        assert 'original_year' not in query
