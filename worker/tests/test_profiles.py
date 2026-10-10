"""The profile of an author, from Wikidata, Wikipedia and Commons (DEC-146)."""
import os
from unittest.mock import MagicMock, patch

import pytest

import profiles
from profiles import MAX_IMAGE, fetch_profile, parse_entity, parse_image, resolve_pending, when
from providers import http
from providers.http import Reply, get_binary


def claim(value, rank='normal', datatype=None):
    return {'mainsnak': {'datavalue': {'value': value}}, 'rank': rank}


def time(text, precision=11):
    return {'time': text, 'precision': precision}


def human(**over):
    entity = {
        'id': 'Q7934',
        'descriptions': {'pt': {'value': 'escritor de ficção científica americano (1920-1986)'}, 'en': {'value': 'American writer (1920–1986)'}},
        'claims': {
            'P31': [claim({'id': 'Q5'})],
            'P569': [claim(time('+1920-10-08T00:00:00Z'))],
            'P570': [claim(time('+1986-02-11T00:00:00Z'))],
            'P18': [claim('Frank Herbert 1984 (square).jpg')],
        },
        'sitelinks': {'ptwiki': {'title': 'Frank Herbert'}, 'enwiki': {'title': 'Frank Herbert'}, 'dewiki': {'title': 'Frank Herbert'}},
    }
    entity.update(over)
    return entity


class TestDates:
    def test_a_date_is_as_exact_as_wikidata_knows_it(self):
        assert when([claim(time('+1920-10-08T00:00:00Z', 11))]) == '1920-10-08'
        assert when([claim(time('+1920-10-00T00:00:00Z', 10))]) == '1920-10'
        assert when([claim(time('+1920-00-00T00:00:00Z', 9))]) == '1920'
        assert when([claim(time('+1920-10-08T00:00:00Z', 9))]) == '1920'      # the precision is the year: the month and the day are not known
        assert when([claim(time('+1920-10-08T00:00:00Z', 10))]) == '1920-10'
        assert when([claim(time('+1920-00-00T00:00:00Z', 11))]) == '1920'     # a month and a day of zero say nothing
        assert when([claim(time('+1920-10-00T00:00:00Z', 11))]) == '1920-10'

    def test_a_year_before_the_common_era_keeps_its_sign_and_a_short_year_is_padded(self):
        assert when([claim(time('-0384-00-00T00:00:00Z', 9))]) == '-0384'
        assert when([claim(time('+0899-01-01T00:00:00Z', 9))]) == '0899'

    def test_the_preferred_value_comes_first_and_a_deprecated_one_never(self):
        entity = {'claims': {'P569': [claim(time('+1775-01-01T00:00:00Z', 9)), claim(time('+1775-12-16T00:00:00Z', 11), rank='preferred')]}}
        assert when(profiles._claims(entity, 'P569')) == '1775-12-16'
        entity = {'claims': {'P569': [claim(time('+1877-07-24T00:00:00Z'), rank='deprecated'), claim(time('+1817-07-18T00:00:00Z'))]}}
        assert when(profiles._claims(entity, 'P569')) == '1817-07-18'

    def test_a_date_with_no_precision_is_taken_as_exact(self):
        assert when([claim({'time': '+1920-10-08T00:00:00Z'})]) == '1920-10-08'

    def test_nothing_in_it_is_no_date(self):
        for claims in ([], [claim('not a time')], [claim({'time': 'x'})], [claim(None)], [{'mainsnak': {}}]):
            assert when(claims) is None


class TestAnEntityThatIsAPerson:
    def test_it_gives_the_description_the_years_the_photo_and_the_pages(self):
        got = parse_entity(human())
        assert got == {'description': 'escritor de ficção científica americano (1920-1986)', 'born': '1920-10-08', 'died': '1986-02-11',
                       'image': 'Frank Herbert 1984 (square).jpg', 'pages': {'pt': 'Frank Herbert', 'en': 'Frank Herbert'}, 'place_id': None}

    def test_the_description_is_the_one_in_portuguese_of_brazil_then_of_portugal_then_english(self):
        e = human(descriptions={'en': {'value': 'writer'}, 'pt': {'value': 'escritor'}, 'pt-br': {'value': 'escritor brasileiro'}})
        assert parse_entity(e)['description'] == 'escritor brasileiro'
        assert parse_entity(human(descriptions={'en': {'value': 'writer'}, 'pt': {'value': 'escritor'}}))['description'] == 'escritor'
        assert parse_entity(human(descriptions={'en': {'value': ' writer '}}))['description'] == 'writer'
        assert parse_entity(human(descriptions={'pt': {'value': ''}, 'en': {'value': 'writer'}}))['description'] == 'writer'
        assert parse_entity(human(descriptions={}))['description'] == ''

    def test_what_is_not_a_human_is_not_an_author(self):
        for claims in ({'P31': [claim({'id': 'Q11424'})]}, {'P31': []}, {}, {'P31': [claim('Q5')]}, {'P31': [claim({'id': 'Q5'}, rank='deprecated')]}):
            assert parse_entity(human(claims=claims)) is None, claims
        assert parse_entity({'missing': ''}) is None and parse_entity(None) is None and parse_entity('x') is None and parse_entity({'id': 'Q1', 'missing': ''}) is None

    def test_a_person_without_the_rest_has_only_what_there_is(self):
        got = parse_entity({'descriptions': {}, 'claims': {'P31': [claim({'id': 'Q5'})]}, 'sitelinks': {}})
        assert got == {'description': '', 'born': None, 'died': None, 'image': None, 'pages': {}, 'place_id': None}

    def test_the_photo_is_the_first_name_of_a_file_and_the_pages_are_only_the_ones_it_reads(self):
        got = parse_entity(human(claims={'P31': [claim({'id': 'Q5'})], 'P18': [claim({'id': 'Q1'}), claim('  '), claim('A.jpg')]}))
        assert got['image'] == 'A.jpg'
        assert parse_entity(human(sitelinks={'dewiki': {'title': 'x'}, 'ptwiki': {'title': ''}}))['pages'] == {}


COMMONS = {'query': {'pages': {'1': {'imageinfo': [{
    'mime': 'image/jpeg', 'thumburl': 'https://upload.wikimedia.org/thumb/a.jpg', 'url': 'https://upload.wikimedia.org/a.jpg',
    'descriptionurl': 'https://commons.wikimedia.org/wiki/File:A.jpg',
    'extmetadata': {'Artist': {'value': '<a href="x">Jane  <b>Doe</b></a> &amp; Co'}, 'Credit': {'value': 'own work'},
                    'LicenseShortName': {'value': 'CC BY-SA 4.0'}, 'LicenseUrl': {'value': 'https://creativecommons.org/licenses/by-sa/4.0'}}}]}}}}


class TestThePhoto:
    def test_what_commons_says_of_a_file(self):
        assert parse_image(COMMONS, 'A.jpg') == {
            'url': 'https://upload.wikimedia.org/thumb/a.jpg', 'mime': 'image/jpeg', 'credit': 'Jane Doe & Co', 'license': 'CC BY-SA 4.0',
            'license_url': 'https://creativecommons.org/licenses/by-sa/4.0', 'page': 'https://commons.wikimedia.org/wiki/File:A.jpg'}

    def test_the_credit_falls_back_to_the_credit_line_and_the_license_to_the_terms_of_use(self):
        data = {'query': {'pages': {'1': {'imageinfo': [{'mime': 'image/png', 'url': 'https://u.org/a.png', 'extmetadata': {
            'Credit': {'value': 'Wikimedia Commons'}, 'UsageTerms': {'value': 'Public domain'}}}]}}}}
        got = parse_image(data, 'My File.png')
        assert (got['url'], got['credit'], got['license'], got['license_url']) == ('https://u.org/a.png', 'Wikimedia Commons', 'Public domain', '')
        assert got['page'] == 'https://commons.wikimedia.org/wiki/File:My_File.png'

    def test_a_file_that_is_not_a_picture_we_take_or_is_not_over_https_is_not_taken(self):
        for mime in ('image/svg+xml', 'image/gif', 'application/pdf', None):
            data = {'query': {'pages': {'1': {'imageinfo': [{'mime': mime, 'thumburl': 'https://u.org/a'}]}}}}
            assert parse_image(data, 'A.x') is None, mime
        assert parse_image({'query': {'pages': {'1': {'imageinfo': [{'mime': 'image/jpeg', 'thumburl': 'http://u.org/a.jpg'}]}}}}, 'A') is None
        assert parse_image({'query': {'pages': {'1': {'imageinfo': [{'mime': 'image/jpeg'}]}}}}, 'A') is None

    def test_nothing_from_commons_is_no_photo(self):
        for data in (None, {}, {'query': {}}, {'query': {'pages': {}}}, {'query': {'pages': {'-1': {'missing': ''}}}}, ['x']):
            assert parse_image(data, 'A.jpg') is None, data

    def test_a_credit_is_text_on_one_line_and_not_longer_than_a_name_can_be(self):
        data = {'query': {'pages': {'1': {'imageinfo': [{'mime': 'image/jpeg', 'thumburl': 'https://u.org/a.jpg', 'extmetadata': {
            'Artist': {'value': 'x' * 500}}}]}}}}
        assert len(parse_image(data, 'A')['credit']) == 200


class Script:
    """get_json and get_binary of the module, answering by what is asked."""

    def __init__(self, entity=None, summary=None, commons=COMMONS, image=b'\xff\xd8\xff\xe0jpeg', fail=False):
        self.entity, self.summary, self.commons, self.image, self.fail = entity, summary, commons, image, fail
        self.json_calls, self.binary_calls = [], []

    def get_json(self, provider, url, params=None, **kw):
        self.json_calls.append((provider, url, params))
        if self.fail:
            return Reply(0, None, 'down')
        if 'wikidata.org' in url:
            qid = params['ids']
            return Reply(200, {'entities': {qid: self.entity if self.entity is not None else {'id': qid, 'missing': ''}}})
        if 'commons.wikimedia.org' in url:
            return Reply(200, self.commons)
        language = url.split('//')[1].split('.')[0]
        page = (self.summary or {}).get(language)
        return Reply(200, page) if page is not None else Reply(404, None, 'HTTP 404')

    def get_binary(self, provider, url, max_bytes, **kw):
        self.binary_calls.append((provider, url, max_bytes))
        return Reply(200, self.image) if self.image is not None else Reply(0, None, 'down')

    def __enter__(self):
        self.patches = [patch.object(profiles, 'get_json', self.get_json), patch.object(profiles, 'get_binary', self.get_binary)]
        for p in self.patches:
            p.start()
        return self

    def __exit__(self, *args):
        for p in self.patches:
            p.stop()


SUMMARY = {'pt': {'type': 'standard', 'extract': 'Frank Herbert foi um escritor americano. Segunda frase.',
                  'content_urls': {'desktop': {'page': 'https://pt.wikipedia.org/wiki/Frank_Herbert'}}},
           'en': {'type': 'standard', 'extract': 'Frank Herbert was an American writer.'}}


class TestReadingAProfile:
    def test_it_asks_wikidata_for_the_identifier_then_wikipedia_for_the_biography_then_commons_for_the_photo(self, tmp_path):
        with Script(entity=human(), summary=SUMMARY) as net:
            status, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True)
        assert status == 'ok'
        assert [(p, u.split('/')[2]) for p, u, _ in net.json_calls] == [('Wikidata', 'www.wikidata.org'), ('Wikipedia', 'pt.wikipedia.org'), ('Wikidata', 'commons.wikimedia.org')]
        entity_params = net.json_calls[0][2]
        assert entity_params['ids'] == 'Q7934' and entity_params['action'] == 'wbgetentities' and 'labels' not in entity_params['props']
        assert entity_params['languages'] == 'pt|pt-br|en' and entity_params['props'] == 'descriptions|claims|sitelinks'
        assert profiles.MAX_IMAGE == 1_500_000 and profiles.MAX_BIO == 1200 and profiles.MAX_ATTEMPTS == 5 and profiles.THUMB_WIDTH == 400
        assert net.json_calls[2][2]['titles'] == 'File:Frank Herbert 1984 (square).jpg' and net.json_calls[2][2]['iiurlwidth'] == 400
        assert net.binary_calls == [('Wikidata', 'https://upload.wikimedia.org/thumb/a.jpg', MAX_IMAGE)]
        assert got['description'].startswith('escritor') and (got['born'], got['died']) == ('1920-10-08', '1986-02-11')
        assert got['bio'] == ('pt', 'Frank Herbert', 'https://pt.wikipedia.org/wiki/Frank_Herbert', 'Frank Herbert foi um escritor americano. Segunda frase.')
        assert got['bio_state'] == 'done'
        assert got['photo'] == {'path': '/covers/person_Q7934.jpg', 'credit': 'Jane Doe & Co', 'license': 'CC BY-SA 4.0',
                                'license_url': 'https://creativecommons.org/licenses/by-sa/4.0', 'page': 'https://commons.wikimedia.org/wiki/File:A.jpg'}
        assert open(os.path.join(tmp_path, 'person_Q7934.jpg'), 'rb').read() == b'\xff\xd8\xff\xe0jpeg'

    def test_nothing_of_the_library_is_in_what_is_asked(self, tmp_path):
        with Script(entity=human(), summary=SUMMARY) as net:
            fetch_profile('Q7934', str(tmp_path), allow_bio=True)
        asked = str(net.json_calls) + str(net.binary_calls)
        assert 'Q7934' in asked and 'Frank Herbert' in asked   # the identifier, the title of the page and the name of the file: that is all
        assert 'Duna' not in asked

    def test_the_biography_is_in_english_when_there_is_no_page_in_portuguese(self, tmp_path):
        with Script(entity=human(sitelinks={'enwiki': {'title': 'Frank Herbert'}}), summary={'en': SUMMARY['en']}) as net:
            _, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True)
        assert got['bio'][0] == 'en' and got['bio'][3] == 'Frank Herbert was an American writer.'
        assert got['bio'][2] == 'https://en.wikipedia.org/wiki/Frank_Herbert'   # no address given: the one of the title

    def test_a_long_biography_is_cut_where_a_sentence_ends(self, tmp_path):
        long = {'pt': {'type': 'standard', 'extract': ' '.join(f'Frase {i} da biografia.' for i in range(300))}}
        with Script(entity=human(), summary=long):
            _, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True)
        assert 800 < len(got['bio'][3]) <= 1200 and got['bio'][3].endswith('biografia.')

    def test_the_title_of_the_page_is_written_so_that_it_cannot_change_the_address(self, tmp_path):
        with Script(entity=human(sitelinks={'ptwiki': {'title': 'AC/DC: Álbum?'}}), summary=SUMMARY) as net:
            fetch_profile('Q7934', str(tmp_path), allow_bio=True)
        assert net.json_calls[1][1] == 'https://pt.wikipedia.org/api/rest_v1/page/summary/AC%2FDC%3A%20%C3%81lbum%3F'

    def test_a_photo_commons_does_not_know_is_no_photo_and_not_an_error(self, tmp_path):
        for commons in ({}, {'query': {'pages': {'-1': {'missing': ''}}}}, None):
            with Script(entity=human(), summary=SUMMARY, commons=commons) as net:
                status, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True)
            assert status == 'ok' and got['photo'] is None and net.binary_calls == []

    def test_an_answer_that_is_not_an_object_is_a_failure(self, tmp_path):
        with patch.object(profiles, 'get_json', lambda *a, **k: Reply(200, ['not', 'an', 'object'])):
            assert fetch_profile('Q7934', str(tmp_path), allow_bio=True) == ('failed', None)

    def test_a_page_that_is_not_an_article_or_is_empty_is_no_biography(self, tmp_path):
        for page in ({'type': 'disambiguation', 'extract': 'x'}, {'type': 'standard', 'extract': '  '}, {'type': 'standard'}):
            with Script(entity=human(sitelinks={'ptwiki': {'title': 'X'}}), summary={'pt': page}):
                _, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True)
            assert got['bio'] is None and got['bio_state'] == 'none', page

    def test_without_wikipedia_the_biography_waits_and_nothing_is_asked_of_it(self, tmp_path):
        with Script(entity=human(), summary=SUMMARY) as net:
            _, got = fetch_profile('Q7934', str(tmp_path), allow_bio=False)
        assert got['bio'] is None and got['bio_state'] == 'pending'
        assert all('wikipedia.org' not in u for _, u, _ in net.json_calls)

    def test_without_the_photo_asked_for_no_commons(self, tmp_path):
        with Script(entity=human(), summary=SUMMARY) as net:
            _, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True, allow_image=False)
        assert got['photo'] is None and net.binary_calls == [] and all('commons' not in u for _, u, _ in net.json_calls)

    def test_a_person_with_no_photo_is_asked_nothing_of_commons(self, tmp_path):
        entity = human()
        del entity['claims']['P18']
        with Script(entity=entity, summary=SUMMARY) as net:
            _, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True)
        assert got['photo'] is None and net.binary_calls == []

    def test_a_download_that_is_not_the_picture_it_says_is_not_kept(self, tmp_path):
        for image in (b'<html>not an image</html>', b'', None):
            with Script(entity=human(), summary=SUMMARY, image=image):
                _, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True)
            assert got['photo'] is None, image
        assert os.listdir(tmp_path) == []

    def test_a_picture_is_kept_by_its_kind(self, tmp_path):
        png = {'query': {'pages': {'1': {'imageinfo': [{'mime': 'image/png', 'thumburl': 'https://u.org/a.png'}]}}}}
        with Script(entity=human(), summary=SUMMARY, commons=png, image=b'\x89PNG\r\n\x1a\nxx'):
            _, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True)
        assert got['photo']['path'] == '/covers/person_Q7934.png' and os.path.exists(os.path.join(tmp_path, 'person_Q7934.png'))
        with Script(entity=human(), summary=SUMMARY, commons=png, image=b'\xff\xd8\xffjpeg-as-png'):
            _, got = fetch_profile('Q7934', str(tmp_path), allow_bio=True)
        assert got['photo'] is None   # it says it is a PNG and is not

    def test_an_entity_that_is_not_a_person_or_does_not_exist_is_missing_and_one_that_is_not_reached_is_failed(self, tmp_path):
        with Script(entity=human(claims={'P31': [claim({'id': 'Q11424'})]})):
            assert fetch_profile('Q7934', str(tmp_path), True) == ('missing', None)
        with Script(entity=None):
            assert fetch_profile('Q7934', str(tmp_path), True) == ('missing', None)
        with Script(fail=True):
            assert fetch_profile('Q7934', str(tmp_path), True) == ('failed', None)

    def test_an_identifier_that_is_not_one_is_never_asked(self, tmp_path):
        with Script(entity=human()) as net:
            for bad in ('', None, 'q7934', 'Q', 'Q79x', '7934', 'Q7934|Q1', 'Q7934&x=1', 'Q' + '9' * 13, '../Q1'):
                assert fetch_profile(bad, str(tmp_path), True) == ('missing', None), bad
        assert net.json_calls == []


class DB:
    def __init__(self, pending=(), bio_pending=(), place_pending=()):
        self.statements, self.pending, self.bio_pending, self.place_pending = [], list(pending), list(bio_pending), list(place_pending)

    def execute(self, query, params=()):
        self.statements.append((' '.join(query.split()), params))

    def fetchall(self, query, params=()):
        self.statements.append((' '.join(query.split()), params))
        if 'bio_state = \'pending\'' in query:
            return [(q,) for q in self.bio_pending]
        if 'NOT place_read' in query:
            return [(q,) for q in self.place_pending]
        return [(q,) for q in self.pending]


def fetcher(results):
    calls = []

    def fetch(qid, covers_dir, allow_bio, allow_image=True):
        calls.append((qid, allow_bio, allow_image))
        return results[qid]
    fetch.calls = calls
    return fetch


PROFILE = {'description': 'escritor', 'born': '1920', 'died': '1986', 'image': 'A.jpg', 'pages': {'pt': 'X'}, 'born_place': 'Tacoma', 'place_read': True,
           'bio': ('pt', 'X', 'https://pt.wikipedia.org/wiki/X', 'Texto.'), 'bio_state': 'done',
           'photo': {'path': '/covers/person_Q1.jpg', 'credit': 'Jane', 'license': 'CC BY-SA 4.0', 'license_url': 'u', 'page': 'p'}}


class TestOverTheDatabase:
    def run(self, db, allowed, fetch, **kw):
        return resolve_pending(db, lambda pid: pid in allowed, '/tmp/covers', sleep=lambda s: None, fetch=fetch, **kw)

    def test_nothing_is_asked_unless_wikidata_is_on(self):
        db = DB(pending=['Q1'])
        fetch = fetcher({'Q1': ('ok', PROFILE)})
        assert self.run(db, {'wikipedia', 'openlibrary'}, fetch) == 0
        assert fetch.calls == [] and db.statements == []

    def test_what_is_read_is_kept_with_what_is_remembered_in_one_statement(self):
        db = DB(pending=['Q1'])
        fetch = fetcher({'Q1': ('ok', PROFILE)})
        assert self.run(db, {'wikidata', 'wikipedia'}, fetch) == 1
        assert fetch.calls == [('Q1', True, True)]
        (query, params), = [s for s in db.statements if 'INSERT INTO person_profile' in s[0] and 'authority_lookups' in s[0]]
        assert 'ON CONFLICT (person_id) DO UPDATE' in query and 'hidden' not in query.split('ON CONFLICT')[1]   # what staff hid is not undone
        assert params == ('escritor', '1920', '1986', 'Texto.', 'pt', 'X', 'https://pt.wikipedia.org/wiki/X', 'done', '/covers/person_Q1.jpg', 'Jane',
                          'CC BY-SA 4.0', 'u', 'p', 'Tacoma', True, 'Q1', 'Q1', 'done')

    def test_a_profile_with_no_biography_or_photo_keeps_none_of_them(self):
        bare = dict(PROFILE, bio=None, bio_state='none', photo=None)
        db = DB(pending=['Q1'])
        self.run(db, {'wikidata', 'wikipedia'}, fetcher({'Q1': ('ok', bare)}))
        (query, params), = [s for s in db.statements if 'INSERT INTO person_profile' in s[0] and 'authority_lookups' in s[0]]
        assert params[3:8] == ('', None, None, None, 'none') and params[8:13] == (None, None, None, None, None)

    def test_with_wikipedia_off_the_biography_is_not_asked_and_waits(self):
        db = DB(pending=['Q1'])
        fetch = fetcher({'Q1': ('ok', dict(PROFILE, bio=None, bio_state='pending'))})
        self.run(db, {'wikidata'}, fetch)
        assert fetch.calls == [('Q1', False, True)]

    def test_a_person_that_does_not_exist_and_one_that_could_not_be_reached_are_remembered_differently(self):
        db = DB(pending=['Q1', 'Q2'])
        n = self.run(db, {'wikidata'}, fetcher({'Q1': ('missing', None), 'Q2': ('failed', None)}))
        remembered = [p for q, p in db.statements if 'INSERT INTO authority_lookups' in q]
        assert remembered == [('Q1', 'missing'), ('Q2', 'failed')] and n == 1   # a failure is not an answer

    def test_a_few_at_a_time_and_the_attempts_are_limited(self):
        db = DB(pending=[])
        self.run(db, {'wikidata'}, fetcher({}), limit=2)
        (query, params), = [s for s in db.statements if s[0].startswith('SELECT DISTINCT a.value')]
        assert params == (profiles.MAX_ATTEMPTS, 2) and "interval '1 day'" in query

    def test_turned_off_in_the_middle_is_not_one_more_request(self):
        state = {'on': True}
        db = DB(pending=['Q1', 'Q2'])

        def allowed(pid):
            return state['on']

        def fetch(qid, covers_dir, allow_bio, allow_image=True):
            state['on'] = False
            return 'ok', PROFILE
        assert resolve_pending(db, allowed, '/tmp', sleep=lambda s: None, fetch=fetch) == 1

    def test_the_biography_of_the_ones_read_while_wikipedia_was_off_is_read_when_it_is_on(self):
        db = DB(bio_pending=['Q1'])
        fetch = fetcher({'Q1': ('ok', dict(PROFILE, photo=None))})
        assert self.run(db, {'wikidata', 'wikipedia'}, fetch) == 1
        assert fetch.calls == [('Q1', True, False)]   # no photo again
        (query, params), = [s for s in db.statements if s[0].startswith('UPDATE person_profile SET bio')]
        assert "bio_state = 'pending'" in query and params == ('Texto.', 'pt', 'X', 'https://pt.wikipedia.org/wiki/X', 'done', 'Q1')

    def test_a_biography_that_could_not_be_read_is_asked_again_and_breaks_nothing(self):
        db = DB(bio_pending=['Q1', 'Q2'])
        fetch = fetcher({'Q1': ('failed', None), 'Q2': ('ok', PROFILE)})
        assert self.run(db, {'wikidata', 'wikipedia'}, fetch) == 1
        assert [c[0] for c in fetch.calls] == ['Q1', 'Q2']
        assert len([s for s in db.statements if s[0].startswith('UPDATE person_profile SET bio')]) == 1

    def test_no_biography_to_wait_for_without_wikipedia(self):
        db = DB(bio_pending=['Q1'])
        fetch = fetcher({'Q1': ('ok', PROFILE)})
        self.run(db, {'wikidata'}, fetch)
        assert fetch.calls == []
        assert not [s for s in db.statements if "bio_state = 'pending'" in s[0]]   # it is not even looked for

    def test_a_profile_that_a_person_with_the_same_identifier_has_is_shared_without_a_request(self):
        db = DB()
        self.run(db, {'wikidata'}, fetcher({}))
        assert any('INSERT INTO person_profile' in q and 'FROM person_authority a' in q and 'ON CONFLICT (person_id) DO NOTHING' in q for q, _ in db.statements)


class TestDownloading:
    def response(self, status=200, body=b'abc', length=None):
        r = MagicMock(status_code=status)
        r.headers = {'Content-Length': str(length)} if length is not None else {}
        r.iter_content.return_value = iter([body[i:i + 2] for i in range(0, len(body), 2)])
        return r

    def test_a_file_is_read_whole_and_the_connection_closed(self):
        resp = self.response(body=b'abcdef')
        with patch('providers.http.requests.get', return_value=resp) as get:
            reply = get_binary('Wikidata', 'https://u.org/a.jpg', 100)
        assert reply.ok and reply.data == b'abcdef' and resp.close.called
        assert get.call_args.kwargs['stream'] is True and 'Codice' in get.call_args.kwargs['headers']['User-Agent']

    def test_a_file_that_is_too_big_is_not_taken_by_what_it_declares_or_by_what_it_sends(self, capsys):
        with patch('providers.http.requests.get', return_value=self.response(body=b'a', length=101)):
            reply = get_binary('Wikidata', 'https://u.org/a.jpg', 100)
        assert not reply.ok and reply.status == 200 and 'bigger than 100' in reply.problem
        resp = self.response(body=b'x' * 101)
        with patch('providers.http.requests.get', return_value=resp):
            reply = get_binary('Wikidata', 'https://u.org/a.jpg', 100)
        assert not reply.ok and 'bigger than 100' in reply.problem and resp.close.called
        with patch('providers.http.requests.get', return_value=self.response(body=b'x' * 100, length=100)):
            assert get_binary('Wikidata', 'https://u.org/a.jpg', 100).ok

    def test_a_refusal_and_a_failure_are_told_and_reported(self, capsys):
        told = []
        with patch.object(http, 'reporter', lambda name, reply: told.append((name, reply.status))):
            with patch('providers.http.requests.get', return_value=self.response(status=429)):
                assert get_binary('Wikidata', 'https://u.org/a.jpg', 100).status == 429
            with patch('providers.http.requests.get', side_effect=RuntimeError('down')):
                assert get_binary('Wikidata', 'https://u.org/a.jpg', 100).status == 0
        assert told == [('Wikidata', 429), ('Wikidata', 0)] and 'HTTP 429' in capsys.readouterr().out

    def test_a_download_that_breaks_half_way_is_a_failure(self):
        resp = self.response()
        resp.iter_content.side_effect = RuntimeError('reset')
        with patch('providers.http.requests.get', return_value=resp):
            reply = get_binary('Wikidata', 'https://u.org/a.jpg', 100)
        assert reply.status == 0 and not reply.ok and resp.close.called

    def test_a_reporter_that_breaks_does_not_lose_the_file(self):
        def boom(name, reply):
            raise RuntimeError('cannot')
        with patch.object(http, 'reporter', boom), patch('providers.http.requests.get', return_value=self.response(body=b'ab')):
            assert get_binary('Wikidata', 'https://u.org/a.jpg', 100).data == b'ab'


class TestThePlaceOfBirth:
    """The place a person was born in (DEC-160): its name, from the entity of the place, kept with the profile."""

    def test_the_place_is_the_first_claim_that_is_one(self):
        entity = human()
        entity['claims']['P19'] = [claim({'id': 'Q36091'}, rank='preferred'), claim({'id': 'Q1'})]
        assert parse_entity(entity)['place_id'] == 'Q36091'

    def test_a_place_that_is_not_an_entity_is_none(self):
        entity = human()
        entity['claims']['P19'] = [claim('Tacoma'), claim({'id': 'not-a-qid'})]
        assert parse_entity(entity)['place_id'] is None

    def test_the_name_is_in_portuguese_when_there_is_one(self):
        data = {'entities': {'Q1': {'labels': {'en': {'value': 'Tacoma'}, 'pt': {'value': 'Tacoma, Washington'}}}}}
        assert profiles.parse_place(data, 'Q1') == 'Tacoma, Washington'
        assert profiles.parse_place({'entities': {'Q1': {'labels': {'en': {'value': ' Tacoma '}}}}}, 'Q1') == 'Tacoma'
        assert profiles.parse_place({'entities': {'Q1': {'labels': {}}}}, 'Q1') is None
        assert profiles.parse_place({'entities': {}}, 'Q1') is None
        assert profiles.parse_place(None, 'Q1') is None

    def test_a_name_is_kept_to_what_the_column_holds(self):
        data = {'entities': {'Q1': {'labels': {'pt': {'value': 'x' * 400}}}}}
        assert len(profiles.parse_place(data, 'Q1')) == 255

    def _fetch(self, place_reply):
        calls = []

        def fake_get_json(source, url, params=None, **kw):
            calls.append(params)
            if params.get('ids') == 'Q7934':
                entity = human()
                entity['claims']['P19'] = [claim({'id': 'Q36091'})]
                return Reply(200, {'entities': {'Q7934': entity}}, True)
            return place_reply
        return fake_get_json, calls

    def test_the_profile_carries_the_place_and_says_it_was_read(self):
        reply = Reply(200, {'entities': {'Q36091': {'labels': {'pt': {'value': 'Tacoma'}}}}}, True)
        fake, calls = self._fetch(reply)
        with patch.object(profiles, 'get_json', fake):
            status, profile = fetch_profile('Q7934', '/tmp/c', allow_bio=False, allow_image=False)
        assert status == 'ok' and profile['born_place'] == 'Tacoma' and profile['place_read'] is True
        assert [c['ids'] for c in calls] == ['Q7934', 'Q36091']

    def test_a_place_that_could_not_be_reached_is_not_final(self):
        fake, _ = self._fetch(Reply(503, None, False))
        with patch.object(profiles, 'get_json', fake):
            status, profile = fetch_profile('Q7934', '/tmp/c', allow_bio=False, allow_image=False)
        assert status == 'ok' and profile['born_place'] is None and profile['place_read'] is False

    def test_a_person_with_no_place_is_final(self):
        def fake(source, url, params=None, **kw):
            return Reply(200, {'entities': {'Q7934': human()}}, True)
        with patch.object(profiles, 'get_json', fake):
            status, profile = fetch_profile('Q7934', '/tmp/c', allow_bio=False, allow_image=False)
        assert profile['born_place'] is None and profile['place_read'] is True

    def test_the_profiles_read_before_are_asked_again_for_the_place_alone(self):
        db = DB(place_pending=['Q1'])
        fetch = fetcher({'Q1': ('ok', dict(PROFILE, born_place='Tacoma', place_read=True))})
        n = resolve_pending(db, lambda p: p == 'wikidata', '/tmp/c', sleep=lambda s: None, fetch=fetch)
        assert fetch.calls == [('Q1', False, False)] and n == 1
        (query, params), = [s for s in db.statements if s[0].startswith('UPDATE person_profile SET born_place')]
        assert params == ('Tacoma', 'Q1')

    def test_a_place_that_failed_is_asked_again_another_day_and_not_marked(self):
        db = DB(place_pending=['Q1'])
        fetch = fetcher({'Q1': ('ok', dict(PROFILE, born_place=None, place_read=False))})
        resolve_pending(db, lambda p: p == 'wikidata', '/tmp/c', sleep=lambda s: None, fetch=fetch)
        assert not [s for s in db.statements if s[0].startswith('UPDATE person_profile SET born_place')]

    def test_nothing_is_asked_while_wikidata_is_off(self):
        db = DB(place_pending=['Q1'])
        fetch = fetcher({'Q1': ('ok', PROFILE)})
        resolve_pending(db, lambda p: False, '/tmp/c', sleep=lambda s: None, fetch=fetch)
        assert fetch.calls == []
