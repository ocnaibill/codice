"""The ISBN of the file is sent to the providers that search by it, and an answer that has it is the book (DEC-142)."""
from unittest.mock import MagicMock, patch

from providers.base import MetadataRecord
from providers.google_books import GoogleBooksProvider, _all_isbns
from providers.match import judge
from providers.openlibrary import MAX_ISBNS, OpenLibraryProvider
from providers.query import read_file_title
from providers.registry import ProviderRegistry
from providers.text import normalize_isbn
from tests.test_google_comicvine import KEY, Net, book, patched

ISBN = '9788554511456'


class TestNormalizingAnIsbn:
    def test_an_isbn_is_thirteen_digits(self):
        for text in (ISBN, '978-85-5451-145-6', 'urn:isbn:9788554511456', 'ISBN 978 85 5451 145 6', 'isbn:978-85-5451-145-6', ' 9788554511456 ', 'URN:ISBN:9788554511456'):
            assert normalize_isbn(text) == ISBN, text

    def test_an_isbn_10_is_made_an_isbn_13(self):
        assert normalize_isbn('0306406152') == '9780306406157' and normalize_isbn('0-306-40615-2') == '9780306406157'
        assert normalize_isbn('080442957X') == '9780804429573' and normalize_isbn('080442957x') == '9780804429573'

    def test_a_check_digit_of_zero_is_a_zero(self):
        assert normalize_isbn('0000000043') == '9780000000040' and normalize_isbn('9780000000040') == '9780000000040'

    def test_a_barcode_of_a_magazine_is_not_a_book(self):
        assert normalize_isbn('9771234567003') is None   # 977 is the prefix of the ISSN, though the check digit is right

    def test_what_is_not_an_isbn_is_none(self):
        for text in ('9788554511457', '0306406153', '030640615X', 'abc', '123', '', '   ', None, 123, '978855451145', '97885545114567', 'XXXXXXXXXX', '9788554511456X',
                     '1234567890123', '9778554511456'):
            assert normalize_isbn(text) is None, text

    def test_a_number_with_the_wrong_check_digit_is_not_taken_for_one(self):
        assert normalize_isbn('9788554511450') is None


class TestTheFileCarriesIt:
    def test_it_is_part_of_what_the_file_says_and_is_normalized(self):
        assert read_file_title('A nuvem', None, 'epub', '978-85-5451-145-6').isbn == ISBN
        assert read_file_title('A nuvem', None, 'epub').isbn is None
        assert read_file_title('A nuvem', None, 'epub', 'not an isbn').isbn is None
        assert read_file_title('A nuvem', None, 'epub', '9788554511457').isbn is None


def record(title='A nuvem', isbn=None, isbns=(), author='Neal Shusterman', prior=0):
    r = MetadataRecord(title=title, isbn=isbn, author=author, prior=prior, raw={'isbns': list(isbns)})
    return r


class TestJudgingByIsbn:
    def query(self, title='A nuvem (Scythe)', author='Neal Shusterman'):
        return read_file_title(title, author, 'epub', ISBN)

    def test_the_book_with_the_isbn_of_the_file_is_the_book_when_the_author_is_the_one_the_file_says_though_the_title_is_another(self):
        for r in (record('Thunderhead', isbn=ISBN, author='N. Shusterman'), record('Outro título', isbns=[ISBN], author='Neal Shusterman'),
                  record('Qualquer', isbn='978-85-5451-145-6', author='Shusterman, Neal')):
            m = judge(self.query(), r)
            assert m.accepted and m.isbn and m.score >= 300 and m.reason == '' and m.author >= 0.5 and m.title < 0.5, r

    def test_or_when_the_title_is_somewhat_close_though_the_author_is_written_another_way_or_is_not_known(self):
        for r in (record('A nuvem', isbn=ISBN, author='Alguém Diferente'), record('A nuvem: Scythe 2', isbn=ISBN, author=None), record('Nuvem', isbn=ISBN, author=None)):
            m = judge(self.query(), r)
            assert m.accepted and m.isbn and m.title >= 0.5, (r, m)

    def test_a_title_in_between_is_enough_and_so_is_the_main_title_of_one_with_a_subtitle(self):
        m = judge(self.query('A nuvem negra', None), record('A nuvem', isbn=ISBN, author=None))   # two words in three
        assert m.accepted and m.isbn and 0.5 <= m.title < 0.9
        m = judge(read_file_title('Sapiens', None, 'epub', ISBN), record('Sapiens: Uma breve história da humanidade', isbn=ISBN, author=None))
        assert m.accepted and m.isbn and m.title == 1.0

    def test_an_isbn_that_nothing_else_agrees_with_is_not_the_book(self):
        # the ISBN a file carries can be a wrong one: the title and the author both say it is another book
        m = judge(self.query('Um Título Qualquer', 'Fulano de Tal'), record('I, Robot', isbn=ISBN, author='Isaac Asimov'))
        assert not m.accepted and not m.isbn and m.reason == 'title'
        # nothing to agree with, nothing against: the ISBN is alone
        m = judge(self.query('Um Título Qualquer', None), record('I, Robot', isbn=ISBN, author=None))
        assert not m.accepted and not m.isbn

    def test_an_isbn_10_of_the_book_is_the_same_isbn(self):
        assert judge(read_file_title('A nuvem', None, 'epub', '0306406152'), record(isbns=['9780306406157'])).isbn
        assert judge(read_file_title('A nuvem', None, 'epub', '9780306406157'), record(isbns=['0-306-40615-2'])).isbn

    def test_another_isbn_proves_nothing_and_the_title_decides_as_always(self):
        m = judge(self.query(), record('A nuvem', isbn='9780306406157', isbns=['9780306406157']))
        assert m.accepted and not m.isbn and m.score < 300
        m = judge(self.query(), record('Cloud Computing', isbn='9780306406157'))
        assert not m.accepted and not m.isbn and m.reason == 'title'

    def test_a_file_without_an_isbn_is_judged_by_the_title_alone(self):
        q = read_file_title('A nuvem', 'Neal Shusterman', 'epub')
        assert not judge(q, record('Thunderhead', isbn=ISBN)).accepted
        assert not judge(read_file_title('A nuvem', None, 'epub', 'junk'), record('Thunderhead', isbn=ISBN)).accepted

    def test_an_answer_that_is_the_isbn_comes_before_one_that_is_only_close(self):
        by_isbn, by_title = judge(self.query(), record('Thunderhead', isbn=ISBN, prior=0)), judge(self.query(), record('A nuvem', prior=10))
        assert by_isbn.score > by_title.score

    def test_the_prior_still_breaks_a_tie_between_two_with_the_isbn(self):
        low, high = judge(self.query(), record(isbn=ISBN, prior=0)), judge(self.query(), record(isbn=ISBN, prior=8))
        assert high.score == low.score + 8

    def test_it_is_told_in_the_evidence_only_when_it_decided(self):
        assert judge(self.query(), record(isbn=ISBN)).as_dict()['isbn'] is True
        assert 'isbn' not in judge(self.query(), record('A nuvem')).as_dict()
        assert judge(self.query(), record(isbn=ISBN)).as_dict()['accepted'] is True


class TestGoogleBooksAsksByIsbn:
    def setup_method(self):
        self.provider = GoogleBooksProvider()
        self.provider.api_key = KEY

    def lookup(self, answer, title='A nuvem (Scythe)'):
        net = Net(answer)
        with patched(net):
            return self.provider.lookup(read_file_title(title, 'Neal Shusterman', 'epub', ISBN)), net

    def test_it_asks_for_the_isbn_first_and_when_it_finds_the_book_asks_nothing_else(self):
        records, net = self.lookup(lambda url, p: {'items': [book('A nuvem', ('Neal Shusterman',), industryIdentifiers=[
            {'type': 'ISBN_10', 'identifier': '8554511457'}, {'type': 'ISBN_13', 'identifier': ISBN}])]})
        assert [p['q'] for p in net.params()] == [f'isbn:{ISBN}'] and len(records) == 1
        assert records[0].raw['isbns'] == [ISBN]

    def test_when_google_does_not_know_the_isbn_it_asks_by_the_title_as_before(self):
        def answer(url, p):
            return {'items': []} if p['q'].startswith('isbn:') else {'items': [book('A nuvem', ('Neal Shusterman',))]}
        records, net = self.lookup(answer)
        assert [p['q'] for p in net.params()] == [f'isbn:{ISBN}', 'A nuvem', 'A nuvem Scythe'] and len(records) == 1

    def test_a_file_without_an_isbn_asks_only_by_the_title(self):
        net = Net(lambda url, p: {'items': []})
        with patched(net):
            self.provider.lookup(read_file_title('A nuvem', None, 'epub'))
        assert [p['q'] for p in net.params()] == ['A nuvem']

    def test_an_isbn_that_google_refuses_falls_back_to_the_title(self):
        def answer(url, p):
            return (429, {}) if p['q'].startswith('isbn:') else {'items': [book('A nuvem')]}
        records, net = self.lookup(answer)
        assert len(records) == 1 and len(net.calls) == 3

    def test_every_isbn_the_volume_has_is_told_as_thirteen_digits(self):
        assert _all_isbns([{'type': 'ISBN_10', 'identifier': '0306406152'}, {'type': 'ISBN_13', 'identifier': '9780306406157'}, {'type': 'OTHER', 'identifier': 'junk'},
                           {'type': 'ISBN_13'}, 'junk']) == ['9780306406157']
        assert _all_isbns(None) == [] and _all_isbns([{'type': 'ISBN_13', 'identifier': ISBN}]) == [ISBN]


def reply(docs):
    return MagicMock(status_code=200, json=MagicMock(return_value={'docs': docs}))


class TestOpenLibraryAsksByIsbn:
    @patch('providers.http.requests.get')
    def test_the_isbn_finds_the_work_and_the_title_is_not_asked(self, mock_get):
        mock_get.return_value = reply([{'key': '/works/OL1W', 'title': 'Thunderhead', 'isbn': ['9781481426312', ISBN, 5], 'author_name': ['Neal Shusterman']}])
        records = OpenLibraryProvider().lookup(read_file_title('A nuvem', None, 'epub', ISBN))
        assert [r.title for r in records] == ['Thunderhead'] and mock_get.call_count == 1
        assert mock_get.call_args.kwargs['params']['isbn'] == ISBN and 'title' not in mock_get.call_args.kwargs['params']
        assert records[0].raw['isbns'] == ['9781481426312', ISBN]

    @patch('providers.http.requests.get')
    def test_when_it_does_not_know_the_isbn_it_asks_by_the_title(self, mock_get):
        mock_get.side_effect = [reply([]), reply([{'key': '/works/OL1W', 'title': 'A nuvem'}])]
        records = OpenLibraryProvider().lookup(read_file_title('A nuvem', None, 'epub', ISBN))
        assert [r.title for r in records] == ['A nuvem']
        assert 'isbn' in mock_get.call_args_list[0].kwargs['params'] and mock_get.call_args_list[1].kwargs['params']['title'] == 'A nuvem'

    @patch('providers.http.requests.get')
    def test_a_file_without_an_isbn_does_not_ask_by_it(self, mock_get):
        mock_get.return_value = reply([])
        OpenLibraryProvider().lookup(read_file_title('A nuvem', None, 'epub'))
        assert all('isbn' not in c.kwargs['params'] for c in mock_get.call_args_list)

    @patch('providers.http.requests.get')
    def test_a_work_with_a_great_many_editions_does_not_carry_them_all(self, mock_get):
        mock_get.return_value = reply([{'key': '/works/OL1W', 'title': 'Dune', 'isbn': [f'978{i:010d}' for i in range(MAX_ISBNS + 50)]}])
        (r,) = OpenLibraryProvider().lookup(read_file_title('Dune', None, 'epub', ISBN))
        assert len(r.raw['isbns']) == 300 == MAX_ISBNS


class TestTheRegistryWithAnIsbn:
    def registry(self, answer):
        provider = GoogleBooksProvider()
        provider.api_key = KEY
        registry = ProviderRegistry(enabled=lambda pid: pid == 'google_books')
        registry._providers = {'default': [provider], 'epub': [provider]}
        return registry

    def test_the_book_with_the_isbn_is_chosen_though_the_title_is_another(self):
        items = [book('Thunderhead', ('Neal Shusterman',), industryIdentifiers=[{'type': 'ISBN_13', 'identifier': ISBN}])]
        with patched(Net(lambda url, p: {'items': items})):
            got = self.registry(None).search_best('A nuvem (Scythe)', 'epub', author='Neal Shusterman', isbn=ISBN)
        assert got.title == 'Thunderhead' and got.match['isbn'] is True

    def test_without_the_isbn_the_same_answer_is_not_the_book(self):
        items = [book('Thunderhead', ('Neal Shusterman',), industryIdentifiers=[{'type': 'ISBN_13', 'identifier': ISBN}])]
        with patched(Net(lambda url, p: {'items': items})):
            assert self.registry(None).search_best('A nuvem (Scythe)', 'epub', author='Neal Shusterman') is None

    def test_an_isbn_that_is_not_one_is_not_sent_and_decides_nothing(self):
        net = Net(lambda url, p: {'items': []})
        with patched(net):
            self.registry(None).search_best('A nuvem', 'epub', isbn='123')
        assert all(not p['q'].startswith('isbn:') for p in net.params())
