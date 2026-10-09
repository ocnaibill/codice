import pytest


@pytest.fixture(autouse=True)
def no_waiting_between_requests(monkeypatch):
    """The providers leave a pause between two requests to the same host; the tests do not wait for it (the pause itself is tested in test_http)."""
    from providers import http
    monkeypatch.setattr(http, '_sleep', lambda seconds: None)
    http._last_call.clear()
    yield
    http._last_call.clear()
