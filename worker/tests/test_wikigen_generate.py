"""The generator from books to bundle, with a model that is a script: no network, no database."""
import json
import re

import pytest

from wikigen.cache import MemoryCache
from wikigen.generate import generate
from wikigen.llm import Llm, Provider, Stop

NAMES = ['Ana', 'Mestre Bruno', 'Vila Velha', 'Anel da Ceifa']
KINDS = {'Ana': 'personagem', 'Mestre Bruno': 'personagem', 'Vila Velha': 'lugar', 'Anel da Ceifa': 'objeto'}


def chapter(index, title, part, paragraphs, href=None):
    return {'index': index, 'id': href or f'Text/c{index}.xhtml', 'href': href or f'Text/c{index}.xhtml', 'title': title, 'part': part,
            'epigraph_paragraphs': 0, 'paragraphs': paragraphs}


def book(bid, title, kind, chapters, sha):
    return {'id': bid, 'titulo': title, 'kind': kind, 'sha256': sha, 'chapters': {c['index']: c for c in chapters}, 'mapa': {}}


def books():
    text = lambda i: [f'Ana chegou a Vila Velha no capítulo {i}, e procurou Mestre Bruno na oficina antes do fim do dia.',
                      'Mestre Bruno guardava o Anel da Ceifa numa gaveta, e disse que Ana aprenderia o ofício devagar.']
    one = [chapter(i, f'{i}. Capítulo {i}', 'Parte I' if i <= 3 else 'Parte II', text(i)) for i in range(1, 7)]
    one.append(chapter(7, 'Prévia do próximo livro', 'Parte II', ['Ana voltará em breve, e Mestre Bruno também, no livro que vem.'] * 3))
    two = [chapter(i, f'{i}. Outro {i}', '', text(i)) for i in range(1, 3)]
    return [book(10, 'O Primeiro', 'main', one, 'a' * 64), book(11, 'O Segundo', 'main', two, 'b' * 64)]


class Script(Llm):
    """The model, as a script: it answers each kind of request from what the prompt holds."""

    def __init__(self, *a, **k):
        super().__init__(*a, **k)
        self.asked = []

    def chat(self, system, user, example, num_predict, timeout=600):
        self.asked.append(system[:30])
        self.usage.add(requests=1)
        return json.dumps(self.answer(system, user), ensure_ascii=False), 'stop'

    def answer(self, system, user):
        if system.startswith('Você é editor'):
            paras = dict(re.findall(r'\[p(\d+)\](?: \(epígrafe\))? (.*)', user))
            ents = []
            for pid, para in paras.items():
                for name in NAMES:
                    if name in para and not any(e['nome'] == name for e in ents):
                        quote = ' '.join(para.split()[:8])
                        ents.append({'nome': name, 'tipo': KINDS[name], 'papel': f'{name} é alguém da história',
                                     'novo': [f'{name} aparece neste capítulo'],
                                     'relacoes': [{'com': 'Mestre Bruno' if name == 'Ana' else 'Ana', 'relacao': 'aprendiz de' if name == 'Ana' else 'mestre de'}],
                                     'estado': f'{name} segue na vila', 'citacao': {'id': f'p{pid}', 'texto': quote}})
            return {'resumo': 'Algo acontece.', 'entidades': ents, 'eventos': []}
        if system.startswith('Você resolve identidades'):
            return {'decisoes': []}
        if system.startswith('Você levanta'):
            notes = []
            for sid, cap, text in re.findall(r'\[(c\d+p\d+)\] \(cap\. (\d+): [^)]*\) (.*)', user)[:3]:
                notes.append({'id': sid, 'cap': int(cap), 'categoria': 'evento', 'nota': 'Ana aprende o ofício com paciência.',
                              'citacao': ' '.join(text.split()[:8])})
            return {'existe': True, 'notas': notes}
        if system.startswith('Você escreve'):
            facts = re.findall(r'^(F\d+) \[', user, re.M)
            return {'frases': [{'par': 1, 'texto': f'Frase escrita sobre o fato {f}.', 'fatos': [f]} for f in facts[:4]]}
        if system.startswith('Você confere'):
            n = len(re.findall(r'^Frase \d+:', user, re.M))
            return {'frases': [{'n': i, 'ok': True, 'problema': ''} for i in range(1, n + 1)]}
        if system.startswith('Você organiza'):
            names = re.findall(r'^- (.+?) \[', user, re.M)
            return {'grupos': [{'grupo': 'Mentores e aprendizes', 'itens': [{'com': n, 'papel': 'mestre'} for n in names[:2]]}]}
        raise AssertionError('pergunta desconhecida: ' + system)


class Progress:
    def __init__(self):
        self.stages, self.errors, self.places = [], [], []

    def begin(self, stage, total=0):
        self.stages.append((stage, total))

    def tick(self, stage=None, place=None):
        pass

    def where(self, place):
        self.places.append(place)

    def error(self, message):
        self.errors.append(message)


def run(cut=2, cache=None, control=lambda: None, bks=None):
    llm = Script(Provider('openai', 'http://x/v1', 'm'), cache=cache if cache is not None else MemoryCache(), control=control)
    progress = Progress()
    bundle, partial = generate(llm, bks or books(), cut, progress, control, 2, 'openai', 'm', False)
    return bundle, partial, llm, progress


def test_the_bundle_has_the_books_the_entities_and_the_graph_of_the_text():
    bundle, partial, llm, progress = run()
    assert partial is False
    assert [b['title'] for b in bundle['books']] == ['O Primeiro', 'O Segundo']
    assert [b['sha256'][0] for b in bundle['books']] == ['a', 'b']
    first = bundle['books'][0]['chapters']
    assert [c['chapter'] for c in first] == [1, 2, 3, 4, 5, 6, 7]
    assert first[0]['href'] == 'Text/c1.xhtml' and first[6]['editorial'] is True and first[5]['editorial'] is False
    by_name = {e['name']: e for e in bundle['entities']}
    assert set(by_name) == {'Ana', 'Mestre Bruno', 'Vila Velha', 'Anel da Ceifa'}
    assert by_name['Ana']['kind'] == 'person' and by_name['Vila Velha']['kind'] == 'place' and by_name['Anel da Ceifa']['kind'] == 'object'
    # the preview of the next book (editorial) never counts as an appearance
    assert all(not (a['book'] == 0 and a['chapter'] == 7) for e in bundle['entities'] for a in e['appearances'])
    assert len(by_name['Ana']['appearances']) == 8
    assert {(e['a'], e['b']) for e in bundle['edges']} >= {(by_name['Ana']['slug'], by_name['Mestre Bruno']['slug'])} or \
        {(e['a'], e['b']) for e in bundle['edges']} >= {(by_name['Mestre Bruno']['slug'], by_name['Ana']['slug'])}
    assert [s for s, _ in progress.stages] == ['map', 'identity', 'pages']


def test_every_sentence_rests_on_a_chapter_that_exists_and_every_chapter_it_cites_is_a_story_one():
    bundle, *_ = run()
    valid = {(b['position'], c['chapter']) for b in bundle['books'] for c in b['chapters'] if not c['editorial']}
    sentences = 0
    for e in bundle['entities']:
        for sec in e['sections']:
            for s in sec['sentences']:
                sentences += 1
                assert s['refs'], (e['slug'], s)
                assert all((r['book'], r['chapter']) in valid for r in s['refs'])
    assert sentences > 0
    assert len({e['slug'] for e in bundle['entities']}) == len(bundle['entities'])


def test_the_cut_decides_who_has_a_full_page_and_everyone_stays_in_the_graph():
    all_pages, *_ = run(cut=2)
    none, *_ = run(cut=100)
    assert sum(1 for e in all_pages['entities'] if e['page']) == 4
    assert sum(1 for e in none['entities'] if e['page']) == 0
    assert len(none['entities']) == len(all_pages['entities']) == 4
    assert len(none['edges']) == len(all_pages['edges']) > 0
    # a page has the sections of the format: who is it, the biography by part, and the relations of the ficha
    ana = next(e for e in all_pages['entities'] if e['name'] == 'Ana')
    assert {s['kind'] for s in ana['sections']} >= {'who', 'bio'}
    assert any(s['part'] == 'Parte I' for s in ana['sections'] if s['kind'] == 'bio')
    assert ana['relations'] and ana['relations'][0]['group'] == 'mentors'
    # an entity with no page still has a ficha, in the group of the others
    cut_none = next(e for e in none['entities'] if e['name'] == 'Ana')
    assert cut_none['page'] is False and cut_none['sections'] == [] and cut_none['states']


def test_what_was_asked_before_is_not_asked_again_not_even_for_a_lower_cut():
    cache = MemoryCache()
    _, _, first, _ = run(cut=100, cache=cache)
    asked_first = len(first.asked)
    assert asked_first > 0
    _, _, again, _ = run(cut=100, cache=cache)
    assert again.asked == [] and again.usage.snapshot()['hits'] >= asked_first
    # a lower cut only asks for the pages that are new
    _, _, lower, _ = run(cut=2, cache=cache)
    assert lower.asked and not any(a.startswith('Você é editor') or a.startswith('Você resolve') for a in lower.asked)


def test_a_pause_stops_the_run_and_what_was_done_is_kept():
    cache = MemoryCache()
    seen = []

    def control():
        seen.append(1)
        if len(seen) > 6:
            raise Stop('paused')
    with pytest.raises(Stop):
        run(cache=cache, control=control)
    kept = len(cache.items)
    assert kept > 0
    _, _, resumed, _ = run(cache=cache)
    assert resumed.usage.snapshot()['hits'] >= kept


def test_a_chapter_that_never_answers_makes_the_wiki_partial_and_is_said():
    class Mute(Script):
        def answer(self, system, user):
            if system.startswith('Você é editor') and 'capítulo 2' in user and 'O Primeiro' in user:
                return {'nada': 1}
            return super().answer(system, user)
    llm = Mute(Provider('openai', 'http://x/v1', 'm'), cache=MemoryCache())
    progress = Progress()
    bundle, partial = generate(llm, books(), 2, progress, lambda: None, 2, 'openai', 'm', False)
    assert partial is True and bundle['partial'] is True
    assert any('sem resposta utilizável' in e for e in progress.errors)
    assert len(bundle['entities']) == 4   # the rest of the book is still there
