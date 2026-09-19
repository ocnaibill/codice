"""The title derived from a stored file name must not carry the server's prefixes."""
import pytest

from extractors import TxtExtractor


@pytest.mark.parametrize("stored, expected", [
    ("7a47b1fb6a98_pdf_digital.pdf", "pdf digital"),        # random id (uploads and bulk import)
    ("123456789012_Duna.epub", "Duna"),                     # a random id made only of digits
    ("1785249535_Absolute Batman 006.cbz", "Absolute Batman 006"),  # older timestamp prefix
    ("1785249535_3_Watchmen.cbz", "Watchmen"),              # older bulk-import prefix (timestamp and counter)
    ("a1b2c3d4e5f6_O Teste da Mãe.epub", "O Teste da Mãe"),
    ("Duna.epub", "Duna"),                                  # nothing to strip
    ("deadbeef_not_a_prefix.epub", "deadbeef not a prefix"),  # only 8 hex characters: part of the title
])
def test_fallback_title_strips_server_prefixes(stored, expected):
    assert TxtExtractor()._fallback_title("/storage/" + stored) == expected
