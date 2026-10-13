"""What is made of the lines of an OCR engine in a comic page (DEC-176): balloons, speech, order."""
from comic_regions import cluster, join_text, judge, locator_region, reading_order, speech


def line(text, x, y, w=120, h=24, conf=98.0):
    return {'text': text, 'x': x, 'y': y, 'w': w, 'h': h, 'conf': conf}


PAGE = (1000, 1500)


class TestBalloons:
    def test_close_lines_are_one_balloon_in_the_order_the_engine_gave_them(self):
        regions = cluster([line('NÃO DEVO', 100, 100), line('TER MEDO.', 100, 130)])
        assert len(regions) == 1
        assert regions[0]['text'] == 'NÃO DEVO TER MEDO.'
        assert regions[0]['box'] == [100, 100, 120, 54]
        assert regions[0]['words'] == 2 and regions[0]['conf'] == 98.0

    def test_the_lines_of_two_balloons_that_touch_are_two_regions(self):
        # Side by side, like two adjacent balloons: the gap between their letters is more than the padding of a line.
        regions = cluster([line('VOCÊ', 100, 100), line('CONSEGUE', 100, 130), line('PODER', 330, 100), line('ESPIRITUAL', 330, 130)])
        assert sorted(r['text'] for r in regions) == ['PODER ESPIRITUAL', 'VOCÊ CONSEGUE']

    def test_a_line_with_no_text_is_not_a_region(self):
        assert cluster([line('  ', 10, 10)]) == []

    def test_a_word_broken_at_the_end_of_a_line_is_joined_and_a_hyphen_inside_a_word_is_kept(self):
        assert join_text(['DEIXA', 'EU', 'ME', 'CONCEN-', 'TRAR!!']) == 'DEIXA EU ME CONCENTRAR!!'
        assert join_text(['BASTÃO', 'DA', 'SEIKO-SAN']) == 'BASTÃO DA SEIKO-SAN'
        assert join_text(['A-', 'BC']) == 'A- BC'   # too short to be a broken word


class TestWhatIsSpeech:
    def test_a_balloon_with_words_is_kept(self):
        kept = speech([line('NÃO ME APRESSE,', 100, 100, w=300), line('POR FAVOR!!', 100, 130, w=240)], *PAGE)
        assert [r['text'] for r in kept] == ['NÃO ME APRESSE, POR FAVOR!!']
        assert kept[0]['box'] == [100, 100, 300, 54]

    def test_letters_too_big_are_art_or_onomatopoeia(self):
        # 130 px of letter on a page 1000 wide: 13%, over the 8%.
        assert speech([line('BOOM', 100, 600, w=500, h=130)], *PAGE) == []
        assert speech([line('AND?!', 100, 600, w=300, h=70)], *PAGE) != []

    def test_what_was_read_with_little_confidence_is_not_speech(self):
        assert speech([line('TEXTO DE LIXO', 100, 100, conf=41)], *PAGE) == []
        two = lambda conf: [line('PRIMEIRA LINHA', 100, 100, conf=conf), line('SEGUNDA LINHA', 100, 130, conf=conf)]
        assert speech(two(55), *PAGE) == []      # two lines, so it is not the rule of the single weak word that drops it
        assert len(speech(two(65), *PAGE)) == 1

    def test_a_single_weak_word_and_a_region_with_no_real_word_are_noise(self):
        assert speech([line('ISSO', 100, 100, conf=70)], *PAGE) == []
        assert speech([line('ISSO', 100, 100, conf=95)], *PAGE) != []
        assert speech([line('ay oh', 100, 100, conf=99)], *PAGE) == []
        assert speech([line('1234 !!', 100, 100, conf=99)], *PAGE) == []

    def test_words_of_the_art_that_were_joined_into_a_huge_region_are_dropped(self):
        wide = [line('PALAVRA', 10, 10, w=900, h=40), line('OUTRA', 10, 700, w=900, h=40), line('MAIS', 10, 1300, w=900, h=40)]
        regions = cluster(wide, pad=40)   # a padding so large that everything is one region
        assert len(regions) == 1
        assert judge(regions[0], *PAGE)[0] is False

    def test_the_regions_come_without_an_order(self):
        kept = speech([line('SEGUNDO BALÃO', 600, 100), line('PRIMEIRO BALÃO', 100, 100)], *PAGE)
        assert [r['text'] for r in kept] == ['SEGUNDO BALÃO', 'PRIMEIRO BALÃO']


class TestReadingOrder:
    boxes = [
        {'box': [600, 100, 100, 30]},   # top right
        {'box': [100, 110, 100, 30]},   # top left
        {'box': [300, 700, 100, 30]},   # middle
        {'box': [100, 1300, 100, 30]},  # bottom
    ]

    def test_left_to_right_by_rows(self):
        assert reading_order(self.boxes, 'ltr', 1500) == [1, 0, 2, 3]

    def test_right_to_left_by_rows(self):
        assert reading_order(self.boxes, 'rtl', 1500) == [0, 1, 2, 3]

    def test_a_strip_is_read_from_the_top_down(self):
        strip = [{'box': [700, 3000, 100, 30]}, {'box': [10, 100, 100, 30]}, {'box': [400, 1500, 100, 30]}]
        assert reading_order(strip, 'webtoon', 4000) == [1, 2, 0]

    def test_a_strip_is_not_read_in_rows(self):
        # Two balloons a few pixels apart in height are one row on a page (left to right), and two steps down a strip.
        pair = [{'box': [700, 100, 100, 30]}, {'box': [10, 140, 100, 30]}]
        assert reading_order(pair, 'ltr', 4000) == [1, 0]
        assert reading_order(pair, 'webtoon', 4000) == [0, 1]

    def test_a_direction_nobody_said_reads_as_left_to_right(self):
        assert reading_order(self.boxes, None, 1500) == reading_order(self.boxes, 'ltr', 1500)

    def test_the_regions_are_not_changed(self):
        before = [dict(b) for b in self.boxes]
        reading_order(self.boxes, 'rtl', 1500)
        assert self.boxes == before


class TestTheLocator:
    def test_the_box_is_in_the_unit_square_of_the_page(self):
        assert locator_region([100, 150, 200, 300], 1000, 1500) == {'x': 0.1, 'y': 0.1, 'w': 0.2, 'h': 0.2}

    def test_a_box_that_leaves_the_page_is_cut_to_it(self):
        region = locator_region([900, 1400, 400, 400], 1000, 1500)
        assert region['x'] + region['w'] <= 1.0 and region['y'] + region['h'] <= 1.0
        assert region == {'x': 0.9, 'y': 0.9333, 'w': 0.1, 'h': 0.0667}
