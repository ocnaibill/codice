"""A prototype of what a better lookup would do, to measure it (not production code): read the file's title (number, author), ask for
several candidates with the title and the author apart, score them against what the file says, accept only what is close, then
ask for the details (description) of the one chosen."""
import re, json, time, unicodedata, difflib, requests
from cases import CASES

UA = {'User-Agent': 'CodiceStudy/1.0 (https://github.com/ocnaibill/codice)'}
def fold(s): return ''.join(c for c in unicodedata.normalize('NFKD', (s or '').lower()) if not unicodedata.combining(c))
def toks(s): return [t for t in re.split(r'[^a-z0-9]+', fold(s)) if t]
STOP = {'the','a','an','o','os','as','e','de','do','da','of','and','in','vol','volume','v'}

def parse_file_title(sent, author=None):
    """'A Nuvem 2 - Neal Shusterman' -> title 'A Nuvem', number 2, author 'Neal Shusterman'."""
    t = re.sub(r'\([^)]*\)|\[[^\]]*\]', ' ', sent).strip()
    guess = None
    if ' - ' in t:
        left, right = t.rsplit(' - ', 1)
        # the part after the dash is a name when it is not the title: no digits and 2-4 words
        if re.fullmatch(r"[A-Za-zÀ-ÿ.'’ ]{3,}", right) and 1 < len(right.split()) <= 4:
            t, guess = left.strip(), right.strip()
    m = re.search(r'(?:^|\s)(?:v|vol\.?|volume|#|n[ºo]\.?|livro|book)?\s*0*(\d{1,4})$', t, re.I)
    number = None
    if m and len(t) > len(m.group(0)) + 1:
        number = int(m.group(1)); t = t[:m.start()].strip(' -:,')
    return t, number, author or guess

def close(a, b):
    ta, tb = [t for t in toks(a) if t not in STOP], [t for t in toks(b) if t not in STOP]
    if not ta or not tb: return 0.0
    inter = len(set(ta) & set(tb)); return 2 * inter / (len(set(ta)) + len(set(tb)))

def ol_candidates(title, author=None, limit=8, free=False):
    if free:
        r = requests.get('https://openlibrary.org/search.json', params={'q': f'{title} {author or ""}'.strip(), 'limit': limit, 'fields': 'key,title,subtitle,author_name,author_key,first_publish_year,isbn,publisher,language,subject,cover_i,edition_count'}, headers=UA, timeout=15)
        return r.json().get('docs', []) if r.status_code == 200 else []
    p = {'limit': limit, 'fields': 'key,title,subtitle,author_name,author_key,first_publish_year,isbn,publisher,language,subject,cover_i,edition_count,number_of_pages_median'}
    if author: p.update(title=title, author=author)
    else: p['title'] = title
    r = requests.get('https://openlibrary.org/search.json', params=p, headers=UA, timeout=15)
    return r.json().get('docs', []) if r.status_code == 200 else []

def main_title(t): return re.split(r'\s*[:–—]\s*', t)[0]
def score(doc, title, author):
    s = max(close(title, doc.get('title', '')), close(main_title(title), doc.get('title', ''))) * 100
    if doc.get('subtitle'): s = max(s, close(title, doc['title'] + ' ' + doc['subtitle']) * 100)
    if author:
        a = max([close(author, n) for n in (doc.get('author_name') or [])] or [0])
        s += a * 60
    s += min(doc.get('edition_count') or 0, 40) * 0.25   # a work many editions of is the one people mean
    return s

def ol_details(key):
    r = requests.get(f'https://openlibrary.org{key}.json', headers=UA, timeout=15)
    if r.status_code != 200: return {}
    j = r.json(); d = j.get('description')
    if isinstance(d, dict): d = d.get('value')
    return {'description': d, 'subjects': (j.get('subjects') or [])[:8], 'first_publish_date': j.get('first_publish_date')}

THRESHOLD = 70
STRICT = 0.85   # how close the title has to be, on its own, whatever the author says
def improved_book(c):
    title, number, author = parse_file_title(c['sent'], c['author'])
    cands = ol_candidates(title, author)
    cands += [d for d in ol_candidates(title, author, free=True) if d['key'] not in {x['key'] for x in cands}]   # the whole text, for a title in another language
    scored = sorted(((score(d, title, author), d) for d in cands), key=lambda x: -x[0])
    if scored:
        d0 = scored[0][1]
        tclose = max(close(title, d0.get('title', '')), close(main_title(title), d0.get('title', '')))
        if tclose < STRICT: scored = []
    if not scored or scored[0][0] < THRESHOLD: return None, dict(parsed=(title, number, author), best=scored[0][0] if scored else None)
    s, d = scored[0]
    det = ol_details(d['key'])
    return dict(title=d.get('title'), author=', '.join(d.get('author_name') or []), date=d.get('first_publish_year'), isbn=(d.get('isbn') or [None])[0],
                publisher=(d.get('publisher') or [None])[0], cover=bool(d.get('cover_i')), desc=bool(det.get('description')), tags=(det.get('subjects') or d.get('subject') or [])[:5],
                score=round(s)), dict(parsed=(title, number, author))

EXACT = {
 'dune': ['dune'], 'neuromancer': ['neuromancer'], 'pragprog': ['the pragmatic programmer'], 'modernos': ['modern operating systems'],
 'cleancode': ['clean code'], 'sicp': ['structure and interpretation of computer programs'], '1984': ['1984', 'nineteen eighty'], 'hobbit': ['the hobbit'],
 'duna_pt': ['duna', 'dune'], 'casmurro': ['dom casmurro'], 'alquimista': ['o alquimista', 'the alchemist', 'alquimista'], 'sapiens': ['sapiens'],
 'lusiadas': ['os lus'], 'hp_pt': ['harry potter and the philosopher', 'harry potter e a pedra', 'harry potter and the sorcerer'],
 'nuvem': ['thunderhead', 'a nuvem'], 'scythe_pt': ['scythe', 'ceifador'],
}
def classify(rec, c):
    """ok: the book the file is; none: nothing was proposed; wrong: something else was proposed."""
    if not rec: return 'none'
    t = fold(rec.get('title') or ''); a = fold(rec.get('author') or '')
    return 'ok' if any(t.startswith(w) for w in EXACT[c['id']]) and (not c['want_author'] or any(w in a for w in c['want_author'])) else 'wrong'
good = lambda rec, c: classify(rec, c) == 'ok'

if __name__ == '__main__':
    cur = json.load(open('resultado_atual_openlibrary.json'))
    rows = []
    for c in CASES:
        if c['kind'] != 'book': continue
        rec, dbg = improved_book(c)
        rows.append((c['id'], classify(cur[c['id']], c), classify(rec, c), rec, dbg))
        time.sleep(0.8)
    json.dump({r[0]: dict(current=r[1], improved=r[2], rec=r[3], debug=r[4]) for r in rows}, open('resultado_prototipo_livros.json', 'w'), ensure_ascii=False, indent=1, default=str)
    for rid, a, b, rec, dbg in rows:
        print(f"{rid:12} atual={a:5}  novo={b:5}  {dbg['parsed']} -> {None if not rec else (rec['title'][:34] + ' / ' + rec['author'][:22] + ' desc=' + str(rec['desc']))}")
    for k in ('ok', 'none', 'wrong'):
        print(k, 'atual', sum(r[1] == k for r in rows), 'novo', sum(r[2] == k for r in rows))
    print('com descrição: atual 0 (a busca da Open Library não traz), novo', sum(1 for r in rows if r[3] and r[3]['desc']), 'de', len(rows))
