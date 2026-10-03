"""The reading of a dictionary (#109): what a word reads as, what of an entry is kept, and the rows it makes. Real
entries from the Portuguese Wiktionary (tests/fixtures/dictionary-sample.jsonl.gz) are the data."""
import gzip
import json
import os

import pytest

import dictionary
from dictionary import normalize, rows_of, trim_entry, LANGS

HERE = os.path.dirname(__file__)
SAMPLE = os.path.join(HERE, 'fixtures', 'dictionary-sample.jsonl.gz')
SHARED = os.path.join(HERE, '..', '..', 'backend', 'internal', 'dictionary', 'testdata', 'normalize.json')


def entries():
    with gzip.open(SAMPLE, 'rt', encoding='utf-8') as f:
        for line in f:
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                continue


def find(lang, word, pos=None):
    for raw in entries():
        if raw.get('lang_code') == lang and raw.get('word') == word and (pos is None or raw.get('pos') == pos):
            return raw
    raise AssertionError(f'{lang}:{word} is not in the sample')


@pytest.mark.parametrize('case', json.load(open(SHARED, encoding='utf-8'))['cases'], ids=lambda c: repr(c['in']))
def test_normalize_follows_the_shared_cases(case):
    assert normalize(case['in']) == case['out']


class TestNormalize:
    def test_an_accent_on_a_latin_letter_goes_and_a_mark_that_makes_a_letter_stays(self):
        assert normalize('ação') == 'acao'
        assert normalize('か') != normalize('が')
        assert normalize('は') != normalize('ば')

    def test_is_the_same_for_what_is_written_and_what_is_selected(self):
        assert normalize('Correram') == normalize('«correram».')

    def test_is_idempotent(self):
        for text in ['Ação', 'Straße', 'l’amour', '走る', 'Ｈａｕｓ']:
            assert normalize(normalize(text)) == normalize(text)


class TestTrimEntry:
    def test_keeps_the_senses_of_a_lemma_and_its_forms(self):
        entry = trim_entry(find('pt', 'correr'))
        assert entry['lang'] == 'pt' and entry['word'] == 'correr' and entry['pos'] == 'verb'
        assert entry['data']['senses'][0]['glosses'] == ['mover-se com rapidez']
        forms = {f['form'] for f in entry['data']['forms']}
        assert {'correndo', 'corrido', 'corro', 'corres'} <= forms
        assert any(t['lang'] == 'ja' and t['word'] == '走る' for t in entry['data']['translations'])

    def test_keeps_the_lemma_an_inflected_form_points_to(self):
        entry = trim_entry(find('pt', 'correram'))
        assert entry['data']['senses'][0]['form_of'] == [{'word': 'correr'}]
        assert 'terceira pessoa do plural' in entry['data']['senses'][0]['glosses'][0]
        assert 'forms' not in entry['data']

    def test_keeps_an_example_with_its_translation_and_not_what_is_not_needed(self):
        entry = trim_entry(find('en', 'book', 'noun'))
        sense = entry['data']['senses'][0]
        assert sense['glosses'] == ['livro']
        assert sense['examples'][0]['text'].startswith('My life is an open book')
        assert sense['examples'][0]['translation'] == 'Minha vida é um livro aberto. (Não tenho segredos.)'
        for noise in ('categories', 'etymology_texts', 'etymology_links', 'anagrams', 'derived', 'head_templates'):
            assert noise not in entry['data']

    def test_a_noun_and_a_verb_form_with_the_same_spelling_are_two_entries(self):
        livros = [trim_entry(r) for r in entries() if r.get('lang_code') == 'pt' and r.get('word') == 'livro']
        assert sorted(e['pos'] for e in livros) == ['noun', 'verb']
        assert next(e for e in livros if e['pos'] == 'verb')['data']['senses'][0]['form_of'] == [{'word': 'livrar'}]

    @pytest.mark.parametrize('lang,word', [('ja', '走る'), ('zh', '跑'), ('zh', '书'), ('zh', '書'), ('de', 'Haus'), ('fr', 'maison'), ('es', 'casa'), ('it', 'casa'), ('en', 'run')])
    def test_keeps_the_words_of_the_other_languages_with_their_translation(self, lang, word):
        entry = trim_entry(find(lang, word))
        assert entry['lang'] == lang and entry['data']['senses'][0]['glosses']

    def test_keeps_the_pronunciation_of_a_chinese_word(self):
        assert trim_entry(find('zh', '爱', 'verb'))['data']['ipa'] == '[ ai˥˩ ]'

    def test_lets_go_a_language_that_is_not_kept(self):
        assert trim_entry(find('gl', 'casa')) is None
        assert trim_entry({'word': '愛', 'lang_code': 'yue', 'senses': [{'glosses': ['amor']}]}) is None
        assert trim_entry(find('pt', 'casa'), headwords=('en',)) is None

    def test_lets_go_an_entry_with_no_sense_and_what_is_not_an_entry(self):
        assert trim_entry({'word': 'vazio', 'lang_code': 'pt', 'senses': [{'tags': ['x']}]}) is None
        assert trim_entry({'word': 'x', 'lang_code': 'pt', 'senses': []}) is None
        for junk in (None, 'texto', 42, [], {}, {'lang_code': 'pt'}, {'word': '', 'lang_code': 'pt', 'senses': [{'glosses': ['a']}]},
                     {'word': ' ', 'lang_code': 'pt', 'senses': [{'glosses': ['a']}]}, {'word': 3, 'lang_code': 'pt', 'senses': [{'glosses': ['a']}]}):
            assert trim_entry(junk) is None

    def test_a_form_is_a_word_not_a_dash_or_a_note(self):
        raw = {'word': 'x', 'lang_code': 'pt', 'senses': [{'glosses': ['a']}], 'forms': [
            {'form': '–'}, {'form': '-'}, {'form': 'a b c d e'}, {'form': 'n' * 41}, {'form': ''}, {'form': 5},
            {'form': 'bom', 'tags': ['masculine']}, {'form': 'bom', 'tags': ['masculine']}, {'form': 'boa', 'tags': ['feminine']}]}
        forms = trim_entry(raw)['data']['forms']
        assert forms == [{'form': 'bom', 'tags': ['masculine']}, {'form': 'boa', 'tags': ['feminine']}]

    def test_the_junk_forms_of_the_real_sample_are_dropped(self):
        for raw in entries():
            if raw.get('lang_code') == 'pt' and any(f.get('form') == '–' for f in raw.get('forms') or []):
                assert all(f['form'] != '–' for f in (trim_entry(raw) or {'data': {}})['data'].get('forms', []))

    def test_cuts_what_is_too_long_or_too_many(self):
        raw = {'word': 'x', 'lang_code': 'pt', 'pos': 'noun', 'senses': [
            {'glosses': ['g' * 2000], 'examples': [{'text': 'e' * 2000, 'translation': 't' * 2000}] + [{'text': f'ex{i}'} for i in range(5)], 'tags': [f't{i}' for i in range(20)]}
        ] + [{'glosses': [f'sentido {i}']} for i in range(100)],
            'forms': [{'form': f'forma{i}'} for i in range(200)],
            'translations': [{'lang_code': 'en', 'word': f'w{i}'} for i in range(100)]}
        data = trim_entry(raw)['data']
        assert len(data['senses']) == dictionary.MAX_SENSES
        first = data['senses'][0]
        assert len(first['glosses'][0]) == dictionary.MAX_GLOSS and first['glosses'][0].endswith('…')
        assert len(first['examples']) == 2
        assert len(first['examples'][0]['text']) == dictionary.MAX_EXAMPLE and len(first['examples'][0]['translation']) == dictionary.MAX_EXAMPLE
        assert len(first['tags']) == 8
        assert len(data['forms']) == dictionary.MAX_FORMS
        assert len(data['translations']) == dictionary.MAX_TRANSLATIONS_PER_LANG  # all of them in one language
        assert len(trim_entry(raw)['links']) == dictionary.MAX_LINKS_PER_LANG

    def test_keeps_the_first_glosses_and_lemmas_of_a_sense_and_no_more(self):
        raw = {'word': 'x', 'lang_code': 'pt', 'senses': [{'glosses': [f'g{i}' for i in range(10)], 'form_of': [{'word': f'l{i}'} for i in range(6)]}]}
        sense = trim_entry(raw)['data']['senses'][0]
        assert sense['glosses'] == [f'g{i}' for i in range(6)]
        assert sense['form_of'] == [{'word': f'l{i}'} for i in range(4)]

    def test_a_sense_that_only_points_to_its_lemma_is_kept(self):
        for key in ('form_of', 'alt_of'):
            sense = trim_entry({'word': 'x', 'lang_code': 'pt', 'senses': [{key: [{'word': 'y'}]}]})['data']['senses'][0]
            assert sense == {'glosses': [], key: [{'word': 'y'}]}

    def test_an_example_that_is_not_text_is_let_go(self):
        raw = {'word': 'x', 'lang_code': 'pt', 'senses': [{'glosses': ['a'], 'examples': [{'text': 5}, {'text': '  '}, 'solto', {'text': 'bom'}]}]}
        assert trim_entry(raw)['data']['senses'][0]['examples'] == [{'text': 'bom'}]
        raw['senses'][0]['examples'] = [{'text': 5}, 'x']
        assert 'examples' not in trim_entry(raw)['data']['senses'][0]

    def test_keeps_the_first_pronunciation_there_is(self):
        raw = {'word': 'x', 'lang_code': 'pt', 'senses': [{'glosses': ['a']}], 'sounds': [{'tags': ['x']}, {'ipa': '/um/'}, {'ipa': '/dois/'}, {'zh_pron': '[ tres ]'}]}
        assert trim_entry(raw)['data']['ipa'] == '/um/'
        assert 'ipa' not in trim_entry({'word': 'x', 'lang_code': 'pt', 'senses': [{'glosses': ['a']}], 'sounds': [{'ipa': ' '}, 'x']})['data']

    def test_keeps_only_the_translations_into_the_languages_kept(self):
        raw = {'word': 'x', 'lang_code': 'pt', 'senses': [{'glosses': ['a']}], 'translations': [
            {'lang_code': 'ay', 'word': 'jala'}, {'lang_code': 'de', 'word': 'rennen', 'sense': 'mover-se'}, {'lang_code': 'ja', 'word': ' '}, {'lang_code': 'fr'}, 'x']}
        assert trim_entry(raw)['data']['translations'] == [{'lang': 'de', 'word': 'rennen', 'sense': 'mover-se'}]

    def test_keeps_the_variant_spelling_an_entry_points_to(self):
        raw = {'word': 'egypto', 'lang_code': 'pt', 'senses': [{'glosses': ['grafia antiga'], 'alt_of': [{'word': 'Egito'}, {'x': 1}]}]}
        assert trim_entry(raw)['data']['senses'][0]['alt_of'] == [{'word': 'Egito'}]

    def test_the_kept_languages_are_the_ones_of_the_library(self):
        assert set(LANGS) == {'pt', 'en', 'es', 'fr', 'de', 'it', 'ja', 'zh'}


class TestRows:
    def test_an_entry_is_looked_up_by_what_its_word_reads_as(self):
        entry_row, _, _ = rows_of('wikt-pt', trim_entry(find('pt', 'ação', 'noun')))
        assert entry_row[1] == 'pt' and entry_row[2] == 'ação' and entry_row[3] == 'acao'

    def test_the_forms_of_a_lemma_point_back_to_it(self):
        _, forms, _ = rows_of('wikt-pt', trim_entry(find('pt', 'correr')))
        by_norm = {f[2]: f for f in forms}
        assert by_norm['corro'][4] == 'correr' and by_norm['corro'][1] == 'pt' and 'first-person' in by_norm['corro'][6]

    def test_a_form_spelled_like_its_lemma_is_not_listed(self):
        raw = {'word': 'casa', 'lang_code': 'pt', 'senses': [{'glosses': ['a']}], 'forms': [{'form': 'casa'}, {'form': 'Casa'}, {'form': 'casas'}]}
        _, forms, _ = rows_of('p', trim_entry(raw))
        assert [f[3] for f in forms] == ['casas']

    def test_a_translation_is_turned_around_to_find_the_lemma(self):
        _, _, links = rows_of('wikt-pt', trim_entry(find('pt', 'correr')))
        ja = [l for l in links if l[1] == 'ja' and l[3] == '走る']
        assert ja and ja[0][4:7] == ('pt', 'correr', 'verb') and ja[0][2] == '走る'

    def test_a_translation_that_reads_as_nothing_makes_no_link(self):
        raw = {'word': 'x', 'lang_code': 'pt', 'senses': [{'glosses': ['a']}], 'translations': [{'lang_code': 'en', 'word': '...'}, {'lang_code': 'en', 'word': 'ok'}]}
        _, _, links = rows_of('p', trim_entry(raw))
        assert [l[3] for l in links] == ['ok']

    def test_an_entry_whose_word_reads_as_nothing_makes_no_rows(self):
        entry = {'lang': 'pt', 'word': '...', 'pos': '', 'data': {'senses': [{'glosses': ['a']}]}}
        assert rows_of('p', entry) == (None, [], [])

    def test_the_data_of_an_entry_is_stored_as_json_with_its_accents(self):
        row, _, _ = rows_of('p', trim_entry(find('pt', 'saudade')))
        assert 'memória' in json.dumps(row[5].adapted, ensure_ascii=False)


def sample(name):
    out = []
    with gzip.open(os.path.join(HERE, 'fixtures', name), 'rt', encoding='utf-8') as f:
        for line in f:
            out.append(json.loads(line))
    return out


def pick(name, lang, word, pos=None):
    for raw in sample(name):
        if raw.get('lang_code') == lang and raw.get('word') == word and (pos is None or raw.get('pos') == pos):
            return raw
    raise AssertionError(f'{lang}:{word} is not in {name}')


class TestOtherEditions:
    """The same schema in every edition: what is kept is read the same way, from the Japanese and the Italian Wiktionaries."""

    def test_an_italian_form_points_to_its_lemma_and_the_lemma_lists_its_translations(self):
        corsero = trim_entry(pick('dictionary-sample-it.jsonl.gz', 'it', 'corsero'), headwords=('it',))
        assert corsero['data']['senses'][0]['form_of'] == [{'word': 'correre'}]
        assert 'terza persona plurale' in corsero['data']['senses'][0]['glosses'][0]
        correre = trim_entry(pick('dictionary-sample-it.jsonl.gz', 'it', 'correre'), headwords=('it',))
        assert correre['data']['senses'][0]['glosses'] == ['procedere velocemente']
        assert any(t['lang'] == 'en' for t in correre['data']['translations'])

    def test_a_japanese_kanji_that_is_the_written_form_of_a_word_points_to_it(self):
        entry = trim_entry(pick('dictionary-sample-ja.jsonl.gz', 'ja', '走る'), headwords=('ja',))
        assert entry['data']['senses'][0]['form_of'] == [{'word': 'はしる'}]
        assert entry['pos'] == 'character'

    def test_a_package_keeps_the_words_of_its_own_language_and_lets_go_the_others(self):
        for name, lang, kept, gone in [('dictionary-sample-it.jsonl.gz', 'it', ('it', 'casa'), ('en', 'house')),
                                       ('dictionary-sample-ja.jsonl.gz', 'ja', ('ja', '本'), ('it', 'casa'))]:
            assert trim_entry(pick(name, *kept), headwords=(lang,)) is not None
            assert trim_entry(pick(name, *gone), headwords=(lang,)) is None

    def test_a_package_can_keep_the_words_of_more_languages(self):
        raw = pick('dictionary-sample-it.jsonl.gz', 'en', 'house')
        assert trim_entry(raw, headwords=('it', 'en'))['lang'] == 'en'

    def test_the_translations_kept_are_those_into_the_languages_of_the_library_and_into_the_languages_kept(self):
        raw = {'word': 'x', 'lang_code': 'ru', 'senses': [{'glosses': ['a']}], 'translations': [
            {'lang_code': 'pt', 'word': 'um'}, {'lang_code': 'ru', 'word': 'два'}, {'lang_code': 'ko', 'word': '셋'}, {'lang_code': 'xx', 'word': 'y'}]}
        kept = trim_entry(raw, headwords=('ru',))['data']['translations']
        assert [t['lang'] for t in kept] == ['pt', 'ru']
        assert [t['lang'] for t in trim_entry(raw, headwords=('ru', 'ko'))['data']['translations']] == ['pt', 'ru', 'ko']
        assert [t['lang'] for t in trim_entry(raw, headwords=('ru',), translations_to=('ko',))['data']['translations']] == ['ru', 'ko']

    def test_a_chinese_package_finds_the_word_by_either_script(self):
        # the simplified and the traditional form are entries of their own, as in the Portuguese edition
        assert normalize('书') != normalize('書')


class TestTheEnglishEdition:
    """The English Wiktionary, by the language of the words (kaikki.org/dictionary/English): the English words, each with
    its translations into the other languages, which is what the bridge through English walks."""

    CATALOG = ('pt', 'ja', 'it', 'ru', 'de', 'es', 'fr', 'zh', 'ko')

    def test_keeps_the_english_words_and_their_translations_into_the_languages_of_the_catalog(self):
        raw = pick('dictionary-sample-en.jsonl.gz', 'en', 'barter', 'noun')
        entry = trim_entry(raw, headwords=('en',), translations_to=self.CATALOG)
        assert entry['lang'] == 'en' and entry['word'] == 'barter'
        assert {l['lang'] for l in entry['links']} >= {'pt', 'ja', 'it', 'ru'}
        assert all(l['lang'] in self.CATALOG + ('en',) for l in entry['links'])

    def test_shows_a_few_translations_of_each_language_and_links_to_more(self):
        raw = pick('dictionary-sample-en.jsonl.gz', 'en', 'book', 'verb')
        entry = trim_entry(raw, headwords=('en',), translations_to=self.CATALOG)
        shown, links = entry['data']['translations'], entry['links']
        for lang in {t['lang'] for t in shown}:
            assert sum(1 for t in shown if t['lang'] == lang) <= dictionary.MAX_TRANSLATIONS_PER_LANG
        assert len(links) >= len(shown)
        assert all(sum(1 for l in links if l['lang'] == lang) <= dictionary.MAX_LINKS_PER_LANG for lang in self.CATALOG)

    def test_what_is_shown_is_in_the_links_too(self):
        raw = pick('dictionary-sample-en.jsonl.gz', 'en', 'accurate', 'adj')
        entry = trim_entry(raw, headwords=('en',), translations_to=self.CATALOG)
        assert all(t in entry['links'] for t in entry['data']['translations'])

    def test_a_translation_with_no_word_is_not_a_link(self):
        raw = {'word': 'x', 'lang_code': 'en', 'senses': [{'glosses': ['a']}], 'translations': [{'lang_code': 'pt', 'note': 'no word'}, {'lang_code': 'pt', 'word': 'x'}]}
        assert trim_entry(raw, headwords=('en',))['links'] == [{'lang': 'pt', 'word': 'x'}]

    def test_the_same_translation_is_listed_once_but_under_two_senses_twice(self):
        raw = {'word': 'x', 'lang_code': 'en', 'senses': [{'glosses': ['a']}], 'translations': [
            {'lang_code': 'pt', 'word': 'y', 'sense': 's1'}, {'lang_code': 'pt', 'word': 'y', 'sense': 's1'}, {'lang_code': 'pt', 'word': 'y', 'sense': 's2'}]}
        assert [l['sense'] for l in trim_entry(raw, headwords=('en',))['links']] == ['s1', 's2']

    def test_an_inflected_english_word_points_to_its_lemma(self):
        raw = pick('dictionary-sample-en.jsonl.gz', 'en', 'pies')
        entry = trim_entry(raw, headwords=('en',))
        assert any(s.get('form_of') for s in entry['data']['senses'])

    def test_it_keeps_nothing_that_is_not_english_when_told_so(self):
        assert trim_entry({'word': 'casa', 'lang_code': 'pt', 'senses': [{'glosses': ['a']}]}, headwords=('en',)) is None

    def test_the_links_keep_more_than_the_entry_shows(self):
        raw = pick('dictionary-sample-en.jsonl.gz', 'en', 'book', 'verb')  # six words in Russian
        entry = trim_entry(raw, headwords=('en',), translations_to=('ru',))
        shown = [t for t in entry['data']['translations'] if t['lang'] == 'ru']
        links = [l for l in entry['links'] if l['lang'] == 'ru']
        assert len(shown) == dictionary.MAX_TRANSLATIONS_PER_LANG and len(links) == 6

    def test_what_the_entry_shows_is_cut_at_a_hundred_whatever_the_languages(self):
        langs = tuple(f'a{c}' for c in 'abcdefghijklmnopqrstuvwxy')  # 25 languages, five words each
        raw = {'word': 'x', 'lang_code': 'en', 'senses': [{'glosses': ['a']}],
               'translations': [{'lang_code': lang, 'word': f'w{i}'} for lang in langs for i in range(5)]}
        entry = trim_entry(raw, headwords=('en',), translations_to=langs)
        assert len(entry['data']['translations']) == dictionary.MAX_TRANSLATIONS == 100
        assert len(entry['links']) == 125
