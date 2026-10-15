"""Stage "import", first half: everything the generator knew becomes the bundle that the backend imports (format 1, backend/internal/wiki).

Everything a page shows carries the point (book, chapter) from which it may be shown, and the backend cuts the reader's view there
(DEC-179). Sentences carry the references that wrote them, with the quote; a sentence whose facts have no valid chapter is left out.
"""
import collections
import datetime
import re

from . import pages as wpg
from .text import core, fold, key, slugify

KIND = {'personagem': 'person', 'lugar': 'place', 'organização': 'organization', 'conceito': 'concept', 'objeto': 'object',
        'evento': 'event', 'cargo': 'other', 'outro': 'other'}
GROUP = {'familia': 'family', 'mentores e aprendizes': 'mentors', 'romance': 'romance', 'aliados e amigos': 'allies',
         'inimigos e rivais': 'enemies', 'organizacoes e lugares': 'places', 'objetos, conceitos e eventos': 'things', 'outros': 'others'}
BIO_CATS = ('evento', 'relacao', 'identidade', 'habilidade')
THINGS = ('objeto', 'conceito', 'evento')


def _limit(text, n):
    text = str(text)
    return text if len(text) <= n else text[:n - 1] + '…'


def build_bundle(books, series, pages, provider, model, remote, partial=False, now=None, suggestions=(), rules=()):
    """`books`: the books as loaded (id, titulo, kind, sha256, chapters). `series`: from assemble.build_series. `pages`: {entity name:
    {'page': build_page result, 'notes': search notes}}."""
    out_books, valid = [], set()
    for bi, b in enumerate(books):
        meta = series['series'][bi]
        parts = meta['partes']
        trailing_final = len(parts) > 1 and parts[-1]['nome'] == 'Final'
        extras = set(meta.get('extras') or [])

        def part_of(cap, parts=parts):
            for p in parts:
                if p['primeiro'] <= cap <= p['ultimo']:
                    return p['nome']
            return ''
        chapters = []
        for idx in sorted(b['chapters']):
            ch = b['chapters'][idx]
            part = part_of(idx)
            chapters.append({'chapter': idx, 'title': ch['title'], 'part': part, 'href': ch.get('href', ''),
                             'editorial': idx in extras or (trailing_final and part == 'Final')})
            valid.add((bi, idx))
        out_books.append({'position': bi, 'title': meta['titulo'], 'side': meta['kind'] != 'main', 'sha256': b['sha256'], 'chapters': chapters})

    def ok(book, cap):
        return (book, cap) in valid

    used = set()
    entities, by_core, slug_of = [], {}, {}
    for ent in series['entidades']:
        apps = [{'book': int(b), 'chapter': c} for b, cs in ent['caps'].items() for c in cs if ok(int(b), c)]
        if not apps:
            continue
        slug = slugify(ent['nome'], used)
        slug_of[ent['nome']] = slug
        for n in [ent['nome']] + list(ent['aliases']):
            c = core(n)
            if c:
                by_core.setdefault(c, set()).add(slug)
        entities.append({'slug': slug, 'name': ent['nome'], 'kind': KIND.get(ent['tipo'], 'other'), 'page': False, 'appearances': apps,
                         'aliases': [], 'states': [], 'sections': [], 'relations': [], '_ent': ent})
    by_core = {c: next(iter(s)) for c, s in by_core.items() if len(s) == 1}
    by_slug = {e['slug']: e for e in entities}

    def resolve(name):
        c = core(name)
        if c in by_core:
            return by_core[c]
        if len(c) == 1:   # "Faraday": the only longer name that holds it
            owners = {s for k, s in by_core.items() if len(k) > 1 and c[0] in k}
            if len(owners) == 1:
                return next(iter(owners))
        return None

    # the first chapter in which each name shows in the map (an alias may be a spoiler: it shows from there on)
    first_name = {}
    for bi, b in enumerate(books):
        for cap in sorted(b['mapa']):
            for rec in b['mapa'][cap].get('entidades', []):
                if rec.get('verificada') and rec.get('nome'):
                    first_name.setdefault(core(rec['nome']), (bi, cap))
    for e in entities:
        first = min((p['book'], p['chapter']) for p in e['appearances'])
        for alias in e['_ent']['aliases']:
            b, c = first_name.get(core(alias), first)
            if (b, c) < first:
                b, c = first
            if ok(b, c):
                e['aliases'].append({'alias': alias, 'book': b, 'chapter': c})
        for r in e['_ent'].get('linha') or []:
            if r.get('estado') and ok(r['book'], r['cap']):
                e['states'].append({'book': r['book'], 'chapter': r['cap'], 'text': r['estado']})

    npages = 0
    for e in entities:
        got = pages.get(e['name'])
        if not got:
            continue
        page, notes, ent = got['page'], got['notes'], e['_ent']
        e['page'] = True
        npages += 1

        def refs_of(fatos, facts):
            res = []
            for x in fatos:
                fact = facts.get(x)
                if not fact or not ok(fact['book'], fact['cap']):
                    continue
                r = {'book': fact['book'], 'chapter': fact['cap'],
                     'fact': _limit(re.sub(r'^\((?:estado|situação ao fim do capítulo)\)\s*', '', fact['texto']), 500)}
                if fact.get('citacao'):
                    r['quote'] = _limit(fact['citacao'], 500)
                res.append(r)
            return res

        part_ranges = {}
        for p in page['partes']:
            part_ranges[(p['book'], p['parte'])] = (p['primeiro'], p['ultimo'])
            facts = dict(wpg.entity_facts(ent, notes, p['book'], p['primeiro'], p['ultimo'], BIO_CATS))
            sents = []
            for st in p['frases']:
                refs = refs_of(st['fatos'], facts)
                if refs:
                    sents.append({'paragraph': st.get('par', 1), 'text': st['texto'], 'refs': refs})
            if sents:
                e['sections'].append({'kind': 'bio', 'book': p['book'], 'chapter_from': p['primeiro'], 'chapter_to': p['ultimo'],
                                      'part': p['parte'], 'sentences': sents})
        main_parts = [p for p in page['partes'] if series['series'][p['book']]['kind'] == 'main']
        for q in page['quem_e']:
            point = (q['ate_livro'], q.get('ate_cap', 10 ** 6))
            facts_list = [f for _, f in wpg.identity_facts(ent, notes, point)]
            for p in main_parts:
                if (p['book'], p['ultimo']) <= point:
                    for st in p['frases']:
                        facts_list.append({'texto': st['texto'], 'cap': max(st['caps'] or [0]), 'book': p['book']})
            facts = {f'F{i}': x for i, x in enumerate(facts_list, 1)}
            sents = []
            for st in q['frases']:
                refs = refs_of(st['fatos'], facts)
                if refs:
                    sents.append({'paragraph': st.get('par', 1), 'text': st['texto'], 'refs': refs})
            first, last = part_ranges.get((q['ate_livro'], q.get('parte', '')), (1, q.get('ate_cap', 1)))
            if sents and ok(q['ate_livro'], q.get('ate_cap', last)):
                e['sections'].append({'kind': 'who', 'book': q['ate_livro'], 'chapter_from': first, 'chapter_to': q.get('ate_cap', last),
                                      'part': q.get('parte', ''), 'sentences': sents})
        t = (page.get('tracos') or {}).get('personalidade')
        if t and t['frases']:
            pool = [n for n in notes if n['categoria'] == 'personalidade']
            facts = {f'F{i}': {'texto': n['nota'], 'cap': n['cap'], 'book': n['book'], 'citacao': n.get('citacao')} for i, n in enumerate(pool, 1)}
            sents = []
            for st in t['frases']:
                refs = refs_of(st['fatos'], facts)
                if refs:
                    sents.append({'paragraph': 1, 'text': st['texto'], 'refs': refs})
            if sents:
                e['sections'].append({'kind': 'personality', 'part': '', 'sentences': sents})
        for g in page.get('relacoes') or []:
            grp = GROUP.get(fold(g['grupo']).replace('  ', ' '), 'others')
            for it in g['itens']:
                b, c = it.get('book', 0), it.get('cap', 1)
                if not ok(b, c):
                    continue
                rel = {'name': _limit(it['com'], 200), 'group': grp, 'role': _limit(it.get('papel', ''), 200), 'book': b, 'chapter': c}
                other = resolve(it['com'])
                if other and other != e['slug']:
                    rel['other'] = other
                e['relations'].append(rel)

    # an entity with no page still has a ficha: what the others are to it, in the group "others" (the model did not group them)
    for e in entities:
        if e['page']:
            continue
        ent = e['_ent']
        by = collections.OrderedDict()
        for r in ent.get('relacoes_inversas') or []:
            if ok(r['book'], r['cap']) and core(r['com']) and r['com'][:1].isupper():
                by.setdefault(r['com'], []).append(r)
        for com, rs in by.items():
            other = resolve(com)
            if not other or other == e['slug']:
                continue
            kind = next((x['_ent']['tipo'] for x in entities if x['slug'] == other), '')
            if len(rs) < (3 if kind in THINGS else 2):
                continue
            first = min(rs, key=lambda r: (r['book'], r['cap']))
            role = collections.Counter(r['relacao'] for r in rs).most_common(1)[0][0]
            e['relations'].append({'name': _limit(com, 200), 'group': 'things' if kind in THINGS else 'others', 'role': _limit(role, 200),
                                   'book': first['book'], 'chapter': first['cap'], 'other': other})

    # the edges of the graph: every tie the map recorded, with its chapter
    edges, seen = [], set()
    for e in entities:
        for n in e['_ent']['notas']:
            for rel in n.get('relacoes') or []:
                if not isinstance(rel, dict) or not rel.get('com') or not ok(n['book'], n['cap']):
                    continue
                other = resolve(str(rel['com']))
                if not other or other == e['slug']:
                    continue
                key = (e['slug'], other, fold(str(rel.get('relacao', '')))[:60], n['book'], n['cap'])
                if key in seen:
                    continue
                seen.add(key)
                edges.append({'a': e['slug'], 'b': other, 'label': str(rel.get('relacao', ''))[:300], 'book': n['book'], 'chapter': n['cap']})

    # The pairs the model took for the same without proof go to the staff (DEC-181), unless they already decided about them.
    decided = {frozenset((r['a'], r['b'])) for r in rules if r.get('kind') in ('same', 'distinct')}
    first_seen = {e['slug']: min((p['book'], p['chapter']) for p in e['appearances']) for e in entities}
    asked, out_suggestions = set(), []
    for sg in suggestions:
        a, b = resolve(sg['a']), resolve(sg['b'])
        if not a or not b or a == b or frozenset((key(sg['a']), key(sg['b']))) in decided or (a, b) in asked or (b, a) in asked:
            continue
        asked.add((a, b))
        book, chapter = max(first_seen[a], first_seen[b])
        evidence = _limit(f"O modelo leu as notas e achou que são a mesma, mas sem prova: {sg['motivo']}"
                          + (f" — {sg['prova']}" if sg.get('prova') else ''), 900)
        out_suggestions.append({'kind': 'merge', 'entity': a, 'other': b, 'evidence': evidence, 'book': book, 'chapter': chapter})
    for e in entities:
        e.pop('_ent')
    when = (now or datetime.datetime.now(datetime.timezone.utc)).isoformat(timespec='seconds')
    return {'format': 1, 'generated_at': when, 'provider': provider, 'model': model, 'remote': bool(remote), 'partial': bool(partial),
            'books': out_books, 'entities': entities, 'edges': edges, 'suggestions': out_suggestions}, npages
