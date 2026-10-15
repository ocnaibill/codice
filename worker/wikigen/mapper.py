"""Stage "map": one request per chapter, and a check of what came back against the text.

The model reads a chapter whole and lists the entities that matter in it (who they are, what is new about them, their ties, their state
at the end), each with a literal quote from a paragraph. The program checks the quote in that paragraph and the name in the chapter: an
entity that fails is kept in the map but never used (`verificada` false). Nothing enters the wiki without a quote that was found.
"""
import difflib
import re

from .text import est_tokens, norm

MAX_CHAPTER_TOKENS = 12000   # a chapter bigger than this is split in parts, never in the middle of a paragraph

MAP_EXAMPLE = {
    'resumo': 'três a cinco frases sobre o que acontece neste capítulo',
    'entidades': [{
        'nome': 'Nome como aparece no texto', 'tipo': 'personagem',
        'papel': 'quem ou o que é, em uma frase: ocupação, posição, parentesco ou função, se o texto disser',
        'novo': ['fato estável que este capítulo revela sobre ela'],
        'relacoes': [{'com': 'outra entidade', 'relacao': 'mentor de / irmã de / membro de ...'}],
        'estado': 'situação ao fim do capítulo, só se mudou',
        'citacao': {'id': 'p12', 'texto': 'cópia literal de até 25 palavras do parágrafo p12'}}],
    'eventos': [{'descricao': 'o que aconteceu', 'participantes': ['nome'], 'id': 'p30'}],
}

SYSTEM = ('Você é editor de uma wiki de ficção. Lê um capítulo e registra o que ele revela, sem inventar. Responda só com JSON.')


def validate_map(d):
    if not isinstance(d, dict) or not isinstance(d.get('resumo'), str) or not isinstance(d.get('entidades'), list):
        return "faltam 'resumo' (texto) e 'entidades' (lista)"
    if any(not isinstance(e, dict) for e in d['entidades']):
        return 'cada entidade deve ser um objeto'
    d['entidades'] = [e for e in d['entidades'] if isinstance(e.get('nome'), str) and e['nome'].strip()]
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
            'Ações de cena ficam em "eventos", curtas.')


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


def verify_entities(chapter, mapped):
    """Each entity gets `verificada`: the quote is in the paragraph it points to, and the name (or part of it) is in the chapter."""
    text = norm(' '.join(chapter['paragraphs']))
    for e in mapped['entidades']:
        cit = e.get('citacao') if isinstance(e.get('citacao'), dict) else {}
        m = re.search(r'\d+', str(cit.get('id', '')))
        quote = norm(str(cit.get('texto', '')))
        ok_quote = False
        if m and int(m.group(0)) < len(chapter['paragraphs']) and len(quote) >= 8:
            para = norm(chapter['paragraphs'][int(m.group(0))])
            if quote in para:
                ok_quote = True
            else:
                # the model cuts, changes punctuation or adds an ellipsis: 80% of the quote (and at least 20 characters) in a row is enough
                match = difflib.SequenceMatcher(None, quote, para, autojunk=False).find_longest_match(0, len(quote), 0, len(para))
                ok_quote = match.size >= max(20, 0.8 * len(quote))
        words = [w for w in norm(e['nome']).split() if len(w) > 2]
        e['citacao_ok'] = ok_quote
        e['verificada'] = ok_quote and any(w in text for w in words)
        if m:
            e['paragrafo'] = int(m.group(0))
    return mapped
