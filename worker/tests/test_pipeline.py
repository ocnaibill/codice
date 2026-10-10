"""Tests for the analysis pipeline: the file writes, providers only suggest."""
from types import SimpleNamespace

from pipeline import analyze_file
from analyzer import Analyzer
from tests.test_analyzer import FakeDB


def meta(**kw):
    base = dict(title='Duna', author='Frank Herbert', format='epub', page_count=500, cover_path=None,
                series=None, series_index=None, isbn=None, language='pt', publisher=None,
                publication_date=None, description=None, tags=[], raw={})
    base.update(kw)
    return SimpleNamespace(**base)


class FakeExtractor:
    def __init__(self, metadata):
        self.metadata = metadata

    def extract(self, file_path, covers_dir):
        return self.metadata


class FakeProviders:
    def __init__(self, record=None, cover='/covers/provider.jpg'):
        self.record = record
        self.cover = cover
        self.downloads = []

    def search_best(self, title, fmt, author=None, isbn=None):
        self.asked = (title, fmt, author)
        self.asked_isbn = isbn
        return self.record

    def download_cover(self, url, file_path, covers_dir):
        self.downloads.append(url)
        return self.cover


def record(**kw):
    base = dict(title='Dune', author='F. Herbert', series=None, series_index=None, isbn='9788576573135',
                language=None, publisher=None, publication_date=None, description='Sinopse do provedor',
                tags=['Sci-Fi'], cover_url='http://x/c.jpg', source='openlibrary', raw={'openlibrary_id': 'OL1'})
    base.update(kw)
    return SimpleNamespace(**base)


def run(db, metadata, providers):
    return analyze_file(7, '/f/duna.epub', FakeExtractor(metadata), Analyzer(db), providers, '/covers')


class TestPipeline:
    def test_provider_data_becomes_suggestions_and_never_overwrites(self):
        db = FakeDB()
        run(db, meta(), FakeProviders(record()))

        # The file's own title is what the work got, not the provider's.
        query, params = db.work_update()
        assert 'Duna' in params and 'Dune' not in params
        assert 'Sinopse do provedor' not in params and '9788576573135' not in params

        suggested = {p[1]: p[2] for q, p in db.matching("INSERT INTO metadata_candidates")}
        assert suggested['title'] == 'Dune' and suggested['isbn'] == '9788576573135'
        assert suggested['description'] == 'Sinopse do provedor'
        # Provenance says what was read from the file, and nothing from the provider.
        assert set(db.recorded_sources().values()) == {'file'}

    def test_the_credits_of_a_provider_travel_as_evidence_of_the_suggestion(self):
        import json
        from providers.base import Credit
        db = FakeDB()
        credits = [Credit('Frank Herbert', ids={'openlibrary': 'OL79034A'}), Credit('John Schoenherr', 'illustrator')]
        run(db, meta(), FakeProviders(record(credits=credits)))
        rows = db.matching("INSERT INTO metadata_candidates")
        assert rows
        for q, p in rows:
            evidence = json.loads(p[-1])
            assert evidence['credits'] == [
                {'name': 'Frank Herbert', 'ids': {'openlibrary': 'OL79034A'}},
                {'name': 'John Schoenherr', 'role': 'illustrator'}]
            assert evidence['query'] == 'Duna'

    def test_how_close_the_answer_is_travels_as_evidence_and_the_providers_are_asked_with_the_files_author(self):
        import json
        db = FakeDB()
        providers = FakeProviders(record(match={'score': 160.0, 'title': 1.0, 'author': 1.0, 'accepted': True, 'reason': ''}))
        run(db, meta(), providers)
        assert providers.asked == ('Duna', 'epub', 'Frank Herbert') and providers.asked_isbn is None
        rows = db.matching("INSERT INTO metadata_candidates")
        assert rows
        for q, p in rows:
            assert json.loads(p[-1])['match'] == {'score': 160.0, 'title': 1.0, 'author': 1.0, 'accepted': True, 'reason': ''}

    def test_the_ids_a_provider_knows_the_work_by_travel_as_evidence(self):
        import json
        db = FakeDB()
        run(db, meta(), FakeProviders(record(raw={'anilist_id': 30002, 'mangadex_id': 'abc-1', 'openlibrary_id': 'OL1W', 'wikidata_id': 'Q190192', 'translated_from': 'Duna', 'other': 'x'})))
        rows = db.matching("INSERT INTO metadata_candidates")
        assert rows
        for q, p in rows:
            evidence = json.loads(p[-1])
            assert (evidence['anilist_id'], evidence['mangadex_id'], evidence['openlibrary_id']) == (30002, 'abc-1', 'OL1W') and 'other' not in evidence
            assert (evidence['wikidata_id'], evidence['translated_from']) == ('Q190192', 'Duna')

    def test_the_origin_of_a_suggestion_never_overflows_the_column(self):
        db = FakeDB()
        run(db, meta(), FakeProviders(record(source='x' * 100)))
        rows = db.matching("INSERT INTO metadata_candidates")
        assert rows and all(len(p[3]) == 64 for q, p in rows)
        db = FakeDB()
        run(db, meta(), FakeProviders(record(source='OpenLibrary + Wikidata + Wikipedia')))
        assert all(p[3] == 'OpenLibrary + Wikidata + Wikipedia' for q, p in db.matching("INSERT INTO metadata_candidates"))

    def test_the_isbn_of_the_file_is_one_of_the_things_the_providers_are_asked_with(self):
        providers = FakeProviders(record())
        run(FakeDB(), meta(isbn='9788554511456'), providers)
        assert providers.asked_isbn == '9788554511456'

    def test_an_answer_that_was_not_judged_carries_no_match(self):
        import json
        db = FakeDB()
        run(db, meta(), FakeProviders(record()))
        for q, p in db.matching("INSERT INTO metadata_candidates"):
            assert 'match' not in json.loads(p[-1])

    def test_the_people_credited_besides_the_author_become_a_contributors_suggestion(self):
        import json
        from providers.base import Credit
        db = FakeDB()
        credits = [Credit('F. Herbert'), Credit('John Schoenherr', 'illustrator', {'openlibrary': 'OL9A'})]
        run(db, meta(), FakeProviders(record(author='F. Herbert', credits=credits)))
        found = {p[1]: json.loads(p[2]) for q, p in db.matching("INSERT INTO metadata_candidates") if p[1] == 'contributors'}
        assert found['contributors'] == [{'name': 'John Schoenherr', 'role': 'illustrator'}]

    def test_a_language_a_provider_gives_is_never_suggested(self):
        db = FakeDB()
        run(db, meta(language=None), FakeProviders(record(language='cat')))
        fields = {p[1] for q, p in db.matching("INSERT INTO metadata_candidates")}
        assert 'language' not in fields and 'title' in fields

    def test_a_record_without_credits_leaves_the_evidence_as_it_was(self):
        import json
        db = FakeDB()
        run(db, meta(), FakeProviders(record()))
        for q, p in db.matching("INSERT INTO metadata_candidates"):
            assert 'credits' not in json.loads(p[-1])

    def test_a_provider_cover_is_used_only_when_the_file_has_none(self):
        no_cover = FakeDB()
        providers = FakeProviders(record())
        run(no_cover, meta(cover_path=None), providers)
        assert providers.downloads == ['http://x/c.jpg']
        assert no_cover.matching("INSERT INTO editions")[0][1][2] == '/covers/provider.jpg'

        has_cover = FakeDB()
        providers = FakeProviders(record())
        run(has_cover, meta(cover_path='/covers/native.jpg'), providers)
        assert providers.downloads == []
        assert has_cover.matching("INSERT INTO editions")[0][1][2] == '/covers/native.jpg'

    def test_confirmed_fields_get_no_suggestion(self):
        db = FakeDB(values={'title': 'Título confirmado'}, locks={'title': True, 'isbn': True})
        run(db, meta(), FakeProviders(record()))
        fields = {p[1] for q, p in db.matching("INSERT INTO metadata_candidates")}
        assert 'title' not in fields and 'isbn' not in fields

    def test_works_without_a_provider_match(self):
        db = FakeDB()
        result = run(db, meta(), FakeProviders(record=None))
        assert result.title == 'Duna'
        assert db.matching("INSERT INTO metadata_candidates") == []


class TestEnsureFile:
    def test_a_missing_file_is_a_permanent_error_not_a_lenient_success(self, tmp_path):
        import pytest
        from pipeline import ensure_file
        from runner import classify

        with pytest.raises(FileNotFoundError) as caught:
            ensure_file(str(tmp_path / "gone.epub"))
        assert classify(caught.value) == 'permanent'
        assert "gone.epub" in str(caught.value) and str(tmp_path) not in str(caught.value)  # no server paths in the message

    def test_a_directory_or_empty_path_is_refused(self, tmp_path):
        import pytest
        from pipeline import ensure_file
        for bad in ("", None, str(tmp_path)):
            with pytest.raises((ValueError, FileNotFoundError)):
                ensure_file(bad)

    def test_an_existing_file_passes(self, tmp_path):
        from pipeline import ensure_file
        f = tmp_path / "ok.epub"
        f.write_bytes(b"x")
        ensure_file(str(f))


class TestTextLayer:
    def test_a_pdf_is_checked_for_pages_without_text(self):
        import os
        scanned = os.path.join(os.path.dirname(__file__), '..', '..', 'testdata', 'corpus', 'pdf_escaneado.pdf')
        db = FakeDB()
        analyze_file(7, scanned, FakeExtractor(meta(format='pdf')), Analyzer(db), FakeProviders(), '/covers')
        (_, params), = db.matching("INSERT INTO text_layers")
        assert params[2] is True and params[1]

    def test_other_formats_are_not(self):
        db = FakeDB()
        run(db, meta(), FakeProviders())
        assert not db.matching("text_layers")


def test_a_pdf_that_asks_for_a_password_is_analysed_without_error_and_marked_so(tmp_path):
    from extractors.pdf_extractor import PdfExtractor
    from tests.test_extractors import protected_pdf

    db = FakeDB()
    path = protected_pdf(str(tmp_path), name='trancado.pdf')
    metadata = analyze_file(7, path, PdfExtractor(), Analyzer(db), FakeProviders(), str(tmp_path / 'covers'))
    assert metadata.protected is True and metadata.title == 'trancado'
    marks = [s for s in db.statements if s[0].startswith('UPDATE files SET protected = TRUE')]
    assert len(marks) == 1 and marks[0][1] == (7,)  # of the work analysed, by its primary file
    # No text layer was looked for: the file is closed.
    assert not [s for s in db.statements if 'text_layers' in s[0]]


def test_a_plain_pdf_is_not_marked_and_has_its_text_layer_looked_for(tmp_path):
    from extractors.pdf_extractor import PdfExtractor
    from tests.test_extractors import pdf_with_page_size

    db = FakeDB()
    path = pdf_with_page_size(str(tmp_path), 595, 842)
    analyze_file(7, path, PdfExtractor(), Analyzer(db), FakeProviders(), str(tmp_path / 'covers'))
    assert not [s for s in db.statements if s[0].startswith('UPDATE files SET protected')]
    assert [s for s in db.statements if 'text_layers' in s[0]]


def test_a_file_that_does_not_say_it_is_protected_is_not_marked():
    for protected in (False, None):
        db = FakeDB()
        run(db, meta(title='Duna', protected=protected) if protected is not None else meta(title='Duna'), FakeProviders())
        assert not [s for s in db.statements if s[0].startswith('UPDATE files SET protected')]


def test_the_work_is_ready_and_says_when_its_file_asks_for_a_password():
    from main import work_ready_event

    assert work_ready_event(7, meta(title='Duna')) == {'type': 'WORK_READY', 'work_id': 7, 'title': 'Duna'}
    assert work_ready_event(7, meta(title='Duna', protected=False)) == {'type': 'WORK_READY', 'work_id': 7, 'title': 'Duna'}
    assert work_ready_event(7, meta(title='trancado', protected=True)) == {'type': 'WORK_READY', 'work_id': 7, 'title': 'trancado', 'protected': True}


class TestTheSeriesFieldsOfTheRecord:
    def test_a_providers_state_and_native_title_go_with_the_record_as_the_librarys_words(self):
        from pipeline import suggest
        calls = {}

        class Spy:
            def save_candidates(self, work_id, record, source, evidence, include_locked=False):
                calls['record'] = record
                return 0
        enriched = SimpleNamespace(title='Berserk', author='Miura', series='Berserk', series_index=None, isbn=None, publisher=None, publication_date=None,
                                   description=None, tags=[], credits=[], source='AniList', match=None,
                                   raw={'status': 'RELEASING', 'native_title': 'ベルセルク'})
        suggest(Spy(), 7, enriched, 'Berserk')
        assert (calls['record']['series_status'], calls['record']['series_original_title']) == ('ongoing', 'ベルセルク')

    def test_nothing_is_none(self):
        from pipeline import suggest
        calls = {}

        class Spy:
            def save_candidates(self, work_id, record, source, evidence, include_locked=False):
                calls['record'] = record
                return 0
        enriched = SimpleNamespace(title='Duna', author='Frank Herbert', series=None, series_index=None, isbn=None, publisher=None, publication_date=None,
                                   description=None, tags=[], credits=[], source='Open Library', match=None, raw={})
        suggest(Spy(), 7, enriched, 'Duna')
        assert (calls['record']['series_status'], calls['record']['series_original_title']) == (None, None)
