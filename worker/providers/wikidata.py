"""Wikidata, to tell what a work is when the title in the file is not the one the book providers know it by (DEC-141).

Wikidata keeps the titles of a work in many languages ("A Nuvem" is *Thunderhead*, "Duna" is *Dune*), who wrote it, when, the series and the genres,
each with an identifier. It is not asked for the suggestion's data (it has little of it) but for the work: the registry uses it to translate
a title the book providers did not find, and to add the identifier, the series and the genres to the answer that was chosen. Public API, no key,
data under CC0; Wikimedia refuses a request that does not say who asks. Only the title is sent (DEC-097)."""
import re
from typing import List, Optional

from .base import BaseProvider, Credit, MetadataRecord
from .http import get_json
from .match import TITLE_MIN
from .text import closeness, main_title

URL = 'https://www.wikidata.org/w/api.php'
LANGUAGES = 'pt|pt-br|en|es|fr'
# What a work of writing is: a novel, a book, a literary work, a story, a poem, a play... (not a film or a game with the same name).
WORKS = frozenset({'Q8261', 'Q7725634', 'Q47461344', 'Q571', 'Q49084', 'Q149537', 'Q747381', 'Q725377', 'Q277759', 'Q1667921', 'Q5185279', 'Q25379', 'Q35760'})
CANDIDATES = 10
KEPT = 5
MAX_TAGS = 6
MAX_ALT_TITLES = 12
_ORDER = ('pt-br', 'pt', 'en')


def label_of(entity, order=_ORDER):
    """The name of an entity, in the language wanted first (Portuguese of Brazil, Portuguese, English), else any it has."""
    labels = entity.get('labels') or {}
    for language in order:
        if (labels.get(language) or {}).get('value'):
            return labels[language]['value']
    return next((v['value'] for v in labels.values() if v.get('value')), None)


def claim_values(entity, prop):
    """The values of a property of an entity: the ids of entities, the strings, the times and the texts, each as Wikidata says it."""
    out = []
    for claim in (entity.get('claims') or {}).get(prop, []):
        value = ((claim.get('mainsnak') or {}).get('datavalue') or {}).get('value')
        if value is not None:
            out.append(value.get('id') if isinstance(value, dict) and 'id' in value else value)
    return out


def is_work(entity):
    return any(v in WORKS for v in claim_values(entity, 'P31'))


def year_of(value):
    """'+1965-00-00T00:00:00Z' -> '1965'."""
    match = re.match(r'^[+-]?(\d{1,4})', (value or {}).get('time', '') if isinstance(value, dict) else '')
    return str(int(match.group(1))) if match else None


class WikidataProvider(BaseProvider):
    # The registry does not set this provider against the book providers: it asks it about the work, to translate and to complete.
    resolver = True

    @property
    def name(self) -> str:
        return 'Wikidata'

    @property
    def id(self) -> str:
        return 'wikidata'

    def _ask(self, **params):
        reply = get_json(self.name, URL, params={'format': 'json', **params})
        return reply.data if reply.ok and isinstance(reply.data, dict) else {}

    def _search(self, title, language) -> List[str]:
        found = self._ask(action='wbsearchentities', search=title, language=language, uselang=language, type='item', limit=CANDIDATES).get('search') or []
        return [item['id'] for item in found if item.get('id')]

    def _entities(self, ids, props='labels|aliases|claims|sitelinks') -> dict:
        if not ids:
            return {}
        return self._ask(action='wbgetentities', ids='|'.join(ids), props=props, languages=LANGUAGES).get('entities') or {}

    def _record(self, entity: dict, names: dict) -> MetadataRecord:
        record = MetadataRecord(source=self.name)
        record.title = label_of(entity)
        record.credits = [Credit(names[a], ids={'wikidata': a}) for a in claim_values(entity, 'P50') if names.get(a)]
        record.author = record.credits[0].name if record.credits else None
        # P577 on the item of a work is when it was first published: the work's year, not the date of an edition (DEC-156).
        dates = [year_of(v) for v in claim_values(entity, 'P577')]
        record.original_year = next((d for d in dates if d), None)
        series = (claim_values(entity, 'P179') or [None])[0]
        if series and names.get(series):
            record.series = names[series]
            for qualifier in ((entity.get('claims') or {}).get('P179') or [{}])[0].get('qualifiers', {}).get('P1545', []):
                try:
                    record.series_index = float((qualifier.get('datavalue') or {}).get('value'))
                    break
                except (TypeError, ValueError):
                    continue
        record.tags = [names[g] for g in claim_values(entity, 'P136') if names.get(g)][:MAX_TAGS]
        sitelinks = entity.get('sitelinks') or {}
        record.prior = min(len(sitelinks) / 10, 10)
        alts = []
        for language in ('en', 'es', 'fr', 'pt', 'pt-br'):
            alts.append(((entity.get('labels') or {}).get(language) or {}).get('value'))
            alts.extend(a.get('value') for a in (entity.get('aliases') or {}).get(language, []))
        alts.extend(v.get('text') for v in claim_values(entity, 'P1476') if isinstance(v, dict))   # the title the work has in its own language
        record.raw = {
            'wikidata_id': entity.get('id'),
            'alt_titles': list(dict.fromkeys(a for a in alts if a and a != record.title))[:MAX_ALT_TITLES],
            'sitelinks': {lang: (sitelinks.get(f'{lang}wiki') or {}).get('title') for lang in ('pt', 'en') if (sitelinks.get(f'{lang}wiki') or {}).get('title')},
            'sitelinks_count': len(sitelinks),
        }
        return record

    def lookup(self, query) -> List[MetadataRecord]:
        """The works the title is, in the language of the library first and in English after, when none of the first is close to the file's."""
        title = query.search_title
        if not title:
            return []
        ids = self._search(title, 'pt')
        works = {k: e for k, e in self._entities(ids).items() if is_work(e)}
        wanted = [text for text, _ in query.variants()]

        def close(entity):
            names = [label_of(entity)] + [((entity.get('labels') or {}).get(l) or {}).get('value') for l in ('en', 'pt', 'pt-br')]
            return any(closeness(w, n or '') >= TITLE_MIN or closeness(main_title(w), main_title(n or '')) >= TITLE_MIN for w in wanted for n in names)

        if not any(close(e) for e in works.values()):
            more = [i for i in self._search(title, 'en') if i not in works and i not in ids]
            works.update({k: e for k, e in self._entities(more).items() if is_work(e)})
        kept = sorted(works.values(), key=lambda e: -len(e.get('sitelinks') or {}))[:KEPT]
        referenced = {v for e in kept for p in ('P50', 'P136', 'P179') for v in claim_values(e, p) if isinstance(v, str) and v.startswith('Q')}
        names = {k: label_of(e) for k, e in self._entities(sorted(referenced)[:50], props='labels').items()}
        return [self._record(e, names) for e in kept]

    def search(self, query: str) -> Optional[MetadataRecord]:
        from .query import read_file_title
        found = self.lookup(read_file_title(query, None, 'default'))
        return found[0] if found else None
