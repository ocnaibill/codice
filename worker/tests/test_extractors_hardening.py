"""A file that comes from outside is hostile until read: the metadata extractors must not
follow an XML entity to a file on the server, nor load into memory a member of an archive
that says it is enormous (RNF-007)."""
import os
import zipfile

import pytest

from extractors.base import read_member
from extractors.cbz_extractor import CbzExtractor
from extractors.epub_extractor import EpubExtractor
from textindex import Limits

CONTAINER = (
    '<?xml version="1.0"?>'
    '<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">'
    '<rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>'
    '</rootfiles></container>'
)


def _secret(tmp_path):
    secret = tmp_path / 'secret.txt'
    secret.write_text('TOP-SECRET-OF-THE-SERVER')
    return secret


def _epub(tmp_path, opf):
    path = tmp_path / 'book.epub'
    with zipfile.ZipFile(path, 'w') as zf:
        zf.writestr('mimetype', 'application/epub+zip')
        zf.writestr('META-INF/container.xml', CONTAINER)
        zf.writestr('content.opf', opf)
    return str(path)


def test_epub_opf_does_not_expand_an_external_entity(tmp_path):
    secret = _secret(tmp_path)
    opf = (
        f'<?xml version="1.0"?><!DOCTYPE package [<!ENTITY xxe SYSTEM "file://{secret}">]>'
        '<package xmlns="http://www.idpf.org/2007/opf" version="2.0">'
        '<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">'
        '<dc:title>Plain title</dc:title><dc:description>&xxe;</dc:description>'
        '</metadata></package>'
    )
    meta = EpubExtractor().extract(_epub(tmp_path, opf), str(tmp_path))
    assert meta.title == 'Plain title'
    assert 'TOP-SECRET' not in meta.description


def test_epub_opf_does_not_expand_an_internal_entity_bomb(tmp_path):
    opf = (
        '<?xml version="1.0"?><!DOCTYPE package [<!ENTITY a "AAAAAAAAAA"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;">]>'
        '<package xmlns="http://www.idpf.org/2007/opf" version="2.0">'
        '<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">'
        '<dc:title>Bomb</dc:title><dc:description>&b;</dc:description>'
        '</metadata></package>'
    )
    meta = EpubExtractor().extract(_epub(tmp_path, opf), str(tmp_path))
    assert 'AAAA' not in meta.description


def test_epub_container_fallback_does_not_expand_an_external_entity(tmp_path):
    secret = _secret(tmp_path)
    path = tmp_path / 'book.epub'
    container = (
        f'<?xml version="1.0"?><!DOCTYPE container [<!ENTITY xxe SYSTEM "file://{secret}">]>'
        '<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">'
        '<rootfiles><rootfile full-path="&xxe;" media-type="application/oebps-package+xml"/>'
        '</rootfiles></container>'
    )
    with zipfile.ZipFile(path, 'w') as zf:
        zf.writestr('META-INF/container.xml', container)
    meta = EpubExtractor().extract(str(path), str(tmp_path))
    assert 'TOP-SECRET' not in repr(meta)


def test_read_member_refuses_a_member_that_declares_more_than_the_limit(tmp_path):
    path = tmp_path / 'a.zip'
    with zipfile.ZipFile(path, 'w', zipfile.ZIP_DEFLATED) as zf:
        zf.writestr('small', b'x' * 10)
        zf.writestr('big', b'0' * 11)
    with zipfile.ZipFile(path) as zf:
        assert read_member(zf, 'small', 10) == b'x' * 10
        with pytest.raises(ValueError):
            read_member(zf, 'big', 10)


def test_read_member_default_is_the_chapter_limit(tmp_path):
    path = tmp_path / 'a.zip'
    with zipfile.ZipFile(path, 'w', zipfile.ZIP_DEFLATED) as zf:
        zf.writestr('big', b'0' * (Limits.MAX_ENTRY_BYTES + 1))
    with zipfile.ZipFile(path) as zf:
        with pytest.raises(ValueError):
            read_member(zf, 'big')


def test_read_member_missing_member_is_still_a_key_error(tmp_path):
    path = tmp_path / 'a.zip'
    with zipfile.ZipFile(path, 'w') as zf:
        zf.writestr('x', b'1')
    with zipfile.ZipFile(path) as zf:
        with pytest.raises(KeyError):
            read_member(zf, 'nope')


def test_epub_cover_over_the_limit_is_not_loaded(tmp_path, monkeypatch):
    monkeypatch.setattr(Limits, 'MAX_ENTRY_BYTES', 1000)
    path = tmp_path / 'book.epub'
    opf = (
        '<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="2.0">'
        '<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>T</dc:title></metadata></package>'
    )
    with zipfile.ZipFile(path, 'w', zipfile.ZIP_DEFLATED) as zf:
        zf.writestr('META-INF/container.xml', CONTAINER)
        zf.writestr('content.opf', opf)
        zf.writestr('cover.jpg', b'0' * 5000)
    covers = tmp_path / 'covers'
    covers.mkdir()
    meta = EpubExtractor().extract(str(path), str(covers))
    assert meta.title == 'T'
    assert meta.cover_path == ''
    assert os.listdir(covers) == []


def test_cbz_comicinfo_over_the_limit_is_not_read(tmp_path, monkeypatch):
    monkeypatch.setattr(Limits, 'MAX_METADATA_BYTES', 100)
    path = tmp_path / 'c.cbz'
    info = '<ComicInfo><Title>Read anyway</Title>' + '<!--' + 'x' * 500 + '--></ComicInfo>'
    with zipfile.ZipFile(path, 'w') as zf:
        zf.writestr('ComicInfo.xml', info)
    meta = CbzExtractor().extract(str(path), str(tmp_path))
    assert meta.title != 'Read anyway'


def test_epub_opf_over_the_limit_is_not_read(tmp_path, monkeypatch):
    monkeypatch.setattr(Limits, 'MAX_ENTRY_BYTES', 150)
    opf = (
        '<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="2.0">'
        '<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Never read</dc:title>'
        '<dc:description>' + 'x' * 400 + '</dc:description></metadata></package>'
    )
    meta = EpubExtractor().extract(_epub(tmp_path, opf), str(tmp_path))
    assert meta.title != 'Never read'


def test_epub_container_fallback_over_the_limit_is_not_read(tmp_path, monkeypatch):
    monkeypatch.setattr(Limits, 'MAX_METADATA_BYTES', 50)
    # no member is named .opf, so only the container can say where the package is
    path = tmp_path / 'book.epub'
    with zipfile.ZipFile(path, 'w') as zf:
        zf.writestr('META-INF/container.xml', CONTAINER.replace('content.opf', 'package.xml'))
        zf.writestr(
            'package.xml',
            '<package xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/">'
            '<dc:title>Via container</dc:title></metadata></package>',
        )
    meta = EpubExtractor().extract(str(path), str(tmp_path))
    assert meta.title != 'Via container'


def test_cbz_cover_over_the_limit_is_not_loaded(tmp_path, monkeypatch):
    monkeypatch.setattr(Limits, 'MAX_ENTRY_BYTES', 1000)
    path = tmp_path / 'c.cbz'
    with zipfile.ZipFile(path, 'w', zipfile.ZIP_DEFLATED) as zf:
        zf.writestr('page1.jpg', b'0' * 5000)
    covers = tmp_path / 'covers'
    covers.mkdir()
    meta = CbzExtractor().extract(str(path), str(covers))
    assert meta.cover_path == ''
    assert os.listdir(covers) == []
