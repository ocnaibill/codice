"""Opens the API key of the wiki provider, which the backend keeps sealed in the database (DEC-185).

The format is the backend's (backend/internal/wiki/sealed.go): "v1:" and the base64 of nonce (12 bytes), ciphertext and tag of
AES-256-GCM, under the HMAC-SHA256 of a fixed purpose with the master key of the deployment (CODICE_SECRET_KEY) as its key, and the
purpose as associated data. The key that comes out is only used for the request to the provider: never logged, never stored.
"""
import base64
import hashlib
import hmac
import os

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

MIN_MASTER_KEY = 32
_VERSION = "v1:"
_PURPOSE = b"codice:wiki-provider-key"


class NoMasterKey(Exception):
    """The deployment has no master key (CODICE_SECRET_KEY): the key of the provider cannot be read."""


class Unreadable(Exception):
    """The sealed key does not open with this master key: it was sealed by another, or the master key changed."""


def master_key() -> str:
    return os.getenv("CODICE_SECRET_KEY", "")


def open_sealed(sealed: str, master: str | None = None) -> str:
    master = master_key() if master is None else master
    if len(master) < MIN_MASTER_KEY:
        raise NoMasterKey()
    if not sealed.startswith(_VERSION):
        raise Unreadable()
    try:
        raw = base64.b64decode(sealed[len(_VERSION):], validate=True)
    except ValueError as err:
        raise Unreadable() from err
    if len(raw) < 12 + 16:
        raise Unreadable()
    key = hmac.new(master.encode(), _PURPOSE, hashlib.sha256).digest()
    try:
        return AESGCM(key).decrypt(raw[:12], raw[12:], _PURPOSE).decode()
    except InvalidTag as err:
        raise Unreadable() from err
