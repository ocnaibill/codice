"""Google Books and ComicVine, asked the way they answer: Google for many volumes and the right one picked, ComicVine for the volume and then its issue."""
from unittest.mock import MagicMock, patch

from providers.base import Credit, MetadataRecord
from providers.comicvine import ComicVineProvider, _role_rank
from providers.google_books import GoogleBooksProvider, _isbn
from providers.query import parenthetical, read_file_title
from providers.registry import ProviderRegistry

KEY = 'SECRETKEY-1234567890'


class Net:
    """Stands for the network: answers by what was asked, and remembers every request."""

    def __init__(self, answer):
        self.answer, self.calls = answer, []

    def __call__(self, url, params=None, headers=None, timeout=None):
        self.calls.append((url, dict(params or {}), headers))
        out = self.answer(url, dict(params or {}))
        if isinstance(out, Exception):
            raise out
        status, body = out if isinstance(out, tuple) else (200, out)
        return MagicMock(status_code=status, json=MagicMock(return_value=body))

    def params(self):
        return [p for _, p, _ in self.calls]


def patched(net):
    return patch('providers.http.requests.get', net)


def book(title='Duna', authors=('Frank Herbert',), **volume):
    return {'id': f'id-{title}-{authors[0] if authors else ""}', 'volumeInfo': {'title': title, 'authors': list(authors), **volume}}


class TestTheParenthesesOfATitle:
    def test_what_a_title_says_in_parentheses_about_the_book_is_kept(self):
        assert parenthetical('A nuvem (Scythe)') == 'Scythe'
        assert parenthetical('Duna (Crônicas)') == 'Crônicas'
        assert parenthetical('Duna (Dune Chronicles)') == ''   # two capitalised words may be a person: better to leave it out than to send an author
        assert parenthetical('Dune (Frank Herbert) (Scythe)', 'Frank Herbert') == 'Scythe'   # the first is a person, not kept; the next one is

    def test_what_is_not_about_the_book_is_not(self):
        for title in ('Dune (1965)', 'Livro (Digital)', 'Livro (Scan OCR)', 'Livro (Retail)', 'Livro ()', 'Livro (   )', 'Livro (a b c d e)',
                      'Livro (Vol 2)', 'Livro (Edição Especial)', 'Livro', '', None, 'Livro (2020) (Digital)', 'Livro (...)', 'Livro (-)', 'Livro (?!)'):
            assert parenthetical(title) == '', title

    def test_the_author_is_never_part_of_it(self):
        assert parenthetical('Sapiens (Yuval Noah Harari)') == ''   # looks like a name
        assert parenthetical('Dom Casmurro (Machado)', 'Machado de Assis') == ''   # is the author the file says
        assert parenthetical('Dom Casmurro (Machado)') == 'Machado'                 # no author to compare: a word is kept
        assert read_file_title('Dom Casmurro (Machado)', 'Machado de Assis', 'epub').extra == ''   # the author of the file is passed on to it
        assert read_file_title('Dom Casmurro (Machado)', None, 'epub').extra == 'Machado'

    def test_the_search_with_it_is_for_books_and_only_when_there_is_something_to_add(self):
        q = read_file_title('A nuvem (Scythe)', 'Neal Shusterman', 'epub')
        assert (q.extra, q.search_title, q.search_extra) == ('Scythe', 'A nuvem', 'A nuvem Scythe')
        assert read_file_title('Duna', None, 'epub').search_extra == ''
        assert read_file_title('Berserk v01 (Hakusensha)', None, 'cbz').extra == 'Hakusensha'
        assert read_file_title('Berserk v01 (Hakusensha)', None, 'cbz').search_extra == ''   # a series file: parentheses are the group, the year
        assert read_file_title('(Scythe)', None, 'epub').search_extra == ''   # no title to add to


class TestGoogleBooksRecords:
    def setup_method(self):
        self.provider = GoogleBooksProvider()
        self.provider.api_key = KEY

    def lookup(self, items, title='Duna', author=None, format='epub'):
        net = Net(lambda url, p: {'items': items})
        with patched(net):
            return self.provider.lookup(read_file_title(title, author, format)), net

    def test_names(self):
        assert (self.provider.id, self.provider.name) == ('google_books', 'Google Books')

    def test_a_volume_is_a_record_with_all_that_google_says(self):
        full = book('Duna', ('Frank Herbert', '  ', 'Brian Herbert'), subtitle='Livro 1', publisher='Aleph', publishedDate='2015-09-16',
                    description='Num planeta de areia.', categories=['Fiction', ' ', 'Science Fiction'], ratingsCount=8,
                    industryIdentifiers=[{'type': 'ISBN_10', 'identifier': '8576570000'}, {'type': 'ISBN_13', 'identifier': '9788576572000'}],
                    imageLinks={'smallThumbnail': 'http://s.jpg', 'thumbnail': 'http://t.jpg'})
        (r,), _ = self.lookup([full])
        assert (r.title, r.author, r.publisher, r.publication_date, r.description, r.isbn, r.source) == (
            'Duna', 'Frank Herbert', 'Aleph', '2015-09-16', 'Num planeta de areia.', '9788576572000', 'Google Books')
        assert r.credits == [Credit('Frank Herbert'), Credit('Brian Herbert')] and r.tags == ['Fiction', 'Science Fiction']
        assert r.cover_url == 'https://t.jpg' and r.language is None   # the language of a volume is not the file's (DEC-096)
        assert r.raw == {'google_id': full['id'], 'subtitle': 'Livro 1', 'isbns': []} and r.prior == 2 + 1 + 1 + 0.5

    def test_the_names_of_the_authors_are_trimmed(self):
        (r,), _ = self.lookup([book('Duna', ('  Frank Herbert ',))])
        assert r.credits == [Credit('Frank Herbert')] and r.author == 'Frank Herbert'

    def test_a_volume_that_says_little_says_little(self):
        (r,), _ = self.lookup([{'id': 'x', 'volumeInfo': {'title': 'Só o título'}}])
        assert (r.author, r.credits, r.isbn, r.cover_url, r.description, r.tags, r.publisher, r.publication_date, r.prior) == (None, [], None, None, None, [], None, None, 0)

    def test_a_cover_is_the_small_one_when_it_is_the_only_one(self):
        (r,), _ = self.lookup([book(imageLinks={'smallThumbnail': 'http://s.jpg'})])
        assert r.cover_url == 'https://s.jpg'

    def test_what_is_known_of_a_volume_is_not_more_than_it_is_worth(self):
        (r,), _ = self.lookup([book(ratingsCount=500)])
        assert r.prior == 5

    def test_the_isbn_is_the_13_else_the_10_else_none(self):
        assert _isbn([{'type': 'ISBN_10', 'identifier': 'a'}, {'type': 'ISBN_13', 'identifier': 'b'}]) == 'b'
        assert _isbn([{'type': 'ISBN_10', 'identifier': 'a'}, {'type': 'OTHER', 'identifier': 'c'}]) == 'a'
        assert _isbn([{'type': 'OTHER', 'identifier': 'c'}]) is None
        assert _isbn([{'type': 'ISBN_13'}, 'junk', None]) is None and _isbn(None) is None
        assert _isbn([{'type': 'ISBN_13'}, {'type': 'ISBN_10', 'identifier': 'a'}]) == 'a'   # an identifier with nothing in it is none

    def test_a_volume_with_no_title_is_not_an_answer(self):
        records, _ = self.lookup([{'id': 'a'}, {'id': 'b', 'volumeInfo': {}}, {'id': 'c', 'volumeInfo': {'title': ''}}, book('Duna')])
        assert [r.title for r in records] == ['Duna']

    def test_the_manual_search_gives_the_first_volume_or_none(self):
        net = Net(lambda url, p: {'items': [book('Duna'), book('Outro')]})
        with patched(net):
            assert self.provider.search('Duna').title == 'Duna'
            assert self.provider.search('') is None
        with patched(Net(lambda url, p: {'items': []})):
            assert self.provider.search('Duna') is None


class TestAskingGoogleBooks:
    def setup_method(self):
        self.provider = GoogleBooksProvider()
        self.provider.api_key = KEY

    def test_it_asks_for_twenty_books_by_the_title_alone_with_the_key(self):
        net = Net(lambda url, p: {'items': []})
        with patched(net):
            self.provider.lookup(read_file_title('Duna - Frank Herbert', 'Frank Herbert', 'epub'))
        (url, params, headers), = net.calls
        assert url == 'https://www.googleapis.com/books/v1/volumes'
        assert params['q'] == 'Duna' and params['maxResults'] == 20 and params['printType'] == 'books' and params['key'] == KEY
        assert params['fields'].startswith('items(id,volumeInfo(') and 'Herbert' not in str(params)
        assert 'Codice' in headers['User-Agent']

    def test_what_the_title_says_in_parentheses_is_asked_too_and_each_volume_is_kept_once(self):
        def answer(url, p):
            return {'items': [book('A nuvem', ('Neal Shusterman',))]} if p['q'] == 'A nuvem Scythe' else {'items': [book('A nuvem Floquinho', ('Isa Colli',)), book('A nuvem', ('Neal Shusterman',))]}
        net = Net(answer)
        with patched(net):
            records = self.provider.lookup(read_file_title('A nuvem (Scythe)', 'Neal Shusterman', 'epub'))
        assert [p['q'] for p in net.params()] == ['A nuvem', 'A nuvem Scythe']
        assert [r.title for r in records] == ['A nuvem Floquinho', 'A nuvem']

    def test_without_a_key_it_asks_nothing_and_says_why(self, capsys):
        self.provider.api_key = ''
        net = Net(lambda url, p: {'items': [book()]})
        with patched(net):
            assert self.provider.lookup(read_file_title('Duna', None, 'epub')) == []
        assert net.calls == [] and 'no API key' in capsys.readouterr().out

    def test_nothing_to_ask_when_the_title_is_empty(self):
        net = Net(lambda url, p: {'items': [book()]})
        with patched(net):
            assert self.provider.lookup(read_file_title('', None, 'epub')) == []
        assert net.calls == []

    def test_a_refusal_is_told_in_the_log_and_never_with_the_key(self, capsys):
        for out in ((429, {}), (403, {}), RuntimeError(f'Max retries url: /v1/volumes?q=Duna&key={KEY}')):
            with patched(Net(lambda url, p, out=out: out)):
                assert self.provider.lookup(read_file_title('Duna', None, 'epub')) == []
        printed = capsys.readouterr().out
        assert 'HTTP 429' in printed and 'HTTP 403' in printed and 'the request failed' in printed
        assert 'SECRET' not in printed and '7890' not in printed

    def test_an_answer_with_no_items_is_none_found(self):
        for body in ({}, {'items': None}, {'totalItems': 0}, ['not', 'an', 'object']):
            with patched(Net(lambda url, p, body=body: body)):
                assert self.provider.lookup(read_file_title('Duna', None, 'epub')) == []


class TestTheRightBookAmongTheAnswers:
    def registry(self, *items_by_query):
        provider = GoogleBooksProvider()
        provider.api_key = KEY
        registry = ProviderRegistry(enabled=lambda pid: pid == 'google_books')
        registry._providers = {'default': [provider], 'cbz': [provider]}
        return registry

    def test_the_book_is_picked_by_the_author_of_the_file_among_twenty_that_are_not_it(self):
        items = [book('Duna', ('Alguém Qualquer',)), book('Dunas costeiras', ('Outra Pessoa',)), book('Duna', ('Frank Herbert',), description='Num planeta de areia.')]
        with patched(Net(lambda url, p: {'items': items})):
            got = self.registry().search_best('Duna', 'epub', author='Frank Herbert')
        assert got.author == 'Frank Herbert' and got.description == 'Num planeta de areia.'

    def test_no_book_is_better_than_the_wrong_one(self, capsys):
        with patched(Net(lambda url, p: {'items': [book('Duna', ('Alguém Qualquer',)), book('Dune Messiah', ('Frank Herbert',))]})):
            assert self.registry().search_best('Duna', 'epub', author='Frank Herbert') is None
        assert 'Nothing close enough' in capsys.readouterr().out

    def test_the_book_the_title_alone_does_not_find_is_found_by_what_the_title_says_in_parentheses(self):
        def answer(url, p):
            return {'items': [book('A nuvem', ('Neal Shusterman',), industryIdentifiers=[{'type': 'ISBN_13', 'identifier': '9788554511456'}])]} if 'Scythe' in p['q'] \
                else {'items': [book('A nuvem Floquinho', ('Isa Colli',))]}
        with patched(Net(answer)):
            got = self.registry().search_best('A nuvem (Scythe)', 'epub', author='Neal Shusterman')
        assert got.isbn == '9788554511456' and got.raw['google_id']

    def test_among_editions_equally_close_the_one_that_says_more_is_chosen(self):
        bare, full = book('Dom Casmurro', ('Machado de Assis',)), book('Dom Casmurro', ('Machado de Assis',), description='Bentinho.', publisher='Melhoramentos')
        bare['id'], full['id'] = 'bare', 'full'
        for items in ([bare, full], [full, bare]):
            with patched(Net(lambda url, p, items=items: {'items': items})):
                assert self.registry().search_best('Dom Casmurro', 'epub', author='Machado de Assis').raw['google_id'] == 'full'


def volume(vid, name, issues, publisher='DC Comics', year='2024', **kw):
    return {'id': vid, 'name': name, 'count_of_issues': issues, 'start_year': year, 'publisher': {'name': publisher} if publisher else None,
            'image': {'super_url': f'https://img/{vid}.jpg'}, 'description': f'<p>Volume {vid}</p>', **kw}


def issue(iid, number, name=None, **kw):
    return {'id': iid, 'issue_number': str(number), 'name': name, 'cover_date': '2025-05-01', 'description': f'<p>Issue {iid}</p>',
            'image': {'super_url': f'https://img/i{iid}.jpg'}, **kw}


class ComicNet(Net):
    """ComicVine as it answers: volumes for a search, the issues of a volume for a filter, the page of an issue."""

    def __init__(self, volumes, issues=None, credits=None):
        self.volumes, self.issues, self.credits = volumes, issues or {}, credits

        def answer(url, p):
            path = url.split('/api/')[1]
            if path == 'search':
                return {'error': 'OK', 'results': self.volumes}
            if path == 'issues':
                found = self.issues.get(p['filter'])
                return {'error': 'OK', 'results': [found] if found else []}
            if path.startswith('issue/'):
                return {'error': 'OK', 'results': {'person_credits': self.credits}}
            raise AssertionError(path)
        super().__init__(answer)


class TestComicVine:
    def setup_method(self):
        self.provider = ComicVineProvider()
        self.provider.api_key = KEY

    def lookup(self, net, title='Absolute Batman 006'):
        with patched(net):
            return self.provider.lookup(read_file_title(title, None, 'cbz'))

    def test_names(self):
        assert (self.provider.id, self.provider.name) == ('comicvine', 'ComicVine')

    def test_the_issue_is_the_one_of_the_volume_that_has_it_with_the_number_of_the_file(self):
        net = ComicNet([volume(1, 'Absolute Batman', 3, year='2025'), volume(2, 'Absolute Batman', 3, 'Urban Comics', '2025'), volume(3, 'Absolute Batman', 24)],
                       {'volume:3,issue_number:6': issue(99, 6, 'The Zoo, Part Six')})
        (r,) = self.lookup(net)
        assert (r.title, r.series, r.series_index, r.publisher, r.publication_date, r.source) == ('The Zoo, Part Six', 'Absolute Batman', 6.0, 'DC Comics', '2025-05-01', 'ComicVine')
        assert r.description == '<p>Issue 99</p>' and r.cover_url == 'https://img/i99.jpg' and r.prior == 2.4
        assert r.raw == {'comicvine_id': '4000-99', 'volume_id': 3, 'issues_in_volume': 24, 'issue_id': 99}
        search, issues = net.params()
        assert search['query'] == 'Absolute Batman' and search['resources'] == 'volume' and search['limit'] == 10 and search['api_key'] == KEY
        assert issues['filter'] == 'volume:3,issue_number:6'

    def test_only_the_volumes_that_can_have_the_issue_are_asked_the_longest_run_first_and_two_at_most(self):
        volumes = [volume(1, 'Saga', 12), volume(2, 'Saga', 72), volume(3, 'Saga', 8), volume(4, 'Saga', 30), volume(5, 'Saga', 50)]
        net = ComicNet(volumes, {'volume:2,issue_number:5': issue(20, 5, 'Chapter Five'), 'volume:5,issue_number:5': issue(50, 5, 'Volume Five'),
                                 'volume:4,issue_number:5': issue(40, 5, 'Never asked')})
        records = self.lookup(net, 'Saga 005')
        assert [r.title for r in records] == ['Chapter Five', 'Volume Five'] and [p['filter'] for p in net.params()[1:]] == ['volume:2,issue_number:5', 'volume:5,issue_number:5']

    def test_a_volume_without_the_issue_leaves_the_next_to_be_asked(self):
        net = ComicNet([volume(1, 'Saga', 72), volume(2, 'Saga', 50)], {'volume:2,issue_number:5': issue(50, 5)})
        (r,) = self.lookup(net, 'Saga 5')
        assert r.raw['volume_id'] == 2 and r.title == 'Saga #5'   # an issue with no name is named by the series and the number

    def test_with_no_issue_the_answer_is_the_volume_that_has_most_issues_the_three_longest(self, capsys):
        net = ComicNet([volume(1, 'Absolute Batman', 3), volume(2, 'Absolute Batman', 24), volume(3, 'Absolute Batman', 1), volume(4, 'Absolute Batman', 5), volume(5, 'Absolute Batman', 2)])
        records = self.lookup(net, 'Absolute Batman 100')
        assert [r.raw['volume_id'] for r in records] == [2, 4, 1]
        r = records[0]
        assert (r.title, r.series, r.series_index, r.publisher, r.publication_date, r.description, r.cover_url) == (
            'Absolute Batman', 'Absolute Batman', None, 'DC Comics', '2024', '<p>Volume 2</p>', 'https://img/2.jpg')
        assert r.raw == {'comicvine_id': '4050-2', 'volume_id': 2, 'issues_in_volume': 24}
        assert len(net.params()) == 1   # one request for the volumes and none for an issue: no volume has room for it

    def test_a_file_with_no_number_is_answered_by_its_series(self):
        net = ComicNet([volume(1, 'Watchmen', 12), volume(2, 'Watchmen', 6)])
        records = self.lookup(net, 'Watchmen')
        assert [r.raw['volume_id'] for r in records] == [1, 2] and all(r.series_index is None for r in records)
        assert [p for p in net.params() if 'filter' in p] == []

    def test_a_volume_that_is_not_called_what_the_file_is_called_is_not_an_answer(self):
        net = ComicNet([volume(1, 'Absolute Batman: The Killing Joke', 1), volume(2, 'Batman', 700), volume(3, 'Absolute Batman', 24)], {'volume:3,issue_number:1': issue(30, 1)})
        (r,) = self.lookup(net, 'Absolute Batman 001')
        assert r.raw['volume_id'] == 3
        assert self.lookup(ComicNet([volume(1, 'Superman', 100)]), 'Batman 001') == []

    def test_a_volume_with_exactly_as_many_issues_as_the_number_has_it(self):
        (r,) = self.lookup(ComicNet([volume(1, 'Saga', 6)], {'volume:1,issue_number:6': issue(10, 6)}), 'Saga 6')
        assert r.series_index == 6.0
        assert [r.raw['comicvine_id'] for r in self.lookup(ComicNet([volume(1, 'Saga', 5)], {'volume:1,issue_number:6': issue(10, 6)}), 'Saga 6')] == ['4050-1']   # five issues: no sixth

    def test_the_cover_of_the_biggest_size_and_the_description_before_the_deck(self):
        v = volume(1, 'Saga', 6, deck='The deck')
        v['image'] = {'original_url': 'https://o.jpg', 'super_url': 'https://s.jpg', 'medium_url': 'https://m.jpg'}
        (r,) = self.lookup(ComicNet([v]), 'Saga')
        assert r.cover_url == 'https://s.jpg' and r.description == '<p>Volume 1</p>'
        v['image'] = {'original_url': 'https://o.jpg', 'medium_url': 'https://m.jpg'}
        assert self.lookup(ComicNet([v]), 'Saga')[0].cover_url == 'https://o.jpg'
        v['image'] = {'medium_url': 'https://m.jpg'}
        assert self.lookup(ComicNet([v]), 'Saga')[0].cover_url == 'https://m.jpg'

    def test_a_series_that_went_on_for_hundreds_of_issues_is_worth_ten_and_no_more(self):
        (r,) = self.lookup(ComicNet([volume(1, 'Spawn', 379)]), 'Spawn')
        assert r.prior == 10

    def test_a_volume_with_a_subtitle_is_the_series_by_its_main_title(self):
        (r,) = self.lookup(ComicNet([volume(1, 'Saga: Book One', 5)], {'volume:1,issue_number:1': issue(10, 1)}), 'Saga 01')
        assert r.series == 'Saga: Book One'

    def test_an_issue_number_that_is_not_a_number_has_no_index(self):
        (r,) = self.lookup(ComicNet([volume(1, 'Saga', 12)], {'volume:1,issue_number:3': issue(10, '3A', 'Special')}), 'Saga 3')
        assert r.series_index is None and r.title == 'Special'
        (r,) = self.lookup(ComicNet([volume(1, 'Saga', 12)], {'volume:1,issue_number:3': issue(10, None, 'Special')}), 'Saga 3')
        assert r.series_index is None

    def test_what_a_volume_lacks_is_not_made_up(self):
        bare = {'id': 7, 'name': 'Obscure'}
        (r,) = self.lookup(ComicNet([bare]), 'Obscure')
        assert (r.publisher, r.publication_date, r.description, r.cover_url, r.prior) == (None, None, None, None, 0)
        (r,) = self.lookup(ComicNet([bare], {'volume:7,issue_number:1': {'id': 8, 'issue_number': '1', 'deck': 'A deck.'}}), 'Obscure 1')
        assert (r.description, r.cover_url, r.publication_date, r.title) == ('A deck.', None, None, 'Obscure #1')
        (r,) = self.lookup(ComicNet([volume(7, 'Obscure', 3, deck='The deck of the volume', description=None)]), 'Obscure')
        assert r.description == 'The deck of the volume'

    def test_the_description_of_the_volume_stands_in_for_an_issue_that_has_none(self):
        (r,) = self.lookup(ComicNet([volume(7, 'Obscure', 3)], {'volume:7,issue_number:1': {'id': 8, 'issue_number': '1'}}), 'Obscure 1')
        assert r.description == '<p>Volume 7</p>' and r.cover_url == 'https://img/7.jpg'

    def test_without_a_key_nothing_is_asked(self, capsys):
        self.provider.api_key = ''
        net = ComicNet([volume(1, 'Saga', 12)])
        assert self.lookup(net, 'Saga 1') == [] and net.calls == [] and 'no API key' in capsys.readouterr().out

    def test_nothing_is_asked_for_an_empty_title(self):
        net = ComicNet([volume(1, 'Saga', 12)])
        assert self.lookup(net, '') == [] and net.calls == []

    def test_what_comicvine_says_is_wrong_is_told_and_nothing_comes_of_it(self, capsys):
        def answer(url, p):
            return {'error': 'Invalid API Key', 'status_code': 100, 'results': []}
        assert self.lookup(Net(answer), 'Saga 1') == []
        assert 'Invalid API Key' in capsys.readouterr().out

    def test_a_refusal_or_a_failure_is_told_and_never_with_the_key(self, capsys):
        for out in ((401, {}), (420, {}), RuntimeError(f'Max retries url: /api/search?api_key={KEY}&query=Saga')):
            assert self.lookup(Net(lambda url, p, out=out: out), 'Saga 1') == []
        printed = capsys.readouterr().out
        assert 'HTTP 401' in printed and 'the request failed' in printed and 'SECRET' not in printed and '7890' not in printed

    def test_an_answer_with_no_results_is_none_found(self):
        for body in ({'error': 'OK', 'results': None}, {'error': 'OK'}, ['not', 'an', 'object']):
            assert self.lookup(Net(lambda url, p, body=body: body), 'Saga 1') == []

    def test_the_manual_search_gives_the_first_answer_or_none(self):
        with patched(ComicNet([volume(1, 'Saga', 12)])):
            assert self.provider.search('Saga').raw['volume_id'] == 1
            assert self.provider.search('') is None
        with patched(ComicNet([])):
            assert self.provider.search('Saga') is None


CREDITS = [{'id': 5, 'name': 'Alex Maleev', 'role': 'cover'}, {'id': 6, 'name': 'Clayton Cowles', 'role': 'letterer'},
           {'id': 7, 'name': 'Nick Dragotta', 'role': 'penciller, cover'}, {'id': 8, 'name': 'Scott Snyder', 'role': 'writer'},
           {'name': 'No Id', 'role': ''}, {'id': 9, 'name': '  ', 'role': 'writer'}, {'id': 10, 'role': 'writer'}]


class TestWhoMadeTheIssue:
    def setup_method(self):
        self.provider = ComicVineProvider()
        self.provider.api_key = KEY

    def record(self, **raw):
        return MetadataRecord(title='x', raw={'issue_id': 99, **raw})

    def enrich(self, credits, record=None):
        net = ComicNet([], credits=credits)
        with patched(net):
            return self.provider.enrich(record or self.record()), net

    def test_the_writer_comes_first_and_is_the_author_then_who_drew_it(self):
        r, net = self.enrich(CREDITS)
        assert [c.name for c in r.credits] == ['Scott Snyder', 'Nick Dragotta', 'Clayton Cowles', 'Alex Maleev', 'No Id'] and r.author == 'Scott Snyder'
        assert [(c.role, c.ids) for c in r.credits][:2] == [('writer', {'comicvine': '8'}), ('penciller, cover', {'comicvine': '7'})]
        assert (r.credits[-1].role, r.credits[-1].ids) == (None, {})
        (path, params, _), = net.calls
        assert path.endswith('/issue/4000-99') and params['field_list'] == 'person_credits' and params['api_key'] == KEY

    def test_without_a_writer_it_is_who_drew_it_and_without_either_nobody(self):
        r, _ = self.enrich([{'id': 1, 'name': 'Inks Only', 'role': 'inker'}])
        assert r.author is None   # who inked is not who made it
        r, _ = self.enrich([c for c in CREDITS if c.get('role') != 'writer'])
        assert r.author == 'Nick Dragotta'
        r, _ = self.enrich([{'id': 1, 'name': 'Cover Only', 'role': 'cover'}, {'id': 2, 'name': 'Colors', 'role': 'colorist'}])
        assert r.author is None and len(r.credits) == 2

    def test_a_person_who_did_two_things_ranks_by_the_first_that_counts(self):
        assert _role_rank('writer, inker') == 0 and _role_rank('cover, penciller') == 1 and _role_rank('Cover') == 7 and _role_rank('') == 8 and _role_rank(None) == 8
        assert _role_rank('translator') == 8

    def test_at_most_eight_people(self):
        r, _ = self.enrich([{'id': i, 'name': f'Person {i}', 'role': 'writer'} for i in range(12)])
        assert len(r.credits) == 8

    def test_what_has_no_issue_page_is_left_as_it_is(self):
        net = ComicNet([], credits=CREDITS)
        series = MetadataRecord(title='Saga', raw={'volume_id': 1})
        with patched(net):
            assert self.provider.enrich(series) is series and series.credits == []
        assert net.calls == []
        self.provider.api_key = ''
        with patched(net):
            self.provider.enrich(self.record())
        assert net.calls == []

    def test_when_the_page_cannot_be_read_the_answer_is_as_it_was(self):
        record = self.record()
        record.credits, record.author = [Credit('Before')], 'Before'
        with patched(Net(lambda url, p: (500, {}))):
            got = self.provider.enrich(record)
        assert got is record and got.author == 'Before' and got.credits == [Credit('Before')]
        for results in ({}, {'person_credits': []}, {'person_credits': [{'id': 1, 'name': ' '}]}, None):
            record = self.record()
            record.credits, record.author = [Credit('Before')], 'Before'
            with patched(Net(lambda url, p, results=results: {'error': 'OK', 'results': results})):
                got = self.provider.enrich(record)
            assert got.credits == [Credit('Before')] and got.author == 'Before'


class TestTheRegistryAndComicVine:
    def test_the_issue_with_the_number_of_the_file_is_the_answer_and_gets_its_credits(self):
        provider = ComicVineProvider()
        provider.api_key = KEY
        registry = ProviderRegistry(enabled=lambda pid: pid == 'comicvine')
        registry._providers = {'cbz': [provider], 'default': [provider]}
        net = ComicNet([volume(1, 'Absolute Batman', 3), volume(3, 'Absolute Batman', 24)], {'volume:3,issue_number:6': issue(99, 6, 'The Zoo, Part Six')}, credits=CREDITS)
        with patched(net):
            got = registry.search_best('Absolute Batman 006', 'cbz')
        assert got.title == 'The Zoo, Part Six' and got.series_index == 6.0 and got.author == 'Scott Snyder' and got.raw['comicvine_id'] == '4000-99'

    def test_a_series_with_fewer_issues_than_the_file_number_is_still_the_series(self):
        provider = ComicVineProvider()
        provider.api_key = KEY
        registry = ProviderRegistry(enabled=lambda pid: pid == 'comicvine')
        registry._providers = {'cbz': [provider], 'default': [provider]}
        with patched(ComicNet([volume(1, 'Absolute Batman', 3)])):
            got = registry.search_best('Absolute Batman 012', 'cbz')
        assert got.series == 'Absolute Batman' and got.series_index is None and got.raw['comicvine_id'] == '4050-1'
