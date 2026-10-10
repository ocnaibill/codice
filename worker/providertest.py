"""Testing one provider, when the owner asks (DEC-145).

Whether a provider answers is the one thing that cannot be told from the list: a key that was refused, a quota that is used up and a service that
is down all look like "found nothing". The test asks it one fixed, public question ("Dune"), with nothing of the library in it, whether the
provider is on or not (the owner asked), and says how it came out: it answered with results, it answered with nothing, the key was refused,
the quota is used up, it did not answer, or there was no key to ask with. The outcome is kept for the administration to show."""
import json
import time
from datetime import datetime, timezone

from providers.base import MetadataRecord
from providers.health import OK
from providers.query import read_file_title

SETTING = 'metadata.providers.tests'
# The question each provider is asked: a title it must know, in the format it is for.
QUESTIONS = {
    'google_books': ('Dune', 'epub'), 'openlibrary': ('Dune', 'epub'), 'wikidata': ('Dune', 'epub'),
    'comicvine': ('Absolute Batman 001', 'cbz'), 'anilist': ('Berserk', 'cbz'), 'mangadex': ('Berserk', 'cbz'),
}
# Wikipedia does not search: it completes a work with the page that Wikidata names, and is asked for a page that exists.
WIKIPEDIA_PAGES = {'en': 'Dune (novel)'}


class ProviderTester:
    def __init__(self, db, registry):
        self.db, self.registry = db, registry

    def _ask(self, provider):
        """How many answers the provider gave to its question (for Wikipedia: whether it gave the page)."""
        if getattr(provider, 'completer', False):
            record = provider.complete(MetadataRecord(title='Dune', raw={'sitelinks': dict(WIKIPEDIA_PAGES)}))
            return 1 if (record.raw or {}).get('wikipedia') else 0
        title, fmt = QUESTIONS[provider.id]
        return len(provider.lookup(read_file_title(title, None, fmt)))

    def run(self, job_id, provider_id, checkpoint=lambda: None) -> dict:
        """Asks the provider its question and keeps how it came out: {ok, state, status, results, ms, at}."""
        provider = self.registry.provider(provider_id)
        if provider is None or (provider.id not in QUESTIONS and not getattr(provider, 'completer', False)):
            raise ValueError(f'there is no provider {provider_id!r} to test')
        health = getattr(self.registry, 'health', None)
        result = {'ok': False, 'state': 'nokey', 'status': 0, 'results': 0, 'ms': 0}
        if hasattr(provider, 'api_key') and not provider.api_key:
            print(f"   🧪 {provider.name}: no API key, so nothing was asked")
        else:
            checkpoint()
            if health is not None:
                health.last.pop(provider.id, None)
                health.status.pop(provider.id, None)
            started = time.monotonic()
            results = 0
            try:
                results = self._ask(provider)
                state = (health.last.get(provider.id) if health is not None else None) or OK   # no request made, or no recorder: the answers are all there is
            except Exception as err:   # a provider that breaks is a finding, not a job to try again
                print(f"   🧪 {provider.name}: it broke ({err})")
                state = 'error'
            if state == OK and not results:
                state = 'empty'
            result = {'ok': state == OK, 'state': state, 'status': (health.status.get(provider.id, 0) if health is not None else 0),
                      'results': results, 'ms': round((time.monotonic() - started) * 1000)}
        result['at'] = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
        print(f"   🧪 Tested {provider.name}: {result}")
        self.db.execute(
            """INSERT INTO settings (key, value) VALUES (%s, jsonb_build_object(%s::text, %s::jsonb))
               ON CONFLICT (key) DO UPDATE SET value = (CASE WHEN jsonb_typeof(settings.value) = 'object' THEN settings.value ELSE '{}'::jsonb END)
                                                      || jsonb_build_object(%s::text, %s::jsonb), updated_at = now()""",
            (SETTING, provider.id, json.dumps(result), provider.id, json.dumps(result)))
        return result
