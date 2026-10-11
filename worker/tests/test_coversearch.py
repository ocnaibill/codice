"""Looking for the covers of a work among the providers that are on, for staff to choose from (DEC-174)."""
import json
import os
import time
from types import SimpleNamespace

from coversearch import CoverSearcher, MAX_CANDIDATES, MAX_IMAGE, KEEP_SECONDS, image_extension
from providers.http import Reply

JPEG = b'\xff\xd8\xff\xe0' + b'0' * 40
PNG = b'\x89PNG\r\n\x1a\n' + b'0' * 40
WEBP = b'RIFF\x24\x00\x00\x00WEBPVP8 ' + b'0' * 20


class FakeDb:
    def __init__(self, work=('cbz', False)):
        self.work, self.calls = work, []

    def fetchone(self, query, params=()):
        return self.work

    def execute(self, query, params=()):
        self.calls.append((query, params))

    @property
    def kept(self):
        """(state, results) of the last write."""
        query, params = self.calls[-1]
        return params[0], json.loads(params[1]), params[2]


class FakeAnalyzer:
    def __init__(self, **values):
        self.values = {'title': 'A Quarta Asa', 'series': 'Empyrean', 'author': 'Rebecca Yarros', 'isbn': '9786559812424'}
        self.values.update(values)

    def _load_state(self, work_id):
        return {'values': self.values}

    def _is_placeholder(self, field, value):
        return str(value).endswith('.epub')


class FakeRegistry:
    def __init__(self, records=(), on=True):
        self.records, self.on, self.asked = list(records), on, []

    def any_on(self, format):
        return self.on

    def search_all(self, title, format, per_provider=5, author=None, isbn=None):
        self.asked.append({'title': title, 'format': format, 'author': author, 'isbn': isbn})
        return self.records

    def provider(self, provider_id):
        return SimpleNamespace(name=f'Provider {provider_id}') if provider_id else None


def record(url, provider='openlibrary', title='A Quarta Asa', author='Rebecca Yarros', source='Open Library'):
    return SimpleNamespace(cover_url=url, provider_id=provider, title=title, author=author, source=source)


class Downloads:
    """What each address gives; an address that is not here is a 404."""

    def __init__(self, files):
        self.files, self.asked = files, []

    def __call__(self, provider, url, max_bytes, **kw):
        self.asked.append((provider, url, max_bytes))
        data = self.files.get(url)
        return Reply(status=200, data=data) if data is not None else Reply(status=404, data=None, problem='HTTP 404')


def searcher(tmp_path, registry, analyzer=None, files=None, db=None, clock=time.time):
    db = db or FakeDb()
    download = Downloads(files or {})
    cache = tmp_path / 'cache'
    s = CoverSearcher(db, registry, analyzer or FakeAnalyzer(), str(cache), download=download, clock=clock)
    return s, db, download, cache


class TestWhatIsKept:
    def test_each_cover_is_downloaded_and_listed_with_where_it_came_from(self, tmp_path):
        registry = FakeRegistry([record('http://a/1.jpg'), record('http://b/2.png', provider='google_books', source='Google Books', title='Quarta Asa')])
        s, db, download, cache = searcher(tmp_path, registry, files={'http://a/1.jpg': JPEG, 'http://b/2.png': PNG})
        assert s.run(7, 12, '') == {'state': 'done', 'found': 2}
        state, results, work = db.kept
        assert state == 'done' and work == 12
        assert [(r['provider'], r['source'], r['title']) for r in results] == [
            ('openlibrary', 'Open Library', 'A Quarta Asa'), ('google_books', 'Google Books', 'Quarta Asa')]
        assert results[0]['file'].startswith('12_0_') and results[0]['file'].endswith('.jpg')
        assert results[1]['file'].startswith('12_1_') and results[1]['file'].endswith('.png')
        assert (cache / results[0]['file']).read_bytes() == JPEG
        assert (cache / results[1]['file']).read_bytes() == PNG
        # The download is asked with the name the provider has, and a limit on its size.
        assert download.asked[0] == ('Provider openlibrary', 'http://a/1.jpg', MAX_IMAGE)

    def test_it_asks_with_what_the_work_says_when_nothing_was_typed(self, tmp_path):
        registry = FakeRegistry([])
        s, db, _, _ = searcher(tmp_path, registry)
        s.run(7, 12, '')
        assert registry.asked == [{'title': 'A Quarta Asa (Empyrean)', 'format': 'cbz', 'author': 'Rebecca Yarros', 'isbn': '9786559812424'}]

    def test_what_was_typed_is_the_title_and_nothing_else_leaves(self, tmp_path):
        registry = FakeRegistry([])
        s, _, _, _ = searcher(tmp_path, registry)
        s.run(7, 12, '  Fourth   Wing ')
        assert registry.asked == [{'title': 'Fourth Wing', 'format': 'cbz', 'author': None, 'isbn': None}]

    def test_a_work_still_named_by_its_file_is_searched_by_that_name(self, tmp_path):
        registry = FakeRegistry([])
        s, _, _, _ = searcher(tmp_path, registry, analyzer=FakeAnalyzer(title='quarta_asa.epub', series='', author=None, isbn=None))
        s.run(7, 12, '')
        assert registry.asked[0]['title'] == 'quarta asa' and registry.asked[0]['author'] is None

    def test_only_what_is_an_image_the_page_can_open_is_kept(self, tmp_path):
        registry = FakeRegistry([record('http://a/html'), record('http://a/gif'), record('http://a/ok.webp'), record('http://a/missing'), record(None)])
        files = {'http://a/html': b'<html>not found</html>', 'http://a/gif': b'GIF89a' + b'0' * 20, 'http://a/ok.webp': WEBP}
        s, db, _, cache = searcher(tmp_path, registry, files=files)
        assert s.run(7, 12, '') == {'state': 'done', 'found': 1}
        _, results, _ = db.kept
        assert results[0]['file'].endswith('.webp')
        assert os.listdir(cache) == [results[0]['file']]

    def test_the_same_cover_from_two_providers_is_one(self, tmp_path):
        registry = FakeRegistry([record('http://a/1.jpg'), record('http://a/1.jpg', provider='google_books')])
        s, db, download, _ = searcher(tmp_path, registry, files={'http://a/1.jpg': JPEG})
        s.run(7, 12, '')
        assert len(download.asked) == 1 and len(db.kept[1]) == 1

    def test_there_is_a_limit_to_what_is_kept(self, tmp_path):
        urls = [f'http://a/{i}.jpg' for i in range(MAX_CANDIDATES + 5)]
        s, db, _, _ = searcher(tmp_path, FakeRegistry([record(u) for u in urls]), files={u: JPEG for u in urls})
        s.run(7, 12, '')
        assert len(db.kept[1]) == MAX_CANDIDATES

    def test_an_empty_answer_is_an_answer(self, tmp_path):
        s, db, _, _ = searcher(tmp_path, FakeRegistry([]))
        assert s.run(7, 12, '') == {'state': 'done', 'found': 0}
        assert db.kept[:2] == ('done', [])


class TestWhenItDoesNotSearch:
    def test_nothing_is_asked_when_no_provider_is_on(self, tmp_path):
        registry = FakeRegistry([record('http://a/1.jpg')], on=False)
        s, db, download, _ = searcher(tmp_path, registry, files={'http://a/1.jpg': JPEG})
        assert s.run(7, 12, '') == {'state': 'off'}
        assert db.kept[0] == 'off' and registry.asked == [] and download.asked == []

    def test_a_work_that_is_gone_or_retired_is_a_failure_that_asks_nobody(self, tmp_path):
        for work in (None, ('cbz', True)):
            registry = FakeRegistry([record('http://a/1.jpg')])
            s, db, _, _ = searcher(tmp_path, registry, db=FakeDb(work))
            assert s.run(7, 12, '') == {'state': 'failed'}
            assert db.kept[0] == 'failed' and registry.asked == []

    def test_a_job_that_names_no_work_fails(self, tmp_path):
        s, _, _, _ = searcher(tmp_path, FakeRegistry([]))
        assert s.run(7, None, '') == {'state': 'failed'}

    def test_a_work_with_nothing_to_search_by_is_an_empty_answer(self, tmp_path):
        registry = FakeRegistry([record('http://a/1.jpg')])
        s, db, _, _ = searcher(tmp_path, registry, analyzer=FakeAnalyzer(title='', series='', author=None, isbn=None))
        assert s.run(7, 12, '') == {'state': 'done', 'found': 0}
        assert registry.asked == []


class TestTheFolderExpires:
    def test_an_earlier_search_of_the_work_goes_and_another_works_stays(self, tmp_path):
        cache = tmp_path / 'cache'
        cache.mkdir()
        (cache / '12_0_aaaaaaaa.jpg').write_bytes(JPEG)
        (cache / '99_0_bbbbbbbb.jpg').write_bytes(JPEG)
        s, _, _, _ = searcher(tmp_path, FakeRegistry([]))
        s.run(7, 12, '')
        assert os.listdir(cache) == ['99_0_bbbbbbbb.jpg']

    def test_what_is_old_goes_whoever_it_was_for(self, tmp_path):
        cache = tmp_path / 'cache'
        cache.mkdir()
        old, young = cache / '99_0_bbbbbbbb.jpg', cache / '98_0_cccccccc.jpg'
        old.write_bytes(JPEG)
        young.write_bytes(JPEG)
        now = time.time()
        os.utime(old, (now - KEEP_SECONDS - 60, now - KEEP_SECONDS - 60))
        s, _, _, _ = searcher(tmp_path, FakeRegistry([]))
        s.run(7, 12, '')
        assert os.listdir(cache) == ['98_0_cccccccc.jpg']

    def test_a_work_whose_name_starts_like_another_is_not_swept_with_it(self, tmp_path):
        cache = tmp_path / 'cache'
        cache.mkdir()
        (cache / '123_0_aaaaaaaa.jpg').write_bytes(JPEG)
        s, _, _, _ = searcher(tmp_path, FakeRegistry([]))
        s.run(7, 12, '')
        assert os.listdir(cache) == ['123_0_aaaaaaaa.jpg']


class TestTheKindOfImage:
    def test_by_the_first_bytes_and_not_by_the_address(self):
        assert image_extension(JPEG) == 'jpg'
        assert image_extension(PNG) == 'png'
        assert image_extension(WEBP) == 'webp'
        assert image_extension(b'GIF89a....') is None
        assert image_extension(b'RIFF\x00\x00\x00\x00WAVEfmt ') is None
        assert image_extension(b'') is None
