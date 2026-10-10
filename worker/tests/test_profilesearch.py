"""Searching Wikidata for a person by name, for staff to choose from (DEC-168)."""
import json

from profilesearch import MAX_CANDIDATES, ProfileSearcher, candidates_of
from providers.http import Reply


def human(description='escritor', born='+1962-11-12T00:00:00Z', label='Neal Shusterman', image=None, pages=None):
    claims = {'P31': [{'rank': 'normal', 'mainsnak': {'datavalue': {'value': {'id': 'Q5'}}}}],
              'P569': [{'rank': 'normal', 'mainsnak': {'datavalue': {'value': {'time': born, 'precision': 11}}}}]}
    if image:
        claims['P18'] = [{'rank': 'normal', 'mainsnak': {'datavalue': {'value': image}}}]
    return {'labels': {'pt': {'value': label}}, 'descriptions': {'pt': {'value': description}}, 'claims': claims,
            'sitelinks': {f'{l}wiki': {'title': label} for l in (pages or [])}}


def thing(label='Scythe'):
    """Something that is not a person: a novel."""
    return {'labels': {'en': {'value': label}}, 'descriptions': {'en': {'value': 'novel'}},
            'claims': {'P31': [{'rank': 'normal', 'mainsnak': {'datavalue': {'value': {'id': 'Q7725634'}}}}]}}


class FakeDb:
    def __init__(self):
        self.calls = []

    def execute(self, query, params=()):
        self.calls.append((query, params))

    @property
    def kept(self):
        """(state, results) of the last write."""
        query, params = self.calls[-1]
        return params[0], json.loads(params[1]), params[2]


class Wikidata:
    """What Wikidata answers: a search by language and the entities by identifier. `down` makes it not answer."""

    def __init__(self, found=None, entities=None, down=False, entities_down=False):
        self.found, self.entities, self.down, self.entities_down = found or {}, entities or {}, down, entities_down
        self.calls = []

    def __call__(self, provider, url, params=None, **kw):
        self.calls.append((provider, dict(params or {})))
        if self.down:
            return Reply(status=0, problem='down')
        if params['action'] == 'wbsearchentities':
            return Reply(status=200, data={'search': [{'id': i} for i in self.found.get(params['language'], [])]})
        if self.entities_down:
            return Reply(status=503, data=None)
        wanted = params['ids'].split('|')
        return Reply(status=200, data={'entities': {i: self.entities[i] for i in wanted if i in self.entities}})


def searcher(db, wikidata, allowed=('wikidata',)):
    return ProfileSearcher(db, lambda provider: provider in allowed, get=wikidata)


class TestTheCandidates:
    def test_only_people_are_candidates_with_what_tells_one_from_another(self):
        entities = {'Q1': human('escritor norte-americano', image='A.jpg', pages=['pt', 'en']), 'Q2': thing(), 'Q3': human(born='+1900-00-00T00:00:00Z')}
        found = candidates_of(entities, ['Q2', 'Q1', 'Q3', 'Q9'])
        assert [c['id'] for c in found] == ['Q1', 'Q3']   # the order of the search; the novel and what was not answered are out
        assert found[0] == {'id': 'Q1', 'label': 'Neal Shusterman', 'description': 'escritor norte-americano', 'born': '1962-11-12', 'died': None,
                            'photo': True, 'wikipedia': True}
        assert found[1]['photo'] is False and found[1]['wikipedia'] is False

    def test_an_identifier_that_is_not_one_is_not_a_candidate(self):
        assert candidates_of({'Q1; DROP': human()}, ['Q1; DROP']) == []

    def test_the_label_falls_back_to_the_identifier(self):
        entity = human()
        entity['labels'] = {}
        assert candidates_of({'Q1': entity}, ['Q1'])[0]['label'] == 'Q1'


class TestTheSearch:
    def test_it_is_kept_for_the_page_with_the_candidates_of_both_languages_once_each(self):
        wd = Wikidata(found={'pt': ['Q1', 'Q2'], 'en': ['Q2', 'Q3']}, entities={'Q1': human(), 'Q2': thing(), 'Q3': human(label='Neal S.')})
        db = FakeDb()
        out = searcher(db, wd).run(7, 12, '  Neal   Shusterman ')
        assert out == {'state': 'done', 'found': 2}
        state, results, person = db.kept
        assert (state, person) == ('done', 12) and [r['id'] for r in results] == ['Q1', 'Q3']
        searches = [c for c in wd.calls if c[1]['action'] == 'wbsearchentities']
        assert [s[1]['language'] for s in searches] == ['pt', 'en'] and {s[1]['search'] for s in searches} == {'Neal Shusterman'}
        assert wd.calls[-1][1]['ids'] == 'Q1|Q2|Q3'
        assert all(c[0] == 'Wikidata' for c in wd.calls)

    def test_nothing_found_is_an_answer_and_asks_for_no_entities(self):
        wd = Wikidata(found={})
        db = FakeDb()
        assert searcher(db, wd).run(7, 12, 'Miguel Nicodelis') == {'state': 'done', 'found': 0}
        assert db.kept[:2] == ('done', [])
        assert [c[1]['action'] for c in wd.calls] == ['wbsearchentities', 'wbsearchentities']

    def test_a_provider_that_is_off_is_not_asked_and_the_page_is_told(self):
        wd = Wikidata(found={'pt': ['Q1']})
        db = FakeDb()
        assert searcher(db, wd, allowed=()).run(7, 12, 'Neal') == {'state': 'off'}
        assert wd.calls == [] and db.kept[:2] == ('off', [])

    def test_wikidata_that_does_not_answer_is_a_failure_the_page_says(self):
        for wd in (Wikidata(down=True), Wikidata(found={'pt': ['Q1']}, entities_down=True)):
            db = FakeDb()
            assert searcher(db, wd).run(7, 12, 'Neal') == {'state': 'failed'}
            assert db.kept[:2] == ('failed', [])

    def test_one_language_that_does_not_answer_is_not_the_end(self):
        class OneDown(Wikidata):
            def __call__(self, provider, url, params=None, **kw):
                if params['action'] == 'wbsearchentities' and params['language'] == 'pt':
                    return Reply(status=503, data=None)
                return super().__call__(provider, url, params=params, **kw)
        db = FakeDb()
        assert searcher(db, OneDown(found={'en': ['Q1']}, entities={'Q1': human()})).run(7, 12, 'Neal')['found'] == 1

    def test_no_more_candidates_than_a_person_can_choose_from(self):
        ids = [f'Q{n}' for n in range(1, 20)]
        wd = Wikidata(found={'pt': ids[:8], 'en': ids[8:16]}, entities={i: human(label=i) for i in ids})
        db = FakeDb()
        assert searcher(db, wd).run(7, 12, 'Neal')['found'] == MAX_CANDIDATES

    def test_an_empty_search_or_a_person_that_is_not_one_asks_nothing(self):
        wd = Wikidata(found={'pt': ['Q1']})
        db = FakeDb()
        for query, person in (('   ', 12), ('Neal', None), ('Neal', '12')):
            assert searcher(db, wd).run(7, person, query) == {'state': 'failed'}
        assert wd.calls == [] and db.calls == []

    def test_the_text_that_goes_out_is_cut(self):
        wd = Wikidata(found={})
        searcher(FakeDb(), wd).run(7, 12, 'a' * 500)
        assert len(wd.calls[0][1]['search']) == 200
