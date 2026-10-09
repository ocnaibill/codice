import json, time, re, requests
from cases import CASES
from improved import parse_file_title, close, fold, UA, main_title

def anilist(q):
    query = '''query($q:String){Media(search:$q,type:MANGA,sort:SEARCH_MATCH){id siteUrl format status countryOfOrigin volumes chapters startDate{year}
      title{romaji english native} description(asHtml:false) genres tags{name rank} averageScore coverImage{extraLarge}
      staff(perPage:6){edges{role node{name{full}}}}}}'''
    r = requests.post('https://graphql.anilist.co', json={'query': query, 'variables': {'q': q}}, timeout=20)
    if r.status_code != 200: return {'error': r.status_code}
    return r.json().get('data', {}).get('Media')

def mangadex(q):
    r = requests.get('https://api.mangadex.org/manga', params={'title': q, 'limit': 3, 'includes[]': ['author', 'artist', 'cover_art'], 'order[relevance]': 'desc'}, headers=UA, timeout=20)
    if r.status_code != 200: return {'error': r.status_code}
    d = r.json().get('data', [])
    return d[0] if d else None

def openlib_volume(series, n):
    r = requests.get('https://openlibrary.org/search.json', params={'q': f'{series} volume {n}', 'limit': 8, 'fields': 'key,title,author_name,first_publish_year,isbn,publisher,cover_i,edition_count'}, headers=UA, timeout=20)
    docs = r.json().get('docs', []) if r.status_code == 200 else []
    for d in docs:
        t = fold(d.get('title') or '')
        if fold(series).split()[0] in t and re.search(rf'(?:vol\.?|volume|v|#|\b)\s*0*{n}\b', t): return d
    return None

out = {}
for c in CASES:
    if c['kind'] not in ('manga', 'comic'): continue
    title, number, _ = parse_file_title(c['sent'])
    row = {'parsed': (title, number)}
    if c['kind'] == 'manga':
        a = anilist(title); time.sleep(1.2)
        md = mangadex(title); time.sleep(0.6)
        row['anilist'] = a; row['mangadex'] = md
        if number: row['ol_volume'] = openlib_volume(title, number); time.sleep(0.8)
    else:
        row['ol_volume'] = openlib_volume(title, c.get('issue') or 1) if c.get('issue') else None; time.sleep(0.8)
    out[c['id']] = row
def strip(o):  # the synopses are the providers' texts: only what is needed to judge the match is kept
    if isinstance(o, dict): return {k: strip(v) for k, v in o.items() if k not in ('description', 'descriptions')}
    if isinstance(o, list): return [strip(x) for x in o]
    return o
json.dump(strip(out), open('resultado_series_sem_textos.json', 'w'), ensure_ascii=False, indent=1, default=str)

def summ_al(a):
    if not a or 'error' in a: return str(a)[:30]
    names = [e['node']['name']['full'] for e in a['staff']['edges']][:3]
    return f"{a['title']['romaji']} | {a['format']} {a['startDate']['year']} | vol={a['volumes']} | autores={names} | gêneros={a['genres'][:3]} | desc={bool(a['description'])} cover={bool(a['coverImage']['extraLarge'])}"
def summ_md(m):
    if not m or 'error' in m: return str(m)[:30]
    at = m['attributes']; t = list(at['title'].values())[0]
    rel = [r['attributes']['name'] for r in m.get('relationships', []) if r['type'] in ('author', 'artist') and r.get('attributes')]
    return f"{t} | {at.get('year')} | demog={at.get('publicationDemographic')} | autores={sorted(set(rel))[:3]} | tags={len(at['tags'])} | desc_langs={list(at['description'].keys())[:3]}"
for c in CASES:
    if c['id'] not in out: continue
    r = out[c['id']]
    print(f"\n{c['id']} {r['parsed']}")
    if 'anilist' in r: print('  AniList :', summ_al(r['anilist'])); print('  MangaDex:', summ_md(r['mangadex']))
    v = r.get('ol_volume'); print('  OL volume:', None if not v else f"{v['title']} / {(v.get('author_name') or [''])[0]} isbn={(v.get('isbn') or [None])[0]} cover={bool(v.get('cover_i'))}")
