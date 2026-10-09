"""Reading the title of a file, and judging what a provider answers against it."""
from providers.base import Credit, MetadataRecord
from providers.match import AUTHOR_MIN, TITLE_MIN, judge
from providers.query import author_closeness, author_names, read_file_title
from providers.text import closeness, fold, main_title, tokens


class TestText:
    def test_fold_takes_accents_and_case(self):
        assert fold('Lusíadas ÇÃO') == 'lusiadas cao'
        assert fold(None) == ''

    def test_tokens_are_the_words_and_numbers_without_the_ones_that_say_nothing(self):
        assert tokens('The Lord of the Rings, 2') == ['lord', 'rings', '2']
        assert tokens('Os Lusíadas') == ['lusiadas']
        assert tokens('A Nuvem 2') == ['nuvem', '2']
        assert tokens('The End', keep_stop=True) == ['the', 'end']
        assert tokens('') == [] and tokens(None) == []

    def test_main_title_leaves_the_subtitle(self):
        assert main_title('Sapiens: Uma breve história da humanidade') == 'Sapiens'
        assert main_title('Duna – Parte 2') == 'Duna'
        assert main_title('Duna') == 'Duna'
        assert main_title(None) == ''

    def test_closeness_is_the_share_of_words_in_common(self):
        assert closeness('Dune', 'dune') == 1.0
        assert closeness('Dune', 'Dune Messiah') == 2 * 1 / (1 + 2)
        assert closeness('Duna', 'Herdeiras de Duna') == 2 / 3
        assert closeness('Dune', 'Neuromancer') == 0.0
        assert closeness('', 'Dune') == 0.0 and closeness('the of', 'Dune') == 0.0
        assert closeness('', '') == 0.0 and closeness('the', 'of') == 0.0   # no word in either: nothing to divide


class TestReadingTheTitle:
    def test_a_plain_title_is_itself(self):
        q = read_file_title('Dune', 'Frank Herbert', 'epub')
        assert (q.title, q.left, q.stripped, q.number, q.hint, q.author) == ('Dune', 'Dune', 'Dune', None, None, 'Frank Herbert')
        assert q.search_title == 'Dune' and q.variants() == [('Dune', 'Frank Herbert')]

    def test_what_is_in_parentheses_and_brackets_is_not_the_title(self):
        q = read_file_title('Absolute Batman 006 (2025) (Digital) [Group]', None, 'cbz')
        assert q.title == 'Absolute Batman 006'

    def test_a_number_at_the_end_is_the_number_of_a_volume_or_an_issue(self):
        for text, number, rest in [('Berserk v01', 1, 'Berserk'), ('Saga 001', 1, 'Saga'), ('Vagabond Vol. 12', 12, 'Vagabond'),
                                   ('Naruto #7', 7, 'Naruto'), ('Livro 3', None, 'Livro 3'), ('A Nuvem 2', 2, 'A Nuvem'), ('Fahrenheit 451', 451, 'Fahrenheit')]:
            q = read_file_title(text)
            assert (q.number, q.stripped) == (number, rest), text

    def test_the_marks_between_a_title_and_its_number_go_with_the_number(self):
        for text in ('Duna: 2', 'Duna - 2', 'Duna, 2'):
            q = read_file_title(text)
            assert (q.number, q.stripped) == (2, 'Duna'), text

    def test_a_short_title_with_a_number_still_has_it(self):
        q = read_file_title('X 3')
        assert (q.number, q.stripped) == (3, 'X')
        assert read_file_title('Livro 3').number is None   # the whole title is the word and the number

    def test_a_title_that_is_only_a_number_has_no_number(self):
        q = read_file_title('1984', 'George Orwell')
        assert q.number is None and q.stripped == '1984'

    def test_the_title_of_a_comic_is_sent_without_its_number_and_a_book_s_with_it(self):
        assert read_file_title('Absolute Batman 006', None, 'cbz').search_title == 'Absolute Batman'
        assert read_file_title('Berserk v01', None, 'cbr').search_title == 'Berserk'
        assert read_file_title('Fahrenheit 451', None, 'epub').search_title == 'Fahrenheit 451'
        assert read_file_title('Fahrenheit 451', None, 'pdf').search_title == 'Fahrenheit 451'

    def test_the_author_after_a_dash_is_a_hint_when_it_looks_like_a_name(self):
        q = read_file_title('A Nuvem 2 - Neal Shusterman', None, 'pdf')
        assert (q.left, q.stripped, q.number, q.hint, q.author) == ('A Nuvem 2', 'A Nuvem', 2, 'Neal Shusterman', None)
        assert q.search_title == 'A Nuvem 2'
        assert q.variants() == [('A Nuvem 2 - Neal Shusterman', None), ('A Nuvem 2', 'Neal Shusterman'), ('A Nuvem', 'Neal Shusterman')]

    def test_what_follows_a_dash_is_the_author_when_it_is_the_one_the_file_says(self):
        q = read_file_title('Scythe - Neal Shusterman', 'Neal Shusterman', 'pdf')
        assert q.left == 'Scythe' and q.hint == 'Neal Shusterman'

    def test_a_subtitle_after_a_dash_is_not_taken_for_an_author(self):
        for text in ('Harry Potter - The Chamber of Secrets', 'Duna - Parte Dois e Mais', 'Dune - 1965 Edition'):
            q = read_file_title(text, None, 'epub')
            assert q.hint is None and q.left == q.title, text

    def test_a_dash_part_that_is_not_the_authors_is_kept_when_the_author_is_known(self):
        q = read_file_title('Dune - Deluxe Edition', 'Frank Herbert', 'epub')
        assert q.hint is None and q.left == 'Dune - Deluxe Edition'

    def test_a_name_has_two_or_three_words(self):
        assert read_file_title('Foo - Neal Peter Smith', None).hint == 'Neal Peter Smith'
        assert read_file_title('Foo - Neal Peter Smith Jones', None).hint is None

    def test_a_capitalised_article_or_a_word_with_a_digit_is_not_in_a_name(self):
        assert read_file_title('Harry Potter - The Chamber', None).hint is None
        assert read_file_title('Dune - Vol2 Deluxe', None).hint is None
        assert read_file_title('Dune - Part Two', None).hint == 'Part Two'   # nothing in it says it is not a name

    def test_two_words_with_a_small_letter_are_not_a_name(self):
        assert read_file_title('Foo - neal shusterman', None).hint is None
        assert read_file_title('Foo - Neal', None).hint is None
        assert read_file_title('Foo - A B C D', None).hint is None

    def test_an_author_that_means_nobody_is_none(self):
        for author in (None, '', 'Unknown Author', 'unknown author', 'Autor Desconhecido', '  '):
            assert read_file_title('Dune', author).author is None
        assert read_file_title('Dune', '  Frank Herbert  ').author == 'Frank Herbert'

    def test_the_format_is_kept_and_defaults(self):
        assert read_file_title('x', None, None).format == 'default'
        assert read_file_title('x', None, 'CBZ').serial is True
        assert read_file_title('x', None, 'epub').serial is False

    def test_the_variants_are_each_once(self):
        assert read_file_title('Dune', None).variants() == [('Dune', None)]
        q = read_file_title('Berserk 1', None, 'cbz')
        assert q.variants() == [('Berserk 1', None), ('Berserk', None)]


class TestAuthors:
    def test_a_line_of_authors_is_the_people_in_it(self):
        assert author_names('Alan Moore; Dave Gibbons') == ['Alan Moore', 'Dave Gibbons']
        assert author_names('Moore, Alan & Dave Gibbons') == ['Moore, Alan', 'Dave Gibbons']
        assert author_names('Neil Gaiman and Terry Pratchett') == ['Neil Gaiman', 'Terry Pratchett']
        assert author_names('Maria e José') == ['Maria', 'José']
        assert author_names('') == [] and author_names(None) == []

    def test_the_same_person_written_in_different_ways(self):
        assert author_closeness('Andrew Hunt', ['Andy Hunt', 'Dave Thomas']) == 0.5
        assert author_closeness('Herbert, Frank', ['Frank Herbert']) == 1.0
        assert author_closeness('J. K. Rowling', ['J.K. Rowling']) == 1.0
        assert author_closeness('Luís de Camões', ['Luís Vaz de Camões']) > 0.7
        assert author_closeness('Alan Moore; Dave Gibbons', ['Dave Gibbons']) == 1.0

    def test_two_people_who_only_share_a_given_name_are_not_the_same(self):
        assert author_closeness('Andrew Hunt', ['Andrew Smith']) == 0.0
        assert author_closeness('Frank Herbert', ['Frank Miller']) == 0.0
        assert author_closeness('Frank Herbert', ['Brian Herbert']) == 0.5   # the same surname: the title decides

    def test_an_initial_is_not_a_surname(self):
        assert author_closeness('Smith, J.', ['Jones, J.']) == 0.0
        assert author_closeness('J. Smith', ['J. Jones']) == 0.0
        assert author_closeness('Smith, J.', ['J. Smith']) == 1.0

    def test_a_text_with_no_word_in_it_names_nobody(self):
        assert author_closeness(',', ['A B']) == 0.0
        assert author_closeness(', ,', [',']) == 0.0

    def test_nothing_to_compare_is_zero(self):
        assert author_closeness('', ['A B']) == 0.0
        assert author_closeness('A B', []) == 0.0


def rec(title, authors=(), **kw):
    r = MetadataRecord(title=title, credits=[Credit(a) for a in authors], source='x', **kw)
    r.author = authors[0] if authors else None
    return r


class TestJudging:
    def test_the_title_and_the_author_that_are_the_file_s_are_accepted(self):
        m = judge(read_file_title('Dune', 'Frank Herbert'), rec('Dune', ['Frank Herbert']))
        assert m.accepted and m.title == 1.0 and m.author == 1.0 and m.reason == ''

    def test_a_title_that_is_not_close_is_not_accepted(self):
        m = judge(read_file_title('Duna', 'Frank Herbert'), rec('Herdeiras de Duna', ['Frank Herbert']))
        assert not m.accepted and m.reason == 'title' and m.title < TITLE_MIN
        m = judge(read_file_title('Saga 001', None, 'cbz'), rec('The Fork, the Witch, and the Worm', ['Christopher Paolini']))
        assert not m.accepted and m.reason == 'title'

    def test_an_answer_by_someone_else_is_not_the_book_whatever_its_title(self):
        m = judge(read_file_title('A Nuvem', 'Neal Shusterman'), rec('A nuvem', ['Carlos Poças Falcão']))
        assert not m.accepted and m.reason == 'author' and m.title == 1.0 and m.author == 0.0

    def test_without_an_author_in_the_file_the_title_decides(self):
        m = judge(read_file_title('Dune', None), rec('Dune', ['Anybody']))
        assert m.accepted and m.author == -1.0

    def test_an_answer_with_no_author_is_not_refused_for_it(self):
        m = judge(read_file_title('Dune', 'Frank Herbert'), rec('Dune', []))
        assert m.accepted and m.author == -1.0

    def test_the_subtitle_of_either_side_does_not_matter(self):
        assert judge(read_file_title('Sapiens: Uma breve história da humanidade', 'Yuval Noah Harari'), rec('Sapiens', ['Yuval Noah Harari'])).accepted
        assert judge(read_file_title('Sapiens', None), rec('Sapiens', ['Yuval Noah Harari'], raw={'subtitle': 'A Brief History'})).accepted
        assert judge(read_file_title('Sapiens A Brief History', None), rec('Sapiens', [], raw={'subtitle': 'A Brief History'})).accepted

    def test_the_series_an_answer_is_of_counts_as_a_name_of_it(self):
        m = judge(read_file_title('Berserk 01', None, 'cbz'), rec('Volume One', ['Kentaro Miura'], series='Berserk'))
        assert m.accepted

    def test_the_title_of_a_file_with_the_author_after_a_dash_is_judged_without_it(self):
        m = judge(read_file_title('Scythe - Neal Shusterman', None, 'pdf'), rec('Scythe', ['Neal Shusterman']))
        assert m.accepted and m.author == 1.0
        m = judge(read_file_title('Scythe - Neal Shusterman', None, 'pdf'), rec('Scythe', ['Someone Else']))
        assert not m.accepted and m.reason == 'author'

    def test_a_title_that_has_a_dash_part_which_was_no_author_is_still_found_whole(self):
        m = judge(read_file_title('Harry Potter - Parte Dois', None, 'epub'), rec('Harry Potter - Parte Dois', ['J. K. Rowling']))
        assert m.accepted

    def test_the_number_of_the_file_has_to_be_the_one_of_the_answer(self):
        q = read_file_title('Absolute Batman 006', None, 'cbz')
        assert judge(q, rec('Absolute Batman', [], series_index=6.0)).accepted
        wrong = judge(q, rec('Absolute Batman', [], series_index=1.0))
        assert not wrong.accepted and wrong.reason == 'number'

    def test_the_number_is_not_asked_of_an_answer_that_has_none(self):
        assert judge(read_file_title('Berserk 01', None, 'cbz'), rec('Berserk', ['Kentaro Miura'])).accepted
        assert judge(read_file_title('Berserk 01', None, 'cbz'), rec('Berserk', [], series_index=0)).accepted

    def test_the_right_number_scores_more(self):
        q = read_file_title('Absolute Batman 006', None, 'cbz')
        right, none = judge(q, rec('Absolute Batman', [], series_index=6)), judge(q, rec('Absolute Batman', []))
        assert right.score == none.score + 20

    def test_the_better_known_work_wins_a_tie(self):
        q = read_file_title('Dune', None)
        well, little = judge(q, rec('Dune', [], prior=10)), judge(q, rec('Dune', [], prior=0))
        assert well.score > little.score
        assert judge(q, rec('Dune', [], prior=99)).score == judge(q, rec('Dune', [], prior=10)).score   # capped
        assert judge(q, rec('Dune', [], prior=-5)).score == little.score

    def test_the_score_has_the_title_and_the_author_in_it(self):
        both = judge(read_file_title('Dune', 'Frank Herbert'), rec('Dune', ['Frank Herbert']))
        assert both.score == 160.0
        assert judge(read_file_title('Dune', None), rec('Dune', ['Frank Herbert'])).score == 100.0

    def test_an_answer_with_no_title_nor_credits_is_not_accepted(self):
        assert not judge(read_file_title('Dune', None), MetadataRecord()).accepted

    def test_a_title_exactly_as_close_as_the_limit_is_accepted_and_one_word_less_is_not(self):
        words = [f'palavra{i}' for i in range(20)]
        file_title = ' '.join(words)
        at = rec(' '.join(words[:17] + ['outra1', 'outra2', 'outra3']))   # 17 of 20 words each way: 34/40 = 0.85
        below = rec(' '.join(words[:16] + ['outra1', 'outra2', 'outra3', 'outra4']))
        assert judge(read_file_title(file_title), at).accepted
        assert judge(read_file_title(file_title), at).title == 0.85
        assert not judge(read_file_title(file_title), below).accepted

    def test_an_accepted_reading_wins_over_a_closer_one_that_was_not(self):
        from providers.query import FileQuery
        words = [f'w{i}' for i in range(10)]
        query = FileQuery(title=' '.join(words[:9] + ['x']), author=None, hint='Neal Shusterman', left=' '.join(words), stripped=' '.join(words))
        record = rec(' '.join(words), ['Ann Bee Shusterman'])
        got = judge(query, record)
        # the whole text is 0.9 close and wants no author (accepted); without the dash it is 1.0 close but the author is only 0.4
        assert got.accepted and got.title == 0.9 and got.score == 90.0

    def test_the_limits_are_the_ones_said(self):
        assert TITLE_MIN == 0.85 and AUTHOR_MIN == 0.5

    def test_the_match_is_told_as_a_dictionary(self):
        d = judge(read_file_title('Dune', 'Frank Herbert'), rec('Dune', ['Frank Herbert'])).as_dict()
        assert d == {'score': 160.0, 'title': 1.0, 'author': 1.0, 'accepted': True, 'reason': ''}
