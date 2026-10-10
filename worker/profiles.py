"""The profile of an author: what Wikidata and Wikipedia say about the person, for the page of the author (DEC-146).

It is read for a person who holds a Wikidata identifier that a human confirmed (DEC-095: the identifier came with an author an administrator
accepted; DEC-098: Open Library said which Wikidata entity that author is). Nothing is looked up by name: a name can be anybody. What is kept
is a short description, the years, the first paragraphs of the biography (CC BY-SA 4.0, kept with the page it came from) and a photo that is
downloaded, with its credit and license, so that whoever reads the library never asks Wikimedia for anything. Only the identifier, the title of
the page and the name of the image leave the server; nothing of the library. A provider that is off is not asked: Wikidata gates all of it,
Wikipedia the biography."""
import os
import re
import time
from urllib.parse import quote

from providers.http import get_binary, get_json
from providers.wikipedia import cut

SOURCE = 'wikidata'
ENTITIES = 'https://www.wikidata.org/w/api.php'
COMMONS = 'https://commons.wikimedia.org/w/api.php'
SUMMARY = 'https://{language}.wikipedia.org/api/rest_v1/page/summary/{title}'
QID = re.compile(r'^Q[0-9]{1,12}$')
HUMAN = 'Q5'
LANGUAGES = ('pt', 'en')   # the biography, the first that has a page
THUMB_WIDTH = 400
MAX_IMAGE = 1_500_000   # bytes
MAX_ATTEMPTS = 5
MAX_BIO = 1200          # characters, ending where a sentence ends
IMAGE_TYPES = {'image/jpeg': ('jpg', b'\xff\xd8\xff'), 'image/png': ('png', b'\x89PNG\r\n\x1a\n'), 'image/webp': ('webp', b'RIFF')}


def _claims(entity, prop):
    """The claims of a property that are not deprecated, the preferred first."""
    found = [c for c in ((entity.get('claims') or {}).get(prop) or []) if isinstance(c, dict) and c.get('rank') != 'deprecated']
    return sorted(found, key=lambda c: 0 if c.get('rank') == 'preferred' else 1)


def _value(claim):
    return ((claim.get('mainsnak') or {}).get('datavalue') or {}).get('value')


def when(claims):
    """'+1920-10-08T00:00:00Z' -> '1920-10-08', as far as Wikidata knows it (the precision says: 9 is the year, 10 the month, 11 the day). A year
    before the common era keeps its sign ('-0384'). None when there is no date."""
    for claim in claims:
        value = _value(claim)
        if not isinstance(value, dict):
            continue
        match = re.match(r'^([+-])(\d{1,12})-(\d{2})-(\d{2})', str(value.get('time') or ''))
        if not match:
            continue
        sign, year, month, day = match.groups()
        text = f"{'-' if sign == '-' else ''}{int(year):04d}"
        precision = value.get('precision') or 11
        if precision >= 10 and month != '00':
            text += f'-{month}'
            if precision >= 11 and day != '00':
                text += f'-{day}'
        return text
    return None


def parse_entity(entity):
    """The profile an entity gives, or None when the entity is not a person: an identifier that is not of a human is not an author's."""
    if not isinstance(entity, dict):
        return None
    if HUMAN not in [_value(c).get('id') for c in _claims(entity, 'P31') if isinstance(_value(c), dict)]:
        return None
    descriptions = entity.get('descriptions') or {}
    description = next((descriptions[l]['value'] for l in ('pt-br', 'pt', 'en') if (descriptions.get(l) or {}).get('value')), '')
    image = next((_value(c) for c in _claims(entity, 'P18') if isinstance(_value(c), str) and _value(c).strip()), None)
    sitelinks = entity.get('sitelinks') or {}
    pages = {l: sitelinks[f'{l}wiki']['title'] for l in LANGUAGES if (sitelinks.get(f'{l}wiki') or {}).get('title')}
    place = next((_value(c).get('id') for c in _claims(entity, 'P19') if isinstance(_value(c), dict) and QID.fullmatch(str(_value(c).get('id') or ''))), None)
    return {'description': description.strip(), 'born': when(_claims(entity, 'P569')), 'died': when(_claims(entity, 'P570')), 'image': image, 'pages': pages,
            'place_id': place}


def parse_place(data, qid):
    """The name of a place, from a wbgetentities answer: in Portuguese when it has one, else in English. None when it has no label."""
    entities = (data or {}).get('entities') if isinstance(data, dict) else None
    entity = (entities or {}).get(qid) if isinstance(entities, dict) else None
    labels = (entity or {}).get('labels') or {}
    for language in ('pt-br', 'pt', 'en'):
        value = (labels.get(language) or {}).get('value')
        if isinstance(value, str) and value.strip():
            return ' '.join(value.split())[:255]
    return None


def _born_place(profile):
    """Puts the name of the place of birth in the profile: `born_place`, and `place_read` saying whether the answer is final (a place the entity
    does not have is final; a request that failed is not, and is asked again)."""
    profile['born_place'], profile['place_read'] = None, True
    qid = profile.get('place_id')
    if not qid:
        return
    reply = get_json('Wikidata', ENTITIES, params={'action': 'wbgetentities', 'ids': qid, 'props': 'labels', 'languages': 'pt|pt-br|en', 'format': 'json'})
    if not reply.ok:
        profile['place_read'] = False
        return
    profile['born_place'] = parse_place(reply.data, qid)


def _plain(html):
    """A credit from Commons is written in HTML: the text of it, on one line."""
    text = re.sub(r'<[^>]*>', ' ', html or '')
    text = text.replace('&amp;', '&').replace('&lt;', '<').replace('&gt;', '>').replace('&quot;', '"').replace('&#39;', "'").replace('&nbsp;', ' ')
    return ' '.join(text.split())[:200]


def parse_image(data, filename):
    """The thumbnail to download and what to say about it: {url, mime, credit, license, license_url, page}. None when Commons has nothing for the
    file or it is not a picture we take."""
    pages = ((data or {}).get('query') or {}).get('pages') if isinstance(data, dict) else None
    page = next(iter(pages.values()), None) if isinstance(pages, dict) else None
    info = ((page or {}).get('imageinfo') or [None])[0] if isinstance(page, dict) else None
    if not isinstance(info, dict) or info.get('mime') not in IMAGE_TYPES:
        return None
    url = info.get('thumburl') or info.get('url')
    if not isinstance(url, str) or not url.startswith('https://'):
        return None
    meta = info.get('extmetadata') or {}

    def said(key):
        return _plain((meta.get(key) or {}).get('value'))
    return {
        'url': url, 'mime': info['mime'], 'credit': said('Artist') or said('Credit'), 'license': said('LicenseShortName') or said('UsageTerms'),
        'license_url': said('LicenseUrl'), 'page': info.get('descriptionurl') or f'https://commons.wikimedia.org/wiki/File:{quote(filename.replace(" ", "_"))}',
    }


def _biography(pages):
    """The summary of the page of the person, in the first language that has one: (language, title, url, text), or None."""
    for language in LANGUAGES:
        title = pages.get(language)
        if not title:
            continue
        reply = get_json('Wikipedia', SUMMARY.format(language=language, title=quote(title, safe='')))
        data = reply.data if reply.ok and isinstance(reply.data, dict) else {}
        extract = (data.get('extract') or '').strip()
        if data.get('type') == 'standard' and extract:
            url = ((data.get('content_urls') or {}).get('desktop') or {}).get('page') or f"https://{language}.wikipedia.org/wiki/{quote(title.replace(' ', '_'))}"
            return language, title, url, cut(extract, MAX_BIO)
    return None


def _photo(filename, qid, covers_dir):
    """Downloads the photo of the person from Commons and returns what to keep of it, or None."""
    reply = get_json('Wikidata', COMMONS, params={'action': 'query', 'titles': f'File:{filename}', 'prop': 'imageinfo', 'format': 'json',
                                                  'iiprop': 'url|size|mime|extmetadata', 'iiurlwidth': THUMB_WIDTH})
    image = parse_image(reply.data if reply.ok else None, filename)
    if image is None:
        return None
    download = get_binary('Wikidata', image['url'], MAX_IMAGE)
    extension, magic = IMAGE_TYPES[image['mime']]
    if not download.ok or not isinstance(download.data, bytes) or not download.data.startswith(magic):
        return None
    name = f'person_{qid}.{extension}'
    os.makedirs(covers_dir, exist_ok=True)
    with open(os.path.join(covers_dir, name), 'wb') as handle:
        handle.write(download.data)
    return {'path': f'/covers/{name}', 'credit': image['credit'], 'license': image['license'], 'license_url': image['license_url'], 'page': image['page']}


def fetch_profile(qid, covers_dir, allow_bio, allow_image=True):
    """('ok', profile), ('missing', None) when there is no such person, or ('failed', None) when Wikidata could not be reached. The identifier is checked
    first: it goes into a URL. `profile` has what parse_entity gives, `bio` ((language, title, url, text) or None), `bio_state` and `photo`."""
    if not QID.fullmatch(qid or ''):
        return 'missing', None
    reply = get_json('Wikidata', ENTITIES, params={'action': 'wbgetentities', 'ids': qid, 'props': 'descriptions|claims|sitelinks',
                                                    'languages': 'pt|pt-br|en', 'format': 'json'})
    if not reply.ok or not isinstance(reply.data, dict):
        return 'failed', None
    entities = reply.data.get('entities')
    entity = next(iter(entities.values()), None) if isinstance(entities, dict) and entities else None
    profile = parse_entity(entity)
    if profile is None:
        return 'missing', None
    profile['bio'], profile['bio_state'], profile['photo'] = None, 'pending', None
    _born_place(profile)
    if allow_bio:
        profile['bio'] = _biography(profile['pages'])
        profile['bio_state'] = 'done' if profile['bio'] else 'none'
    if allow_image and profile['image']:
        profile['photo'] = _photo(profile['image'], qid, covers_dir)
    return 'ok', profile


# A person holds a profile for each Wikidata identifier it holds: one statement, so what was read is never marked read without being kept.
_KEEP = """
    INSERT INTO person_profile (person_id, wikidata_id, description, born, died, bio, bio_language, bio_title, bio_url, bio_state,
                                image_path, image_credit, image_license, image_license_url, image_page_url, born_place, place_read, fetched_at)
    SELECT person_id, value, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, now()
    FROM person_authority WHERE scheme = 'wikidata' AND value = %s
    ON CONFLICT (person_id) DO UPDATE SET wikidata_id = EXCLUDED.wikidata_id, description = EXCLUDED.description, born = EXCLUDED.born,
        born_place = EXCLUDED.born_place, place_read = EXCLUDED.place_read,
        died = EXCLUDED.died, bio = EXCLUDED.bio, bio_language = EXCLUDED.bio_language, bio_title = EXCLUDED.bio_title, bio_url = EXCLUDED.bio_url,
        bio_state = EXCLUDED.bio_state, image_path = EXCLUDED.image_path, image_credit = EXCLUDED.image_credit, image_license = EXCLUDED.image_license,
        image_license_url = EXCLUDED.image_license_url, image_page_url = EXCLUDED.image_page_url, fetched_at = now()
    WHERE NOT person_profile.manual;"""
_REMEMBER = """
    INSERT INTO authority_lookups (source, key, state) VALUES ('wikidata', %s, %s)
    ON CONFLICT (source, key) DO UPDATE SET state = EXCLUDED.state, attempts = authority_lookups.attempts + 1, attempted_at = now();"""
_PENDING = """
    SELECT DISTINCT a.value FROM person_authority a
    WHERE a.scheme = 'wikidata' AND NOT EXISTS (SELECT 1 FROM person_profile p WHERE p.person_id = a.person_id)
      AND NOT EXISTS (
        SELECT 1 FROM authority_lookups l
        WHERE l.source = 'wikidata' AND l.key = a.value
          AND (l.state <> 'failed' OR l.attempts >= %s OR l.attempted_at > now() - interval '1 day'))
    ORDER BY a.value LIMIT %s"""
# A profile read before the place of birth was kept: asked again, once, for the place alone.
_PLACE_PENDING = "SELECT DISTINCT wikidata_id FROM person_profile WHERE NOT place_read AND NOT manual AND wikidata_id IS NOT NULL ORDER BY wikidata_id LIMIT %s"
_BIO_PENDING = "SELECT DISTINCT wikidata_id FROM person_profile WHERE bio_state = 'pending' AND NOT manual AND wikidata_id IS NOT NULL ORDER BY wikidata_id LIMIT %s"
# Another person with the same identifier (an author written two ways, not merged yet) has the profile already: no request for it.
_SHARE = """
    INSERT INTO person_profile (person_id, wikidata_id, description, born, died, bio, bio_language, bio_title, bio_url, bio_state,
                                image_path, image_credit, image_license, image_license_url, image_page_url, born_place, place_read, fetched_at)
    SELECT a.person_id, p.wikidata_id, p.description, p.born, p.died, p.bio, p.bio_language, p.bio_title, p.bio_url, p.bio_state,
           p.image_path, p.image_credit, p.image_license, p.image_license_url, p.image_page_url, p.born_place, p.place_read, p.fetched_at
    FROM person_authority a
    JOIN (SELECT DISTINCT ON (wikidata_id) * FROM person_profile WHERE NOT manual ORDER BY wikidata_id, fetched_at DESC) p ON p.wikidata_id = a.value
    WHERE a.scheme = 'wikidata' AND NOT EXISTS (SELECT 1 FROM person_profile q WHERE q.person_id = a.person_id)
    ON CONFLICT (person_id) DO NOTHING"""


def _keep_params(profile, qid):
    bio, photo = profile['bio'] or (None, None, None, ''), profile['photo'] or {}
    return (profile['description'], profile['born'], profile['died'], bio[3], bio[0], bio[1], bio[2], profile['bio_state'],
            photo.get('path'), photo.get('credit'), photo.get('license'), photo.get('license_url'), photo.get('page'),
            profile.get('born_place'), bool(profile.get('place_read')), qid)


def resolve_pending(db, allowed, covers_dir, limit=3, fetch=fetch_profile, sleep=time.sleep):
    """Reads the profile of a few authors that have a Wikidata identifier and no profile yet, and the biography of the ones that were read while
    Wikipedia was off. Only for what the owner turned on. Returns how many it got an answer for."""
    if not allowed(SOURCE):
        return 0
    answered = 0
    db.execute(_SHARE)
    for (qid,) in db.fetchall(_PENDING, (MAX_ATTEMPTS, limit)) or []:
        if not allowed(SOURCE):   # turned off meanwhile: not one more request
            break
        status, profile = fetch(qid, covers_dir, allow_bio=allowed('wikipedia'))
        if status == 'failed':
            db.execute(_REMEMBER, (qid, 'failed'))
        elif status == 'missing':
            db.execute(_REMEMBER, (qid, 'missing'))
            answered += 1
        else:
            db.execute(_KEEP + _REMEMBER, _keep_params(profile, qid) + (qid, 'done'))
            answered += 1
        sleep(0.5)
    for (qid,) in db.fetchall(_PLACE_PENDING, (limit,)) or []:
        if not allowed(SOURCE):
            break
        status, profile = fetch(qid, covers_dir, allow_bio=False, allow_image=False)
        if status == 'ok' and profile.get('place_read'):
            db.execute("UPDATE person_profile SET born_place = %s, place_read = TRUE WHERE wikidata_id = %s AND NOT place_read AND NOT manual",
                       (profile.get('born_place'), qid))
            answered += 1
        elif status == 'missing':
            db.execute("UPDATE person_profile SET place_read = TRUE WHERE wikidata_id = %s AND NOT place_read AND NOT manual", (qid,))   # no such person: no place to wait for
        sleep(0.5)
    if allowed('wikipedia'):
        for (qid,) in db.fetchall(_BIO_PENDING, (limit,)) or []:
            if not (allowed(SOURCE) and allowed('wikipedia')):
                break
            status, profile = fetch(qid, covers_dir, allow_bio=True, allow_image=False)
            if status == 'ok':
                bio = profile['bio'] or (None, None, None, '')
                db.execute("""UPDATE person_profile SET bio = %s, bio_language = %s, bio_title = %s, bio_url = %s, bio_state = %s
                              WHERE wikidata_id = %s AND bio_state = 'pending' AND NOT manual""",
                           (bio[3], bio[0], bio[1], bio[2], profile['bio_state'], qid))
                answered += 1
            sleep(0.5)
    return answered
