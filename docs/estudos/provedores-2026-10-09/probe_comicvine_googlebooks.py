"""Read-only probe of the two providers that the study could not measure without a key: ComicVine and Google Books.

It changes nothing and writes nothing: it asks a few questions and prints what came back. Run it with the keys in the environment,
which it only sends to the provider they belong to (it never prints them):

    COMICVINE_API_KEY=... GOOGLE_BOOKS_API_KEY=... worker/venv/bin/python docs/estudos/provedores-2026-10-09/probe_comicvine_googlebooks.py

What it shows:
  ComicVine: what the search that the worker does today (resources=issue, limit=1) answers for three issues of one series, and
             what the search by volume and then the issue of that volume (filter) answers.
  Google Books: what the worker asks today (q=<title>, one result) against the title and the author apart, with the language.
It also shows the status Google Books answers WITHOUT a key, which is what the worker does when GOOGLE_BOOKS_API_KEY is empty.
"""
import os
import sys
import requests

UA = {'User-Agent': 'CodiceStudy/1.0 (https://github.com/ocnaibill/codice)'}


def scrub(text, *secrets):
    for secret in secrets:
        if secret:
            text = str(text).replace(secret, '***')
    return str(text)


def comicvine(key):
    print('\n== ComicVine ==')
    if not key:
        print('COMICVINE_API_KEY is empty: skipped.')
        return
    base = 'https://comicvine.gamespot.com/api'

    def get(path, **params):
        r = requests.get(f'{base}/{path}', params={'api_key': key, 'format': 'json', **params}, headers=UA, timeout=20)
        return r.status_code, (r.json() if r.status_code == 200 else {})

    print('\n-- what the worker does today: search resources=issue, limit=1, the file name as the query')
    for query in ('Absolute Batman 001', 'Absolute Batman 006', 'Absolute Batman 012'):
        status, data = get('search', query=query, resources='issue', limit=1)
        hit = (data.get('results') or [{}])[0]
        print(f'  {query!r}: HTTP {status} -> issue id {hit.get("id")}, name {hit.get("name")!r}, #{hit.get("issue_number")}, '
              f'volume {(hit.get("volume") or {}).get("name")!r}')

    print('\n-- search the volume first, then ask for the issue of that volume')
    status, data = get('search', query='Absolute Batman', resources='volume', limit=5)
    volumes = data.get('results') or []
    for v in volumes:
        print(f'  volume {v.get("id")}: {v.get("name")!r} ({v.get("start_year")}), {v.get("count_of_issues")} issues, publisher {(v.get("publisher") or {}).get("name")!r}')
    if volumes:
        volume = volumes[0]['id']
        for number in (1, 6, 12):
            status, data = get('issues', filter=f'volume:{volume},issue_number:{number}', field_list='id,name,issue_number,cover_date,volume,image,description')
            issues = data.get('results') or []
            hit = issues[0] if issues else {}
            print(f'  volume {volume} issue #{number}: HTTP {status} -> {len(issues)} found, id {hit.get("id")}, name {hit.get("name")!r}, '
                  f'date {hit.get("cover_date")}, description {"yes" if hit.get("description") else "no"}')
        if issues:
            status, detail = get(f'issue/4000-{issues[0]["id"]}', field_list='person_credits,character_credits,description')
            credits = (detail.get('results') or {}).get('person_credits') or []
            print(f'  the issue detail: HTTP {status}, {len(credits)} people credited, e.g. {[(c.get("name"), c.get("role")) for c in credits[:3]]}')


def google_books(key):
    print('\n== Google Books ==')
    base = 'https://www.googleapis.com/books/v1/volumes'

    def get(params, with_key):
        p = dict(params)
        if with_key and key:
            p['key'] = key
        r = requests.get(base, params=p, headers=UA, timeout=20)
        body = r.json() if r.headers.get('content-type', '').startswith('application/json') else {}
        return r.status_code, body

    status, body = get({'q': 'dune', 'maxResults': 1}, with_key=False)
    why = (body.get('error') or {}).get('message', '')[:110]
    print(f'without a key (what the worker does when GOOGLE_BOOKS_API_KEY is empty): HTTP {status} {scrub(why)}')
    if not key:
        print('GOOGLE_BOOKS_API_KEY is empty: the comparison below was skipped.')
        return
    cases = [
        ('Harry Potter e a Pedra Filosofal', 'J. K. Rowling', 'pt'),
        ('Duna', 'Frank Herbert', 'pt'),
        ('A Nuvem', 'Neal Shusterman', 'pt'),
        ('Dune', 'Frank Herbert', 'en'),
    ]
    for title, author, lang in cases:
        print(f'\n-- {title!r} / {author}')
        for label, params in (
            ('today: q=<title>, 1 result', {'q': title, 'maxResults': 1}),
            ('title+author+language, 3 results', {'q': f'intitle:{title} inauthor:{author}', 'langRestrict': lang, 'printType': 'books', 'maxResults': 3}),
        ):
            status, body = get(params, with_key=True)
            for item in (body.get('items') or [])[:3]:
                info = item.get('volumeInfo', {})
                print(f'  [{label}] HTTP {status}: {info.get("title")!r} / {info.get("authors")} ({info.get("language")}) '
                      f'description {"yes" if info.get("description") else "no"}, isbn {"yes" if info.get("industryIdentifiers") else "no"}')
            if not body.get('items'):
                print(f'  [{label}] HTTP {status}: nothing')


if __name__ == '__main__':
    cv, gb = os.getenv('COMICVINE_API_KEY', ''), os.getenv('GOOGLE_BOOKS_API_KEY', '')
    try:
        comicvine(cv)
        google_books(gb)
    except Exception as e:  # a probe that fails says why and leaves nothing behind
        print('The probe stopped:', scrub(e, cv, gb))
        sys.exit(1)
