"""AniList and MangaDex (the series of a manga), the volume of a series, and the two levels together."""
from unittest.mock import MagicMock, patch

from providers.anilist import AniListProvider, clean_description, pick_tags, staff_role, QUERY
from providers.base import BaseProvider, Credit, MetadataRecord
from providers.mangadex import MangaDexProvider, first_text
from providers.openlibrary import OpenLibraryProvider
from providers.query import read_file_title
from providers.registry import ProviderRegistry
from providers.text import is_volume


def reply(status=200, data=None):
    response = MagicMock()
    response.status_code = status
    response.json.return_value = data
    return response


BERSERK = {
    'id': 30002, 'format': 'MANGA', 'status': 'RELEASING', 'countryOfOrigin': 'JP', 'volumes': 42, 'chapters': None, 'popularity': 95000, 'isAdult': False,
    'startDate': {'year': 1989, 'month': 8, 'day': 25},
    'title': {'romaji': 'Berserk', 'english': 'Berserk', 'native': 'ベルセルク'}, 'synonyms': ['Berserk: Hunter'],
    'description': 'A mercenary.<br><br>Dark fantasy.<br />(Source: Dark Horse)<i>x</i>', 'genres': ['Action', 'Drama'],
    'tags': [{'name': 'Seinen', 'rank': 95, 'isMediaSpoiler': False, 'isAdult': False}, {'name': 'Gore', 'rank': 80, 'isMediaSpoiler': False, 'isAdult': False},
             {'name': 'Twist', 'rank': 90, 'isMediaSpoiler': True, 'isAdult': False}, {'name': 'Faint', 'rank': 20, 'isMediaSpoiler': False, 'isAdult': False}],
    'coverImage': {'extraLarge': 'https://s4.anilist.co/x-xl.jpg', 'large': 'https://s4.anilist.co/x-l.jpg'},
    'staff': {'edges': [{'role': 'Story & Art', 'node': {'name': {'full': 'Kentarou Miura'}}}, {'role': 'Assistant', 'node': {'name': {'full': 'Someone'}}},
                        {'role': 'Story', 'node': {'name': {'full': 'Kouji Mori'}}}]},
}


class TestHowFastTheyAreAsked:
    def test_anilist_leaves_a_second_between_requests_and_mangadex_half(self):
        with patch('providers.anilist.get_json') as asked:
            asked.return_value = MagicMock(ok=False)
            AniListProvider().lookup(read_file_title('Berserk', None, 'cbz'))
        assert asked.call_args[1]['interval'] == 1.0
        with patch('providers.mangadex.get_json') as asked:
            asked.return_value = MagicMock(ok=False)
            MangaDexProvider().lookup(read_file_title('Berserk', None, 'cbz'))
        assert asked.call_args[1]['interval'] == 0.5

    @patch('providers.http.requests.post')
    @patch('providers.http.requests.get')
    def test_a_title_with_nothing_in_it_asks_nothing(self, mock_get, mock_post):
        assert AniListProvider().lookup(read_file_title('', None, 'cbz')) == [] and MangaDexProvider().lookup(read_file_title('', None, 'cbz')) == []
        assert mock_get.call_count == 0 and mock_post.call_count == 0


class TestAniList:
    def setup_method(self):
        self.provider = AniListProvider()

    def test_names(self):
        assert (self.provider.id, self.provider.name) == ('anilist', 'AniList')

    @patch('providers.http.requests.post')
    def test_it_asks_for_manga_by_the_title_only(self, mock_post):
        mock_post.return_value = reply(200, {'data': {'Page': {'media': [BERSERK]}}})
        self.provider.lookup(read_file_title('Berserk v01', 'Kentaro Miura', 'cbz'))
        url, kwargs = mock_post.call_args[0][0], mock_post.call_args[1]
        assert url == 'https://graphql.anilist.co'
        assert kwargs['json']['variables'] == {'q': 'Berserk'} and kwargs['json']['query'] == QUERY
        assert 'type: MANGA' in QUERY and 'Miura' not in str(kwargs['json'])

    @patch('providers.http.requests.post')
    def test_a_record_is_the_series(self, mock_post):
        mock_post.return_value = reply(200, {'data': {'Page': {'media': [BERSERK]}}})
        (r,) = self.provider.lookup(read_file_title('Berserk', None, 'cbz'))
        assert (r.title, r.series, r.source, r.publication_date) == ('Berserk', 'Berserk', 'AniList', '1989-08-25')
        assert [(c.name, c.role) for c in r.credits] == [('Kentarou Miura', 'author, illustrator'), ('Kouji Mori', 'author')]
        assert r.author == 'Kentarou Miura'
        assert r.description == 'A mercenary.\n\nDark fantasy.\n(Source: Dark Horse)x'
        assert r.tags == ['Action', 'Drama', 'Seinen', 'Gore']   # the spoiler and the faint tag are left out
        assert r.cover_url == 'https://s4.anilist.co/x-xl.jpg'
        assert r.raw['anilist_id'] == 30002 and r.raw['alt_titles'] == ['Berserk', 'ベルセルク', 'Berserk: Hunter']
        assert (r.raw['volumes'], r.raw['status'], r.raw['country'], r.raw['format']) == (42, 'RELEASING', 'JP', 'MANGA')
        assert r.raw['native_title'] == 'ベルセルク'   # the title in its own script, for the series (DEC-171)
        assert 9 < r.prior <= 9.5 and r.series_index is None

    @patch('providers.http.requests.post')
    def test_less_in_the_answer_is_less_in_the_record(self, mock_post):
        bare = {'id': 1, 'title': {'native': '作品'}, 'startDate': {'year': 2001}, 'staff': {'edges': [{'role': 'Art', 'node': {'name': {'full': 'Só Desenha'}}}]}, 'popularity': 10**9}
        mock_post.return_value = reply(200, {'data': {'Page': {'media': [bare]}}})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert r.title == '作品' and r.publication_date == '2001' and r.description is None and r.cover_url is None and r.tags == []
        assert r.author == 'Só Desenha' and r.credits[0].role == 'illustrator' and r.prior == 10

    @patch('providers.http.requests.post')
    def test_a_work_for_adults_is_left_out(self, mock_post):
        mock_post.return_value = reply(200, {'data': {'Page': {'media': [dict(BERSERK, isAdult=True), dict(BERSERK, id=2)]}}})
        assert [r.raw['anilist_id'] for r in self.provider.lookup(read_file_title('Berserk', None, 'cbz'))] == [2]

    @patch('providers.http.requests.post')
    def test_what_does_not_answer_gives_nothing(self, mock_post):
        for answer in (reply(429), reply(200, {'data': None}), reply(200, {'data': {'Page': None}}), reply(200, {})):
            mock_post.return_value = answer
            assert self.provider.lookup(read_file_title('Berserk', None, 'cbz')) == []
        assert self.provider.lookup(read_file_title('', None, 'cbz')) == []

    @patch('providers.http.requests.post')
    def test_the_manual_search_gives_the_first(self, mock_post):
        mock_post.return_value = reply(200, {'data': {'Page': {'media': [BERSERK]}}})
        assert self.provider.search('berserk').title == 'Berserk'
        mock_post.return_value = reply(200, {'data': {'Page': {'media': []}}})
        assert self.provider.search('x') is None

    @patch('providers.http.requests.post')
    def test_at_most_four_people_of_the_staff_are_credited_and_the_one_who_wrote_it_comes_first(self, mock_post):
        edges = [{'role': 'Art', 'node': {'name': {'full': 'Desenhista'}}}] + [{'role': 'Story', 'node': {'name': {'full': f'Roteirista {i}'}}} for i in range(6)]
        mock_post.return_value = reply(200, {'data': {'Page': {'media': [dict(BERSERK, staff={'edges': edges})]}}})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert len(r.credits) == 4 and r.author == 'Roteirista 0' and r.credits[0].name == 'Desenhista'

    @patch('providers.http.requests.post')
    def test_a_start_date_with_no_day_is_the_year(self, mock_post):
        for start, want in [({'year': 1989, 'month': 8, 'day': None}, '1989'), ({'year': 1989, 'month': None, 'day': 5}, '1989'), ({'year': 1989, 'month': 8, 'day': 5}, '1989-08-05'), ({}, None)]:
            mock_post.return_value = reply(200, {'data': {'Page': {'media': [dict(BERSERK, startDate=start)]}}})
            (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
            assert r.publication_date == want, start

    @patch('providers.http.requests.post')
    def test_only_the_first_six_synonyms_are_other_names(self, mock_post):
        mock_post.return_value = reply(200, {'data': {'Page': {'media': [dict(BERSERK, synonyms=[f'Sinônimo {i}' for i in range(10)])]}}})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert r.raw['alt_titles'] == ['Berserk', 'ベルセルク'] + [f'Sinônimo {i}' for i in range(6)]

    def test_the_synopsis_loses_the_markup(self):
        assert clean_description('a<br>b<BR/>c<p>d</p>\n\n\n\ne') == 'a\nb\ncd\n\ne'
        assert clean_description(None) == '' and clean_description('  ') == ''

    def test_what_a_person_of_the_staff_is_to_the_library(self):
        assert staff_role('Story & Art') == 'author, illustrator'
        assert staff_role('Original Creator') == 'author' and staff_role('Illustration') == 'illustrator'
        assert staff_role('Art') == 'illustrator' and staff_role('Story') == 'author'
        assert staff_role('Assistant') == '' and staff_role(None) == ''

    def test_the_tags_that_matter_come_after_the_genres_each_once(self):
        tags = [{'name': 'Adult', 'rank': 99, 'isAdult': True}] + [{'name': f'T{i}', 'rank': 60 + i} for i in range(20)] + [{'name': 'action', 'rank': 99}]
        got = pick_tags(['Action', 'Drama', 'Action', '', None], tags)
        assert got[:2] == ['Action', 'Drama'] and len(got) == 12 and 'Adult' not in got and got.count('Action') == 1
        assert pick_tags(None, None) == []
        assert pick_tags([], [{'name': 'Edge', 'rank': 60}, {'name': 'Below', 'rank': 59}]) == ['Edge']


DEX = {
    'id': 'abc-1', 'attributes': {
        'title': {'en': 'Berserk'}, 'altTitles': [{'ja': 'ベルセルク'}, {'pt-br': 'Berserk (BR)'}], 'description': {'en': 'A mercenary.', 'pt-br': 'Um mercenário.'},
        'year': 1989, 'status': 'ongoing', 'publicationDemographic': 'seinen', 'originalLanguage': 'ja',
        'tags': [{'attributes': {'name': {'en': 'Action'}, 'group': 'genre'}}, {'attributes': {'name': {'en': 'Monsters'}, 'group': 'theme'}},
                 {'attributes': {'name': {'en': 'Full Color'}, 'group': 'format'}}, {'attributes': {'name': {'en': 'Action'}, 'group': 'genre'}}],
    },
    'relationships': [{'type': 'author', 'attributes': {'name': 'Miura Kentarou'}}, {'type': 'artist', 'attributes': {'name': 'Miura Kentarou'}},
                      {'type': 'author', 'attributes': {'name': 'Mori Kouji'}}, {'type': 'artist', 'attributes': {'name': 'Studio Gaga'}},
                      {'type': 'cover_art', 'attributes': {'fileName': 'cover.jpg'}}, {'type': 'manga', 'id': 'x'}],
}


class TestMangaDex:
    def setup_method(self):
        self.provider = MangaDexProvider()

    def test_names(self):
        assert (self.provider.id, self.provider.name) == ('mangadex', 'MangaDex')

    @patch('providers.http.requests.get')
    def test_it_asks_for_the_title_with_the_people_and_the_cover(self, mock_get):
        mock_get.return_value = reply(200, {'data': [DEX]})
        self.provider.lookup(read_file_title('Berserk 01', 'Kentaro Miura', 'cbz'))
        assert mock_get.call_args[0][0] == 'https://api.mangadex.org/manga'
        params = mock_get.call_args[1]['params']
        assert params['title'] == 'Berserk' and params['includes[]'] == ['author', 'artist', 'cover_art'] and params['limit'] == 10
        assert 'Miura' not in str(params)

    @patch('providers.http.requests.get')
    def test_a_record_is_the_series(self, mock_get):
        mock_get.return_value = reply(200, {'data': [DEX]})
        (r,) = self.provider.lookup(read_file_title('Berserk', None, 'cbz'))
        assert (r.title, r.series, r.source, r.publication_date) == ('Berserk', 'Berserk', 'MangaDex', '1989')
        assert [(c.name, c.role) for c in r.credits] == [('Miura Kentarou', 'author, illustrator'), ('Mori Kouji', 'author'), ('Studio Gaga', 'illustrator')]
        assert r.author == 'Miura Kentarou'
        assert r.description == 'Um mercenário.'
        assert r.tags == ['Seinen', 'Action', 'Monsters']   # the demographic first; only genres and themes; each once
        assert r.cover_url == 'https://uploads.mangadex.org/covers/abc-1/cover.jpg.512.jpg'
        assert r.raw['mangadex_id'] == 'abc-1' and r.raw['alt_titles'] == ['ベルセルク', 'Berserk (BR)'] and r.raw['demographic'] == 'seinen'
        assert r.raw['status'] == 'ongoing' and r.raw['language'] == 'ja'
        assert r.raw['native_title'] == 'ベルセルク'   # the alternative title in the language the series was written in

    @patch('providers.http.requests.get')
    def test_less_in_the_answer_is_less_in_the_record(self, mock_get):
        bare = {'id': 'z', 'attributes': {'title': {'ja-ro': 'Sakuhin'}, 'description': {}}, 'relationships': [{'type': 'artist', 'attributes': {'name': 'Desenhista'}}]}
        mock_get.return_value = reply(200, {'data': [bare]})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert r.title == 'Sakuhin' and r.description is None and r.cover_url is None and r.publication_date is None and r.tags == []
        assert r.author == 'Desenhista' and r.credits[0].role == 'illustrator'

    @patch('providers.http.requests.get')
    def test_no_title_in_the_original_language_is_no_native_title(self, mock_get):
        only_en = {'id': 'w', 'attributes': {'title': {'en': 'Attack on Titan'}, 'altTitles': [{'pt-br': 'Ataque dos Titãs'}], 'originalLanguage': 'ja'}, 'relationships': []}
        mock_get.return_value = reply(200, {'data': [only_en]})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert r.raw['native_title'] is None
        no_language = {'id': 'v', 'attributes': {'title': {'en': 'X'}, 'altTitles': [{'ja': 'エックス'}]}, 'relationships': []}
        mock_get.return_value = reply(200, {'data': [no_language]})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert r.raw['native_title'] is None   # without knowing the language, no title is the native one

    @patch('providers.http.requests.get')
    def test_the_other_names_of_the_title_count(self, mock_get):
        twin = {'id': 'w', 'attributes': {'title': {'en': 'Attack on Titan', 'ja-ro': 'Shingeki no Kyojin'}}, 'relationships': []}
        mock_get.return_value = reply(200, {'data': [twin]})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert r.title == 'Attack on Titan' and r.raw['alt_titles'] == ['Shingeki no Kyojin']

    @patch('providers.http.requests.get')
    def test_what_does_not_answer_gives_nothing(self, mock_get):
        for answer in (reply(429), reply(200, {}), reply(200, {'data': []})):
            mock_get.return_value = answer
            assert self.provider.lookup(read_file_title('Berserk', None, 'cbz')) == []
        assert self.provider.lookup(read_file_title('', None, 'cbz')) == []

    @patch('providers.http.requests.get')
    def test_the_manual_search_gives_the_first(self, mock_get):
        mock_get.return_value = reply(200, {'data': [DEX]})
        assert self.provider.search('berserk').title == 'Berserk'
        mock_get.return_value = reply(200, {'data': []})
        assert self.provider.search('x') is None

    @patch('providers.http.requests.get')
    def test_the_params_ask_for_the_most_relevant_first(self, mock_get):
        mock_get.return_value = reply(200, {'data': []})
        self.provider.lookup(read_file_title('Berserk', None, 'cbz'))
        assert mock_get.call_args[1]['params']['order[relevance]'] == 'desc'

    @patch('providers.http.requests.get')
    def test_what_is_not_a_person_nor_the_cover_is_not_a_credit(self, mock_get):
        manga = dict(DEX, relationships=[{'type': 'scanlation_group', 'attributes': {'name': 'Grupo'}}, {'type': 'author', 'attributes': {'name': 'Autor'}},
                                         {'type': 'author', 'attributes': {'name': 'Autor'}}])
        mock_get.return_value = reply(200, {'data': [manga]})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert [(c.name, c.role) for c in r.credits] == [('Autor', 'author')]

    @patch('providers.http.requests.get')
    def test_the_one_who_wrote_it_is_the_author_and_a_cover_needs_the_manga_id(self, mock_get):
        manga = dict(DEX, id=None, relationships=[{'type': 'artist', 'attributes': {'name': 'Desenhista'}}, {'type': 'author', 'attributes': {'name': 'Roteirista'}},
                                                  {'type': 'cover_art', 'attributes': {'fileName': 'c.jpg'}}])
        mock_get.return_value = reply(200, {'data': [manga]})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert r.author == 'Roteirista' and r.credits[0].name == 'Desenhista' and r.cover_url is None

    @patch('providers.http.requests.get')
    def test_at_most_four_people_twelve_tags_and_twelve_other_names(self, mock_get):
        manga = dict(DEX)
        manga['attributes'] = dict(DEX['attributes'], tags=[{'attributes': {'name': {'en': f'Tag {i}'}, 'group': 'genre'}} for i in range(20)],
                                   altTitles=[{'ja': f'Nome {i}'} for i in range(20)])
        manga['relationships'] = [{'type': 'author', 'attributes': {'name': f'Autor {i}'}} for i in range(6)]
        mock_get.return_value = reply(200, {'data': [manga]})
        (r,) = self.provider.lookup(read_file_title('x', None, 'cbz'))
        assert len(r.credits) == 4 and len(r.tags) == 12 and r.tags[0] == 'Seinen' and len(r.raw['alt_titles']) == 12

    def test_a_text_that_comes_by_language_is_read_in_the_order_wanted(self):
        assert first_text({'en': 'a', 'pt-br': 'b'}, ('pt-br', 'en')) == 'b'
        assert first_text({'en': 'a'}, ('pt-br', 'pt')) == 'a'
        assert first_text({'ja': ''}, ('en',)) is None and first_text(None, ('en',)) is None and first_text({'x': 5}) is None


class TestOneVolumeOfASeries:
    NARUTO = ['Naruto']

    def test_the_name_of_the_series_the_number_and_nothing_else(self):
        for title in ('Naruto 01', 'NARUTO 1', 'Naruto, Volume 1', 'Naruto vol. 1', 'Naruto Vol 01', 'Naruto: Tome 1', 'Naruto Manga 1'):
            assert is_volume(title, self.NARUTO, 1), title

    def test_another_volume_a_box_or_another_edition_is_not(self):
        for title in ('Naruto 02', 'Naruto', 'Naruto Deluxe Volume 1', 'Naruto Box Set Volumes 1-27', 'Naruto Collection 13 Book Set', 'Naruto Complete Edition 1',
                      'Naruto 1 and 2', 'Naruto: The Official Guide 1', 'Naruto Shippuden 1', '1', 'Volume 1', 'Boruto 1', 'Naruto Omnibus 1', 'Naruto Artbook 1'):
            assert not is_volume(title, self.NARUTO, 1), title

    def test_a_title_with_no_word_in_latin_letters_is_no_volume_of_any_series(self):
        # "no words" is not the same as "the same words": a title in Japanese is nothing to compare, and must not match another series in Japanese
        assert not is_volume('ベルセルク 1', ['Berserk', 'ベルセルク'], 1)
        assert not is_volume('ワンピース 1', ['ベルセルク'], 1)
        assert not is_volume('バガボンド(1)', ['Vagabond'], 1)
        assert not is_volume('1', ['Naruto'], 1) and not is_volume('Volume 1', ['Naruto'], 1)

    def test_a_name_that_is_only_part_of_the_series_name_is_not_the_series(self):
        assert not is_volume('Attack 1', ['Attack on Titan'], 1)
        assert not is_volume('Attack on Titan Before the Fall 1', ['Attack on Titan'], 1)
        assert is_volume('Attack on Titan 1', ['Attack on Titan'], 1)

    def test_any_name_of_the_series_will_do(self):
        assert is_volume('Attack on Titan 1', ['Shingeki no Kyojin', 'Attack on Titan'], 1)
        assert not is_volume('Attack on Titan 1', ['Shingeki no Kyojin'], 1)
        assert not is_volume('Naruto 1', [], 1)

    def test_a_volume_of_ten_is_not_volume_one(self):
        assert is_volume('Naruto 10', self.NARUTO, 10) and not is_volume('Naruto 10', self.NARUTO, 1)


class TestOpenLibraryVolume:
    @patch('providers.http.requests.get')
    def test_the_volume_is_asked_by_the_name_and_the_number_and_chosen_by_what_it_is(self, mock_get):
        docs = [{'key': '/works/OL1W', 'title': 'Naruto Box Set 1', 'edition_count': 50},
                {'key': '/works/OL2W', 'title': 'Naruto 01', 'edition_count': 3, 'isbn': ['9781569319000'], 'cover_i': 7},
                {'key': '/works/OL3W', 'title': 'NARUTO 1', 'edition_count': 9, 'isbn': ['9781421500000']}]
        mock_get.return_value = reply(200, {'docs': docs})
        got = OpenLibraryProvider().volume(['Naruto'], 1)
        assert mock_get.call_args[1]['params']['q'] == 'Naruto volume 1'
        assert got.raw['openlibrary_work'] == '/works/OL3W' and got.isbn == '9781421500000'

    @patch('providers.http.requests.get')
    def test_none_when_no_book_is_exactly_that_volume(self, mock_get):
        mock_get.return_value = reply(200, {'docs': [{'key': '/works/OL1W', 'title': 'Naruto Deluxe Volume 1'}]})
        assert OpenLibraryProvider().volume(['Naruto'], 1) is None
        mock_get.return_value = reply(429)
        assert OpenLibraryProvider().volume(['Naruto'], 1) is None
        mock_get.reset_mock()
        assert OpenLibraryProvider().volume([], 1) is None and OpenLibraryProvider().volume(['Naruto'], None) is None and mock_get.call_count == 0

    @patch('providers.http.requests.get')
    def test_a_record_of_a_work_has_the_id_of_the_work(self, mock_get):
        mock_get.return_value = reply(200, {'docs': [{'key': '/works/OL893414W', 'title': 'Dune'}]})
        (r,) = OpenLibraryProvider().lookup(read_file_title('Dune', None, 'epub'))
        assert r.raw['openlibrary_id'] == 'OL893414W'


class Series(BaseProvider):
    def __init__(self, pid, record, fail=False):
        self._id, self.record, self.fail = pid, record, fail
        self.asked = []

    id = property(lambda self: self._id)
    name = property(lambda self: self._id.title())

    def search(self, query):
        raise AssertionError

    def lookup(self, query):
        self.asked.append(query)
        return [self.record]


class Books(BaseProvider):
    def __init__(self, volume=None, fail=False, pid='openlibrary'):
        self._id, self.found, self.fail, self.asked = pid, volume, fail, []

    id = property(lambda self: self._id)
    name = property(lambda self: 'OpenLibrary')

    def search(self, query):
        raise AssertionError

    def lookup(self, query):
        return []

    def volume(self, titles, number):
        self.asked.append((titles, number))
        if self.fail:
            raise RuntimeError('down')
        return self.found


def berserk():
    r = MetadataRecord(title='Berserk', series='Berserk', source='AniList', cover_url='https://a/series.jpg', publication_date='1989-08-25', raw={'alt_titles': ['ベルセルク']},
                       credits=[Credit('Kentarou Miura', 'author')])
    r.author = 'Kentarou Miura'
    return r


def volume_one():
    return MetadataRecord(title='Berserk, Vol. 1', source='OpenLibrary', isbn='9781593070205', publisher='Dark Horse', publication_date='2003', cover_url='https://a/vol1.jpg',
                          raw={'openlibrary_work': '/works/OL9W'})


def registry(allowed, *providers):
    r = ProviderRegistry(enabled=lambda pid: pid in allowed)
    r._providers = {'cbz': list(providers), 'default': list(providers)}
    return r


class TestMangaCatalogsBeforeTheComicsDatabase:
    def issue(self):
        r = MetadataRecord(title='Vol. 1', series='Berserk', series_index=1.0, publisher='Hakusensha', source='ComicVine')
        r.raw = {'comicvine_id': '4000-1'}
        return r

    def test_a_manga_catalog_that_knows_the_title_is_taken_though_the_other_has_the_number_of_the_volume(self):
        for order in (('anilist', 'comicvine'), ('comicvine', 'anilist')):
            providers = {'anilist': Series('anilist', berserk()), 'comicvine': Series('comicvine', self.issue())}
            got = registry({'anilist', 'comicvine'}, *[providers[i] for i in order]).search_best('Berserk v01', 'cbz')
            assert got.provider_id == 'anilist' and got.series_index == 1.0

    def test_without_one_the_comics_database_is_the_answer(self):
        got = registry({'anilist', 'comicvine'}, Series('anilist', MetadataRecord(title='Outra Obra', series='Outra Obra', source='AniList')),
                       Series('comicvine', self.issue())).search_best('Berserk v01', 'cbz')
        assert got.provider_id == 'comicvine' and got.publisher == 'Hakusensha'

    def test_a_manga_catalog_that_is_off_leaves_the_comics_database(self):
        got = registry({'comicvine'}, Series('anilist', berserk()), Series('comicvine', self.issue())).search_best('Berserk v01', 'cbz')
        assert got.provider_id == 'comicvine'


class TestTwoLevels:
    def test_the_series_gets_the_number_of_the_volume_and_what_the_volume_has(self, capsys):
        books = Books(volume_one())
        got = registry({'anilist', 'openlibrary'}, Series('anilist', berserk()), books).search_best('Berserk v01', 'cbz')
        assert got.series_index == 1.0 and got.series == 'Berserk' and got.title == 'Berserk'
        assert (got.isbn, got.publisher, got.publication_date, got.cover_url) == ('9781593070205', 'Dark Horse', '2003', 'https://a/vol1.jpg')
        assert got.source == 'AniList + OpenLibrary' and got.raw['volume_work'] == '/works/OL9W' and got.raw['volume_title'] == 'Berserk, Vol. 1'
        assert got.raw['alt_titles'] == ['ベルセルク'] and books.asked == [(['Berserk', 'ベルセルク'], 1)]
        out = capsys.readouterr().out
        assert "Volume 1 from OpenLibrary: 'Berserk, Vol. 1'" in out and 'failed for the volume' not in out

    def test_the_series_provider_itself_is_not_asked_for_a_volume(self, capsys):
        registry({'anilist'}, Series('anilist', berserk())).search_best('Berserk v01', 'cbz')
        assert 'failed for the volume' not in capsys.readouterr().out

    def test_both_manga_providers_are_series_providers(self):
        for pid in ('anilist', 'mangadex'):
            got = registry({pid, 'openlibrary'}, Series(pid, berserk()), Books(volume_one())).search_best('Berserk v01', 'cbz')
            assert got.isbn == '9781593070205' and got.series_index == 1.0, pid

    def test_what_the_series_has_stays_when_the_volume_has_not_it(self):
        series = berserk()
        series.isbn, series.publisher = '9780000000000', 'Panini'
        bare = MetadataRecord(title='Berserk 1', source='OpenLibrary')
        got = registry({'anilist', 'openlibrary'}, Series('anilist', series), Books(bare)).search_best('Berserk v01', 'cbz')
        assert (got.isbn, got.publisher, got.publication_date, got.cover_url) == ('9780000000000', 'Panini', '1989-08-25', 'https://a/series.jpg')

    def test_the_first_book_provider_with_the_volume_is_the_one(self):
        first, second = Books(volume_one(), pid='google_books'), Books(volume_one(), pid='openlibrary')
        registry({'anilist', 'google_books', 'openlibrary'}, Series('anilist', berserk()), first, second).search_best('Berserk v01', 'cbz')
        assert len(first.asked) == 1 and second.asked == []

    def test_the_name_of_the_series_the_provider_gives_is_kept(self):
        record = berserk()
        record.series = 'Berserk (série)'
        got = registry({'anilist'}, Series('anilist', record)).search_best('Berserk v01', 'cbz')
        assert got.series == 'Berserk (série)'

    def test_without_a_book_for_the_volume_the_suggestion_is_the_series_with_its_number(self, capsys):
        got = registry({'anilist', 'openlibrary'}, Series('anilist', berserk()), Books(None)).search_best('Berserk v02', 'cbz')
        assert got.series_index == 2.0 and got.isbn is None and got.cover_url == 'https://a/series.jpg' and got.source == 'AniList'
        assert 'No book that is exactly volume 2' in capsys.readouterr().out

    def test_the_book_provider_that_is_off_is_not_asked(self):
        books = Books(volume_one())
        got = registry({'anilist'}, Series('anilist', berserk()), books).search_best('Berserk v01', 'cbz')
        assert books.asked == [] and got.series_index == 1.0 and got.isbn is None

    def test_a_book_provider_that_fails_does_not_lose_the_series(self, capsys):
        got = registry({'anilist', 'openlibrary'}, Series('anilist', berserk()), Books(fail=True)).search_best('Berserk v01', 'cbz')
        assert got is not None and got.source == 'AniList' and 'failed for the volume: down' in capsys.readouterr().out

    def test_the_next_book_provider_is_asked_when_the_first_has_no_volume(self):
        first, second = Books(None, pid='google_books'), Books(volume_one(), pid='openlibrary')
        got = registry({'anilist', 'google_books', 'openlibrary'}, Series('anilist', berserk()), first, second).search_best('Berserk v01', 'cbz')
        assert len(first.asked) == 1 and len(second.asked) == 1 and got.isbn == '9781593070205'

    def test_a_file_with_no_number_is_the_series_as_it_is(self):
        books = Books(volume_one())
        got = registry({'anilist', 'openlibrary'}, Series('anilist', berserk()), books).search_best('Berserk', 'cbz')
        assert got.series_index is None and books.asked == [] and got.series == 'Berserk'

    def test_a_series_of_a_provider_that_has_no_series_with_the_title_in_it_takes_the_title(self):
        record = berserk()
        record.series = None
        got = registry({'anilist'}, Series('anilist', record)).search_best('Berserk v01', 'cbz')
        assert got.series == 'Berserk' and got.series_index == 1.0

    def test_an_answer_that_is_not_about_a_series_is_left_as_it_is(self):
        book = MetadataRecord(title='Dune', source='OpenLibrary', raw={})
        books = Books(volume_one(), pid='openlibrary')
        got = registry({'openlibrary'}, Series('openlibrary', book), books).search_best('Dune 2', 'cbz')
        assert got.series_index is None and books.asked == [] and got.source == 'OpenLibrary'

    def test_the_other_names_of_a_manga_find_it(self):
        record = berserk()
        record.title, record.series, record.raw = 'Shingeki no Kyojin', 'Shingeki no Kyojin', {'alt_titles': ['Attack on Titan']}
        got = registry({'anilist'}, Series('anilist', record)).search_best('Attack on Titan 01', 'cbz')
        assert got is not None and got.title == 'Shingeki no Kyojin' and got.series_index == 1.0

    def test_the_record_says_which_provider_answered(self):
        got = registry({'anilist'}, Series('anilist', berserk())).search_best('Berserk', 'cbz')
        assert got.provider_id == 'anilist'

    def test_a_cover_is_downloaded_saying_who_asks(self, tmp_path):
        with patch('requests.get') as mock_get:
            mock_get.return_value = MagicMock(status_code=200, content=b'jpg')
            out = MangaDexProvider().download_cover('https://uploads.mangadex.org/x.jpg', 'f.cbz', str(tmp_path))
        assert out.startswith('/covers/provider_') and mock_get.call_args[1]['headers']['User-Agent'].startswith('Codice (+https://github.com/ocnaibill/codice')
