"""The client of the model that writes the wiki: against a server that speaks chat completions in a thread."""
import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest

from wikigen.cache import MemoryCache
from wikigen.llm import Fatal, Llm, Provider, Stop, Usage, extract_json


class Fake:
    """A chat completions server that answers from a script: each item is (status, body or text, headers)."""

    def __init__(self, script):
        self.script, self.requests = list(script), []
        fake = self

        class H(BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                fake.requests.append({'path': self.path, 'body': body, 'auth': self.headers.get('Authorization')})
                status, payload, headers = fake.script.pop(0) if fake.script else (200, '{}', {})
                self.send_response(status)
                for k, v in headers.items():
                    self.send_header(k, v)
                if status != 200:
                    self.end_headers()
                    self.wfile.write(json.dumps({'error': {'message': payload}}).encode())
                    return
                self.send_header('Content-Type', 'text/event-stream')
                self.end_headers()
                half = max(1, len(payload) // 2)
                for piece in (payload[:half], payload[half:]):
                    self.wfile.write(('data: ' + json.dumps({'choices': [{'delta': {'content': piece}}]}) + '\n\n').encode())
                self.wfile.write(('data: ' + json.dumps({'choices': [{'delta': {}, 'finish_reason': 'stop'}],
                                                         'usage': {'prompt_tokens': 11, 'completion_tokens': 7}}) + '\n\n').encode())
                self.wfile.write(b'data: [DONE]\n\n')

        self.server = HTTPServer(('127.0.0.1', 0), H)
        self.url = f'http://127.0.0.1:{self.server.server_port}/v1'
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def close(self):
        self.server.shutdown()


@pytest.fixture
def make():
    made = []

    def build(script, kind='openai', **kw):
        fake = Fake(script)
        made.append(fake)
        llm = Llm(Provider(kind, fake.url, 'modelo-x', 'sk-chave'), sleep=lambda s: None, min_interval=0, **kw)
        return fake, llm
    yield build
    for f in made:
        f.close()


def ok(obj):
    return (200, json.dumps(obj), {})


def need_ok(d):
    return None if isinstance(d, dict) and 'ok' in d else "falta 'ok'"


EXAMPLE = {'ok': True}


def test_asks_and_returns_the_json_with_the_key_and_the_cost_counted(make):
    fake, llm = make([ok({'ok': True})])
    data, status = llm.ask_json('sistema', 'pergunta', EXAMPLE, need_ok)
    assert (data, status) == ({'ok': True}, 'ok')
    req = fake.requests[0]
    assert req['path'] == '/v1/chat/completions' and req['auth'] == 'Bearer sk-chave'
    assert req['body']['model'] == 'modelo-x' and req['body']['temperature'] == 0 and req['body']['response_format'] == {'type': 'json_object'}
    assert 'thinking' not in req['body']          # a generic server is not sent the DeepSeek field
    snap = llm.usage.snapshot()
    assert (snap['requests'], snap['tokensIn'], snap['tokensOut']) == (1, 11, 7)


def test_deepseek_is_asked_not_to_reason(make):
    fake, llm = make([ok({'ok': 1})], kind='deepseek')
    llm.ask_json('s', 'p', EXAMPLE, need_ok)
    assert fake.requests[0]['body']['thinking'] == {'type': 'disabled'}


def test_a_fence_or_text_around_the_json_is_not_a_problem(make):
    fake, llm = make([(200, 'Claro! ```json\n{"ok": 2}\n``` pronto', {})])
    assert llm.ask_json('s', 'p', EXAMPLE, need_ok)[0] == {'ok': 2}


def test_an_answer_that_does_not_serve_is_asked_again_saying_what_was_wrong(make):
    fake, llm = make([ok({'outro': 1}), ok({'ok': 3})])
    data, status = llm.ask_json('s', 'p', EXAMPLE, need_ok)
    assert (data, status) == ({'ok': 3}, 'corrigido')
    assert "falta 'ok'" in fake.requests[1]['body']['messages'][1]['content']


def test_three_answers_that_do_not_serve_give_up_without_raising(make):
    fake, llm = make([ok({'x': 1})] * 3)
    assert llm.ask_json('s', 'p', EXAMPLE, need_ok) == (None, 'falhou')
    assert len(fake.requests) == 3


def test_what_was_asked_before_is_not_asked_again(make):
    cache = MemoryCache()
    fake, llm = make([ok({'ok': 4}), ok({'ok': 8})], cache=cache)
    assert llm.ask_json('s', 'p', EXAMPLE, need_ok) == ({'ok': 4}, 'ok')
    assert llm.ask_json('s', 'p', EXAMPLE, need_ok) == ({'ok': 4}, 'cache')
    assert len(fake.requests) == 1 and llm.usage.snapshot()['hits'] == 1
    # another question, or another model, is another answer
    llm.ask_json('s', 'outra', EXAMPLE, need_ok)
    assert len(fake.requests) == 2


def test_a_failure_is_never_kept(make):
    cache = MemoryCache()
    fake, llm = make([ok({'x': 1})] * 3 + [ok({'ok': 5})], cache=cache)
    assert llm.ask_json('s', 'p', EXAMPLE, need_ok)[1] == 'falhou'
    assert llm.ask_json('s', 'p', EXAMPLE, need_ok) == ({'ok': 5}, 'ok')


def test_a_limit_of_requests_waits_for_what_the_server_says_and_asks_again(make):
    slept = []
    fake, llm = make([(429, 'devagar', {'Retry-After': '7'}), ok({'ok': 6})])
    llm.sleep = slept.append
    assert llm.ask_json('s', 'p', EXAMPLE, need_ok)[0] == {'ok': 6}
    assert sum(slept) == 7.0 and llm.usage.snapshot()['throttled'] == 1   # in steps, so that a pause is seen while it waits


@pytest.mark.parametrize('code,text', [(401, 'recusou a chave'), (403, 'recusou a chave'), (402, 'falta saldo'), (404, 'endereço ou este modelo')])
def test_a_provider_that_refuses_stops_the_run_with_a_sentence(make, code, text):
    fake, llm = make([(code, 'x', {})])
    with pytest.raises(Fatal) as err:
        llm.ask_json('s', 'p', EXAMPLE, need_ok)
    assert text in str(err.value)
    assert len(fake.requests) == 1


def test_a_server_that_does_not_take_a_field_is_asked_without_it(make):
    fake, llm = make([(400, 'response_format not supported', {}), ok({'ok': 7})])
    assert llm.ask_json('s', 'p', EXAMPLE, need_ok)[0] == {'ok': 7}
    assert 'response_format' in fake.requests[0]['body'] and 'response_format' not in fake.requests[1]['body']


def test_a_pause_asked_while_it_runs_stops_before_the_next_request(make):
    calls = []

    def control():
        calls.append(1)
        if len(calls) > 1:
            raise Stop('paused')
    fake, llm = make([ok({'ok': 1})] * 3, control=control)
    llm.ask_json('s', 'p1', EXAMPLE, need_ok)
    with pytest.raises(Stop):
        llm.ask_json('s', 'p2', EXAMPLE, need_ok)
    assert len(fake.requests) == 1


def test_the_key_does_not_go_to_the_server_when_there_is_none(make):
    fake = Fake([ok({'ok': 1})])
    try:
        llm = Llm(Provider('ollama', fake.url, 'm', ''), sleep=lambda s: None, min_interval=0)
        llm.ask_json('s', 'p', EXAMPLE, need_ok)
        assert fake.requests[0]['auth'] is None
    finally:
        fake.close()


def test_extract_json_takes_the_schema_filled_in_as_the_data():
    assert extract_json('{"type": "object", "properties": {"a": 1}}') == {'a': 1}
    assert extract_json('nada') is None
    assert extract_json('') is None


def test_a_server_that_stops_answering_pauses_the_run_instead_of_losing_it():
    llm = Llm(Provider('openai', 'http://127.0.0.1:1/v1', 'm', 'k'), sleep=lambda s: None, min_interval=0, max_failures=4)
    with pytest.raises(Stop) as stop:
        for n in range(10):
            llm.ask_json('s', f'p{n}', EXAMPLE, need_ok, retries=0)
    assert stop.value.reason == 'paused' and 'retome quando ele voltar' in stop.value.message


def test_a_long_wait_asked_by_the_server_pauses_the_run_with_what_it_said(make):
    fake, llm = make([(429, "You've used this period's free allowance. Next period: 22 Oct. Key sk-chave.", {'Retry-After': '3600'})])
    with pytest.raises(Stop) as stop:
        llm.ask_json('s', 'p', EXAMPLE, need_ok)
    assert stop.value.reason == 'paused'
    assert 'esperar 60 min' in stop.value.message and 'free allowance' in stop.value.message
    assert 'sk-chave' not in stop.value.message and 'retome quando puder' in stop.value.message
    assert len(fake.requests) == 1


def test_a_short_wait_is_still_waited_for_in_steps(make):
    slept = []
    fake, llm = make([(429, 'devagar', {'Retry-After': '120'}), ok({'ok': 9})])
    llm.sleep = slept.append
    assert llm.ask_json('s', 'p', EXAMPLE, need_ok)[0] == {'ok': 9}
    assert sum(slept) == 120.0
