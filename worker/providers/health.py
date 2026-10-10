"""How each provider answers, told to the administration (DEC-144).

A key that was refused, a quota that is used up and a service that is down look the same from outside: the provider "found nothing". Every request
now leaves how it came out (providers.http reports it here), and the administration says it next to the provider. Only the kind of answer is kept:
the status, a few words about it and when; never the key, never a title."""

OK, KEY, QUOTA, DOWN, ERROR = 'ok', 'key', 'quota', 'down', 'error'


def _key_refused(problem):
    """Google answers 400, and not 401, to a key that is not valid ("API key not valid. Please pass a valid API key.")."""
    text = (problem or '').lower()
    return 'api key' in text and any(word in text for word in ('not valid', 'invalid', 'expired'))


def state_of(status, ok=True, problem=''):
    """What a request came to: the HTTP status (0 when it never got there), whether the answer could be read, and what the provider said."""
    if status == 200:
        return OK if ok else ERROR
    if status in (401, 403) or (status == 400 and _key_refused(problem)):
        return KEY
    if status == 429:
        return QUOTA
    if status == 0 or status >= 500:
        return DOWN
    return ERROR


class HealthRecorder:
    """Writes, per provider, how its last request went. `names` maps what the providers call themselves in the log ("Google Books") to their ids."""

    def __init__(self, db, names):
        self.db, self.names = db, dict(names)
        self.last = {}   # provider id -> the state of its last request, in this process

    def reply(self, provider_name, reply):
        """Called for every request of a provider: records how it came out. Never raises: the health of a provider is not worth an analysis."""
        provider = self.names.get(provider_name)
        if provider is None:
            return
        state = state_of(reply.status, reply.data is not None, reply.problem)
        self.last[provider] = state
        try:
            self.db.execute(
                """INSERT INTO provider_health (provider, state, status, problem, checked_at, last_ok_at)
                   VALUES (%s, %s, %s, %s, now(), CASE WHEN %s = 'ok' THEN now() END)
                   ON CONFLICT (provider) DO UPDATE SET state = EXCLUDED.state, status = EXCLUDED.status, problem = EXCLUDED.problem,
                       checked_at = now(), last_ok_at = CASE WHEN EXCLUDED.state = 'ok' THEN now() ELSE provider_health.last_ok_at END""",
                (provider, state, reply.status, (reply.problem or '')[:300], state))
        except Exception as err:
            print(f"   ⚠️ Could not record how {provider_name} answered ({err})")

    def answered(self, provider_id, count):
        """Called with how many answers a provider gave for a search: nothing, many searches in a row, with every request answered, says it is not working."""
        if self.last.get(provider_id) != OK:
            return   # a request that failed already says why it gave nothing
        try:
            self.db.execute(
                "UPDATE provider_health SET empty_streak = CASE WHEN %s = 0 THEN empty_streak + 1 ELSE 0 END WHERE provider = %s",
                (count, provider_id))
        except Exception as err:
            print(f"   ⚠️ Could not record the answers of {provider_id} ({err})")


def install(db, registry):
    """Starts telling the administration how the providers of a registry answer: every request they make is recorded, and so is every search that
    comes back with nothing. Returns the recorder."""
    from . import http
    recorder = HealthRecorder(db, registry.names())
    registry.health = recorder
    http.reporter = recorder.reply
    return recorder
