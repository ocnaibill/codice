"""The stages after the text, with no database: books in, bundle out. The service feeds it and keeps its progress."""
import concurrent.futures

from . import assemble, pages as wpg
from .assemble import chapters_seen
from .bundle import build_bundle
from .mapper import map_chapter, verify_entities


class RunFailed(Exception):
    """A run that cannot go on, with the sentence that is said to the administration."""


def generate(llm, books, cut, progress, control, parallel, provider_kind, model, remote, rules=()):
    """`books`: [{'id','titulo','kind','sha256','chapters': {index: chapter}, 'mapa': {}}]. `cut`: the least chapters an entity must
    appear in to have a full page (DEC-186). `rules`: what the staff decided about who is who (DEC-181), {'kind', 'a', 'b'} with names
    folded as `text.key` does. Returns (bundle, partial)."""
    for b in books:
        b['extras'] = assemble.editorial_chapters(b)

    # ---- map: one request per chapter; the ones that fail are tried again before the wiki is called partial
    todo = [(bi, i) for bi, b in enumerate(books) for i in sorted(b['chapters']) if i not in b['extras']]
    progress.begin('map', len(todo))

    def do_chapter(job):
        bi, idx = job
        control()
        b = books[bi]
        ch = b['chapters'][idx]
        progress.where(f"{b['titulo']}, capítulo {idx}")
        mapped = map_chapter(llm, b['titulo'], ch)
        return bi, idx, verify_entities(ch, mapped)

    pending, failed = todo, []
    for rnd in range(3):
        failed = []
        with concurrent.futures.ThreadPoolExecutor(parallel) as ex:
            futures = [ex.submit(do_chapter, j) for j in pending]
            try:
                for f in concurrent.futures.as_completed(futures):
                    bi, idx, mapped = f.result()
                    if mapped['falhou']:
                        failed.append((bi, idx))
                    else:
                        books[bi]['mapa'][idx] = mapped
                        progress.tick('map')
            except BaseException:
                for f in futures:
                    f.cancel()
                raise
        if not failed:
            break
        progress.error(f'{len(failed)} capítulos sem resposta utilizável (tentativa {rnd + 1} de 3)')
        pending = failed
    partial = bool(failed)
    for bi, idx in failed:   # what never answered is kept as empty: the wiki goes out partial
        books[bi]['mapa'][idx] = {'chapter': idx, 'resumo': '', 'entidades': [], 'falhou': True}

    # ---- identity
    progress.begin('identity', 1)
    progress.where('Juntando as entidades dos livros')
    series, log = assemble.build_series(llm, books, rules)
    progress.tick('identity')
    if not series['entidades']:
        raise RunFailed('O modelo não achou entidades verificáveis no texto: confira o modelo escolhido.')

    # ---- pages
    picked = [e for e in series['entidades'] if chapters_seen(e) >= cut]
    progress.begin('pages', len(picked))
    pages = {}

    def do_entity(ent):
        control()
        progress.where(f"Página de {ent['nome']}")
        pending_books, found = list(ent['livros']), {}
        for _ in range(3):   # a search that failed is not "no notes" for ever: only the books that failed are asked again
            if not pending_books:
                break
            still = []
            for bi in pending_books:
                n, info = wpg.search_notes(llm, books[bi], bi, ent['nome'], ent['aliases'])
                if info.get('status') == 'falhou':
                    still.append(bi)
                else:
                    found[bi] = n
            pending_books = still
        notes = [n for bi in sorted(found) for n in found[bi]]
        page = wpg.build_page(llm, ent, series, books, notes, 2, series['entidades'])
        return ent, page, notes, bool(pending_books)

    with concurrent.futures.ThreadPoolExecutor(max(1, parallel // 2)) as ex:
        futures = [ex.submit(do_entity, e) for e in picked]
        try:
            for f in concurrent.futures.as_completed(futures):
                ent, page, notes, incomplete = f.result()
                pages[ent['nome']] = {'page': page, 'notes': notes}
                partial = partial or incomplete
                progress.tick('pages')
        except BaseException:
            for f in futures:
                f.cancel()
            raise
    bundle, _count = build_bundle(books, series, pages, provider_kind, model, remote, partial=partial,
                                  suggestions=log.get('sugestoes', []), rules=rules)
    return bundle, partial
