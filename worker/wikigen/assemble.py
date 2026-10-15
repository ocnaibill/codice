"""From the maps of the books and the identity groups to the series: the parts of each book, its editorial chapters, the registry of
entities (all of them go to the graph), the line of states of each one, and the ties in both directions."""
import collections
import re

from .registry import build_registry, head_of, merge_series
from .text import CONNECTORS, core, fold, kind_of

MIN_CHAPTERS = 2   # an entity seen in a single chapter is not an entity of the series


def book_parts(chapters):
    """Parts are runs of contiguous chapters with the same part name. Chapters with none at the start ("Abertura") or at the end ("Final",
    in general editorial) cannot stretch a part over the whole book, as grouping by name did."""
    runs = []
    for idx in sorted(chapters):
        name = chapters[idx].get('part') or None
        if runs and runs[-1]['nome'] == name:
            runs[-1]['ultimo'] = idx
        else:
            runs.append({'nome': name, 'primeiro': idx, 'ultimo': idx})
    for r in runs:
        r['nome'] = r['nome'] or ('Abertura' if r is runs[0] and len(runs) > 1 else 'Final')
    return runs


def editorial_chapters(book):
    """The chapters after the last numbered one in a main-line book: previews of other books, acknowledgements. Only when the titles are
    numbered ("12. ...") so that there is a last numbered chapter to speak of."""
    if book['kind'] != 'main':
        return set()
    numbered = [i for i, c in book['chapters'].items() if re.match(r'^\d+[.)]\s', c['title'])]
    if not numbered:
        return set()
    last = max(numbered)
    return {i for i in book['chapters'] if i > last}


def _looks_like_name(a):
    words = [w for w in re.split(r'\s+', a.strip()) if w and w.lower() not in CONNECTORS]
    return len(words) >= 2 and all(w[:1].isupper() for w in words) and not re.search(r'\d', a)


def build_series(llm, books, progress=None):
    """Returns (series, merge_log). `series` is:
      {'series': [{'id','titulo','kind','partes','capitulos','extras'}],
       'entidades': [{'nome','tipo','aliases','livros','caps','linha','relacoes','relacoes_inversas','notas'}]}
    for every entity seen in at least MIN_CHAPTERS story chapters, most present first."""
    for b in books:
        b['partes'] = book_parts(b['chapters'])
        b['extras'] = editorial_chapters(b)
    entries = build_registry(books)
    for e in entries:
        e['caps'] = [c for c in e['caps'] if c not in books[e['book']]['extras']]
        e['notas'] = [n for n in e['notas'] if n['cap'] not in books[e['book']]['extras']]
    entries = [e for e in entries if e['caps']]
    for i, e in enumerate(entries):
        e['id'] = i
    groups, log = merge_series(llm, entries)
    groups = [g for g in groups if sum(len(x['caps']) for x in g) >= MIN_CHAPTERS]
    groups.sort(key=lambda g: (-sum(len(x['caps']) for x in g), head_of(g)[0]))

    ents = []
    for g in groups:
        name, names = head_of(g)
        aliases = [n for n, _ in names.most_common(14) if core(n) and n != name]
        kind = collections.Counter(x['tipo'] for x in g for _ in x['caps']).most_common(1)[0][0]
        # a one-word name that is a title ("Timbre") with a person's name among the aliases: the person's name becomes the main one
        if kind == 'personagem' and len(core(name)) == 1:
            cand = [a for a in aliases if len(core(a)) >= 2 and _looks_like_name(a)]
            if cand:
                name, aliases = cand[0], [name] + [a for a in aliases if a != cand[0]]
        ents.append({'nome': name, 'tipo': kind, 'aliases': aliases, 'livros': sorted({x['book'] for x in g}),
                     'caps': {str(bi): sorted({c for x in g if x['book'] == bi for c in x['caps']}) for bi in range(len(books))},
                     'notas': [n for x in g for n in x['notas']], 'linha': [], 'relacoes': [], 'relacoes_inversas': []})

    # owner of each core name; one that two entities have is ambiguous and counts for none
    owner = collections.defaultdict(set)
    for i, e in enumerate(ents):
        for n in [e['nome']] + list(e['aliases']):
            c = core(n)
            if c and not (len(c) == 1 and len(c[0]) < 3):   # "Luc", "Bea": three letters are enough
                owner[c].add(i)
    owner = {c: next(iter(s)) for c, s in owner.items() if len(s) == 1}

    seen = set()
    for bi, b in enumerate(books):
        for cap in sorted(b['mapa']):
            if cap in b['extras']:
                continue
            for rec in b['mapa'][cap].get('entidades', []):
                if not rec.get('verificada'):
                    continue
                src = owner.get(core(rec.get('nome', '')))
                estado = str(rec.get('estado') or '').strip()
                papel = str(rec.get('papel') or '').strip()
                if src is not None and not re.match(r'^(n[aã]o aparece|ausente|n[aã]o presente|not (present|appear))', estado.lower()) \
                        and not re.search(r'figura hist[óo]rica|pessoa hist[óo]rica|personagem hist[óo]rico', papel.lower()):
                    ents[src]['linha'].append({'book': bi, 'cap': cap, 'estado': estado, 'papel': papel,
                                               'novo': [str(x).strip() for x in (rec.get('novo') or []) if x]})
                for r in rec.get('relacoes') or []:
                    if not isinstance(r, dict) or not r.get('com') or not r.get('relacao'):
                        continue
                    dst = owner.get(core(str(r['com'])))
                    if src is not None and dst != src:
                        k = ('f', src, fold(r['com']), fold(r['relacao'])[:60])
                        if k not in seen:
                            seen.add(k)
                            ents[src]['relacoes'].append({'com': str(r['com']), 'relacao': str(r['relacao']), 'book': bi, 'cap': cap})
                    if dst is not None and src != dst:
                        k = ('i', dst, fold(rec['nome']), fold(r['relacao'])[:60])
                        if k not in seen:
                            seen.add(k)
                            ents[dst]['relacoes_inversas'].append({'com': rec['nome'], 'relacao': str(r['relacao']), 'book': bi, 'cap': cap})
    for e in ents:
        e['linha'].sort(key=lambda r: (r['book'], r['cap']))
    series = {
        'series': [{'id': b['id'], 'titulo': b['titulo'], 'kind': b['kind'], 'partes': b['partes'],
                    'capitulos': {str(i): c['title'] for i, c in sorted(b['chapters'].items())}, 'extras': sorted(b['extras'])} for b in books],
        'entidades': ents,
    }
    return series, log


def chapters_seen(ent):
    """In how many chapters (all books) the entity appears."""
    return sum(len(c) for c in ent['caps'].values())
