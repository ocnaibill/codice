"""Searching the providers again for a work that is already in the library (DEC-143)."""
import json
from types import SimpleNamespace

import pytest

from analyzer import Analyzer
from providers.base import Credit, MetadataRecord
from rematch import MetadataRematch, title_of, with_series
from tests.test_analyzer import FakeDB


class RematchDB(FakeDB):
    """The database of a work: its format and whether it was retired, and how many rows an INSERT really added."""

    def __init__(self, fmt='epub', retired=False, exists=True, added=None, **kw):
        super().__init__(**kw)
        self.format, self.retired, self.exists, self.added = fmt, retired, exists, added

    def fetchone(self, query, params=()):
        if 'file_format FROM work_primary' in query:
            return (self.format, self.retired) if self.exists else None
        return super().fetchone(query, params)

    def execute(self, query, params=()):
        super().execute(query, params)
        if 'INSERT INTO metadata_candidates' in query and self.added is not None:
            return self.added(params)
        return None


class Registry:
    def __init__(self, record=None, boom=None):
        self.record, self.boom, self.asked = record, boom, []

    def search_best(self, title, fmt, author=None, isbn=None):
        self.asked.append((title, fmt, author, isbn))
        if self.boom:
            raise self.boom
        return self.record


def answer(**kw):
    base = dict(title='A nuvem', author='Neal Shusterman', series=None, series_index=None, isbn='9788554511456', language=None, publisher='Seguinte',
                publication_date='2018', description='Sinopse', tags=['Fiction'], source='Google Books', raw={'google_id': 'g1'}, cover_url=None,
                credits=[Credit('Neal Shusterman')], match={'score': 300.0, 'isbn': True})
    base.update(kw)
    return SimpleNamespace(**base)


def run(db, registry, job_id=7, work_id=3):
    return MetadataRematch(db, Analyzer(db), registry).run(job_id, work_id)


def candidates(db):
    return {p[1]: p for q, p in db.matching('INSERT INTO metadata_candidates')}


class TestTitleToSearch:
    def test_the_title_of_the_work_is_what_is_searched(self):
        assert title_of(Analyzer(None), '  A nuvem (Scythe) ') == 'A nuvem (Scythe)' and title_of(Analyzer(None), None) == ''

    def test_a_work_still_called_by_its_file_is_searched_by_the_name_without_the_extension(self):
        for name, title in (('A_nuvem.epub', 'A nuvem'), ('Duna.PDF', 'Duna'), ('upload.cbz', 'upload'), ('Dom_Casmurro.mobi', 'Dom Casmurro')):
            assert title_of(Analyzer(None), name) == title, name


class TestTheSeriesFindsWhatTheTitleAloneDoesNot:
    def test_a_series_the_title_does_not_say_is_added_as_what_the_title_says_in_parentheses(self):
        assert with_series('A nuvem', 'Scythe') == 'A nuvem (Scythe)'
        assert with_series('A nuvem', '  Scythe ') == 'A nuvem (Scythe)'

    def test_nothing_is_added_when_there_is_no_series_or_the_title_already_says_it(self):
        for title, series in (('A nuvem', None), ('A nuvem', ''), ('A nuvem', '   '), ('Scythe: A nuvem', 'Scythe'), ('A nuvem (Scythe)', 'scythe'),
                              ('Harry Potter e a Pedra Filosofal', 'Harry Potter'), ('', 'Scythe'), (None, 'Scythe')):
            assert with_series(title, series) == (title if title else title), (title, series)

    def test_a_series_with_a_word_the_title_lacks_is_added_whole(self):
        assert with_series('Duna', 'Crônicas de Duna') == 'Duna (Crônicas de Duna)'

    def test_the_search_asks_with_it(self):
        registry = Registry(answer())
        run(RematchDB(values={'title': 'A nuvem', 'series': 'Scythe', 'author': 'Neal Shusterman'}), registry)
        assert registry.asked == [('A nuvem (Scythe)', 'epub', 'Neal Shusterman', None)]


class TestSearchingAgain:
    def test_it_asks_with_what_the_work_says_now_and_stores_what_comes_back_as_suggestions(self, capsys):
        db = RematchDB(values={'title': 'A nuvem (Scythe)', 'author': 'Neal Shusterman', 'isbn': '9788554511456', 'description': 'Antiga'})
        registry = Registry(answer())
        result = run(db, registry)
        assert registry.asked == [('A nuvem (Scythe)', 'epub', 'Neal Shusterman', '9788554511456')]
        assert result == {'found': True, 'source': 'Google Books', 'title': 'A nuvem', 'new': 5}
        got = candidates(db)
        assert set(got) == {'title', 'publisher', 'publication_date', 'description', 'tags'}
        assert json.loads(got['title'][4])['match'] == {'score': 300.0, 'isbn': True} and json.loads(got['title'][4])['query'] == 'A nuvem (Scythe)'
        assert 'Searched again' in capsys.readouterr().out

    def test_the_outcome_is_written_into_the_job_for_the_page_to_read(self):
        db = RematchDB(values={'title': 'Duna'})
        run(db, Registry(answer(title='Dune')), job_id=41)
        (query, params), = db.matching('UPDATE jobs SET payload')
        assert 'payload = payload || %s::jsonb' in query and 'WHERE id = %s' in query and params[1] == 41
        assert json.loads(params[0]) == {'result': {'found': True, 'source': 'Google Books', 'title': 'Dune', 'new': 7}}

    def test_a_field_a_person_confirmed_gets_a_suggestion_too_because_whoever_asked_wants_to_change_it(self):
        db = RematchDB(values={'title': 'Duna', 'publisher': 'Antiga'}, locks={'title': True, 'publisher': True, 'description': True})
        run(db, Registry(answer(title='Dune')))
        assert {'title', 'publisher', 'description'} <= set(candidates(db))

    def test_what_the_work_already_has_is_not_suggested_and_what_was_rejected_before_does_not_come_back(self):
        db = RematchDB(values={'title': 'A nuvem', 'publisher': 'Seguinte', 'author': 'Neal Shusterman', 'isbn': '9788554511456'}, added=lambda p: 0 if p[1] == 'description' else 1)
        result = run(db, Registry(answer()))
        assert set(candidates(db)) == {'publication_date', 'description', 'tags'} and result['new'] == 2   # the description was rejected: the database added no row

    def test_nothing_new_is_a_search_that_found_the_work_and_has_nothing_to_add(self):
        db = RematchDB(values={'title': 'A nuvem'}, added=lambda p: 0)
        assert run(db, Registry(answer())) == {'found': True, 'source': 'Google Books', 'title': 'A nuvem', 'new': 0}

    def test_no_provider_finding_the_work_is_told(self):
        db = RematchDB(values={'title': 'Obra Desconhecida'})
        assert run(db, Registry(None)) == {'found': False, 'new': 0}
        (query, params), = db.matching('UPDATE jobs SET payload')
        assert json.loads(params[0]) == {'result': {'found': False, 'new': 0}} and candidates(db) == {}

    def test_a_work_with_no_title_asks_nobody(self):
        db = RematchDB(values={'title': ''})
        registry = Registry(answer())
        assert run(db, registry) == {'found': False, 'new': 0} and registry.asked == []

    def test_what_the_work_does_not_say_is_not_asked(self):
        registry = Registry(None)
        run(RematchDB(fmt='', values={'title': 'Duna'}), registry)
        assert registry.asked == [('Duna', 'default', None, None)]

    def test_a_work_that_is_gone_or_retired_is_an_error_that_will_not_be_retried(self):
        from runner import classify
        for db in (RematchDB(exists=False), RematchDB(retired=True)):
            with pytest.raises(ValueError) as caught:
                run(db, Registry(answer()))
            assert classify(caught.value) == 'permanent'
        assert RematchDB(exists=False).matching('UPDATE jobs') == []

    def test_a_provider_that_breaks_fails_the_job_so_that_it_is_tried_again(self):
        from runner import classify
        db = RematchDB(values={'title': 'Duna'})
        with pytest.raises(ConnectionError) as caught:
            run(db, Registry(boom=ConnectionError('down')))
        assert classify(caught.value) == 'temporary' and db.matching('UPDATE jobs') == []

    def test_the_job_can_be_cancelled_before_the_providers_are_asked(self):
        class Cancelled(Exception):
            pass

        def stop():
            raise Cancelled()
        registry = Registry(answer())
        with pytest.raises(Cancelled):
            MetadataRematch(RematchDB(values={'title': 'Duna'}), Analyzer(RematchDB()), registry).run(1, 3, checkpoint=stop)
        assert registry.asked == []


class TestAnalyzerSuggestions:
    def test_the_first_analysis_still_leaves_confirmed_fields_alone(self):
        db = FakeDB(values={'title': 'Duna'}, locks={'title': True})
        Analyzer(db).save_candidates(7, {'title': 'Dune', 'publisher': 'Aleph'}, 'x')
        assert set(candidates(db)) == {'publisher'}

    def test_a_locked_author_is_given_co_authors_only_when_asked(self):
        record = {'author': 'A One', 'credits': [{'name': 'A One', 'role': 'author'}, {'name': 'B Two', 'role': 'author'}]}
        db = FakeDB(values={'author': 'A One'}, locks={'author': True})
        Analyzer(db).save_candidates(7, dict(record), 'x')
        assert 'contributors' not in candidates(db)
        db = FakeDB(values={'author': 'A One'}, locks={'author': True})
        Analyzer(db).save_candidates(7, dict(record), 'x', include_locked=True)
        assert json.loads(candidates(db)['contributors'][2]) == [{'name': 'B Two', 'role': 'author'}]

    def test_the_count_is_of_the_rows_the_database_added(self):
        for added, expected in ((None, 2), (0, 0), (1, 2)):
            db = RematchDB(values={}, added=(lambda p: added) if added is not None else None)
            assert Analyzer(db).save_candidates(7, {'title': 'Duna', 'publisher': 'Aleph'}, 'x') == expected


class TestTheDatabaseSaysHowManyRowsWentIn:
    def test_execute_gives_the_rows_it_changed(self):
        from unittest.mock import MagicMock, patch
        from db import CodiceDatabase
        for rowcount in (0, 1, 3):
            cursor = MagicMock(rowcount=rowcount)
            connection = MagicMock()
            connection.__enter__.return_value.cursor.return_value.__enter__.return_value = cursor
            with patch('db.psycopg2.connect', return_value=connection):
                assert CodiceDatabase().execute('INSERT INTO x VALUES (%s)', (1,)) == rowcount
            cursor.execute.assert_called_once_with('INSERT INTO x VALUES (%s)', (1,))
