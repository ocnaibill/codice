"""Searching Wikidata for the person a page is about, when staff ask (DEC-168).

The profile of an author is read for a person who holds a Wikidata identifier that a human confirmed (DEC-095, DEC-098, DEC-146); a name alone
can be anybody, so nothing is ever looked up by name on its own. Here the lookup by name is the human's own request, and what it brings is a list
of candidates, each with what tells one person from another (the description, the years), for the human to choose: nothing is linked by it. Only
the text typed leaves the server, and only when the owner turned Wikidata on."""
import json

from profiles import ENTITIES, QID, SOURCE, parse_entity
from providers.http import get_json

LANGUAGES = ('pt', 'en')   # the search is made in each, and the answers are put together
PER_LANGUAGE = 8
MAX_CANDIDATES = 8
MAX_QUERY = 200


def _label(entity, fallback):
    labels = entity.get('labels') or {}
    for language in ('pt-br', 'pt', 'en'):
        value = (labels.get(language) or {}).get('value')
        if isinstance(value, str) and value.strip():
            return ' '.join(value.split())[:200]
    return fallback


def candidates_of(entities, order):
    """The humans among the entities, in the order the search gave them: [{id, label, description, born, died, photo, wikipedia}]. An entity that is
    not a person is left out, and so is one that came with an identifier that is not one."""
    found = []
    for qid in order:
        entity = (entities or {}).get(qid) if isinstance(entities, dict) else None
        profile = parse_entity(entity) if QID.fullmatch(qid) else None
        if profile is None:
            continue
        found.append({'id': qid, 'label': _label(entity, qid), 'description': profile['description'], 'born': profile['born'], 'died': profile['died'],
                      'photo': bool(profile['image']), 'wikipedia': bool(profile['pages'])})
    return found


class ProfileSearcher:
    """Runs the job `profile_search`: {person, query}. The answer is kept in person_profile_searches for the page to read."""

    def __init__(self, db, allowed, get=get_json):
        self.db, self.allowed, self.get = db, allowed, get

    def _keep(self, person_id, state, results):
        self.db.execute(
            "UPDATE person_profile_searches SET state = %s, results = %s::jsonb, finished_at = now() WHERE person_id = %s",
            (state, json.dumps(results), person_id))

    def _ids(self, query):
        """The identifiers the search gives, in the first language that has them first. None when Wikidata could not be reached."""
        ids, reached = [], False
        for language in LANGUAGES:
            reply = self.get('Wikidata', ENTITIES, params={'action': 'wbsearchentities', 'search': query, 'language': language, 'uselang': language,
                                                           'type': 'item', 'limit': PER_LANGUAGE, 'format': 'json'})
            if not reply.ok or not isinstance(reply.data, dict):
                continue
            reached = True
            for item in reply.data.get('search') or []:
                qid = item.get('id') if isinstance(item, dict) else None
                if isinstance(qid, str) and QID.fullmatch(qid) and qid not in ids:
                    ids.append(qid)
        return ids if reached else None

    def run(self, job_id, person_id, query, checkpoint=lambda: None):
        query = ' '.join(str(query or '').split())[:MAX_QUERY]
        if not query or not isinstance(person_id, int):
            return {'state': 'failed'}
        if not self.allowed(SOURCE):
            self._keep(person_id, 'off', [])
            return {'state': 'off'}
        checkpoint()
        ids = self._ids(query)
        if ids is None:
            self._keep(person_id, 'failed', [])
            return {'state': 'failed'}
        results = []
        if ids:
            checkpoint()
            reply = self.get('Wikidata', ENTITIES, params={'action': 'wbgetentities', 'ids': '|'.join(ids[:2 * PER_LANGUAGE]),
                                                           'props': 'labels|descriptions|claims|sitelinks', 'languages': 'pt|pt-br|en',
                                                           'format': 'json'})
            if not reply.ok or not isinstance(reply.data, dict):
                self._keep(person_id, 'failed', [])
                return {'state': 'failed'}
            results = candidates_of(reply.data.get('entities'), ids)[:MAX_CANDIDATES]
        self._keep(person_id, 'done', results)
        return {'state': 'done', 'found': len(results)}
