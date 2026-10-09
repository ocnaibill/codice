"""What the code does today, run as it is: the real providers of worker/providers, with the query the pipeline sends (the title only)."""
import sys, json, time
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[3] / 'worker'))   # the real providers of the worker
from providers.openlibrary import OpenLibraryProvider
from providers.registry import ProviderRegistry
from cases import CASES

ol = OpenLibraryProvider()
out = {}
for c in CASES:
    q = ProviderRegistry._sanitize_query(c['sent'])
    r = ol.search(q)
    out[c['id']] = None if r is None else dict(title=r.title, author=r.author, credits=[x.name for x in r.credits], publisher=r.publisher,
        date=r.publication_date, isbn=r.isbn, cover=bool(r.cover_url), desc=bool(r.description), tags=r.tags, query=q)
    time.sleep(0.6)
json.dump(out, open('resultado_atual_openlibrary.json', 'w'), ensure_ascii=False, indent=1)
for c in CASES:
    r = out[c['id']]
    print(f"{c['id']:12} {c['sent'][:34]:34} -> {None if r is None else (r['title'] or '')[:40] + ' / ' + str(r['author'])[:22]}")
