"""Stage "map": one request per chapter, and a check of what came back against the text.

The model reads a chapter whole and lists the entities that matter in it (who they are, what is new about them, their ties, their state
at the end), each with a literal quote from a paragraph. The program checks the quote in that paragraph and the name in the chapter: an
entity that fails is kept in the map but never used (`verificada` false). Nothing enters the wiki without a quote that was found.
"""
import difflib
import re

from .text import core, est_tokens, fold, norm

MAX_CHAPTER_TOKENS = 12000   # a chapter bigger than this is split in parts, never in the middle of a paragraph

MAP_EXAMPLE = {
    'resumo': 'três a cinco frases sobre o que acontece neste capítulo',
    'entidades': [{
        'nome': 'Nome como aparece no texto', 'tipo': 'personagem',
        'papel': 'quem ou o que é, em uma frase: ocupação, posição, parentesco ou função, se o texto disser',
        'novo': ['fato estável que este capítulo revela sobre ela'],
        'relacoes': [{'com': 'outra entidade', 'relacao': 'mentor de / irmã de / membro de ...'}],
        'estado': 'situação ao fim do capítulo, só se mudou',
        'citacao': {'id': 'p12', 'texto': 'cópia literal de até 25 palavras do parágrafo p12'},
        'outros_nomes': [{'nome': 'outro nome que ESTE capítulo usa para ela',
                          'citacao': {'id': 'p7', 'texto': 'cópia literal do parágrafo p7 em que o texto mostra que é a mesma'}}]}],
    'eventos': [{'descricao': 'o que aconteceu', 'participantes': ['nome'], 'id': 'p30'}],
}

SYSTEM = ('Você é editor de uma wiki de ficção. Lê um capítulo e registra o que ele revela, sem inventar. Responda só com JSON.')


# DEC-189: the other names an entity has in this chapter, each with the copy that shows it, and the program checks the copy.
OTHER_NAMES = (' Para cada entidade, liste também em "outros_nomes" os outros nomes que ESTE capítulo usa para ela: apelido, nome completo, '
               'nome antigo ou novo, título, ou a forma comum com que o texto se refere a ela (por exemplo, "o sebo" para uma livraria que '
               'tem nome). Para cada um, copie literalmente o trecho de um parágrafo em que o texto mostra que é a mesma pessoa ou coisa, '
               'com os dois nomes no trecho. Deixe a lista vazia se o capítulo não usar outro nome. Não ponha como outro nome uma entidade '
               'diferente (um parente, um amigo, o dono de um lugar), uma comparação ou uma pergunta do texto ("um monstro", "um deus") '
               'nem uma descrição que serviria a outra coisa do capítulo ("a garota", "o anel").')


def clean_other_names(e):
    """The list of other names as the model sent it, only the well-formed items: a bad item is lost, never the entity or the chapter."""
    items = e.get('outros_nomes')
    e['outros_nomes'] = [o for o in (items if isinstance(items, list) else [])
                         if isinstance(o, dict) and isinstance(o.get('nome'), str) and o['nome'].strip()]


def validate_map(d):
    if not isinstance(d, dict) or not isinstance(d.get('resumo'), str) or not isinstance(d.get('entidades'), list):
        return "faltam 'resumo' (texto) e 'entidades' (lista)"
    if any(not isinstance(e, dict) for e in d['entidades']):
        return 'cada entidade deve ser um objeto'
    d['entidades'] = [e for e in d['entidades'] if isinstance(e.get('nome'), str) and e['nome'].strip()]
    for e in d['entidades']:
        clean_other_names(e)
    # the quote is what lets the program check what was said: with it missing in most entities, the whole chapter is lost in the check
    cited = sum(1 for e in d['entidades'] if isinstance(e.get('citacao'), dict) and e['citacao'].get('id') and e['citacao'].get('texto'))
    if d['entidades'] and cited < 0.6 * len(d['entidades']):
        return "a maioria das entidades está sem 'citacao' (com 'id' do parágrafo e 'texto' copiado dele)"
    return None


def chapter_prompt(book_title, chapter, part_no, parts):
    ids = chapter.get('paragraph_ids') or list(range(len(chapter['paragraphs'])))   # the number is of the whole chapter, not of the part
    lines = []
    for i, p in zip(ids, chapter['paragraphs']):
        label = 'epígrafe' if i < chapter['epigraph_paragraphs'] else ''
        lines.append(f'[p{i}]{" (" + label + ")" if label else ""} {p}')
    where = f' (parte {part_no} de {parts} do capítulo)' if parts > 1 else ''
    part = f" {chapter['part']}." if chapter.get('part') else ''
    return (f"Livro: {book_title}.\nCapítulo {chapter['index']}: {chapter['title']}{where}.{part}\n\n"
            'Texto, com o número de cada parágrafo:\n' + '\n'.join(lines) +
            '\n\nTarefa: registre o que este capítulo mostra, para uma wiki do livro. Liste até 15 entidades que importam (personagens '
            'primeiro; também lugares, organizações, cargos, objetos e conceitos que o capítulo apresenta ou desenvolve). Para cada uma: '
            'quem ou o que é (ocupação, posição, parentesco, função: o que a define, não só o que ela fez nesta cena), o que o capítulo '
            'revela de novo sobre ela, suas relações com outras entidades e seu estado ao fim do capítulo. Dê a citação literal de um '
            'parágrafo que sustenta o que você disse, no idioma do livro. Use só o que está no texto; não complete com o que sabe de fora. '
            'Ações de cena ficam em "eventos", curtas.' + OTHER_NAMES)


def split_chapter(chapter):
    parts, cur, size = [], [], 0
    for i, p in enumerate(chapter['paragraphs']):
        t = est_tokens(p)
        if cur and size + t > MAX_CHAPTER_TOKENS:
            parts.append(cur)
            cur, size = [], 0
        cur.append(i)
        size += t
    if cur:
        parts.append(cur)
    return parts


def map_chapter(llm, book_title, chapter, max_out=3500):
    """{'chapter', 'resumo', 'entidades', 'falhou'}; `falhou` is true when some part of the chapter got no usable answer."""
    records, summaries, failed = [], [], False
    parts = split_chapter(chapter)
    for n, idxs in enumerate(parts, 1):
        sub = dict(chapter)
        sub['paragraphs'] = [chapter['paragraphs'][i] for i in idxs]
        sub['paragraph_ids'] = idxs
        data, status = llm.ask_json(SYSTEM, chapter_prompt(book_title, sub, n, len(parts)), MAP_EXAMPLE, validate_map, num_predict=max_out, timeout=900)
        if data is None:
            failed = True
            continue
        summaries.append(data['resumo'])
        for e in data['entidades']:
            e['_parte'] = idxs
            records.append(e)
    return {'chapter': chapter['index'], 'resumo': ' '.join(summaries), 'entidades': records, 'falhou': failed}


def quote_in(quote, text):
    """The copy is in the text: whole, or 80% of it (and at least 20 characters) in a row. The model cuts, changes punctuation, adds an ellipsis."""
    if quote in text:
        return True
    match = difflib.SequenceMatcher(None, quote, text, autojunk=False).find_longest_match(0, len(quote), 0, len(text))
    return match.size >= max(20, 0.8 * len(quote))


def stem_in(token, words):
    """A word of the text that is the token or starts like it ("Bea" in "Beatrice", and the other way round): a short form of a name."""
    return any(w == token or (len(token) >= 3 and len(w) >= 3 and (w.startswith(token) or token.startswith(w))) for w in words)


def check_alias(chapter, entity, other):
    """(ok, reason) for one other name: the copy is in the paragraph it points to (or in it and the next, a copy may cross the break), it
    has the other name, and the name of the entity is in those paragraphs, whole or in a short form. It proves the two names are written
    together there; whether they are the same is the model's reading, which the copy lets a person read too."""
    cit = other.get('citacao') if isinstance(other.get('citacao'), dict) else {}
    m = re.search(r'\d+', str(cit.get('id', '')))
    quote = str(cit.get('texto', ''))
    if not m or not quote.strip():
        return False, 'sem citação'
    i = int(m.group())
    paragraphs = chapter['paragraphs']
    if i >= len(paragraphs):
        return False, f'parágrafo p{i} não existe'
    q = norm(quote)
    near = ' '.join(paragraphs[max(0, i - 1):i + 2])
    if not (quote_in(q, norm(paragraphs[i])) or quote_in(q, norm(near))):
        return False, f'a cópia não está em p{i}'
    words = re.findall(r'[a-z0-9]+', fold(quote))
    if not any(t in words for t in core(other['nome'])):
        return False, 'o outro nome não está no trecho'
    if not any(stem_in(t, re.findall(r'[a-z0-9]+', fold(near))) for t in core(entity)):
        return False, 'o nome da entidade não está perto do trecho'
    if set(core(other['nome'])) <= set(core(entity)):
        return False, 'é o mesmo nome (já se junta por regra)'
    return True, 'conferido'


def verify_entities(chapter, mapped):
    """Each entity gets `verificada`: the quote is in the paragraph it points to, and the name (or part of it) is in the chapter."""
    text = norm(' '.join(chapter['paragraphs']))
    for e in mapped['entidades']:
        cit = e.get('citacao') if isinstance(e.get('citacao'), dict) else {}
        m = re.search(r'\d+', str(cit.get('id', '')))
        quote = norm(str(cit.get('texto', '')))
        ok_quote = False
        if m and int(m.group(0)) < len(chapter['paragraphs']) and len(quote) >= 8:
            ok_quote = quote_in(quote, norm(chapter['paragraphs'][int(m.group(0))]))
        words = [w for w in norm(e['nome']).split() if len(w) > 2]
        e['citacao_ok'] = ok_quote
        e['verificada'] = ok_quote and any(w in text for w in words)
        if m:
            e['paragrafo'] = int(m.group(0))
        checked = []
        for o in e.get('outros_nomes') or []:
            ok, why = check_alias(chapter, e['nome'], o) if e['verificada'] else (False, 'a entidade não foi verificada')
            cit = o.get('citacao') if isinstance(o.get('citacao'), dict) else {}
            checked.append({'nome': o['nome'].strip(), 'conferido': ok, 'motivo': why, 'citacao': str(cit.get('texto', ''))[:300]})
        e['outros_nomes'] = checked
    return mapped
