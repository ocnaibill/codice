"""The client of the model that writes the wiki: chat completions of OpenAI (DeepSeek, any compatible server, Ollama at /v1), one
question and one answer in JSON, with the cost counted and every answer kept by what was asked (see cache.py)."""
import hashlib
import json
import re
import threading
import time
import urllib.error
import urllib.request

VERSION = 1   # changes when the way requests are made changes enough to make kept answers wrong


class Stop(Exception):
    """The run is to stop here: paused (asked to, or because the server stopped answering), cancelled, or lost to another worker. What was
    done is kept, so that resuming goes on from there. `message` is what the administration is told, when there is something to tell."""

    def __init__(self, reason, message=''):
        super().__init__(reason)
        self.reason, self.message = reason, message


class Fatal(RuntimeError):
    """The provider refuses to work for a reason that asking again will not fix (the key, the balance, the address)."""


class Provider:
    def __init__(self, kind, base_url, model, key=''):
        self.kind, self.base_url, self.model, self.key = kind, base_url.rstrip('/'), model, key

    def sends_thinking(self):
        """DeepSeek reasons by default; "disabled" is cheap and predictable. Some servers route to DeepSeek models and take the same field."""
        host = self.base_url.lower()
        return self.kind == 'deepseek' or 'deepseek' in host or 'tokenharbor' in host


class Usage:
    """What a run spent, summed over its threads."""

    def __init__(self):
        self.lock = threading.Lock()
        self.requests = self.hits = self.tokens_in = self.tokens_out = self.failures = self.throttled = self.stalled = 0

    def add(self, **kw):
        with self.lock:
            for k, v in kw.items():
                setattr(self, k, getattr(self, k) + v)

    def snapshot(self):
        with self.lock:
            return {'requests': self.requests, 'hits': self.hits, 'tokensIn': self.tokens_in, 'tokensOut': self.tokens_out,
                    'failures': self.failures, 'throttled': self.throttled, 'stalled': self.stalled}


def cache_key(provider, system, user, example, num_predict):
    raw = json.dumps([VERSION, provider.kind, provider.model, system, user, example, num_predict], ensure_ascii=False, sort_keys=True)
    return hashlib.sha256(raw.encode()).hexdigest()


def extract_json(text):
    """The JSON object of an answer that may come with a code fence, text around it or the schema filled in. None when there is none."""
    if not text:
        return None
    text = re.sub(r'^```(?:json)?\s*|\s*```$', '', text.strip())

    def parse(s):
        try:
            return json.loads(s)
        except ValueError:
            return None
    data = parse(text)
    start = text.find('{')
    while data is None and start != -1:
        depth = 0
        for i in range(start, len(text)):
            depth += text[i] == '{'
            depth -= text[i] == '}'
            if depth == 0:
                data = parse(text[start:i + 1])
                break
        start = text.find('{', start + 1) if data is None else start
    if isinstance(data, dict) and isinstance(data.get('properties'), dict) and data.get('type') == 'object':
        data = data['properties']   # a model that returns the schema with the values inside "properties"
    return data


class Llm:
    def __init__(self, provider, cache=None, control=None, usage=None, min_interval=0.2, stall=300.0, max_request=3600.0,
                 sleep=time.sleep, max_failures=25, longest_wait=300.0):
        self.provider, self.cache, self.control = provider, cache, control or (lambda: None)
        self.usage = usage or Usage()
        self.min_interval, self.stall, self.max_request, self.sleep = min_interval, stall or None, max_request, sleep
        self.max_failures = max_failures
        self.longest_wait = longest_wait   # a wait asked for by the server that is longer than this pauses the run
        self._lock = threading.Lock()
        self._last = 0.0
        self._streak = 0

    @staticmethod
    def _human(seconds):
        minutes = round(seconds / 60)
        return f'{minutes} min' if minutes < 120 else f'{round(minutes / 60)} h'

    @staticmethod
    def _said(detail, key):
        """What the server said in its error, short, and never the key."""
        try:
            data = json.loads(detail)
            msg = data.get('error', data) if isinstance(data, dict) else detail
            msg = msg.get('message', '') if isinstance(msg, dict) else str(msg)
        except ValueError:
            msg = detail
        msg = ' '.join(str(msg).split())
        if key:
            msg = msg.replace(key, '[chave]')
        return msg[:240] if msg else 'sem explicação.'

    # ---------------------------------------------------------------- one request
    def _wait_turn(self):
        with self._lock:   # the requests of the threads leave spaced, to stay under the limit per minute
            wait = self.min_interval - (time.time() - self._last)
            if wait > 0:
                self.sleep(wait)
            self._last = time.time()

    def _sleep(self, seconds):
        """Waits in short steps, so that a pause or a cancel is seen while the run is held back by the server."""
        left = float(seconds)
        while True:
            self.control()
            if left <= 0:
                return
            step = min(1.0, left)
            self.sleep(step)
            left -= step

    def _read_stream(self, req):
        content, usage, finish = [], {}, None
        deadline = time.time() + self.max_request
        try:
            r = urllib.request.urlopen(req, timeout=self.stall)
        except TimeoutError:
            self.usage.add(stalled=1)
            raise
        with r:
            while True:
                try:
                    raw = r.readline()
                except TimeoutError:
                    self.usage.add(stalled=1)
                    raise
                if not raw:
                    break
                if time.time() > deadline:
                    raise TimeoutError(f'request took more than {self.max_request:.0f} s')
                line = raw.decode('utf-8', 'replace').strip()
                if not line.startswith('data:'):
                    continue
                payload = line[5:].strip()
                if payload == '[DONE]':
                    break
                try:
                    chunk = json.loads(payload)
                except ValueError:
                    continue
                if chunk.get('usage'):
                    usage = chunk['usage']
                for choice in chunk.get('choices') or []:
                    delta = choice.get('delta') or {}
                    if delta.get('content'):
                        content.append(delta['content'])
                    if choice.get('finish_reason'):
                        finish = choice['finish_reason']
        return ''.join(content), usage, finish

    def chat(self, system, user, example, num_predict, timeout=600):
        """(text, finish). Raises Fatal when the provider refuses, RuntimeError when it does not answer."""
        p = self.provider
        if example:
            # an example, not the schema: with the schema some models return the schema filled in
            system += '\nResponda só com um objeto JSON no formato deste exemplo (troque os valores): ' + json.dumps(example, ensure_ascii=False)
        body = {'model': p.model, 'messages': [{'role': 'system', 'content': system}, {'role': 'user', 'content': user}],
                'temperature': 0, 'max_tokens': max(4000, num_predict * 3), 'stream': True, 'stream_options': {'include_usage': True}}
        if p.sends_thinking():
            body['thinking'] = {'type': 'disabled'}
        if example:
            body['response_format'] = {'type': 'json_object'}
        headers = {'Content-Type': 'application/json', 'Accept': 'text/event-stream'}
        if p.key:
            headers['Authorization'] = 'Bearer ' + p.key
        dropped = []
        for attempt in range(8):
            self.control()
            self._wait_turn()
            req = urllib.request.Request(f'{p.base_url}/chat/completions', data=json.dumps(body).encode(), headers=headers)
            try:
                text, usage, finish = self._read_stream(req)
                self.usage.add(requests=1, tokens_in=usage.get('prompt_tokens', 0), tokens_out=usage.get('completion_tokens', 0))
                self._streak = 0
                return re.sub(r'^```(?:json)?\s*|\s*```$', '', text.strip()), finish
            except urllib.error.HTTPError as err:
                detail = err.read().decode('utf-8', 'replace')
                if err.code in (401, 403):
                    raise Fatal('O servidor recusou a chave.')
                if err.code == 402:
                    raise Fatal('O servidor diz que falta saldo na conta.')
                if err.code == 404:
                    raise Fatal('O servidor não conhece este endereço ou este modelo.')
                if err.code == 429 and attempt < 7:
                    retry = err.headers.get('Retry-After') if err.headers else None
                    wait = float(retry) if retry and retry.replace('.', '', 1).isdigit() else min(60, 15 * (attempt + 1))
                    self.usage.add(throttled=1)
                    if wait > self.longest_wait:
                        # not a limit per minute: the allowance of the period is spent. Waiting here for hours says nothing to anyone, and
                        # holds the queue: the run is paused with what the server said, and resumed when it can
                        raise Stop('paused', f'O servidor pediu para esperar {self._human(wait)}: {self._said(detail, p.key)}'
                                             ' A geração foi pausada e guarda o que já fez; retome quando puder.')
                    self._sleep(wait)
                    continue
                if err.code == 400 and attempt < 7:
                    # the server does not take a field: without the JSON mode first, then without the one for reasoning
                    for field in ('response_format', 'thinking'):
                        if field in body and field not in dropped:
                            body.pop(field)
                            dropped.append(field)
                            break
                    else:
                        raise RuntimeError(f'400: {detail[:200]}')
                    continue
                if err.code in (500, 502, 503, 504) and attempt < 3:
                    self._sleep(5)
                    continue
                raise RuntimeError(f'{err.code}: {detail[:200]}')
            except (TimeoutError, urllib.error.URLError, OSError) as err:
                if attempt < 4:
                    self._sleep(5)
                    continue
                raise RuntimeError(f'sem resposta: {err}')
        raise RuntimeError('sem resposta')

    # ---------------------------------------------------------------- one question, one JSON
    def ask_json(self, system, user, example, validate, num_predict=1200, timeout=600, retries=2):
        """(data, status). Asks, checks with `validate(data) -> error or None` and, if the answer does not serve, asks again saying what
        was wrong. Status: 'ok', 'corrigido' (after another request), 'falhou', 'cache' (kept before). Never raises because of the
        answer; raises Stop (pause, cancel, or a server that stopped answering) and Fatal (the provider refuses)."""
        key = cache_key(self.provider, system, user, example, num_predict)
        if self.cache is not None:
            kept = self.cache.get(key)
            if kept is not None:
                self.usage.add(hits=1)
                return kept, 'cache'
        asked = user
        error = 'sem resposta'
        for attempt in range(retries + 1):
            try:
                text, finish = self.chat(system, asked, example, num_predict * (2 if attempt else 1), timeout)
            except (Stop, Fatal):
                raise
            except Exception as err:
                error = f'pedido: {str(err)[:100]}'
                self.usage.add(failures=1)
                self._streak += 1
                if self._streak >= self.max_failures:
                    # a server that is away is not a reason to lose the run: it waits, paused, for someone to resume it
                    raise Stop('paused', f'O servidor parou de responder ({str(err)[:160]}). A geração foi pausada: retome quando ele voltar.')
                continue
            data = extract_json(text)
            problem = ('resposta cortada pelo limite de tokens' if finish == 'length' and data is None else
                       'não é JSON' if data is None else validate(data))
            if problem is None:
                if self.cache is not None:
                    self.cache.put(key, data, self.provider.model)
                return data, ('ok' if attempt == 0 else 'corrigido')
            error = problem
            asked = user + f'\n\nSua resposta anterior não serviu ({problem}). Responda de novo, só com o JSON no formato pedido.'
        self.usage.add(failures=1)
        return None, 'falhou'
