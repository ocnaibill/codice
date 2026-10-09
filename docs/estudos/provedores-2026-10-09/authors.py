import requests, json, time
from improved import UA
AUTHORS = ['Frank Herbert', 'Machado de Assis', 'Paulo Coelho', 'Kentaro Miura', 'Andrew S. Tanenbaum', 'Neal Shusterman']
out = {}
for name in AUTHORS:
    row = {}
    # Open Library
    r = requests.get('https://openlibrary.org/search/authors.json', params={'q': name, 'limit': 1}, headers=UA, timeout=20).json().get('docs', [])
    if r:
        key = r[0]['key']; j = requests.get(f'https://openlibrary.org/authors/{key}.json', headers=UA, timeout=20).json()
        bio = j.get('bio'); bio = bio.get('value') if isinstance(bio, dict) else bio
        row['openlibrary'] = dict(key=key, name=r[0].get('name'), bio=bool(bio), bio_chars=len(bio or ''), birth=j.get('birth_date'), death=j.get('death_date'), photos=bool(j.get('photos')),
                                   links=len(j.get('links') or []), remote_ids=sorted((j.get('remote_ids') or {}).keys()))
    # Wikidata search + entity
    s = requests.get('https://www.wikidata.org/w/api.php', params={'action': 'wbsearchentities', 'search': name, 'language': 'en', 'format': 'json', 'limit': 3, 'type': 'item'}, headers=UA, timeout=20).json().get('search', [])
    if s:
        qid = s[0]['id']
        e = requests.get('https://www.wikidata.org/w/api.php', params={'action': 'wbgetentities', 'ids': qid, 'props': 'labels|descriptions|claims|sitelinks', 'languages': 'pt|en', 'format': 'json'}, headers=UA, timeout=20).json()['entities'][qid]
        cl = e.get('claims', {})
        def val(p):
            try: return cl[p][0]['mainsnak']['datavalue']['value']
            except Exception: return None
        row['wikidata'] = dict(qid=qid, desc_en=s[0].get('description'), label_pt=(e['labels'].get('pt') or {}).get('value'), desc_pt=(e['descriptions'].get('pt') or {}).get('value'),
                               birth=(val('P569') or {}).get('time') if isinstance(val('P569'), dict) else None, image=bool(val('P18')), viaf=val('P214'), isni=val('P213'),
                               ol=val('P648'), pt_wikipedia='ptwiki' in e.get('sitelinks', {}), en_wikipedia='enwiki' in e.get('sitelinks', {}), ids=sum(1 for p in ('P214', 'P213', 'P227', 'P648', 'P244') if p in cl))
        for lang in ('pt', 'en'):
            link = e.get('sitelinks', {}).get(f'{lang}wiki')
            if link:
                w = requests.get(f"https://{lang}.wikipedia.org/api/rest_v1/page/summary/{requests.utils.quote(link['title'])}", headers=UA, timeout=20)
                if w.status_code == 200: row.setdefault('wikipedia', {})[lang] = dict(extract_chars=len(w.json().get('extract', '')), thumbnail=bool(w.json().get('thumbnail')), url=w.json().get('content_urls', {}).get('desktop', {}).get('page'))
    out[name] = row
    time.sleep(0.8)
json.dump(out, open('resultado_autores.json', 'w'), ensure_ascii=False, indent=1)
for n, r in out.items():
    print('\n', n)
    for k, v in r.items(): print('  ', k, json.dumps(v, ensure_ascii=False)[:300])
