"""How the providers are asked: who is asking, how fast, and what is said when the answer is not an answer.

A provider that cannot answer (no key, quota used up, refused) used to look the same as one that had nothing to say: the answer was dropped and
nothing was written. Now the status is told in the log, and the one who reads it can tell a library that has no such book from a key that is wrong."""
import os
import time
from dataclasses import dataclass
from typing import Any, Optional
from urllib.parse import urlparse

import requests

from .gate import scrub

PROJECT = 'https://github.com/ocnaibill/codice'
MAX_SAID = 120   # what a provider says about a refusal, at most, in the log and for the administration

# What each status says about the one who asked, for whoever reads the log.
STATUS_HINTS = {
    400: 'the request was refused as badly made',
    401: 'the key was refused or is missing',
    403: 'the key is not allowed to ask this',
    404: 'nothing there',
    429: 'too many requests, or the daily quota is used up (a key of your own has a quota of its own)',
}


def user_agent(environ=None):
    """Who is asking: the project, and the owner's contact when they set one (CODICE_CONTACT). Open Library gives three times the rate to
    who says who they are, and Wikimedia and MangaDex refuse who does not. The contact is the owner's to give: without it, only the project."""
    environ = os.environ if environ is None else environ
    contact = (environ.get('CODICE_CONTACT') or '').strip()
    return f'Codice (+{PROJECT}; {contact})' if contact else f'Codice (+{PROJECT})'


def min_interval(environ=None):
    """How many seconds to leave between two requests to the same host: Open Library allows 3 a second to who gives a contact, 1 to who does not."""
    environ = os.environ if environ is None else environ
    return 0.35 if (environ.get('CODICE_CONTACT') or '').strip() else 1.05


# Who is told how each request came out (providers.health.HealthRecorder.reply): the administration says it next to the provider.
reporter = None

_last_call = {}
_clock = time.monotonic
_sleep = time.sleep


def _wait(host, interval):
    """Leaves the interval between two requests to a host, so that a batch of works does not make a burst."""
    now = _clock()
    due = _last_call.get(host, 0) + interval
    if due > now:
        _sleep(due - now)
    _last_call[host] = _clock()


@dataclass
class Reply:
    """What a request came to: the HTTP status (0 when it never got there), the JSON if there was one, and a few words when it failed."""
    status: int
    data: Optional[Any] = None
    problem: str = ''

    @property
    def ok(self):
        return self.status == 200 and self.data is not None


def get_binary(provider, url, max_bytes, timeout=20, secrets=(), interval=None) -> Reply:
    """Asks for a file (an image) and says why it did not come. Never more than `max_bytes` is read: a file that is bigger is not taken. The
    bytes are `data`. It leaves the same pause between two requests to a host, and is told to the administration like any other request."""
    reply = _download(provider, url, max_bytes, timeout, secrets, interval)
    if reporter is not None:
        try:
            reporter(provider, reply)
        except Exception as err:
            print(f"   ⚠️ {provider}: could not report how it answered ({err})")
    return reply


def _download(provider, url, max_bytes, timeout, secrets, interval) -> Reply:
    _wait(urlparse(url).netloc, min_interval() if interval is None else interval)
    try:
        resp = requests.get(url, headers={'User-Agent': user_agent()}, timeout=timeout, stream=True)
    except Exception as err:
        problem = f'the request failed ({scrub(err, *secrets)})'
        print(f"   ⚠️ {provider}: {problem}")
        return Reply(0, None, problem)
    try:
        if resp.status_code != 200:
            problem = f'HTTP {resp.status_code}' + (f': {STATUS_HINTS[resp.status_code]}' if resp.status_code in STATUS_HINTS else '')
            print(f"   ⚠️ {provider}: {problem}")
            return Reply(resp.status_code, None, problem)
        declared = resp.headers.get('Content-Length') if getattr(resp, 'headers', None) else None
        if declared and declared.isdigit() and int(declared) > max_bytes:
            problem = f'the file is bigger than {max_bytes} bytes'
            print(f"   ⚠️ {provider}: {problem}")
            return Reply(200, None, problem)
        body = b''
        for chunk in resp.iter_content(chunk_size=64 * 1024):
            body += chunk
            if len(body) > max_bytes:
                problem = f'the file is bigger than {max_bytes} bytes'
                print(f"   ⚠️ {provider}: {problem}")
                return Reply(200, None, problem)
        return Reply(200, body)
    except Exception as err:
        problem = f'the download failed ({scrub(err, *secrets)})'
        print(f"   ⚠️ {provider}: {problem}")
        return Reply(0, None, problem)
    finally:
        resp.close()


def _said(resp, secrets=()):
    """What the provider says about why it refused, in a few words (Google: {"error": {"message": ...}}; others: {"error": "..."}). Without it a
    key that is not valid looks like any badly made request."""
    try:
        data = resp.json()
        error = data.get('error') if isinstance(data, dict) else None
        message = error.get('message') if isinstance(error, dict) else error
    except Exception:
        return ''
    if not isinstance(message, str):
        return ''
    return scrub(' '.join(message.split()), *secrets)[:MAX_SAID]


def get_json(provider, url, params=None, headers=None, timeout=10, secrets=(), interval=None, post=None) -> Reply:
    """Asks a provider and says, in the log, why it did not answer. The secrets (the key the URL carries) are never written."""
    reply = _ask(provider, url, params, headers, timeout, secrets, interval, post)
    if reporter is not None:
        try:
            reporter(provider, reply)
        except Exception as err:   # how a provider answered is not worth losing its answer
            print(f"   ⚠️ {provider}: could not report how it answered ({err})")
    return reply


def _ask(provider, url, params, headers, timeout, secrets, interval, post) -> Reply:
    host = urlparse(url).netloc
    _wait(host, min_interval() if interval is None else interval)
    sent = {'User-Agent': user_agent(), 'Accept': 'application/json'}
    sent.update(headers or {})
    try:
        if post is not None:
            resp = requests.post(url, json=post, headers=sent, timeout=timeout)
        else:
            resp = requests.get(url, params=params, headers=sent, timeout=timeout)
    except Exception as err:
        problem = f'the request failed ({scrub(err, *secrets)})'
        print(f"   ⚠️ {provider}: {problem}")
        return Reply(0, None, problem)
    if resp.status_code != 200:
        problem = f'HTTP {resp.status_code}' + (f': {STATUS_HINTS[resp.status_code]}' if resp.status_code in STATUS_HINTS else '')
        said = _said(resp, secrets)
        if said:
            problem += f' ({said})'
        print(f"   ⚠️ {provider}: {problem}")
        return Reply(resp.status_code, None, problem)
    try:
        return Reply(200, resp.json())
    except ValueError:
        problem = 'the answer is not JSON'
        print(f"   ⚠️ {provider}: {problem}")
        return Reply(200, None, problem)
