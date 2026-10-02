"""The metrics and the page maker of the OCR evaluation (benchmarks/ocr, #24). They need no engine; the one test that
reads a page with the engine is skipped where it is not installed."""
import itertools
import random
import shutil
import sys
from pathlib import Path

import fitz
import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'benchmarks' / 'ocr'))

import metrics  # noqa: E402
import pages  # noqa: E402
from pages import Look  # noqa: E402


def naive(a, b):
    previous = list(range(len(b) + 1))
    for i, x in enumerate(a, 1):
        row = [i]
        for j, y in enumerate(b, 1):
            row.append(min(previous[j] + 1, row[j - 1] + 1, previous[j - 1] + (x != y)))
        previous = row
    return previous[-1]


class TestLevenshtein:
    def test_it_is_the_edit_distance_of_the_textbook_table(self):
        rng = random.Random(5)
        for _ in range(400):
            a = ''.join(rng.choice('abcé ') for _ in range(rng.randint(0, 14)))
            b = ''.join(rng.choice('abcé ') for _ in range(rng.randint(0, 14)))
            assert metrics.levenshtein(a, b) == naive(a, b), (a, b)

    def test_also_for_texts_longer_than_a_machine_word(self):
        rng = random.Random(9)
        for _ in range(12):
            a = ''.join(rng.choice('abcdef ') for _ in range(rng.randint(70, 200)))
            b = ''.join(rng.choice('abcdef ') for _ in range(rng.randint(70, 200)))
            assert metrics.levenshtein(a, b) == naive(a, b)

    def test_the_known_cases(self):
        assert metrics.levenshtein('', '') == 0
        assert metrics.levenshtein('abc', '') == 3 and metrics.levenshtein('', 'abc') == 3
        assert metrics.levenshtein('kitten', 'sitting') == 3
        assert metrics.levenshtein('igual', 'igual') == 0
        assert metrics.levenshtein('a', 'b') == 1

    def test_a_page_of_text_takes_no_time_at_all(self):
        text = ('uma frase de teste com acentuação e pontuação, ' * 120)
        assert metrics.levenshtein(text, text[:-5] + 'xxxxx') == 5


class TestErrorRates:
    def test_cer_is_the_wrong_characters_over_the_ones_there_should_be(self):
        assert metrics.cer('abcdefghij', 'abcdefghij') == 0
        assert metrics.cer('abcdefghij', 'abcdefghix') == pytest.approx(0.1)
        assert metrics.cer('abcd', '') == 1
        assert metrics.cer('ab', 'abcdef') == 2  # an engine that makes things up can pass 1

    def test_where_the_lines_break_and_how_many_spaces_are_not_measured(self):
        assert metrics.cer('uma  frase\ncortada', 'uma frase cortada') == 0
        assert metrics.cer('uma frase', ' uma\n\n frase ') == 0

    def test_accents_and_case_are_measured_here(self):
        assert metrics.cer('ação', 'acao') > 0 and metrics.cer('Casa', 'casa') > 0
        assert metrics.cer('ação', 'ação') == 0  # composed or decomposed is the same letter
        assert metrics.cer('a̧ão', 'a̧ão') == 0

    def test_nothing_to_read_and_nothing_read(self):
        assert metrics.cer('', '') == 0 and metrics.wer('', '') == 0
        assert metrics.cer('', 'lixo') == 1 and metrics.wer('', 'lixo') == 1

    def test_wer_counts_a_word_with_one_wrong_letter_as_wrong(self):
        assert metrics.wer('a casa é velha', 'a casa e velha') == pytest.approx(0.25)
        assert metrics.wer('a casa é velha', 'a casa é') == pytest.approx(0.25)
        assert metrics.wer('um dois três', 'três dois um') == pytest.approx(2 / 3)
        assert metrics.wer('a b c', 'a b c') == 0

    def test_wer_does_not_confuse_two_different_words_for_one(self):
        # Words are compared whole, however many different ones there are.
        truth = ' '.join(f'p{i}' for i in range(300))
        got = ' '.join(f'q{i}' for i in range(300))
        assert metrics.wer(truth, got) == 1


class TestSearchRecall:
    def test_the_share_of_the_distinct_words_that_a_page_can_be_found_by(self):
        truth = 'a catedral antiga ficava no alto da colina'
        assert metrics.search_recall(truth, 'a catedral antiga ficava no alto da colina') == 1
        assert metrics.search_recall(truth, 'catedral antiga') == pytest.approx(2 / 5)  # catedral antiga ficava alto colina
        assert metrics.search_recall(truth, '') == 0

    def test_a_word_is_counted_once_and_short_words_do_not_count(self):
        truth = 'casa casa casa de do com'
        assert metrics.search_recall(truth, 'casa') == 1  # only "casa" and "com" is shorter than four letters
        assert metrics.search_recall('de do em', 'x') is None

    def test_without_accents_the_search_finds_it_either_way_and_with_them_it_does_not(self):
        truth = 'ação decisão coração'
        assert metrics.search_recall(truth, 'acao decisao coracao') == 1
        assert metrics.search_recall(truth, 'acao decisao coracao', accents=True) == 0
        assert metrics.search_recall(truth, 'AÇÃO decisão', accents=True) == pytest.approx(2 / 3)
        assert metrics.search_recall('Casa', 'casa') == 1

    def test_words_are_the_letters_between_the_spaces_and_the_marks(self):
        assert metrics.words("n'um livro, aberto-à hora; 1882.") == ['n', 'um', 'livro', 'aberto', 'à', 'hora', '1882']


class TestReadingOrder:
    LEFT = ' '.join(f'esquerda{n}' for n in range(60))
    RIGHT = ' '.join(f'direita{n}' for n in range(60))

    def test_a_page_read_in_order_scores_one(self):
        assert metrics.reading_order(self.LEFT + ' ' + self.RIGHT, self.LEFT + ' ' + self.RIGHT) == (1, 1)

    def test_two_columns_read_across_score_about_half(self):
        left, right = self.LEFT.split(), self.RIGHT.split()
        across = ' '.join(w for pair in zip(left, right) for w in pair)
        coverage, order = metrics.reading_order(self.LEFT + ' ' + self.RIGHT, across)
        # The runs of three words are broken by the other column, so few are found.
        assert coverage is not None and coverage < 0.2

    def test_columns_in_the_wrong_order_score_about_half(self):
        coverage, order = metrics.reading_order(self.LEFT + ' ' + self.RIGHT, self.RIGHT + ' ' + self.LEFT)
        assert coverage == 1
        # Each half is in order inside itself: the pairs inside a column are right, the ones across are wrong.
        assert 0.4 < order < 0.6

    def test_a_page_read_backwards_scores_zero(self):
        text = ' '.join(f'palavra{n}' for n in range(120))
        backwards = ' '.join(reversed(text.split()))
        # Reversed words break every run of three: nothing is found, and nothing is said of the order.
        assert metrics.reading_order(text, backwards) == (0.0, None)

    def test_blocks_swapped_are_found_and_wrong(self):
        a = ' '.join(f'um{n}' for n in range(45))
        b = ' '.join(f'dois{n}' for n in range(45))
        c = ' '.join(f'tres{n}' for n in range(45))
        coverage, order = metrics.reading_order(f'{a} {b} {c}', f'{c} {b} {a}')
        assert coverage == 1
        assert order < 0.5

    def test_accents_and_case_do_not_hide_where_something_is(self):
        text = ' '.join(f'Ação{n}' for n in range(60))
        assert metrics.reading_order(text, text.lower().replace('ç', 'c').replace('ã', 'a')) == (1, 1)

    def test_a_run_that_occurs_twice_in_the_truth_is_not_an_anchor(self):
        repeated = ' '.join(['mesma frase aqui'] * 20)
        assert metrics.reading_order(repeated, repeated) == (None, None)

    def test_a_run_that_occurs_twice_is_no_anchor_either(self):
        twice = 'mesma frase aqui ' + ' '.join(f'a{n}' for n in range(40)) + ' mesma frase aqui ' + ' '.join(f'b{n}' for n in range(40))
        coverage, order = metrics.reading_order(twice, twice)
        assert coverage == 1 and order == 1  # only the runs that are found in one place are asked about
        # The runs of "mesma frase aqui" would be found, and wrongly, in a text that has them once.
        assert metrics.reading_order('mesma frase aqui ' + ' '.join(f'c{n}' for n in range(40)) + ' mesma frase aqui', 'mesma frase aqui') == (0.0, None)

    def test_a_text_the_engine_repeated_is_judged_by_where_it_first_says_it(self):
        a = ' '.join(f'um{n}' for n in range(45))
        b = ' '.join(f'dois{n}' for n in range(45))
        assert metrics.reading_order(f'{a} {b}', f'{a} {b} {a}') == (1, 1)

    def test_too_little_to_say(self):
        assert metrics.reading_order('uma duas', 'uma duas') == (None, None)
        text = ' '.join(f'unica{n}' for n in range(40))
        coverage, order = metrics.reading_order(text, 'nada a ver com isto')
        assert coverage == 0 and order is None
        # One found is not a pair.
        only = ' '.join(f'unica{n}' for n in range(3))
        assert metrics.reading_order(text, only) == (pytest.approx(1 / 3), None)


class TestMean:
    def test_it_leaves_out_what_could_not_be_measured(self):
        assert metrics.mean([1, None, 3]) == 2
        assert metrics.mean([None]) is None and metrics.mean([]) is None


# --- the page maker ---

BOOK = ('*** START OF THE PROJECT GUTENBERG EBOOK TESTE ***\n\n'
        + '\n\n'.join(' '.join(f'palavra{p}x{n}' for n in range(40)) + '.' for p in range(120))
        + '\n\nTITULO CURTO\n\nversos\ncurtos\n\n*** END OF THE PROJECT GUTENBERG EBOOK TESTE ***\n\nlicença que não é do livro\n')


@pytest.fixture(scope='module')
def paras():
    return pages.paragraphs(pages.clean_source(BOOK))


class TestSource:
    def test_the_licence_around_the_book_is_left_out(self):
        text = pages.clean_source(BOOK)
        assert 'START OF' not in text and 'END OF' not in text and 'licença' not in text and 'palavra0x0' in text

    def test_a_text_without_the_markers_is_kept_whole(self):
        assert pages.clean_source('so texto\r\nlinha') == 'so texto\nlinha'

    def test_only_paragraphs_that_are_prose_are_kept(self, paras):
        assert len(paras) == 120
        assert all('\n' not in p and len(p) >= 160 for p in paras)
        assert 'TITULO CURTO' not in paras

    def test_what_the_page_fonts_cannot_draw_is_replaced_so_the_truth_is_what_is_drawn(self):
        font = fitz.Font('tiro')
        assert pages.drawable('“olá” — não… ’é’', font) == '"olá" - não... \'é\''
        assert pages.drawable('a​b c', font) == 'ab c'
        assert pages.drawable('texto 中 com han', font) == 'texto com han'

    def test_an_excerpt_is_the_same_for_the_same_seed_and_long_enough(self, paras):
        a = pages.excerpt(paras, random.Random(1), 300)
        assert a == pages.excerpt(paras, random.Random(1), 300)
        assert a != pages.excerpt(paras, random.Random(2), 300)
        assert len(a.split()) >= 300

    def test_a_mixed_text_changes_language_by_the_sentence(self, paras):
        other = [p.replace('palavra', 'word') for p in paras]
        text = pages.alternate(paras, other, random.Random(3), 200)
        assert 'palavra' in text and 'word' in text and len(text.split()) >= 200


def render(look, paras, seed=1, **kw):
    rng = random.Random(seed)
    return pages.draw(look, pages.excerpt(paras, rng), rng, **kw)


class TestDraw:
    def test_the_truth_is_the_beginning_of_the_text_and_fills_the_page(self, paras):
        rng = random.Random(1)
        text = pages.excerpt(paras, rng)
        data, truth = pages.draw(Look('x'), text, random.Random(1))
        assert text.startswith(truth) and 200 < len(truth.split()) < len(text.split())
        assert data[:8] == b'\x89PNG\r\n\x1a\n'

    def test_with_two_columns_the_first_is_read_before_the_second_and_all_of_it_is_drawn(self, paras):
        text = pages.excerpt(paras, random.Random(1))
        _, one = pages.draw(Look('x'), text, random.Random(1))
        _, two = pages.draw(Look('x', columns=2), text, random.Random(1))
        assert text.startswith(two) and len(two) > 0.7 * len(one)

    def test_a_page_that_will_be_turned_holds_less_so_that_it_still_fits_the_sheet(self, paras):
        _, upright = render(Look('x'), paras)
        _, sideways = render(Look('x', rotate=90), paras)
        assert len(sideways) < len(upright)

    def test_the_running_head_and_the_number_are_part_of_the_truth(self, paras):
        _, truth = render(Look('x', header=True), paras, title='Dom Casmurro', number=17)
        assert truth.startswith('DOM CASMURRO palavra') and truth.endswith(' 17')

    def test_a_page_is_drawn_the_same_every_time(self, paras):
        assert render(Look('x', speckles=50), paras) == render(Look('x', speckles=50), paras)

    @pytest.mark.parametrize('change', [
        {'speckles': 80}, {'skew': 2.0}, {'rotate': 180}, {'paper': 0.8}, {'dpi': 100}, {'font': 'sans'}, {'size': 9.0}, {'ink': 0.5},
    ])
    def test_every_look_changes_the_picture(self, paras, change):
        assert render(Look('x'), paras)[0] != render(Look('x', **change), paras)[0]

    def test_a_lower_resolution_is_a_smaller_picture_and_jpeg_is_jpeg(self, paras):
        data = render(Look('x', dpi=100), paras)[0]
        assert fitz.Pixmap(data).width == 850 and fitz.Pixmap(render(Look('x', dpi=200), paras)[0]).width == 1700
        jpeg = render(Look('x', jpeg=30), paras)[0]
        assert jpeg[:2] == b'\xff\xd8'

    def test_dirt_is_drawn_dark(self, paras):
        clean = fitz.Pixmap(render(Look('x', size=1.0), paras)[0])
        dirty = fitz.Pixmap(render(Look('x', size=1.0, speckles=300), paras)[0])
        assert sum(dirty.samples) < sum(clean.samples)

    def test_the_text_is_the_same_whatever_the_look_when_it_fits_the_same(self, paras):
        _, a = render(Look('x'), paras)
        _, b = render(Look('x', speckles=100, jpeg=50), paras)
        assert a == b


def ink_at_the_edges(data, margin=12):
    """Whether any dark pixel is in the outermost `margin` pixels of the picture."""
    pix = fitz.Pixmap(data)
    w, h, rows = pix.width, pix.height, pix.samples
    top = rows[:margin * w]
    bottom = rows[(h - margin) * w:]
    sides = b''.join(rows[y * w:y * w + margin] + rows[y * w + w - margin:(y + 1) * w] for y in range(h))
    return min(top + bottom + sides) < 200


class TestFitsTheSheet:
    @pytest.mark.parametrize('look', [Look('x', rotate=90), Look('x', rotate=270, columns=2), Look('x', rotate=180), Look('x', skew=3.0),
                                      Look('x', rotate=90, header=True)])
    def test_a_turned_or_crooked_page_keeps_its_text_inside_the_sheet(self, paras, look):
        assert not ink_at_the_edges(render(look.but(dpi=100), paras)[0])

    def test_the_head_and_the_number_of_a_turned_page_are_on_the_sheet(self, paras):
        def bands(look):
            pix = fitz.Pixmap(render(look.but(dpi=100), paras)[0])
            left, right = int(55 / 72 * 100), int(557 / 72 * 100)
            dark = [0, 0]
            for y in range(pix.height):
                row = pix.samples[y * pix.width:(y + 1) * pix.width]
                dark[0] += sum(1 for v in row[:left] if v < 128)
                dark[1] += sum(1 for v in row[right:] if v < 128)
            return dark
        assert bands(Look('x', rotate=90)) == [0, 0]
        head_side, number_side = bands(Look('x', rotate=90, header=True))
        assert head_side > 0 and number_side > 0

    def test_two_columns_have_white_between_them_and_one_column_does_not(self, paras):
        def gutter(look):
            pix = fitz.Pixmap(render(look.but(dpi=100), paras)[0])
            x0, x1 = int(300 / 72 * 100), int(312 / 72 * 100)
            return min(pix.samples[y * pix.width + x] for y in range(int(100 / 72 * 100), int(700 / 72 * 100)) for x in range(x0, x1))
        assert gutter(Look('x', columns=2)) == 255
        assert gutter(Look('x')) < 200


class TestScan:
    def test_a_scan_is_pictures_and_no_text_layer(self, paras):
        pictures = [render(Look('x'), paras, seed=s)[0] for s in (1, 2, 3)]
        doc = fitz.open(stream=pages.scan(pictures), filetype='pdf')
        assert len(doc) == 3
        for page in doc:
            assert page.get_text().strip() == '' and len(page.get_images()) == 1

    def test_it_is_scanned_at_the_resolution_the_look_says(self, paras):
        import ocr
        for dpi in (150, 300):
            doc = fitz.open(stream=pages.scan([render(Look('x', dpi=dpi), paras)[0]]), filetype='pdf')
            assert abs(ocr.effective_dpi(doc[0]) - dpi) <= 1


@pytest.mark.skipif(shutil.which('tesseract') is None, reason='the OCR engine is not installed')
class TestWithTheEngine:
    def test_a_clean_page_is_read_by_the_engine_almost_exactly(self, paras):
        import ocr
        engine = ocr.Tesseract()
        if 'eng' not in engine.languages():
            pytest.skip('the engine has no English')
        text = ' '.join(f'The old cathedral stood on top of the hill number {n} and the bell rang.' for n in range(60))
        data, truth = pages.draw(Look('x'), text, random.Random(1))
        doc = fitz.open(stream=pages.scan([data]), filetype='pdf')
        image, dpi = ocr.render(doc[0])
        got = engine.recognize(image, 'eng', dpi)
        assert metrics.cer(truth, got) < 0.03


# --- the runner ---

import benchmark  # noqa: E402


class FakeEngine:
    name = 'tesseract'

    def __init__(self, languages=('eng', 'por', 'spa', 'fra', 'ita', 'osd'), output=None):
        self._languages, self.output, self.calls = list(languages), output, []

    def installed(self):
        return True

    def version(self):
        return '5.5.0'

    def languages(self):
        return [l for l in self._languages if l != 'osd']

    def recognize(self, image, language, dpi):
        self.calls.append((language, dpi, len(image)))
        return self.output if self.output is not None else 'texto lido pelo motor falso'


class TestRunner:
    def test_the_plan_asks_for_what_the_report_needs(self):
        plan = benchmark.default_plan(None)
        names = {(lang, look.name) for lang, look, _ in plan}
        assert ('pt', 'limpa') in names and ('pt', 'duas colunas') in names and ('misto', 'suja') in names and ('fr', 'limpa') in names
        by = {(lang, look.name): configs for lang, look, configs in plan}
        assert by[('pt', 'duas colunas')] == ['por', 'por+eng']
        assert by[('fr', 'limpa')] == ['fra', 'por+eng']
        assert by[('en', 'limpa')] == ['por', 'por+eng', 'eng']
        assert {l.name for l in benchmark.LOOKS} >= {'girada 90°', 'inclinada 3°', '100 dpi', '300 dpi', 'JPEG 25', 'manchas'}

    def test_a_part_of_it_can_be_asked_for_by_look_or_by_language(self):
        assert {look.name for _, look, _ in benchmark.default_plan({'100 dpi'})} == {'100 dpi'}
        assert {lang for lang, _, _ in benchmark.default_plan({'fr'})} == {'fr'}

    def test_the_seed_of_a_page_is_the_same_every_time_and_not_the_same_for_every_page(self):
        assert benchmark.seed_of(7, 'pt', 0) == benchmark.seed_of(7, 'pt', 0)
        assert len({benchmark.seed_of(7, 'pt', i) for i in range(20)}) == 20
        assert benchmark.seed_of(7, 'pt', 0) != benchmark.seed_of(8, 'pt', 0)

    def test_the_same_text_is_drawn_for_every_look(self, paras):
        import fitz as _fitz  # noqa: F401
        books = {'pt': paras, 'en': paras}
        a = benchmark.make_pages(books, 'pt', Look('a'), 2, 7)
        b = benchmark.make_pages(books, 'pt', Look('b', speckles=50), 2, 7)
        assert [t for _, t, _ in a] == [t for _, t, _ in b]
        assert a[0][1] != a[1][1]

    def test_a_page_is_read_with_each_language_asked_for_and_once_for_each(self, paras):
        import ocr
        engine = FakeEngine()
        books = {'pt': paras}
        rows = benchmark.run_matrix(engine, ocr, books, [('pt', Look('x'), ['por', 'por+eng']), ('pt', Look('x'), ['por+eng', 'eng'])],
                                    2, 7, log=lambda m: None)
        assert [c[0] for c in engine.calls] == ['por', 'por', 'por+eng', 'por+eng', 'eng', 'eng']  # por+eng not read twice
        assert len(rows) == 6 and {r['config'] for r in rows} == {'por', 'por+eng', 'eng'}
        assert all(r['text'] == 'pt' and r['look'] == 'x' and 0 <= r['cer'] and r['dpi'] == 200 for r in rows)
        assert all(r['read'] == 'texto lido pelo motor falso' for r in rows)

    def test_the_picture_is_told_to_the_engine_at_the_resolution_it_was_scanned(self, paras):
        import ocr
        engine = FakeEngine()
        benchmark.run_matrix(engine, ocr, {'pt': paras}, [('pt', Look('x', dpi=150), ['por'])], 1, 7, log=lambda m: None)
        assert abs(engine.calls[0][1] - 150) <= 1

    def test_the_rows_are_averaged_by_book_look_and_language(self):
        rows = [{'text': 'pt', 'look': 'a', 'config': 'por', 'cer': 0.1, 'wer': 0.2, 'search': 1.0, 'search_accents': 0.9,
                 'order': 1.0, 'coverage': 1.0, 'seconds': 1.0},
                {'text': 'pt', 'look': 'a', 'config': 'por', 'cer': 0.3, 'wer': 0.4, 'search': 0.8, 'search_accents': 0.7,
                 'order': None, 'coverage': 0.5, 'seconds': 3.0},
                {'text': 'pt', 'look': 'a', 'config': 'eng', 'cer': 0.9, 'wer': 1.0, 'search': 0.1, 'search_accents': 0.1,
                 'order': 0.5, 'coverage': 0.5, 'seconds': 2.0}]
        agg = benchmark.aggregate(rows)
        assert agg[('pt', 'a', 'por')]['cer'] == pytest.approx(0.2) and agg[('pt', 'a', 'por')]['pages'] == 2
        assert agg[('pt', 'a', 'por')]['order'] == 1.0  # the page with nothing to say is left out
        assert agg[('pt', 'a', 'por')]['seconds'] == 2.0 and agg[('pt', 'a', 'eng')]['cer'] == 0.9

    def test_percentages(self):
        assert benchmark.pct(0.1234) == '12.3' and benchmark.pct(1, 0) == '100' and benchmark.pct(None) == '—'

    def test_pages_of_real_scans_are_the_images_that_have_their_text_beside_them(self, tmp_path, paras):
        import ocr
        (tmp_path / 'a.png').write_bytes(render(Look('x'), paras)[0])
        (tmp_path / 'a.txt').write_text('o texto da página a', encoding='utf-8')
        (tmp_path / 'b.png').write_bytes(b'x')  # no text beside it
        (tmp_path / 'c.txt').write_text('sem imagem', encoding='utf-8')
        (tmp_path / 'd.pdf').write_bytes(pages.scan([render(Look('x', dpi=150), paras)[0]]))
        (tmp_path / 'd.txt').write_text('o texto da página d', encoding='utf-8')
        (tmp_path / 'notas.md').write_text('x')
        assert [name for name, *_ in benchmark.load_real(tmp_path)] == ['a', 'd']
        engine = FakeEngine(output='o texto da página a')
        rows = benchmark.run_real(engine, ocr, tmp_path, ['por', 'eng'], 300)
        assert [(r['look'], r['config'], r['text']) for r in rows] == [('a', 'por', 'real'), ('a', 'eng', 'real'), ('d', 'por', 'real'), ('d', 'eng', 'real')]
        assert [c[1] for c in engine.calls][:2] == [300, 300] and abs(engine.calls[2][1] - 150) <= 1  # told, and as the PDF says
        assert rows[0]['cer'] == 0 and rows[2]['cer'] > 0

    def test_the_detector_is_asked_about_clean_text_of_each_length(self, paras):
        books = {k: paras for k in benchmark.SOURCES}
        out = benchmark.detect_clean(books, 7)
        assert len(out) == len(benchmark.SOURCES) * len(benchmark.DETECT_LENGTHS)
        assert all(r['right'] + r['wrong'] + r['unsure'] == benchmark.DETECT_SAMPLES for r in out)

    def test_the_language_of_what_was_read_is_told_from_the_pages_read_with_the_default_only(self):
        from textindex.language import detect
        english = 'The old cathedral stood on top of the hill, and the people of the town climbed up every Sunday to hear the bell. ' * 3
        rows = [{'text': 'en', 'look': 'limpa', 'config': 'por+eng', 'read': english},
                {'text': 'en', 'look': 'limpa', 'config': 'por', 'read': 'a catedral antiga ficava no alto da colina e os moradores subiam'}]
        out = benchmark.detect_read(rows)
        assert out == [{'language': 'en', 'look': 'limpa', 'detected': detect(english)}] and out[0]['detected'] == 'en'

    def test_a_run_writes_the_report_and_the_results(self, tmp_path, paras, monkeypatch, capsys):
        import ocr
        books = {k: paras for k in benchmark.SOURCES}
        monkeypatch.setattr(benchmark, 'load_books', lambda cache: books)
        engine = FakeEngine(output='A catedral antiga ficava no alto da colina e os moradores subiam todos os domingos')
        monkeypatch.setattr(ocr, 'Tesseract', lambda: engine)
        out = tmp_path / 'out'
        assert benchmark.main(['--cache', str(tmp_path), '--out', str(out), '--pages', '1', '--only', 'limpa,duas colunas']) == 0
        text = (out / 'report.md').read_text(encoding='utf-8')
        assert text.startswith('# Avaliação do OCR') and 'tesseract 5.5.0' in text and 'Nenhum limiar foi aprovado' in text
        for heading in ('### Como a página é', '### O idioma dito ao motor', '### Outros idiomas', '### Descobrir o idioma pelo texto limpo',
                        '### Descobrir o idioma pelo texto que o OCR leu'):
            assert heading in text, heading
        assert '| duas colunas | `por` |' in text and '| es / limpa | `spa` |' in text
        results = (out / 'results.json').read_text(encoding='utf-8')
        assert '"engine": "5.5.0"' in results and '"read"' not in results

    def test_without_the_engine_it_says_where_to_run(self, monkeypatch):
        import ocr

        class Missing(FakeEngine):
            def installed(self):
                return False

        monkeypatch.setattr(ocr, 'Tesseract', lambda: Missing())
        with pytest.raises(SystemExit, match='make benchmark-ocr'):
            benchmark.main(['--pages', '1'])

    def test_what_the_engine_lacks_is_left_out_and_said(self, tmp_path, paras, monkeypatch, capsys):
        import ocr
        books = {k: paras for k in benchmark.SOURCES}
        monkeypatch.setattr(benchmark, 'load_books', lambda cache: books)
        engine = FakeEngine(languages=('eng', 'por'))
        monkeypatch.setattr(ocr, 'Tesseract', lambda: engine)
        benchmark.main(['--cache', str(tmp_path), '--pages', '1', '--only', 'limpa', '--no-detect'])
        assert 'o motor não tem' in capsys.readouterr().err
        assert {c[0] for c in engine.calls} <= {'por', 'por+eng', 'eng'}
