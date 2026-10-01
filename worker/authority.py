"""What a reference source knows about an author, to tell when two people are one (#63, DEC-098).

When an administrator accepts an author a provider suggested, the person keeps the provider's key for it (Open
Library: "OL79034A"). Open Library also knows, for that key, the identifiers other authority files give the
same person: Wikidata, VIAF and ISNI. Those are the strongest evidence that two spellings are one person, and
this reads them. It asks Open Library about a key, nothing else: no name, no title. It is as far as the owner's
choice goes: a provider that is off is not asked.

Nothing is decided here. What is read is kept as identifiers of the person; two people that end up holding the
same one are proposed for merging by the backend, and a person decides.
"""
import re
import time

import requests

SOURCE = 'openlibrary'
URL = 'https://openlibrary.org/authors/{key}.json'
KEY = re.compile(r'^OL[0-9]{1,12}A$')
# The identifiers worth keeping, each as the source writes it: anything else is not an identifier of that kind.
WANTED = {
    'wikidata': re.compile(r'^Q[0-9]{1,12}$'),
    'viaf': re.compile(r'^[0-9]{1,12}$'),
    'isni': re.compile(r'^[0-9]{15}[0-9X]$'),
}
MAX_ATTEMPTS = 5
PAUSE_SECONDS = 0.5


def parse(doc):
    """('redirect', key) when the author was merged into another, else ('ids', {scheme: value}) with the
    identifiers of the kinds above that the record has, written as the source writes them."""
    if not isinstance(doc, dict):
        return 'ids', {}
    kind = (doc.get('type') or {}).get('key') if isinstance(doc.get('type'), dict) else None
    if kind == '/type/redirect':
        location = doc.get('location')
        target = location.rsplit('/', 1)[-1] if isinstance(location, str) else ''
        return ('redirect', target) if KEY.fullmatch(target) else ('ids', {})
    remote = doc.get('remote_ids')
    ids = {}
    for scheme, pattern in WANTED.items():
        value = remote.get(scheme) if isinstance(remote, dict) else None
        if isinstance(value, str) and pattern.fullmatch(value.strip()):
            ids[scheme] = value.strip()
    return 'ids', ids


def fetch(key, get=requests.get):
    """('ok', document), ('missing', None) or ('failed', None). The key is checked first: it goes into a URL."""
    if not KEY.fullmatch(key or ''):
        return 'missing', None
    try:
        response = get(URL.format(key=key), headers={'User-Agent': 'Codice/1.0'}, timeout=10)
        if response.status_code == 404:
            return 'missing', None
        if response.status_code != 200:
            return 'failed', None
        return 'ok', response.json()
    except Exception as err:
        print(f"   ⚠️ Open Library author {key} not reached: {type(err).__name__}")
        return 'failed', None


_PENDING = """
    SELECT DISTINCT a.value FROM person_authority a
    WHERE a.scheme = 'openlibrary'
      AND NOT EXISTS (
        SELECT 1 FROM authority_lookups l
        WHERE l.source = 'openlibrary' AND l.key = a.value
          AND (l.state <> 'failed' OR l.attempts >= %s OR l.attempted_at > now() - interval '1 day'))
    ORDER BY a.value LIMIT %s"""

# One statement, so a lookup is never marked done without what it found being kept. The `%s` are, in order:
# the scheme and value to keep, the key they were found for (twice) and the state to remember.
_KEEP = """
    INSERT INTO person_authority (person_id, scheme, value, source)
    SELECT person_id, %s, %s, 'Open Library' FROM person_authority WHERE scheme = 'openlibrary' AND value = %s
    ON CONFLICT DO NOTHING;"""
_REMEMBER = """
    INSERT INTO authority_lookups (source, key, state) VALUES ('openlibrary', %s, %s)
    ON CONFLICT (source, key) DO UPDATE SET state = EXCLUDED.state, attempts = authority_lookups.attempts + 1, attempted_at = now();"""
_ASK_FOR_COMPARISON = """
    INSERT INTO jobs (type, payload, priority)
    SELECT 'dedupe', '{}'::jsonb, 0
    WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE type = 'dedupe' AND work_id IS NULL AND state IN ('pending', 'running'))"""


def resolve_pending(db, allowed, limit=5, get=requests.get, sleep=time.sleep):
    """Looks up, in Open Library, a few keys that people hold and nobody has looked up yet. Returns how many it
    got an answer for. Only when the owner turned Open Library on."""
    if not allowed(SOURCE):
        return 0
    answered, found = 0, False
    for (key,) in db.fetchall(_PENDING, (MAX_ATTEMPTS, limit)) or []:
        if not allowed(SOURCE):  # turned off meanwhile: not one more request
            break
        status, doc = fetch(key, get)
        if status == 'failed':
            db.execute(_REMEMBER, (key, 'failed'))
        elif status == 'missing':
            db.execute(_REMEMBER, (key, 'missing'))
            answered += 1
        else:
            kind, value = parse(doc)
            statements, params = [], []
            if kind == 'redirect':
                # The author was merged into another key: whoever holds this one holds that one too.
                statements.append(_KEEP); params += ['openlibrary', value, key]
            else:
                for scheme, identifier in value.items():
                    statements.append(_KEEP); params += [scheme, identifier, key]
            statements.append(_REMEMBER); params += [key, 'redirect' if kind == 'redirect' else 'done']
            db.execute(''.join(statements), tuple(params))
            found = found or bool(value)
            answered += 1
        sleep(PAUSE_SECONDS)
    if found:
        db.execute(_ASK_FOR_COMPARISON)
    return answered
