"""The other names of an entity (DEC-189): checked against the paragraph, a name joins and an epithet only says (ensaio da Addie, 16/10/2026)."""
import collections

from wikigen.mapper import check_alias, clean_other_names, validate_map, verify_entities
from wikigen.registry import build_registry, merge_series
from wikigen.text import common_words, is_proper

COMMON = frozenset({'sebo', 'garota', 'anel', 'sombra', 'velha', 'senhora', 'gato', 'diabo', 'monstro', 'deus', 'velho'})


def chapter(*paragraphs):
    return {'index': 1, 'title': 'I', 'paragraphs': list(paragraphs), 'epigraph_paragraphs': 0}


def alias(name, pid, text):
    return {'nome': name, 'citacao': {'id': pid, 'texto': text}}


def test_a_name_is_told_from_an_epithet():
    for name in ['Addie LaRue', 'Beatrice', 'Henry Samuel', 'Adeline', 'A Última Palavra']:
        assert is_proper(name, COMMON), name
    for name in ['o sebo', 'a garota', 'o anel', 'um monstro', 'o velho gato do sebo', 'a sombra', 'A sombra']:
        assert not is_proper(name, COMMON), name


def test_the_copy_must_be_in_the_paragraph_and_have_the_other_name():
    ch = chapter('Nada aqui.', 'Adeline é a mulher que ela deixou para trás. Mas Addie… Addie foi um presente que ganhou de Estele.', 'Outra.', 'Mais outra.')
    ok = alias('Addie LaRue', 'p1', 'Mas Addie… Addie foi um presente que ganhou de Estele')
    assert check_alias(ch, 'Adeline LaRue', ok) == (True, 'conferido')   # one word of the other name is enough: "Addie LaRue" is written "Addie"
    assert check_alias(ch, 'Adeline LaRue', alias('Addie', 'p3', 'Mas Addie… Addie foi um presente'))[1] == 'a cópia não está em p3'
    assert check_alias(ch, 'Adeline LaRue', alias('Lucie', 'p1', 'Mas Addie… Addie foi um presente que ganhou de Estele'))[1] == 'o outro nome não está no trecho'
    assert check_alias(ch, 'Adeline LaRue', alias('Addie', 'p9', 'x'))[1] == 'parágrafo p9 não existe'
    assert check_alias(ch, 'Adeline LaRue', {'nome': 'Addie'})[1] == 'sem citação'
    assert check_alias(ch, 'Estele Magritte', alias('Estele', 'p1', 'Addie foi um presente que ganhou de Estele'))[1] == 'é o mesmo nome (já se junta por regra)'


def test_the_name_of_the_entity_may_be_near_the_copy_and_a_short_form_counts():
    # "Bea" is not written in the copy, which says "Beatrice Helen": the paragraph before has her, and "Bea" is how she is called
    ch = chapter('Bea ri, e Henry também.', '— Beatrice Helen — repreende ele.')
    assert check_alias(ch, 'Bea', alias('Beatrice Helen', 'p1', '— Beatrice Helen — repreende ele.')) == (True, 'conferido')
    far = chapter('Fala de outra pessoa.', 'Outra cena.', 'Mais outra.', 'Mais uma.', '— Beatrice Helen — repreende ele.')
    assert check_alias(far, 'Gerald', alias('Beatrice Helen', 'p4', '— Beatrice Helen — repreende ele.'))[1] == 'o nome da entidade não está perto do trecho'


def test_a_copy_that_crosses_the_break_between_two_paragraphs_is_found():
    ch = chapter('Ela olha para o mar. Luc', 'responde baixo. “Sombra”, diz ela, e ele sorri.')
    assert check_alias(ch, 'Luc', alias('Sombra', 'p0', 'Luc responde baixo. “Sombra”, diz ela'))[0]


def test_a_bad_item_of_the_list_is_lost_and_not_the_entity():
    d = {'resumo': 'x', 'entidades': [{'nome': 'Addie', 'citacao': {'id': 'p0', 'texto': 'texto'}, 'outros_nomes': ['Adeline', {'nome': ''}, {'nome': 'Adeline'}, 7]}]}
    assert validate_map(d) is None
    assert [o['nome'] for o in d['entidades'][0]['outros_nomes']] == ['Adeline']
    e = {'nome': 'Addie', 'outros_nomes': 'Adeline'}
    clean_other_names(e)
    assert e['outros_nomes'] == []


def mapped(ch, *entities):
    return verify_entities(ch, {'chapter': 1, 'resumo': '', 'entidades': list(entities), 'falhou': False})


def test_verify_entities_checks_each_other_name_and_not_the_unverified():
    ch = chapter('Addie caminha por Villon-sur-Sarthe. Antes ela era Adeline, em Villon.')
    good = {'nome': 'Addie', 'citacao': {'id': 'p0', 'texto': 'Addie caminha por Villon-sur-Sarthe'}, 'outros_nomes': [alias('Adeline', 'p0', 'Antes ela era Adeline, em Villon')]}
    ghost = {'nome': 'Fantasma', 'citacao': {'id': 'p0', 'texto': 'texto que não existe aqui'}, 'outros_nomes': [alias('Espectro', 'p0', 'Antes ela era Adeline')]}
    out = mapped(ch, good, ghost)['entidades']
    assert out[0]['outros_nomes'] == [{'nome': 'Adeline', 'conferido': True, 'motivo': 'conferido', 'citacao': 'Antes ela era Adeline, em Villon'}]
    assert out[1]['outros_nomes'][0]['motivo'] == 'a entidade não foi verificada'


def rec(name, kind='personagem', others=(), cap=1):
    return {'nome': name, 'tipo': kind, 'papel': '', 'novo': ['fato'], 'estado': '', 'relacoes': [], 'verificada': True, 'outros_nomes': list(others)}


def conferido(name, quote='citação'):
    return {'nome': name, 'conferido': True, 'motivo': 'conferido', 'citacao': quote}


def book(*caps):
    return {'mapa': {cap: {'entidades': ents} for cap, ents in caps}}


def groups_of(groups):
    return sorted(sorted(n for x in g for n in x['nomes']) for g in groups)


def test_a_name_the_chapter_gave_joins_the_entities_that_carry_it_with_the_copy_as_the_reason():
    b = book((1, [rec('Adeline LaRue', others=[conferido('Addie', 'Mas Addie… Addie foi um presente')])]), (30, [rec('Addie')]), (31, [rec('Addie')]))
    entries = build_registry([b], COMMON)
    assert [e['aliases'] for e in entries if 'Adeline LaRue' in e['nomes']] == [{'Addie': {'citacao': 'Mas Addie… Addie foi um presente', 'cap': 1}}]
    groups, log = merge_series(None, entries, (), COMMON)
    assert len(groups) == 1
    assert 'diz que também é "Addie"' in log['fusoes'][0]['por']


def test_an_epithet_is_kept_to_be_said_and_never_joins():
    b = book((1, [rec('Addie', others=[conferido('a garota', 'uma garota que já viveu trezentos anos')]), rec('Ana')]), (2, [rec('a Garota')]), (3, [rec('a Garota')]))
    entries = build_registry([b], COMMON)
    addie = next(e for e in entries if 'Addie' in e['nomes'])
    assert addie['aliases'] == {} and [x['nome'] for x in addie['epitetos']] == ['a garota']
    groups, _ = merge_series(None, entries, (), COMMON)
    assert not any({'Addie', 'a Garota'} <= {n for x in g for n in x['nomes']} for g in groups)


def test_an_epithet_that_is_an_entity_of_the_chapter_is_not_one_for_the_entity():
    b = book((1, [rec('Addie', others=[conferido('o anel')]), rec('o anel', 'objeto')]))
    entries = build_registry([b], COMMON)
    assert next(e for e in entries if 'Addie' in e['nomes'])['epitetos'] == []


def test_a_name_not_conferred_is_lost_and_a_staff_decision_keeps_the_names_apart():
    b = book((1, [rec('Henry Strauss', others=[{'nome': 'Henry Samuel', 'conferido': False, 'motivo': 'x', 'citacao': ''}])]), (5, [rec('Henry Samuel')]))
    entries = build_registry([b], COMMON)
    assert all(not e['aliases'] for e in entries)
    b = book((1, [rec('Henry Strauss', others=[conferido('Henry Samuel')])]), (5, [rec('Henry Samuel')]))
    entries = build_registry([b], COMMON)
    from wikigen.text import key
    groups, _ = merge_series(None, entries, [{'kind': 'distinct', 'a': key('Henry Strauss'), 'b': key('Henry Samuel')}], COMMON)
    assert groups_of(groups) == [['Henry Samuel'], ['Henry Strauss']]
    groups, _ = merge_series(None, entries, (), COMMON)
    assert len(groups) == 1


def test_common_words_come_from_the_text_so_that_a_name_is_not_taken_for_a_noun():
    common = common_words(['o sebo fechou. a garota riu.', 'Addie viu o sebo e a garota.', 'Addie sorriu.'])
    assert is_proper('Addie', common) and not is_proper('o sebo', common) and not is_proper('a garota', common)
