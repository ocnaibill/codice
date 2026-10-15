import pytest

from wiki_secret import NoMasterKey, Unreadable, open_sealed

MASTER = "0123456789abcdef0123456789abcdef"
# Sealed by the backend with a fixed nonce (backend/internal/wiki/provider_test.go has the same vector).
VECTOR = "v1:AAECAwQFBgcICQoLEhqiAGK5tK8XIukvvXcM6gmq//yh3OvU"


def test_opens_what_the_backend_sealed():
    assert open_sealed(VECTOR, MASTER) == "sk-teste"


def test_reads_the_master_key_from_the_environment(monkeypatch):
    monkeypatch.setenv("CODICE_SECRET_KEY", MASTER)
    assert open_sealed(VECTOR) == "sk-teste"


@pytest.mark.parametrize("sealed", [VECTOR[:-4] + "AAAA", "v2:" + VECTOR[3:], "v1:not base64!", "v1:AAEC"])
def test_a_changed_or_foreign_seal_does_not_open(sealed):
    with pytest.raises(Unreadable):
        open_sealed(sealed, MASTER)


def test_another_master_key_does_not_open_it():
    with pytest.raises(Unreadable):
        open_sealed(VECTOR, "x" * 32)


@pytest.mark.parametrize("master", ["", "curta"])
def test_without_a_master_key_nothing_is_read(master):
    with pytest.raises(NoMasterKey):
        open_sealed(VECTOR, master)
