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

    def search_best(self, title, fmt):
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
