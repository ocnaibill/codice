"""Who is who (DEC-181): the decisions of the staff are followed, and what the model cannot prove becomes a suggestion."""
import collections

from wikigen.registry import merge_series
from wikigen.text import key


def entry(i, book, name, kind, caps, notes=()):
    return {'id': i, 'book': book, 'nomes': collections.Counter({name: len(caps)}), 'nome': name, 'tipo': kind, 'caps': list(caps),
            'notas': [{'book': book, 'cap': caps[0], 'papel': '', 'novo': [n], 'estado': '', 'relacoes': []} for n in notes]}


class Model:
    """A model that answers every pair the same way, and keeps what it was asked."""

    def __init__(self, same, proof=''):
        self.same, self.proof, self.asked = same, proof, []

    def ask_json(self, system, user, example, validate, num_predict=1200, timeout=600, retries=2):
        import re
        pairs = sorted({int(n) for n in re.findall(r'^Par (\d+)', user, re.M)})
        self.asked.append(pairs)
        return {'decisoes': [{'par': p, 'mesma': self.same, 'motivo': 'parecem a mesma', 'prova': self.proof} for p in pairs]}, 'ok'


def groups_of(groups):
    return sorted(sorted(n for x in g for n in x['nomes']) for g in groups)


def test_what_the_staff_joined_is_joined_with_no_proof_in_the_notes():
    entries = [entry(0, 0, 'Bea', 'personagem', [1, 2]), entry(1, 1, 'Beatriz', 'personagem', [1, 2]), entry(2, 0, 'Luc', 'personagem', [1, 2])]
    groups, _ = merge_series(Model(False), entries)
    assert groups_of(groups) == [['Bea'], ['Beatriz'], ['Luc']]
    groups, _ = merge_series(Model(False), entries, [{'kind': 'same', 'a': key('Bea'), 'b': key('BEATRIZ')}])
    assert groups_of(groups) == [['Bea', 'Beatriz'], ['Luc']]


def test_what_the_staff_kept_apart_is_never_joined_nor_asked_again():
    # the notes of one say the other is the name it took: the program would join them by itself
    entries = [
        entry(0, 0, 'Addie', 'personagem', [1, 2, 3], ['ela adotou o nome Adeline como seu nome']),
        entry(1, 1, 'Adeline', 'personagem', [1, 2], ['adotou o nome Addie']),
    ]
    groups, _ = merge_series(Model(False), entries)
    assert groups_of(groups) == [['Addie', 'Adeline']]
    model = Model(True, 'adotou o nome Adeline')
    groups, log = merge_series(model, entries, [{'kind': 'distinct', 'a': key('Addie'), 'b': key('Adeline')}])
    assert groups_of(groups) == [['Addie'], ['Adeline']]
    assert model.asked == []          # a pair that was answered is not paid for again
    assert log['sugestoes'] == []


def test_a_pair_the_model_takes_for_the_same_without_proof_is_a_suggestion_not_a_join():
    entries = [
        entry(0, 0, 'A Escuridão', 'conceito', [1, 2, 3], ['depois do pacto a escuridão ganha o nome Lucifer']),
        entry(1, 1, 'Lucifer', 'personagem', [1, 2], ['o nome que ela dá à escuridão']),
    ]
    model = Model(True, 'nenhuma frase que prove')
    groups, log = merge_series(model, entries)
    assert groups_of(groups) == [['A Escuridão'], ['Lucifer']]
    assert [(s['a'], s['b']) for s in log['sugestoes']] == [('A Escuridão', 'Lucifer')]
    assert 'parecem a mesma' in log['sugestoes'][0]['motivo']


def mapped_book(book, order):
    """A book whose chapters were mapped in `order` (the one they finished in), each with the same two entities."""
    def rec(name, kind, note):
        return {'nome': name, 'tipo': kind, 'papel': '', 'novo': [note], 'estado': '', 'relacoes': [], 'verificada': True}
    mapa = {}
    for cap in order:
        ents = [rec('Addie', 'personagem', f'ela adotou o nome Adeline no capítulo {cap}'), rec('A Escuridão', 'conceito', f'a escuridão do capítulo {cap} ganha o nome Lucifer')]
        mapa[cap] = {'chapter': cap, 'resumo': '', 'entidades': ents}
    return {'id': 1, 'titulo': 'L', 'kind': 'main', 'chapters': {c: {'title': f'{c}. x', 'part': ''} for c in order}, 'mapa': mapa}


def test_the_order_the_chapters_finished_in_does_not_change_what_the_model_is_asked():
    """The chapters are mapped in parallel: their order is luck, and the requests are the key of the cache."""
    from wikigen.assemble import build_series

    class Spy(Model):
        def ask_json(self, system, user, *a, **k):
            self.users = getattr(self, 'users', []) + [user]
            return super().ask_json(system, user, *a, **k)

    def asked(order):
        book = mapped_book(1, order)
        other = mapped_book(2, order)
        other['mapa'] = {c: {**m, 'entidades': [{**e, 'nome': 'Adeline' if e['nome'] == 'Addie' else 'Lucifer', 'tipo': 'personagem'} for e in m['entidades']]}
                         for c, m in other['mapa'].items()}
        spy = Spy(False)
        series, _ = build_series(spy, [book, other])
        return getattr(spy, 'users', []), [e['notas'] for e in series['entidades']]

    forward = asked([1, 2, 3, 4])
    assert forward[0], 'the test must make the model be asked something'
    assert asked([4, 2, 1, 3]) == forward
