"""Stage "identity": the entities of every book become one registry.

Inside a book, the same name (and a short name that is part of a single longer one) is one entity. Across books: the same core name, a
short name inside a single longer one, a note that says the entity took another name, and finally pairs the notes tie together with a
word of naming, which the model decides, only with proof copied from the notes (the proof must be in them, must have a word of naming and
must name the other side whole). A pair the model said is different is never joined again, not even through a third.
"""
import collections
import re

from .text import CONNECTORS, TITLE_WORDS, core, fold, kind_of  # noqa: F401


def prefold2(records):
    """Joins, inside a book, what is the same name. A name that is only a title ("Doctor") identifies no one and goes. A one-word name that is
    part of a single longer name of the same kind ("Souza" / "Ana Souza") goes into it."""
    groups = collections.OrderedDict()
    for r in records:
        c = core(r['nome'])
        if not c:
            continue
        groups.setdefault(c, []).append(r)
    kind = {c: collections.Counter(kind_of(x.get('tipo')) for x in rs).most_common(1)[0][0] for c, rs in groups.items()}
    canon = {}
    for c in groups:
        if len(c) == 1 and len(c[0]) >= 4:
            owners = [o for o in groups if len(o) > 1 and c[0] in o and kind[o] == kind[c]]
            if len(owners) == 1:
                canon[c] = owners[0]
                continue
        canon[c] = c
    merged = collections.OrderedDict()
    for c, rs in groups.items():
        merged.setdefault(canon[c], []).extend(rs)
    return merged


def build_registry(books):
    """A list of entries {book, nomes, tipo, caps, notas}, by book (names already joined inside the book)."""
    entries = []
    for bi, b in enumerate(books):
        records = []
        for cap, m in b['mapa'].items():
            for e in m['entidades']:
                if e.get('verificada'):
                    e = dict(e)
                    e['_cap'] = cap
                    records.append(e)
        for key, rs in prefold2(records).items():
            names = collections.Counter(r['nome'] for r in rs)
            # "apprentice of the baker", "knife": a generic description in lower case, not an entity with a name
            if not any(re.search(r'[A-ZÀ-Ý]', n) for n in names):
                continue
            entries.append({
                'id': len(entries), 'book': bi, 'nomes': names, 'nome': max(names, key=lambda n: (len(core(n)), names[n])),
                'tipo': collections.Counter(kind_of(r.get('tipo')) for r in rs).most_common(1)[0][0],
                'caps': sorted({r['_cap'] for r in rs}),
                'notas': [{'book': bi, 'cap': r['_cap'], 'papel': r.get('papel') or '', 'novo': [str(x) for x in (r.get('novo') or []) if x],
                           'estado': r.get('estado') or '', 'relacoes': [x for x in (r.get('relacoes') or []) if isinstance(x, dict)],
                           'fonte': 'mapa'} for r in rs],
            })
    return entries



CUE = re.compile(r'adot|assum|escolh|\bnome|chama|conhecid|apelid|identidade|alcunh|alias|sob o |passou a se|rebatiz|batiz|na verdade|verdadeir|disfarc|codinome|titulo|tambem')

PAIR_EXAMPLE = {'decisoes': [
    {'par': 1, 'mesma': True, 'motivo': 'B é o nome que A adotou depois', 'prova': 'frase copiada das notas que diz isso, citando o outro nome'},
    {'par': 2, 'mesma': False, 'motivo': 'as notas tratam de duas pessoas que convivem', 'prova': ''}]}


def validate_pairs(d):
    if not isinstance(d, dict) or not isinstance(d.get('decisoes'), list):
        return "falta a lista 'decisoes'"
    for x in d['decisoes']:
        if not isinstance(x, dict) or not isinstance(x.get('par'), int) or not isinstance(x.get('mesma'), bool):
            return "cada decisão precisa de 'par' (inteiro) e 'mesma' (verdadeiro/falso)"
    return None


def notes_text(g):
    """O texto das notas de um grupo, sem acento, para procurar nomes."""
    parts = []
    for x in g:
        for n in x['notas']:
            parts.append(n.get('papel') or '')
            parts.extend(n.get('novo') or [])
            parts.append(n.get('estado') or '')
            parts.extend(f"{r.get('relacao', '')} {r.get('com', '')}" for r in n.get('relacoes') or [])
    return fold(' '.join(parts))


FLEX = {'personagem', 'cargo', 'outro'}


def compatible(k1, k2):
    """Two kinds that may be the same entity: equal, or both among person, title and other (an adopted name becomes a "title" in one book
    and a "character" in another). A place is never a person."""
    return k1 == k2 or (k1 in FLEX and k2 in FLEX)


def merge_series(llm, entries):
    """Joins entries of different books. (1) the same core name; (2) a one-word name that is part of a single longer name of the same
    kind; (3) pairs that the text of the notes ties ("Rowan" shows in the notes of "Lucifer") go to the model, which only joins with proof
    that it is the same person or thing (adopted name, nickname, assumed identity). Returns (groups, {fusoes, recusadas, pares})."""
    parent = list(range(len(entries)))

    def find(x):
        while parent[x] != x:
            parent[x] = parent[parent[x]]
            x = parent[x]
        return x

    fusoes, recusadas, log = [], [], []
    negativas = {}   # pairs of entries the model said are different: no join unites them later, not even through a third

    def members(root):
        return [e['id'] for e in entries if find(e['id']) == root]

    def chapters_of(root):
        return {(e['book'], c) for e in entries if find(e['id']) == root for c in e['caps']}

    def union(a, b, why, check=False):
        ra, rb = find(a), find(b)
        if ra != rb:
            if check:
                ma, mb = members(ra), members(rb)
                hit = next(((x, y) for x in ma for y in mb if (min(x, y), max(x, y)) in negativas), None)
                if hit:
                    recusadas.append({'a': entries[a]['nome'], 'b': entries[b]['nome'],
                                      'por': f"{entries[hit[0]]['nome']} e {entries[hit[1]]['nome']} já foram dados como diferentes ({negativas[(min(hit), max(hit))]})"})
                    return
            parent[ra] = rb
            fusoes.append({'a': entries[a]['nome'], 'b': entries[b]['nome'], 'por': why})

    # (1) the same core name, in compatible kinds
    by_core = collections.defaultdict(list)
    for e in entries:
        for n in e['nomes']:
            c = core(n)
            if c:
                by_core[c].append(e['id'])
    for c, ids in by_core.items():
        for i in ids[1:]:
            ka, kb = entries[ids[0]]['tipo'], entries[i]['tipo']
            # the same name in different kinds ("The Order" as a concept and as an organization) is the same entity; only a place is not a person
            if compatible(ka, kb) or ({ka, kb} != {'lugar', 'personagem'} and len(' '.join(c)) >= 6):
                union(ids[0], i, 'mesmo nome sem título')
    # (2) a name that is part of a single longer name (one word: "Souza"; several: "Ana Souza" in "Ana Maria Souza"), of the same kind, in
    # any book
    long_names = [(core(n), e) for e in entries for n in e['nomes'] if core(n)]
    for e in entries:
        for n in list(e['nomes']):
            c = core(n)
            if not c or (len(c) == 1 and len(c[0]) < 4):
                continue
            owners = {}
            for oc, o in long_names:
                if len(oc) > len(c) and set(c) <= set(oc) and compatible(o['tipo'], e['tipo']) and o['id'] != e['id']:
                    owners[find(o['id'])] = o['id']
            if len(owners) == 1:
                union(e['id'], next(iter(owners.values())), 'nome curto de um único nome longo')

    # (3) the note itself says the entity adopted or chose to be known by another name ("chooses to be known as the baker Ana"): narrow
    # patterns, no model; "chooses Rowan as apprentice" does not match
    SELF = re.compile(r'(?:ser conhecid[ao]s? como|conhecid[ao]s? como|(?:adot|escolh|assum)\w+ (?:o |a )?(?:nome|apelido|titulo|identidade)(?: de)?)'
                      r'\s+(?:' + '|'.join(sorted(TITLE_WORDS, key=len, reverse=True)) + r')?\s*(?:o\s+|a\s+)*([a-z0-9]+(?: [a-z0-9]+){0,2})')
    by_name = collections.defaultdict(set)
    for e in entries:
        for n in e['nomes']:
            c = core(n)
            if c:
                by_name[' '.join(c)].add(e['id'])
    for e in entries:
        for n in e['notas']:
            text = fold(' '.join([n.get('papel') or '', ' '.join(n.get('novo') or []), n.get('estado') or '']))
            for mm in SELF.finditer(text):
                words = mm.group(1).split()
                for k in range(len(words), 0, -1):
                    phrase = ' '.join(words[:k])
                    ids = by_name.get(phrase)
                    if ids and len(phrase) >= 6:
                        for j in ids:
                            if j != e['id'] and compatible(entries[j]['tipo'], e['tipo']):
                                # an adopted name usually has few appearances; two big entities ("known as the Bell" in a note of
                                # another person) would drag a whole character into another: those go to the model, with proof
                                if min(len(chapters_of(find(e['id']))), len(chapters_of(find(j)))) <= 8:
                                    union(e['id'], j, 'a própria nota diz: ' + mm.group(0)[:100], check=False)
                        break

    for rnd in range(2):
        groups = collections.defaultdict(list)
        for e in entries:
            groups[find(e['id'])].append(e)
        # "conceito" comes in only to be paired with a person: a book calls the antagonist "the Darkness" (a concept) until the protagonist
        # gives it a name; the join only holds with proof copied from the notes, like the others
        gl = [g for g in groups.values() if sum(len(x['caps']) for x in g) >= 2 and g[0]['tipo'] in ('personagem', 'organização', 'objeto', 'cargo', 'outro', 'conceito')]
        info = []
        for g in gl:
            names = collections.Counter()
            for x in g:
                names.update(x['nomes'])
            cores = {core(n) for n in names if core(n)}
            # palavras que identificam o grupo: as de nomes longos, e as de uma palavra com 4 letras ou mais
            toks = {t for c in cores for t in c if len(t) >= 4}
            info.append({'g': g, 'names': names, 'toks': toks, 'cores': cores, 'text': notes_text(g), 'root': find(g[0]['id'])})
        # Candidates: pairs in which a note speaks of both names and has a word of naming ("adopts the name", "known as", "identity").
        # Mentioning each other is not enough: the protagonists mention each other all the time and that drowned the real pairs.

        def note_texts(X):
            out = []
            for x in X['g']:
                for n in x['notas']:
                    out.append(fold(' '.join([n.get('papel') or '', ' '.join(n.get('novo') or []), n.get('estado') or '',
                                              ' '.join(f"{r.get('relacao', '')} {r.get('com', '')}" for r in n.get('relacoes') or [])])))
            return out

        for X in info:
            X['notes'] = note_texts(X)
        cand = []
        for i, A in enumerate(info):
            for B in info[i + 1:]:
                ta, tb = A['g'][0]['tipo'], B['g'][0]['tipo']
                if A['root'] == B['root'] or not (compatible(ta, tb) or {ta, tb} == {'conceito', 'personagem'}) or ta == tb == 'conceito':
                    continue
                hits = 0
                for X, Y in ((A, B), (B, A)):
                    for t in X['notes']:
                        if CUE.search(t) and any(w in t for w in Y['toks']) and any(w in t for w in X['toks']):
                            hits += 1
                    # or: the entity's own note names the other with the word of naming, even without repeating its own name
                    for t in X['notes']:
                        if CUE.search(t) and any(w in t for w in Y['toks']) and not any(w in t for w in X['toks']):
                            hits += 1
                if hits >= 1:
                    cand.append((hits, A, B))
        cand.sort(key=lambda x: -x[0])
        cand = cand[:70]
        if not cand:
            break
        blocks = []
        for k, (_, A, B) in enumerate(cand, 1):
            def show(X, other):
                nm = ', '.join(n for n, _ in X['names'].most_common(4))
                books = sorted({x['book'] for x in X['g']})
                notes = [n for x in X['g'] for n in x['notas']]

                def flat(n):
                    return fold(' '.join([n.get('papel') or '', ' '.join(n.get('novo') or []), n.get('estado') or '']))
                # the notes that speak of the other name and have a word of naming come first
                notes.sort(key=lambda n: -(sum(flat(n).count(t) for t in other['toks']) + (3 if CUE.search(flat(n)) else 0)))
                snip = []
                for n in notes[:3]:
                    snip.append(' | '.join(filter(None, [n.get('papel') or '', '; '.join(n.get('novo') or [])[:200], n.get('estado') or '']))[:300])
                return f"{nm} (livros {books}) — " + ' // '.join(snip)
            blocks.append(f"Par {k}\n  A: {show(A, B)}\n  B: {show(B, A)}")
        header = ('Pares de entidades de uma série de livros, tiradas livro a livro. Para cada par, diga se A e B são a MESMA pessoa ou coisa '
                  'com nomes diferentes (nome adotado depois, apelido, identidade assumida, título, forma curta). "mesma" só se uma frase das '
                  'notas diz isso; copie essa frase em "prova". Responda um item para CADA par. NÃO são a mesma: mencionar-se, conviver, ser '
                  'parente ou aliado, nomes parecidos, uma ser ramo, filial, região ou parte da outra, ou uma ter sido morta ou substituída '
                  'pela outra. ATENÇÃO: quando alguém escolhe ou adota um nome em homenagem a outra pessoa ou figura (por exemplo, "escolhe ser '
                  'conhecida como a padeira X"), o nome adotado designa QUEM O ADOTOU, e a entidade com esse nome é a mesma que ela.\n\n')
        decisoes = {}
        m_total = {'seconds': 0, 'prompt_tokens': 0, 'output_tokens': 0, 'requests': 0}
        for start in range(0, len(blocks), 6):          # poucos pares por pedido: com dezenas, o modelo responde "diferente" a todos
            chunk = blocks[start:start + 6]
            user = header + '\n'.join(re.sub(r'^Par \d+', f'Par {start + j + 1}', b) for j, b in enumerate(chunk))
            data, status = llm.ask_json('Você resolve identidades de entidades de uma série. Responda só com JSON.', user,
                                        PAIR_EXAMPLE, validate_pairs, num_predict=2000, timeout=900)
            for x in (data or {}).get('decisoes', []):
                decisoes[x['par']] = x
        data = {'decisoes': list(decisoes.values())}
        changed = 0
        for k, (score, A, B) in enumerate(cand, 1):
            dec = next((x for x in (data or {}).get('decisoes', []) if x.get('par') == k), {})
            log.append({'rodada': rnd + 1, 'a': A['g'][0]['nome'], 'b': B['g'][0]['nome'], 'mencoes': score, 'mesma': dec.get('mesma'),
                        'motivo': str(dec.get('motivo', ''))[:120]})
        for x in (data or {}).get('decisoes', []):
            if not (1 <= x['par'] <= len(cand)):
                continue
            _, A, B = cand[x['par'] - 1]
            ida, idb = A['g'][0]['id'], B['g'][0]['id']
            if not x['mesma']:
                for p_ in A['g']:
                    for q_ in B['g']:
                        negativas[(min(p_['id'], q_['id']), max(p_['id'], q_['id']))] = f"par {x['par']}: {A['g'][0]['nome']} / {B['g'][0]['nome']}"
                continue
            raw = str(x.get('prova', ''))
            # the model sometimes copies the header of the block ("Name (books [0]) — ..."): each piece after the dash and the "//" counts
            segs = [t.strip() for part in re.split(r'\s//\s|\s—\s', raw) for t in [part] if t.strip()] or [raw]
            def sq(t):
                return re.sub(r'[^a-z0-9]+', ' ', fold(t)).strip()
            prova = fold(next((t for t in segs if sq(t) and (sq(t) in sq(A['text']) or sq(t) in sq(B['text']))), segs[-1])).strip()
            # The proof must: (1) be in the notes of one of the two; (2) have a word of naming; (3) name the OTHER side of the one that
            # has it in the notes. Without that, the model copies the same proof for different pairs.
            def squash(t):
                return re.sub(r'[^a-z0-9]+', ' ', t).strip()
            prova = squash(prova)
            in_a, in_b = prova in squash(A['text']), prova in squash(B['text'])
            # the name of the other side must show WHOLE in the proof (a shared surname does not tie two people)
            def names_in(X, text):
                return any(' '.join(c) in text for c in X['cores'] if len(' '.join(c)) >= 5)
            ok = len(prova) >= 15 and (in_a or in_b) and CUE.search(prova) and (
                (in_a and names_in(B, prova)) or (in_b and names_in(A, prova)))
            if ok:
                union(ida, idb, 'modelo: ' + str(x.get('motivo', ''))[:100] + ' · prova: ' + str(x.get('prova'))[:160], check=True)
                changed += 1
            else:
                recusadas.append({'a': A['g'][0]['nome'], 'b': B['g'][0]['nome'], 'por': 'prova que não confere: ' + str(x.get('prova', ''))[:120]})
        if not changed:
            break
    out = collections.defaultdict(list)
    for e in entries:
        out[find(e['id'])].append(e)
    return list(out.values()), {'fusoes': fusoes, 'recusadas': recusadas, 'pares': log}


def head_of(g):
    """The name of the entity: among the names used more than once, the one with most identifying words (a person's name beats a one-word
    title, like "the Bell"); with none, that of the entry with most chapters."""
    names = collections.Counter()
    for x in g:
        names.update(x['nomes'])
    base = max(g, key=lambda x: (len(x['caps']), len(core(x['nome']))))['nome']
    frequent = [n for n in names if names[n] >= 2 and core(n)]
    if not frequent:
        return base, names
    best = max(frequent, key=lambda n: (len(core(n)), names[n]))
    return (best if len(core(best)) > len(core(base)) else base), names
