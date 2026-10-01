"""Which external metadata providers the owner allowed (DEC-045, #68).

Nothing is sent to a third party unless the owner turned that provider on, one by one, in the administration. The
choice is the `metadata.providers` setting ({"openlibrary": true, ...}); a provider that is not in it is off, and
so is every provider when the setting cannot be read: a failure never turns a service on.
"""
import json
import os

SETTING = 'metadata.providers'
KEYS_SETTING = 'metadata.providers.keys'

# The environment variable each provider takes its API key from. A provider that is not here has no key. The
# secret itself never leaves the worker: only whether it is set is told, so the administration can say so.
KEY_ENV = {'google_books': 'GOOGLE_BOOKS_API_KEY', 'comicvine': 'COMICVINE_API_KEY'}


def key_configured(provider_id, environ=None):
    """Whether the API key of a provider is set to something. Blank is not a key."""
    environ = os.environ if environ is None else environ
    return bool((environ.get(KEY_ENV[provider_id]) or '').strip())


def asks_providers(environ=None):
    """Whether this worker analyses files, the only work that asks the providers. Another worker (the one that
    reads the text for the semantic matching) runs this same code with other job types, has no API keys, and
    must not say it has none."""
    environ = os.environ if environ is None else environ
    types = [t.strip() for t in (environ.get('WORKER_JOB_TYPES') or '').split(',') if t.strip()]
    return not types or 'ingest' in types


def report_keys(db, environ=None):
    """Tells the administration which API keys this worker has (true or false, never the key), so it can say
    "missing" before the owner turns on a provider that cannot work. Asked once at start: a key is changed in the
    environment and the worker restarts to read it. Only the worker that asks the providers says it; the others
    return None and say nothing."""
    if not asks_providers(environ):
        return None
    state = {pid: key_configured(pid, environ) for pid in KEY_ENV}
    try:
        db.execute(
            """INSERT INTO settings (key, value) VALUES (%s, %s::jsonb)
               ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()""",
            (KEYS_SETTING, json.dumps(state)))
    except Exception as err:
        print(f"   ⚠️ Could not tell which provider keys are set ({err})")
    return state


def scrub(text, *secrets):
    """The text without the secrets in it: an error from a request carries the whole URL, with the key."""
    text = str(text)
    for secret in secrets:
        if secret and secret.strip():
            text = text.replace(secret, '***')
    return text


def db_gate(db):
    """A function that says whether a provider is allowed, reading the setting every time it is asked, so that
    turning a provider off takes effect for the next work."""
    def enabled(provider_id):
        try:
            row = db.fetchone("SELECT value FROM settings WHERE key = %s", (SETTING,))
            value = row[0] if row else None
            if isinstance(value, (str, bytes)):
                value = json.loads(value)
            return isinstance(value, dict) and value.get(provider_id) is True
        except Exception as err:
            print(f"   ⚠️ Could not read which providers are allowed ({err}); none is used")
            return False
    return enabled


def nothing_allowed(_provider_id):
    return False
