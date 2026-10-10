"""Wikidata (what a work is, in every language) and Wikipedia (the text about it), and how the registry uses them."""
from unittest.mock import MagicMock, patch

from providers.base import BaseProvider, Credit, MetadataRecord
from providers.query import read_file_title
from providers.registry import ProviderRegistry
from providers.wikidata import WikidataProvider, claim_values, is_work, label_of, year_of
from providers.wikipedia import WikipediaProvider, attribution, cut


def claim(prop, value, qualifiers=None):
    snak = {'mainsnak': {'datavalue': {'value': value}}}
    if qualifiers:
        snak['qualifiers'] = qualifiers
    return prop, snak


def entity(qid, labels, claims=(), aliases=None, sitelinks=None):
    by_prop = {}
    for prop, snak in claims:
        by_prop.setdefault(prop, []).append(snak)
    return {'id': qid, 'labels': {l: {'value': v} for l, v in labels.items()}, 'aliases': {l: [{'value': a} for a in al] for l, al in (aliases or {}).items()},
            'claims': by_prop, 'sitelinks': {k: {'title': v} for k, v in (sitelinks or {}).items()}}


DUNE = entity('Q190192', {'pt': 'Duna', 'en': 'Dune', 'es': 'Dune (novela)', 'fr': 'Dune'},
              claims=[claim('P31', {'id': 'Q7725634'}), claim('P50', {'id': 'Q7934'}), claim('P577', {'time': '+1965-00-00T00:00:00Z'}),
                      claim('P136', {'id': 'Q101'}), claim('P136', {'id': 'Q102'}), claim('P136', {'id': 'Q103'}),
                      claim('P179', {'id': 'Q900'}, {'P1545': [{'datavalue': {'value': '1'}}]}),
                      claim('P1476', {'text': 'Dune', 'language': 'en'})],
              aliases={'en': ['Dune (1965)'], 'pt': ['Duna']},
              sitelinks={**{f'{l}wiki': f'T{l}' for l in ('de', 'it', 'ru', 'ja')}, 'ptwiki': 'Duna (romance)', 'enwiki': 'Dune (novel)'})
FILM = entity('Q114819', {'pt': 'Duna'}, claims=[claim('P31', {'id': 'Q11424'})])
NAMES = {'Q7934': 'Frank Herbert', 'Q101': 'ficção científica soft', 'Q102': 'romance planetário', 'Q103': 'ficção social', 'Q900': 'Crônicas de Duna'}


def reply(data=None, ok=True):
    r = MagicMock()
    r.ok = ok
    r.data = data
    return r


def script(*answers):
    """get_json answers in the order the provider asks."""
    calls = []
    queue = list(answers)

    def fake(provider, url, params=None, **kw):
        calls.append(params)
        return queue.pop(0) if queue else reply(None, ok=False)
    return fake, calls


class TestReadingWikidata:
    def test_the_name_of_an_entity_is_in_the_language_wanted_first(self):
        e = {'labels': {'en': {'value': 'Dune'}, 'pt': {'value': 'Duna'}, 'pt-br': {'value': 'Duna (BR)'}}}
        assert label_of(e) == 'Duna (BR)'
        assert label_of({'labels': {'en': {'value': 'Dune'}, 'pt': {'value': 'Duna'}}}) == 'Duna'
        assert label_of({'labels': {'en': {'value': 'Dune'}}}) == 'Dune'
        assert label_of({'labels': {'ja': {'value': '砂の惑星'}}}) == '砂の惑星'
        assert label_of({'labels': {'pt': {'value': ''}, 'en': {'value': 'Dune'}}}) == 'Dune'
        assert label_of({}) is None

    def test_the_values_of_a_property_are_what_wikidata_says(self):
        assert claim_values(DUNE, 'P50') == ['Q7934'] and claim_values(DUNE, 'P577') == [{'time': '+1965-00-00T00:00:00Z'}]
        assert claim_values(DUNE, 'P999') == [] and claim_values({}, 'P50') == []
        assert claim_values({'claims': {'P1': [{'mainsnak': {}}, {'mainsnak': {'datavalue': {'value': 'x'}}}]}}, 'P1') == ['x']

    def test_a_work_of_writing_is_not_a_film(self):
        assert is_work(DUNE) and not is_work(FILM) and not is_work({})

    def test_the_year_is_the_first_four_digits_of_the_time(self):
        assert year_of({'time': '+1965-00-00T00:00:00Z'}) == '1965' and year_of({'time': '+0899-01-01T00:00:00Z'}) == '899'
        assert year_of({'time': '-0044-03-15T00:00:00Z'}) == '44' and year_of({'time': 'x'}) is None and year_of(None) is None and year_of('1965') is None


class TestWikidata:
    def setup_method(self):
        self.provider = WikidataProvider()

    def test_names(self):
        assert (self.provider.id, self.provider.name, self.provider.resolver) == ('wikidata', 'Wikidata', True)

    def lookup(self, title='Duna', author='Frank Herbert', answers=None):
        answers = answers or [reply({'search': [{'id': 'Q190192'}, {'id': 'Q114819'}]}), reply({'entities': {'Q190192': DUNE, 'Q114819': FILM}}),
                              reply({'entities': {k: {'id': k, 'labels': {'pt': {'value': v}}} for k, v in NAMES.items()}})]
        fake, calls = script(*answers)
        with patch('providers.wikidata.get_json', fake):
            return self.provider.lookup(read_file_title(title, author, 'epub')), calls

    def test_it_asks_by_the_title_only_then_for_the_entities_then_for_their_names(self):
        records, calls = self.lookup()
        assert calls[0] == {'format': 'json', 'action': 'wbsearchentities', 'search': 'Duna', 'language': 'pt', 'uselang': 'pt', 'type': 'item', 'limit': 10}
        assert calls[1]['action'] == 'wbgetentities' and calls[1]['ids'] == 'Q190192|Q114819' and calls[1]['props'] == 'labels|aliases|claims|sitelinks'
        assert calls[1]['languages'] == 'pt|pt-br|en|es|fr'
        assert calls[2]['props'] == 'labels' and calls[2]['ids'] == 'Q101|Q102|Q103|Q7934|Q900'
        assert 'Herbert' not in str(calls) and len(calls) == 3 and len(records) == 1

    def test_a_record_is_the_work(self):
        (r,), _ = self.lookup()
        assert (r.title, r.author, r.original_year, r.series, r.series_index, r.source) == ('Duna', 'Frank Herbert', '1965', 'Crônicas de Duna', 1.0, 'Wikidata')
        assert r.publication_date is None   # P577 of a work is its first publication, not an edition's date (DEC-156)
        assert r.credits[0].ids == {'wikidata': 'Q7934'}
        assert r.tags == ['ficção científica soft', 'romance planetário', 'ficção social']
        assert r.prior == 0.6 and r.raw['sitelinks_count'] == 6
        assert r.raw['sitelinks'] == {'pt': 'Duna (romance)', 'en': 'Dune (novel)'} and r.raw['wikidata_id'] == 'Q190192'
        assert r.raw['alt_titles'] == ['Dune', 'Dune (1965)', 'Dune (novela)']   # not the title itself, each once, and the title it has in its own language too

    def test_the_other_names_are_at_most_twelve_and_each_once(self):
        many = entity('Q1', {'pt': 'Obra', 'en': 'Work'}, claims=[claim('P31', {'id': 'Q8261'})], aliases={'en': [f'Alias {i}' for i in range(20)]})
        (r,), _ = self.lookup('Obra', None, [reply({'search': [{'id': 'Q1'}]}), reply({'entities': {'Q1': many}}), reply({'entities': {}})])
        assert len(r.raw['alt_titles']) == 12 and r.raw['alt_titles'][0] == 'Work' and len(set(r.raw['alt_titles'])) == 12

    def test_less_in_the_entity_is_less_in_the_record(self):
        bare = entity('Q2', {'en': 'Bare'}, claims=[claim('P31', {'id': 'Q571'})])
        (r,), _ = self.lookup('Bare', None, [reply({'search': [{'id': 'Q2'}]}), reply({'entities': {'Q2': bare}}), reply({'entities': {}})])
        assert (r.title, r.author, r.credits, r.original_year, r.series, r.series_index, r.tags, r.prior) == ('Bare', None, [], None, None, None, [], 0.0)
        assert r.raw['sitelinks'] == {} and r.raw['alt_titles'] == []

    def test_a_series_with_no_ordinal_has_no_number_and_a_bad_one_is_left_out(self):
        cases = [(None, None), ({'P1545': [{'datavalue': {'value': 'two'}}, {'datavalue': {'value': '3'}}]}, 3.0), ({'P1545': [{'nodatavalue': 1}]}, None)]
        for qualifiers, number in cases:
            e = entity('Q3', {'pt': 'Livro'}, claims=[claim('P31', {'id': 'Q8261'}), claim('P179', {'id': 'Q900'}, qualifiers)])
            (r,), _ = self.lookup('Livro', None, [reply({'search': [{'id': 'Q3'}]}), reply({'entities': {'Q3': e}}), reply({'entities': {'Q900': {'id': 'Q900', 'labels': {'pt': {'value': 'Saga'}}}}})])
            assert (r.series, r.series_index) == ('Saga', number)

    def test_the_year_comes_from_the_first_date_that_has_one(self):
        e = entity('Q4', {'pt': 'Livro'}, claims=[claim('P31', {'id': 'Q8261'}), claim('P577', {'time': 'x'}), claim('P577', {'time': '+1999-05-01T00:00:00Z'})])
        (r,), _ = self.lookup('Livro', None, [reply({'search': [{'id': 'Q4'}]}), reply({'entities': {'Q4': e}}), reply({'entities': {}})])
        assert r.original_year == '1999'

    def test_when_no_work_is_close_it_asks_again_in_english_and_keeps_each_once(self):
        other = entity('Q9', {'pt': 'Outro Livro'}, claims=[claim('P31', {'id': 'Q8261'})])
        english = entity('Q10', {'en': 'Thunderhead', 'pt': 'A Nuvem'}, claims=[claim('P31', {'id': 'Q8261'})], sitelinks={'enwiki': 'Thunderhead'})
        records, calls = self.lookup('Thunderhead', None, [
            reply({'search': [{'id': 'Q9'}]}), reply({'entities': {'Q9': other}}),
            reply({'search': [{'id': 'Q9'}, {'id': 'Q10'}]}), reply({'entities': {'Q10': english}}), reply({'entities': {}})])
        assert [c['language'] for c in calls if c.get('action') == 'wbsearchentities'] == ['pt', 'en']
        assert [r.raw['wikidata_id'] for r in records] == ['Q10', 'Q9'] and calls[3]['ids'] == 'Q10'

    def test_when_a_work_is_close_it_does_not_ask_again(self):
        _, calls = self.lookup()
        assert [c['language'] for c in calls if c.get('action') == 'wbsearchentities'] == ['pt']

    def test_a_title_close_by_its_name_in_english_or_without_its_subtitle_is_close(self):
        e = entity('Q5', {'pt': 'Ficções', 'en': 'Sapiens'}, claims=[claim('P31', {'id': 'Q571'})])
        _, calls = self.lookup('Sapiens: Uma breve história', None, [reply({'search': [{'id': 'Q5'}]}), reply({'entities': {'Q5': e}}), reply({'entities': {}})])
        assert len([c for c in calls if c.get('action') == 'wbsearchentities']) == 1

    def test_only_the_five_best_known_works_are_kept(self):
        works = {f'Q{i}': entity(f'Q{i}', {'pt': 'Obra'}, claims=[claim('P31', {'id': 'Q8261'})], sitelinks={f'l{n}wiki': 'x' for n in range(i)}) for i in range(1, 9)}
        records, _ = self.lookup('Obra', None, [reply({'search': [{'id': k} for k in works]}), reply({'entities': works}), reply({'entities': {}})])
        assert [r.raw['wikidata_id'] for r in records] == ['Q8', 'Q7', 'Q6', 'Q5', 'Q4']

    def test_a_search_with_nothing_or_that_fails_gives_nothing(self):
        for answers in ([reply({'search': []})], [reply(None, ok=False)], [reply({'search': [{'id': 'Q114819'}]}), reply({'entities': {'Q114819': FILM}}), reply(None, ok=False)]):
            fake, _ = script(*answers)
            with patch('providers.wikidata.get_json', fake):
                assert self.provider.lookup(read_file_title('Duna', None, 'epub')) == []

    def test_a_title_with_nothing_in_it_asks_nothing(self):
        fake, calls = script()
        with patch('providers.wikidata.get_json', fake):
            assert self.provider.lookup(read_file_title('', None, 'epub')) == [] and calls == []

    def test_a_value_that_is_zero_is_still_a_value(self):
        assert claim_values({'claims': {'P1': [{'mainsnak': {'datavalue': {'value': 0}}}]}}, 'P1') == [0]

    def test_the_names_are_not_asked_when_there_is_no_work(self):
        records, calls = self.lookup('Duna', None, [reply({'search': []}), reply({'search': []})])
        assert records == [] and [c['action'] for c in calls] == ['wbsearchentities', 'wbsearchentities']

    def test_a_work_with_many_genres_keeps_six(self):
        e = entity('Q6', {'pt': 'Livro'}, claims=[claim('P31', {'id': 'Q8261'})] + [claim('P136', {'id': f'Q{200 + i}'}) for i in range(8)])
        names = {f'Q{200 + i}': {'id': f'Q{200 + i}', 'labels': {'pt': {'value': f'gênero {i}'}}} for i in range(8)}
        (r,), _ = self.lookup('Livro', None, [reply({'search': [{'id': 'Q6'}]}), reply({'entities': {'Q6': e}}), reply({'entities': names})])
        assert r.tags == [f'gênero {i}' for i in range(6)]

    def test_what_makes_a_work_well_known_has_a_ceiling(self):
        e = entity('Q7', {'pt': 'Livro'}, claims=[claim('P31', {'id': 'Q8261'})], sitelinks={f'l{i}wiki': 'x' for i in range(150)})
        (r,), _ = self.lookup('Livro', None, [reply({'search': [{'id': 'Q7'}]}), reply({'entities': {'Q7': e}}), reply({'entities': {}})])
        assert r.prior == 10 and r.raw['sitelinks_count'] == 150

    def test_the_other_names_include_the_portuguese_ones_and_the_title_in_its_own_language(self):
        e = entity('Q8', {'pt': 'Livro', 'pt-br': 'Livro BR', 'en': 'Book'}, claims=[claim('P31', {'id': 'Q8261'}), claim('P1476', {'text': 'Buch', 'language': 'de'})],
                   aliases={'pt': ['Tomo'], 'pt-br': ['Tomo BR']})
        (r,), _ = self.lookup('Livro', None, [reply({'search': [{'id': 'Q8'}]}), reply({'entities': {'Q8': e}}), reply({'entities': {}})])
        assert r.title == 'Livro BR' and r.raw['alt_titles'] == ['Book', 'Livro', 'Tomo', 'Tomo BR', 'Buch']

    def test_the_second_search_does_not_ask_again_for_what_was_not_a_work(self):
        english = entity('Q10', {'en': 'Thunderhead'}, claims=[claim('P31', {'id': 'Q8261'})])
        _, calls = self.lookup('Thunderhead', None, [
            reply({'search': [{'id': 'Q114819'}]}), reply({'entities': {'Q114819': FILM}}),
            reply({'search': [{'id': 'Q114819'}, {'id': 'Q10'}]}), reply({'entities': {'Q10': english}}), reply({'entities': {}})])
        assert calls[3]['ids'] == 'Q10'

    def test_a_work_close_by_its_name_in_portuguese_when_the_first_name_is_another(self):
        e = entity('Q11', {'pt': 'Duna', 'pt-br': 'Duna do Brasil'}, claims=[claim('P31', {'id': 'Q8261'})])
        _, calls = self.lookup('Duna', None, [reply({'search': [{'id': 'Q11'}]}), reply({'entities': {'Q11': e}}), reply({'entities': {}})])
        assert len([c for c in calls if c.get('action') == 'wbsearchentities']) == 1

    def test_only_entities_are_asked_for_their_names(self):
        e = entity('Q12', {'pt': 'Livro'}, claims=[claim('P31', {'id': 'Q8261'}), claim('P136', {'id': 'texto'}), claim('P50', {'id': 'Q7934'})])
        _, calls = self.lookup('Livro', None, [reply({'search': [{'id': 'Q12'}]}), reply({'entities': {'Q12': e}}), reply({'entities': {}})])
        assert calls[2]['ids'] == 'Q7934'

    def test_an_answer_that_is_not_an_object_is_nothing(self):
        records, _ = self.lookup('Duna', None, [reply(['not', 'an', 'object']), reply(['x'])])
        assert records == []

    def test_the_manual_search_gives_the_first(self):
        fake, _ = script(reply({'search': [{'id': 'Q190192'}]}), reply({'entities': {'Q190192': DUNE}}), reply({'entities': {}}))
        with patch('providers.wikidata.get_json', fake):
            assert self.provider.search('Duna').title == 'Duna'
        fake, _ = script(reply({'search': []}))
        with patch('providers.wikidata.get_json', fake):
            assert self.provider.search('x') is None

    @patch('providers.http.requests.get')
    def test_the_author_is_never_sent(self, mock_get):
        mock_get.return_value = MagicMock(status_code=200, json=MagicMock(return_value={'search': []}))
        self.provider.lookup(read_file_title('Duna - Frank Herbert', 'Frank Herbert', 'epub'))
        assert mock_get.called
        for call in mock_get.call_args_list:
            assert 'Herbert' not in str(call)


def page(extract='Duna é um romance. Segunda frase.', kind='standard', url='https://pt.wikipedia.org/wiki/Duna_(romance)'):
    return reply({'type': kind, 'extract': extract, 'content_urls': {'desktop': {'page': url}}})


class TestWikipedia:
    def setup_method(self):
        self.provider = WikipediaProvider()

    def record(self, **kw):
        return MetadataRecord(title='Dune', raw={'sitelinks': {'pt': 'Duna (romance)', 'en': 'Dune (novel)'}}, **kw)

    def test_names(self):
        assert (self.provider.id, self.provider.name, self.provider.completer) == ('wikipedia', 'Wikipedia', True)
        assert self.provider.lookup(read_file_title('Dune', None)) == [] and self.provider.search('Dune') is None

    def test_the_summary_of_the_page_in_portuguese_is_the_description_with_its_source(self):
        fake, calls = script(page())
        with patch('providers.wikipedia.get_json', fake):
            r = self.provider.complete(self.record())
        assert r.description == 'Duna é um romance. Segunda frase.\n\nFonte: Wikipédia (pt), CC BY-SA 4.0 — https://pt.wikipedia.org/wiki/Duna_(romance)'
        assert r.raw['wikipedia'] == {'language': 'pt', 'title': 'Duna (romance)', 'url': 'https://pt.wikipedia.org/wiki/Duna_(romance)', 'license': 'CC BY-SA 4.0'}

    def test_the_title_of_the_page_is_in_the_address_without_anything_that_would_change_it(self):
        urls = []
        with patch('providers.wikipedia.get_json', lambda provider, url, **kw: (urls.append(url), page())[1]):
            self.provider.complete(MetadataRecord(raw={'sitelinks': {'pt': 'AC/DC: Álbum?'}}))
        assert urls == ['https://pt.wikipedia.org/api/rest_v1/page/summary/AC%2FDC%3A%20%C3%81lbum%3F']

    def test_without_a_page_in_portuguese_it_is_the_one_in_english(self):
        urls = []
        with patch('providers.wikipedia.get_json', lambda provider, url, **kw: (urls.append(url), page('Dune is a novel.', url='https://en.wikipedia.org/wiki/Dune_(novel)'))[1]):
            r = self.provider.complete(MetadataRecord(raw={'sitelinks': {'en': 'Dune (novel)'}}))
        assert len(urls) == 1 and urls[0].startswith('https://en.wikipedia.org/') and r.description.endswith('Fonte: Wikipédia (en), CC BY-SA 4.0 — https://en.wikipedia.org/wiki/Dune_(novel)')

    def test_a_page_that_is_not_about_the_work_or_empty_or_missing_goes_to_the_next_language(self):
        for first in (page(kind='disambiguation'), page(extract='  '), reply(None, ok=False), reply({'type': 'standard'})):
            fake, calls = script(first, page('Dune is a novel.'))
            with patch('providers.wikipedia.get_json', fake):
                r = self.provider.complete(self.record())
            assert len(calls) == 0 or r.raw['wikipedia']['language'] == 'en', first.data
            assert r.description.startswith('Dune is a novel.')

    def test_nothing_is_changed_without_a_page_or_without_pages_at_all(self):
        fake, _ = script(reply(None, ok=False), reply(None, ok=False))
        with patch('providers.wikipedia.get_json', fake):
            r = self.provider.complete(self.record())
        assert r.description is None and 'wikipedia' not in r.raw
        for raw in (None, {}, {'sitelinks': None}, {'sitelinks': {}}):
            assert self.provider.complete(MetadataRecord(raw=raw)).description is None

    def test_a_description_that_the_answer_has_is_kept_and_the_page_is_still_told(self):
        fake, _ = script(page())
        with patch('providers.wikipedia.get_json', fake):
            r = self.provider.complete(self.record(description='A blurb.'))
        assert r.description == 'A blurb.' and r.raw['wikipedia']['title'] == 'Duna (romance)'

    def test_a_page_with_no_address_gets_the_one_it_has_by_its_title(self):
        fake, _ = script(reply({'type': 'standard', 'extract': 'Texto.'}))
        with patch('providers.wikipedia.get_json', fake):
            r = self.provider.complete(self.record())
        assert r.raw['wikipedia']['url'] == 'https://pt.wikipedia.org/wiki/Duna_%28romance%29'

    def test_a_long_summary_is_cut_at_the_end_of_a_sentence(self):
        sentences = ' '.join(f'Frase número {i} do resumo.' for i in range(200))
        out = cut(sentences)
        assert len(out) <= 1500 and out.endswith('.') and out.endswith('resumo.')
        assert cut('Curto.') == 'Curto.' and cut('  ') == '' and cut(None) == ''
        assert cut('x' * 3000).endswith('…') and len(cut('x' * 3000)) == 1501
        assert cut('a. ' + 'x' * 3000).endswith('…')   # the end of the sentence is too early to be worth it
        assert attribution('pt', 'u') == 'Fonte: Wikipédia (pt), CC BY-SA 4.0 — u'

    def test_a_text_that_has_exactly_the_limit_is_kept_whole(self):
        assert cut('x' * 1500) == 'x' * 1500

    def test_a_sentence_can_end_at_a_line_break(self):
        text = '\n'.join(f'Frase número {i} do resumo.' for i in range(200))
        out = cut(text)
        assert out.endswith('resumo.') and len(out) <= 1500 and not out.endswith('…')

    def test_an_answer_that_is_not_an_object_is_no_page(self):
        fake, _ = script(reply(['x']), reply(['y']))
        with patch('providers.wikipedia.get_json', fake):
            r = self.provider.complete(self.record())
        assert r.description is None and 'wikipedia' not in r.raw


class Resolver(BaseProvider):
    resolver = True

    def __init__(self, records, fail=False):
        self.records, self.fail, self.asked = records, fail, []

    id = property(lambda self: 'wikidata')
    name = property(lambda self: 'Wikidata')

    def search(self, q):
        raise AssertionError

    def lookup(self, query):
        self.asked.append(query)
        if self.fail:
            raise RuntimeError('down')
        return list(self.records)


class Books(BaseProvider):
    """A book provider that knows the books by title."""

    def __init__(self, known, pid='openlibrary'):
        self.known, self._id, self.asked = known, pid, []

    id = property(lambda self: self._id)
    name = property(lambda self: 'OpenLibrary')

    def search(self, q):
        raise AssertionError

    def lookup(self, query):
        self.asked.append(query.search_title)
        return [r for t, r in self.known if t == query.search_title]

    def enrich(self, record):
        record.raw = dict(record.raw or {}, enriched=True)
        return record


class Completer(BaseProvider):
    completer = True

    def __init__(self, fail=False):
        self.fail, self.completed = fail, []

    id = property(lambda self: 'wikipedia')
    name = property(lambda self: 'Wikipedia')

    def search(self, q):
        raise AssertionError

    def lookup(self, q):
        return []

    def complete(self, record):
        self.completed.append(record)
        if self.fail:
            raise RuntimeError('cannot')
        record.raw = dict(record.raw, wikipedia={'language': 'pt'})
        record.description = record.description or 'Texto da Wikipédia.'
        return record


def work(title='Dune', author='Frank Herbert', **kw):
    r = MetadataRecord(title=title, credits=[Credit(author)] if author else [], source='OpenLibrary', **kw)
    r.author = author
    return r


def entity_record(title='Dune', author='Frank Herbert', alts=('Duna',), **kw):
    r = MetadataRecord(title=title, credits=[Credit(author)] if author else [], source='Wikidata', tags=['ficção científica soft'], series='Duna', series_index=1.0, **kw)
    r.author = author
    r.raw = {'wikidata_id': 'Q190192', 'alt_titles': list(alts), 'sitelinks': {'pt': 'Duna (romance)'}}
    return r


def registry(allowed, *providers):
    r = ProviderRegistry(enabled=lambda pid: pid in allowed)
    r._providers = {'default': list(providers), 'cbz': list(providers)}
    return r


ALL = {'openlibrary', 'wikidata', 'wikipedia'}


class TestUsingThem:
    def test_the_answer_of_a_book_provider_gets_what_wikidata_knows_of_the_work(self):
        books, resolver = Books([('Dune', work())]), Resolver([entity_record()])
        got = registry(ALL, books, resolver).search_best('Dune', 'epub', author='Frank Herbert')
        assert got.raw['wikidata_id'] == 'Q190192' and got.raw['sitelinks'] == {'pt': 'Duna (romance)'}
        assert got.tags == ['ficção científica soft'] and (got.series, got.series_index) == ('Duna', 1.0) and got.source == 'OpenLibrary + Wikidata'
        assert 'translated_from' not in got.raw and len(resolver.asked) == 1

    def test_what_the_answer_has_stays_and_the_genres_are_added_once(self):
        book = work(tags=['Ficção Científica Soft', 'Fiction'])
        book.series, book.series_index = 'Dune Chronicles', 1.0
        got = registry(ALL, Books([('Dune', book)]), Resolver([entity_record()])).search_best('Dune', 'epub')
        assert got.tags == ['Ficção Científica Soft', 'Fiction'] and got.series == 'Dune Chronicles'

    def test_a_title_the_book_provider_does_not_know_is_translated_by_wikidata(self, capsys):
        books, resolver = Books([('Dune', work())]), Resolver([entity_record()])
        got = registry(ALL, books, resolver).search_best('Duna', 'epub', author='Frank Herbert')
        assert books.asked == ['Duna', 'Dune'] and got.title == 'Dune' and got.raw['translated_from'] == 'Duna'
        assert got.raw['wikidata_id'] == 'Q190192' and got.source == 'OpenLibrary + Wikidata'
        assert "Wikidata: 'Duna' is 'Dune'" in capsys.readouterr().out

    def test_the_work_wikidata_says_it_is_has_to_be_the_one_the_file_says_by_its_author(self):
        books, resolver = Books([('Dune', work())]), Resolver([entity_record(author='Someone Else')])
        got = registry(ALL, Books([('Dune', work())]), Resolver([entity_record(author='Someone Else')])).search_best('Duna', 'epub', author='Frank Herbert')
        assert got is None   # neither the translation nor the entity

    def test_the_translation_does_not_ask_for_what_the_file_already_says_and_stops_at_three(self):
        books = Books([])
        resolver = Resolver([entity_record(alts=('Duna', 'Dune (1965)', 'Arrakis', 'Wydma', 'Extra'))])
        registry(ALL, books, resolver).search_best('Duna', 'epub', author='Frank Herbert')
        assert books.asked == ['Duna', 'Dune', 'Arrakis', 'Wydma']   # the file's, then three others at most; 'Dune (1965)' is 'Dune' again

    def test_when_only_wikidata_knows_the_work_what_it_has_is_the_suggestion(self, capsys):
        got = registry(ALL, Books([]), Resolver([entity_record()]), Completer()).search_best('Dune', 'epub', author='Frank Herbert')
        assert got.source == 'Wikidata + Wikipedia' and got.provider_id == 'wikidata' and got.raw['wikidata_id'] == 'Q190192'
        assert "Only Wikidata knows it: 'Dune'" in capsys.readouterr().out

    def test_wikidata_that_is_off_changes_nothing(self):
        books, resolver = Books([('Dune', work())]), Resolver([entity_record()])
        got = registry({'openlibrary'}, books, resolver).search_best('Dune', 'epub')
        assert resolver.asked == [] and got.source == 'OpenLibrary' and 'wikidata_id' not in got.raw
        assert registry({'openlibrary', 'wikidata'}, Books([]), Resolver([])).search_best('Duna', 'epub') is None

    def test_wikidata_that_fails_does_not_lose_the_answer(self, capsys):
        got = registry(ALL, Books([('Dune', work())]), Resolver([], fail=True)).search_best('Dune', 'epub')
        assert got.source == 'OpenLibrary' and 'Wikidata failed: down' in capsys.readouterr().out
        assert registry(ALL, Books([]), Resolver([], fail=True)).search_best('Duna', 'epub') is None

    def test_wikidata_is_not_set_against_the_book_providers(self):
        books, resolver = Books([('Dune', work(prior=0))]), Resolver([entity_record(prior=10)])
        got = registry({'openlibrary', 'wikidata'}, books, resolver).search_best('Dune', 'epub')
        assert got.provider_id == 'openlibrary'

    def test_the_text_of_wikipedia_completes_the_answer(self):
        completer = Completer()
        got = registry(ALL, Books([('Dune', work())]), Resolver([entity_record()]), completer).search_best('Dune', 'epub')
        assert got.description == 'Texto da Wikipédia.' and got.source == 'OpenLibrary + Wikidata + Wikipedia' and completer.completed == [got]

    def test_wikipedia_that_is_off_is_not_asked_and_one_that_fails_is_forgiven(self, capsys):
        completer = Completer()
        got = registry({'openlibrary', 'wikidata'}, Books([('Dune', work())]), Resolver([entity_record()]), completer).search_best('Dune', 'epub')
        assert completer.completed == [] and got.description is None
        bad = Completer(fail=True)
        got = registry(ALL, Books([('Dune', work())]), Resolver([entity_record()]), bad).search_best('Dune', 'epub')
        assert got is not None and got.source == 'OpenLibrary + Wikidata' and 'Wikipedia could not complete the answer: cannot' in capsys.readouterr().out

    def test_without_the_pages_of_the_work_wikipedia_has_nothing_to_do(self):
        completer = Completer()
        got = registry(ALL, Books([('Dune', work())]), completer).search_best('Dune', 'epub')   # no Wikidata: nobody says what the pages are
        assert completer.completed == [] and got.source == 'OpenLibrary'

    def test_the_description_a_book_provider_gives_is_not_replaced(self):
        got = registry(ALL, Books([('Dune', work(description='A blurb.'))]), Resolver([entity_record()]), Completer()).search_best('Dune', 'epub')
        assert got.description == 'A blurb.' and got.source.endswith('+ Wikipedia')

    def test_the_manual_search_shows_the_works_of_wikidata_too(self):
        got = registry(ALL, Books([('Dune', work())]), Resolver([entity_record()])).search_all('Dune', 'epub')
        assert sorted(r.provider_id for r in got) == ['openlibrary', 'wikidata']

    def test_the_provider_that_only_completes_is_never_asked_to_look_anything_up(self, capsys):
        completer = Completer()
        completer.lookup = lambda q: (_ for _ in ()).throw(AssertionError('asked'))
        registry(ALL, Books([('Dune', work())]), Resolver([entity_record()]), completer).search_best('Dune', 'epub')
        registry(ALL, Books([('Dune', work())]), Resolver([entity_record()]), completer).search_all('Dune', 'epub')
        assert 'Wikipedia' not in capsys.readouterr().out.replace('+ Wikipedia', '')

    def test_a_comic_or_a_manga_is_not_asked_of_wikidata_nor_of_wikipedia(self):
        ids = [p.id for p in ProviderRegistry()._providers['cbz']]
        assert 'wikidata' not in ids and 'wikipedia' not in ids
        assert [p.id for p in ProviderRegistry()._providers['default']] == ['google_books', 'openlibrary', 'wikidata', 'wikipedia']

    def test_the_translated_answer_is_enriched_by_the_provider_that_gave_it(self):
        got = registry(ALL, Books([('Dune', work())]), Resolver([entity_record()])).search_best('Duna', 'epub', author='Frank Herbert')
        assert got.raw['enriched'] is True

    def test_what_wikidata_adds_comes_after_the_provider_completes_the_answer(self):
        class Overwrites(Books):
            def enrich(self, record):   # Open Library puts its own subjects in the place of the tags
                record.tags, record.series = ['Fiction'], None
                return record
        for title in ('Dune', 'Duna'):   # found by the title of the file, and found by the one Wikidata translated it to
            got = registry(ALL, Overwrites([('Dune', work())]), Resolver([entity_record()])).search_best(title, 'epub', author='Frank Herbert')
            assert got.tags == ['Fiction', 'ficção científica soft'] and got.series == 'Duna' and got.raw['wikidata_id'] == 'Q190192'

    def test_what_wikidata_already_said_is_not_asked_twice(self):
        resolver = Resolver([entity_record()])
        registry(ALL, Books([('Dune', work())]), resolver).search_best('Duna', 'epub', author='Frank Herbert')
        assert len(resolver.asked) == 1

    def test_the_closest_work_is_the_one_used(self):
        far, near = entity_record(title='Far', alts=(), prior=0), entity_record(title='Near', alts=(), prior=5)   # both are the series the file names
        for records in ([far, near], [near, far]):
            got = registry(ALL, Books([]), Resolver(records)).search_best('Duna', 'epub', author='Frank Herbert')
            assert got.title == 'Near'

    def test_only_the_two_best_works_are_tried_for_a_translation(self):
        def entities(*alts):
            return [entity_record(title='Duna', alts=a) for a in alts]
        for known, found in (('Alt2', 'Alt2'), ('Alt3', None)):
            books = Books([(known, work(known))])
            got = registry(ALL, books, Resolver(entities(('Alt1',), ('Alt2',), ('Alt3',)))).search_best('Duna', 'epub', author='Frank Herbert')
            assert (got.raw.get('translated_from') and got.title) == found

    def test_a_work_has_four_other_names_tried_at_most_and_three_asked(self):
        for known, found in (('Known', None), ('Alt3', 'Alt3')):
            books = Books([(known, work(known))])
            got = registry(ALL, books, Resolver([entity_record(title='Duna', alts=('Alt1', 'Alt2', 'Alt3', 'Known'))])).search_best('Duna', 'epub', author='Frank Herbert')
            assert (got.raw.get('translated_from') and got.title) == found
        books = Books([('Known', work('Known'))])
        got = registry(ALL, books, Resolver([entity_record(title='Duna', alts=('Duna', 'Duna', 'Duna', 'Duna', 'Known'))])).search_best('Duna', 'epub', author='Frank Herbert')
        assert 'translated_from' not in got.raw   # the fifth name is not one of the four

    def test_the_best_of_the_answers_found_with_the_other_title_is_the_one(self):
        better, worse = work('Dune'), work('Dune', author=None)
        for order in ([('Dune', worse), ('Dune', better)], [('Dune', better), ('Dune', worse)]):
            got = registry(ALL, Books(order), Resolver([entity_record()])).search_best('Duna', 'epub', author='Frank Herbert')
            assert got.author == 'Frank Herbert'

    def test_a_wikipedia_that_finds_no_page_does_not_sign_the_answer(self):
        class Nothing(Completer):
            def complete(self, record):
                return record
        got = registry(ALL, Books([('Dune', work())]), Resolver([entity_record()]), Nothing()).search_best('Dune', 'epub')
        assert got.source == 'OpenLibrary + Wikidata'
