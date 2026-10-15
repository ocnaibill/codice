"""Stage "pages": the page of an entity, written once from numbered facts.

Each section is written ONCE from facts that carry their chapter (the lines of the map and the notes of a search in the text, with a quote
the program checked), and every sentence says which facts it rests on. A second pass checks sentence by sentence (same subject, nothing
added) and drops what does not hold. The biography is one subsection for each part of the book; a later book adds subsections and the
earlier ones do not change. "Quem é" is written from scratch at the end of each part, from the checked biography up to there, never from
the previous "Quem é", which made it shrink and lose facts in the laboratory.
"""
import collections
import concurrent.futures
import difflib
import re

from .text import core, fold, norm

HITS_PER_BOOK = 70
MAX_CHARS = 600
CATS = ('evento', 'relacao', 'identidade', 'personalidade', 'aparencia', 'habilidade')
MAX_FACTS = 90

def alias_regex(names):
    # three letters are enough (Luc, Bea): the pattern already asks for word boundaries.
    words = sorted({fold(n).strip() for n in names if len(fold(n).strip()) >= 3}, key=len, reverse=True)
    return re.compile(r'(?<![a-z0-9])(' + '|'.join(re.escape(w) for w in words) + r')(?![a-z0-9])') if words else None


def spread_hits(hits, n):
    """Up to n paragraphs: at least one for each chapter when they fit, then the rest spread over the book."""
    if len(hits) <= n:
        return hits
    first = {}
    for h in hits:
        first.setdefault(h[0], h)
    picks = list(first.values())
    if len(picks) >= n:
        return [picks[int(i * len(picks) / n)] for i in range(n)]
    rest = [h for h in hits if h not in set(picks)]
    need = n - len(picks)
    picks += [rest[int(i * len(rest) / need)] for i in range(need)] if rest else []
    return sorted(set(picks))


def quote_ok(quote, para):
    q, p = norm(quote), norm(para)
    if len(q) < 8:
        return False
    if q in p:
        return True
    m = difflib.SequenceMatcher(None, q, p, autojunk=False).find_longest_match(0, len(q), 0, len(p))
    return m.size >= max(20, .8 * len(q))




# ---------------------------------------------------------------- notas da busca (guardadas)
NOTE_EXAMPLE = {'existe': True, 'notas': [{'id': 'c3p12', 'cap': 3, 'categoria': 'evento',
                                           'nota': 'Citra recusou o convite para ser aprendiz.', 'citacao': 'cópia literal de até 25 palavras'}]}


def validate_notes(d):
    if not isinstance(d, dict) or not isinstance(d.get('existe'), bool) or not isinstance(d.get('notas', []), list):
        return "faltam 'existe' (verdadeiro/falso) e 'notas' (lista)"
    for n in d.get('notas', []):
        if not isinstance(n, dict) or not isinstance(n.get('nota'), str) or not isinstance(n.get('id'), str):
            return "cada nota precisa de 'nota' e 'id' em texto"
    return None


def search_notes(llm, book, bi, name, aliases):
    rx = alias_regex([name] + list(aliases))
    hits = [(ci, pi) for ci, c in book['chapters'].items() for pi, p in enumerate(c['paragraphs']) if rx and rx.search(fold(p))]
    if not hits:
        return [], {'hits': 0}
    picks = spread_hits(hits, HITS_PER_BOOK)
    texts, lines = {}, []
    for ci, pi in picks:
        sid = f'c{ci}p{pi}'
        texts[sid] = book['chapters'][ci]['paragraphs'][pi]
        lines.append(f"[{sid}] (cap. {ci}: {book['chapters'][ci]['title']}) {texts[sid][:MAX_CHARS]}")
    shown = [a for a in aliases if a != name][:8]
    user = (f"Livro: {book['titulo']}.\nEntidade: {name}" + (f" (também: {', '.join(shown)})" if shown else '') +
            '\n\nTrechos em que ela aparece, em ordem do livro:\n' + '\n'.join(lines) +
            '\n\nLevante o que os trechos mostram sobre a entidade. Cada nota é UM fato curto, com o sujeito escrito por extenso (o nome, não '
            '"ela"), com o id do trecho, a citação literal e a categoria: evento (algo que ela faz ou que lhe acontece), relacao (como se liga a '
            'alguém ou a algo), identidade (o que ou quem é, o que a define), personalidade (como é, o que valoriza, como age), aparencia '
            '(como é vista, o corpo, o lugar), habilidade (o que sabe ou pode fazer). Atenção: se o trecho trata de OUTRA entidade com nome '
            'próprio (outra pessoa, outra inteligência, uma instituição), o fato é dessa outra: não o escreva como da entidade. '
            'No máximo 40 notas, em ordem do livro, cobrindo o livro todo.')
    data, status = llm.ask_json('Você levanta, em um livro, o que ele diz de uma entidade, só com os trechos dados. Responda só com JSON.',
                                user, NOTE_EXAMPLE, validate_notes, num_predict=5000, timeout=900)
    info = {'hits': len(hits), 'usados': len(picks), 'status': status}
    notes, bad = [], 0
    for n in (data or {}).get('notas', []) if (data or {}).get('existe') else []:
        found = re.search(r'c(\d+)p(\d+)', str(n.get('id', '')))
        sid = found.group(0) if found else ''
        if sid in texts and quote_ok(str(n.get('citacao', '')), texts[sid]):
            cat = fold(n.get('categoria', ''))
            notes.append({'book': bi, 'cap': int(found.group(1)), 'nota': n['nota'].strip(), 'id': sid,
                          'categoria': cat if cat in CATS else 'evento', 'citacao': str(n.get('citacao'))})
        else:
            bad += 1
    info['rejeitadas'] = bad
    return notes, info


# ---------------------------------------------------------------- escrita com fatos citados
WRITE_EXAMPLE = {'frases': [{'par': 1, 'texto': 'Frase de enciclopédia, em terceira pessoa.', 'fatos': ['F3', 'F7']},
                            {'par': 1, 'texto': 'Outra frase do mesmo parágrafo.', 'fatos': ['F8']},
                            {'par': 2, 'texto': 'Primeira frase de outro parágrafo.', 'fatos': ['F12']}]}
CHECK_EXAMPLE = {'frases': [{'n': 1, 'ok': True, 'problema': ''}, {'n': 2, 'ok': False, 'problema': 'o fato fala do Cirro, não dela'}]}


def validate_write(d):
    if not isinstance(d, dict) or not isinstance(d.get('frases'), list) or not d['frases']:
        return "falta 'frases' (lista de {texto, fatos})"
    for f in d['frases']:
        if not isinstance(f, dict) or not isinstance(f.get('texto'), str) or not isinstance(f.get('fatos'), list):
            return "cada frase precisa de 'texto' e 'fatos' (lista de ids como F3)"
    return None


def validate_check(d):
    if not isinstance(d, dict) or not isinstance(d.get('frases'), list):
        return "falta 'frases'"
    for f in d['frases']:
        if not isinstance(f, dict) or not isinstance(f.get('ok'), bool):
            return "cada item precisa de 'n' e 'ok' (verdadeiro/falso)"
    return None


def facts_block(facts):
    return '\n'.join(f"{fid} [{f['onde']}] {f['texto']}" for fid, f in facts)


def write_section(llm, who, scope, facts, instruction, max_sentences, verify=True):
    """facts: lista de (id, {texto, onde, cap, book}). Devolve (frases conferidas, descartadas)."""
    if not facts:
        return [], []
    user = (f'Entidade: {who}\nEscopo: {scope}\n\nFatos numerados (a única fonte permitida):\n{facts_block(facts)}\n\n{instruction}\n'
            f'No máximo {max_sentences} frases, agrupadas em parágrafos ("par" = número do parágrafo). Cada frase cita em "fatos" os ids em que '
            'se apoia. Regras: escreva só o que os fatos dizem, sem acrescentar causa, tempo ou lugar; mantenha o sujeito de cada fato (se o '
            'fato fala de outro nome, a frase fala desse nome); NUNCA escreva "no capítulo N" nem cite capítulos no texto; não repita o que já '
            'disse; texto corrido e fluente, juntando fatos que se ligam, não uma lista de fatos soltos; depois da primeira vez, chame a '
            'entidade pelo nome curto ou pelo apelido, sem repetir o nome completo em toda frase; português do Brasil.')
    data, status = llm.ask_json('Você escreve uma seção de enciclopédia sobre uma entidade de um livro, só com os fatos dados. '
                                'Responda só com JSON.', user, WRITE_EXAMPLE, validate_write, num_predict=2500, timeout=600)
    if not data:
        return [], []
    byid = dict(facts)
    drafted = []
    for f in data['frases']:
        ids = [x for x in (str(i).strip() for i in f['fatos']) if x in byid]
        text = f['texto'].strip()
        if text and ids:
            try:
                par = int(f.get('par', 1))
            except (TypeError, ValueError):
                par = 1
            drafted.append({'par': par, 'texto': text, 'fatos': ids})
    if not verify:
        return drafted, []
    return check_sentences(llm, who, byid, drafted)


def check_sentences(llm, who, byid, drafted):
    if not drafted:
        return [], []
    lines = []
    for i, f in enumerate(drafted, 1):
        lines.append(f'Frase {i}: "{f["texto"]}"\n   fatos citados: ' + ' | '.join(f"{x}: {byid[x]['texto']}" for x in f['fatos']))
    user = (f'Entidade da página: {who}\n\n' + '\n'.join(lines) +
            '\n\nConfira cada frase contra os fatos que ela cita. ok=true só se (1) tudo o que a frase afirma está nos fatos citados; (2) o '
            'SUJEITO é o mesmo: a frase fala da entidade e o fato fala da entidade, não de outra pessoa, de outra inteligência, de uma '
            'instituição; (3) a frase não acrescenta causa, tempo, lugar, número ou intenção que os fatos não dizem. Caso contrário ok=false e '
            '"problema" diz em poucas palavras o que não bate.')
    data, status = llm.ask_json('Você confere frases de uma wiki contra os fatos que elas citam. Responda só com JSON.', user,
                                CHECK_EXAMPLE, validate_check, num_predict=1500, timeout=600)
    verdict = {}
    for f in (data or {}).get('frases', []):
        try:
            verdict[int(f.get('n'))] = f
        except (TypeError, ValueError):
            continue
    kept, dropped = [], []
    for i, f in enumerate(drafted, 1):
        v = verdict.get(i)
        if v is None or v.get('ok') is True:
            kept.append(f)
        else:
            dropped.append({**f, 'problema': str(v.get('problema', ''))})
    return kept, dropped


# ---------------------------------------------------------------- dados da entidade
def entity_facts(ent, notes, bi, first, last, with_cats):
    """The facts of a part: the state and the "new" of the lines of the map by chapter, plus the notes of the search in the categories asked."""
    out, seen = [], set()

    def add(text, cap, kind, quote=None):
        key = fold(re.sub(r'\W+', ' ', text))
        if key and key not in seen:
            seen.add(key)
            out.append({'texto': text.strip(), 'cap': cap, 'book': bi, 'onde': f'cap. {cap}', 'tipo': kind, 'citacao': quote})
    for r in sorted((r for r in ent['linha'] if r['book'] == bi and first <= r['cap'] <= last), key=lambda r: r['cap']):
        if r.get('estado'):
            add(f"(estado) {r['estado']}", r['cap'], 'estado')
        for n in r.get('novo') or []:
            add(str(n), r['cap'], 'novo')
    for n in notes:
        if n['book'] == bi and first <= n['cap'] <= last and n['categoria'] in with_cats:
            add(n['nota'], n['cap'], 'busca', n.get('citacao'))
    out.sort(key=lambda f: f['cap'])
    if len(out) > MAX_FACTS:
        keep = [f for f in out if f['tipo'] == 'estado']
        rest = [f for f in out if f['tipo'] != 'estado']
        step = len(rest) / max(1, MAX_FACTS - len(keep))
        out = sorted(keep + [rest[int(i * step)] for i in range(MAX_FACTS - len(keep))], key=lambda f: f['cap'])
    return [(f'F{i}', f) for i, f in enumerate(out, 1)]


REL_EXAMPLE = {'grupos': [{'grupo': 'Família', 'itens': [{'com': 'Jenny Terranova', 'papel': 'mãe'}]},
                          {'grupo': 'Mentores', 'itens': [{'com': 'Faraday', 'papel': 'primeiro mentor'}]}]}


def validate_rel(d):
    if not isinstance(d, dict) or not isinstance(d.get('grupos'), list):
        return "falta 'grupos'"
    for g in d['grupos']:
        if not isinstance(g, dict) or not isinstance(g.get('itens'), list):
            return "cada grupo precisa de 'grupo' e 'itens'"
    return None


def canonical_names(registro):
    """name (without titles) → head of the entity of the series, only when it is unambiguous; and the kind of each head."""
    idx, kind, weight = collections.defaultdict(set), {}, {}
    for r in registro:
        kind[r['nome']] = r['tipo']
        weight[r['nome']] = sum(len(c) for c in r['caps'].values())
        for n in [r['nome']] + list(r['aliases']):
            c = core(n)
            if c:
                idx[c].add(r['nome'])
    return idx, kind, weight


def resolve(name, idx, weight=None):
    c = core(name)
    if len(c) == 1:   # "Faraday", "Henry": a longer name that holds it (the only one, or the one that dominates in chapters)
        owners = {h for k, hs in idx.items() if len(k) > 1 and c[0] in k for h in hs}
        if len(owners) == 1:
            return next(iter(owners))
        if len(owners) > 1 and weight:
            ranked = sorted(owners, key=lambda h: -weight.get(h, 0))
            if weight.get(ranked[0], 0) >= 3 * max(weight.get(ranked[1], 0), 1):
                return ranked[0]
    if c in idx and len(idx[c]) == 1:
        return next(iter(idx[c]))
    return name


ROMANCE = re.compile(r'namor|amante|romanc|interesse rom|apaix|casad|noiv|marido|espos|beij|parceir|ex-parceiro|ex-namor|atra[cç]|paix[aã]o')
FAMILIA = re.compile(r'm[aã]e|pai\b|irm[aã]|filh|marido|espos|tio|tia|av[oó]|prim[oa]|sobrinh|cunhad|sogr|genro|nora|famil|pais\b|parente|neto|neta')


def relation_groups(llm, ent, registro):
    """Ties in both directions: what the map recorded in the entity's own records ("she is ... of X") and in the records of the others who
    cite her ("X is ... of hers"). Looking only at her side left a lover as "customer and potential thief" (the first meeting), with the
    romance recorded only on his side."""
    idx, kind, weight = canonical_names(registro)
    own = {core(n) for n in [ent['nome']] + list(ent['aliases'])}
    by = collections.OrderedDict()
    for sentido, key in (('ela', 'relacoes'), ('ele', 'relacoes_inversas')):
        for r in ent.get(key) or []:
            if core(r['com']) in own or not r['com'][:1].isupper():   # "humanity", "people": not a name
                continue
            com = resolve(r['com'], idx, weight)
            by.setdefault(com, []).append({**r, 'sentido': sentido})
    # objects, concepts and events tell the life of the universe (a wooden ring), but cited in passing they are noise ("wine", "a wool
    # coat"): they come in with 3 records or more; a name outside the registry, with 2 or more
    THINGS = ('objeto', 'conceito', 'evento')
    by = collections.OrderedDict((k, v) for k, v in by.items()
                                 if (kind.get(k) in THINGS and len(v) >= 3) or (kind.get(k) not in THINGS and (k in kind or len(v) >= 2)))
    if not by:
        return []
    # the 40 that show the most: a page with 100 ties is not a ficha
    ranked = sorted(by.items(), key=lambda kv: -len(kv[1]))[:40]
    by = collections.OrderedDict(sorted(ranked, key=lambda kv: min(r['book'] * 1000 + r['cap'] for r in kv[1])))
    lines = []
    for com, rs in by.items():
        parts = []
        for sentido, rotulo in (('ele', 'ELE é para a entidade'), ('ela', 'a ENTIDADE é para ele')):
            freq = collections.Counter(r['relacao'] for r in rs if r['sentido'] == sentido)
            if freq:
                parts.append(f"{rotulo}: " + ' / '.join(t for t, _ in freq.most_common(3)))
        lines.append(f"- {com} [{kind.get(com, '?')}] ({len(rs)} registros, cap. {min(r['cap'] for r in rs)} a {max(r['cap'] for r in rs)}): " + ' | '.join(parts))
    user = (f"Entidade: {ent['nome']}\n\nRelações observadas (pessoa [tipo]):\n" + '\n'.join(lines) +
            '\n\nAgrupe em Família, Mentores e aprendizes, Romance, Aliados e amigos, Inimigos e rivais, Organizações e lugares, Objetos, '
            'conceitos e eventos (o que tem tipo objeto, conceito ou evento), Outros (omita '
            'grupo vazio). "ELE é para a entidade" já diz o que a pessoa é para a entidade; "a ENTIDADE é para ele" diz o contrário ("filha de" '
            'quer dizer que a entidade é filha dele, logo ele é o pai ou a mãe). Cada item: "com" (exatamente como acima, sem o tipo) e '
            '"papel" (2 a 6 palavras: o que a PESSOA é para a entidade). Textos mais frequentes primeiro; prefira o vínculo que dura e mais pesa '
            'na história a um primeiro contato de passagem. Romance só com palavra de amor, desejo ou parceria nos textos; Família só com '
            'parentesco; Inimigos e rivais só com oposição, ameaça ou conflito (ser monitorado ou punido por alguém não faz inimigo).')
    data, status = llm.ask_json('Você organiza as relações de uma entidade em grupos. Responda só com JSON.', user, REL_EXAMPLE,
                                validate_rel, num_predict=3000, timeout=600)
    out, used = [], set()
    for g in (data or {}).get('grupos', []):
        items = []
        for it in g['itens']:
            if not (isinstance(it, dict) and it.get('com') in by and it['com'] not in used):
                continue
            rs = by[it['com']]
            texto = fold(' '.join(r['relacao'] for r in rs))
            grupo = str(g.get('grupo', 'Outros'))
            # the model sometimes groups by mood, not by text: Romance and Family only hold with a word of the subject in some record
            if fold(grupo).startswith('romance') and not ROMANCE.search(texto):
                grupo = 'Outros'
            if fold(grupo).startswith('familia') and not FAMILIA.search(texto):
                grupo = 'Outros'
            if kind.get(it['com']) in ('objeto', 'conceito', 'evento'):   # a thing is not a relative, a friend nor a romance
                grupo = 'Objetos, conceitos e eventos'
            used.add(it['com'])
            items.append((grupo, {'com': it['com'], 'papel': str(it.get('papel', '')), 'cap': min(r['cap'] for r in rs),
                                  'book': min(rs, key=lambda r: (r['book'], r['cap']))['book']}))
        for grupo, item in items:
            for o in out:
                if o['grupo'] == grupo:
                    o['itens'].append(item)
                    break
            else:
                out.append({'grupo': grupo, 'itens': [item]})
    return out


def identity_facts(ent, notes, upto=None):
    """What the entity is, with no event: the most repeated roles in the lines of the map and the notes of identity. `upto` (book,
    chapter) limits it to what the reader has read: without it, the "Quem é" at the end of a part could cite a role from 40 chapters
    later (found when importing into Códice, which came to hide the sentence)."""
    count = collections.OrderedDict()
    for r in sorted(ent['linha'], key=lambda r: (r['book'], r['cap'])):
        if upto and (r['book'], r['cap']) > tuple(upto):
            continue
        p = (r.get('papel') or '').strip()
        if p and not re.search(r'figura hist[óo]rica|n[ãa]o aparece', p.lower()):
            c = count.setdefault(fold(p), [p, 0, r['cap'], r['book']])
            c[1] += 1
    facts = [{'texto': p, 'cap': cap, 'book': b, 'onde': f'papel visto {n}×, desde o cap. {cap}'}
             for p, n, cap, b in sorted(count.values(), key=lambda x: -x[1])[:18]]
    for n in notes:
        if upto and (n['book'], n['cap']) > tuple(upto):
            continue
        if n['categoria'] in ('identidade', 'habilidade') and len(facts) < 40:
            facts.append({'texto': n['nota'], 'cap': n['cap'], 'book': n['book'], 'onde': f"cap. {n['cap']}"})
    return [(f'F{i}', f) for i, f in enumerate(facts, 1)]


# ---------------------------------------------------------------- page
def build_page(llm, ent, series, books, notes, parallel, registro, tick=lambda: None):
    who = ent['nome']
    shown = [a for a in ent['aliases'] if a != who][:6]
    who_full = who + (f" (também: {', '.join(shown)})" if shown else '')
    jobs = []
    for bi, b in enumerate(series['series']):
        for p in b['partes']:
            if p['nome'] == 'Final' and len(b['partes']) > 1:   # what is left after the last part (adverts, acknowledgements)
                continue
            if all(c in set(b.get('extras') or []) for c in range(p['primeiro'], p['ultimo'] + 1)):
                continue
            facts = entity_facts(ent, notes, bi, p['primeiro'], p['ultimo'], ('evento', 'relacao', 'identidade', 'habilidade'))
            if facts:
                jobs.append((bi, b, p, facts))

    def part_job(job):
        bi, b, p, facts = job
        n = len(facts)
        scope = f"{b['titulo']}, {p['nome']} (capítulos {p['primeiro']} a {p['ultimo']})"
        kept, dropped = write_section(
            llm, who_full, scope, facts,
            'Escreva a seção de BIOGRAFIA desta parte, como numa wiki de fãs: o que acontece com a entidade, em ordem cronológica, no '
            'passado, em terceira pessoa, em 1 a 3 parágrafos. Dê peso ao que muda a história dela (decisões, perdas, encontros, '
            'descobertas, mudança de posição, de nome ou de lado) e deixe de fora hábito, objeto, detalhe de cena e o que só descreve o '
            'jeito dela (isso vai em outra seção). Não termine com avaliação nem resumo.', max(4, min(12, n // 5 + 2)))
        return {'book': bi, 'parte': p['nome'], 'primeiro': p['primeiro'], 'ultimo': p['ultimo'], 'fatos': n,
                'frases': [{**f, 'caps': sorted({dict(facts)[x]['cap'] for x in f['fatos']})} for f in kept], 'descartadas': dropped}

    def lead_job(point, bio, verify):
        """The "Quem é" after each part: written from scratch from the biography up to there (already checked), never from the previous one."""
        facts = [f for _, f in identity_facts(ent, notes, (point['book'], point['ultimo']))]
        for f in facts:
            f['onde'] = 'identidade: ' + f['onde']
        for p in bio:
            for f in p['frases']:
                facts.append({'texto': f['texto'], 'cap': max(f['caps'] or [0]), 'book': p['book'],
                              'onde': f"{series['series'][p['book']]['titulo']}, {p['parte'].split(':')[0].split(' - ')[0]}"})
        facts = [(f'F{i}', f) for i, f in enumerate(facts, 1)]
        books_read = sorted({p['book'] for p in bio})
        kept, dropped = write_section(
            llm, who_full, f"até o fim de {series['series'][point['book']]['titulo']}, {point['parte']}", facts,
            'Escreva o "QUEM É": o parágrafo de abertura da página, de 3 a 6 frases. A PRIMEIRA frase começa pelo nome da entidade e diz o '
            'que ou quem ela é (use os fatos de "identidade"; ex.: "Fulano é um livreiro de Nova York"); sem "Ele"/"Ela" na abertura. Depois '
            'o papel dela na história (protagonista, antagonista, amigo, etc., só se os fatos mostrarem) e a trajetória até aqui, com ao menos '
            'uma frase para ' +
            ('cada livro lido, e o mais recente pesa mais' if len(books_read) > 1 else 'o que a marcou de mais importante') +
            '. Sem lista, sem detalhe de cena, sem avaliação, e sem repetir a mesma ideia com outras palavras.', 6, verify=verify)
        return {'ate_livro': point['book'], 'ate_cap': point['ultimo'], 'parte': point['parte'], 'frases': kept, 'descartadas': dropped}

    def traits_job():
        pool = [n for n in notes if n['categoria'] in ('personalidade', 'aparencia', 'habilidade')]
        flist = [{'texto': n['nota'], 'cap': n['cap'], 'book': n['book'], 'onde': f"{series['series'][n['book']]['titulo']}, cap. {n['cap']}",
                  'categoria': n['categoria']} for n in pool]
        out = {}
        # Appearance and abilities are not generated: the map does not keep that data and the maintainer preferred no section to a poor one.
        for key, cats, instruction in (
                ('personalidade', ('personalidade',), 'Escreva a seção PERSONALIDADE: como a entidade é e age, o que valoriza, como muda.'),):
            sub = [(f'F{i}', f) for i, f in enumerate([f for f in flist if f['categoria'] in cats], 1)]
            if len(sub) >= 2:
                kept, dropped = write_section(llm, who_full, 'a série inteira', sub, instruction, 12)
                out[key] = {'frases': [{**f, 'caps': sorted({dict(sub)[x]['cap'] for x in f['fatos']}),
                                        'livros': sorted({dict(sub)[x]['book'] for x in f['fatos']}),
                                        'pontos': sorted({(dict(sub)[x]['book'], dict(sub)[x]['cap']) for x in f['fatos']})} for f in kept], 'descartadas': dropped}
        return out

    with concurrent.futures.ThreadPoolExecutor(parallel) as ex:
        f_parts = [ex.submit(part_job, j) for j in jobs]
        for f in f_parts:
            f.add_done_callback(lambda _: tick())
        f_traits = ex.submit(traits_job)
        f_rel = ex.submit(relation_groups, llm, ent, registro)
        parts = [f.result() for f in f_parts]
        main_parts = [p for p in parts if series['series'][p['book']]['kind'] == 'main']
        last_of_book = {}
        for p in main_parts:
            last_of_book[p['book']] = p
        f_leads = [ex.submit(lead_job, p, main_parts[:i + 1], last_of_book[p['book']] is p) for i, p in enumerate(main_parts)]
        for f in f_leads:
            f.add_done_callback(lambda _: tick())
        leads = [f.result() for f in f_leads]
        traits, rels = f_traits.result(), f_rel.result()
    return {'nome': who, 'tipo': ent['tipo'], 'aliases': ent['aliases'], 'livros': ent['livros'], 'caps': ent['caps'],
            'quem_e': leads, 'partes': parts, 'tracos': traits, 'relacoes': rels,
            'linha': sorted(({k: r.get(k) for k in ('book', 'cap', 'estado', 'novo')} for r in ent['linha'] if r.get('estado')),
                            key=lambda r: (r['book'], r['cap']))}
