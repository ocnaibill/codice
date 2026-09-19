#!/usr/bin/env python3
"""Generate a small, deterministic, fully synthetic test corpus for Codice.

Standard library only. Output goes to testdata/corpus/ (next to this script)
and is reproducible: running it twice yields byte-identical files, so hashes
are stable and the duplicate / altered-content cases stay meaningful.

Everything here is invented content; nothing is copied from real books.
"""
import hashlib
import json
import struct
import zipfile
import zlib
from pathlib import Path

OUT = Path(__file__).resolve().parent / "corpus"
FIXED_TIME = (2026, 1, 1, 0, 0, 0)


# ---------------------------------------------------------------- helpers
def png_gray(width, height, pixel):
    """Minimal 8-bit grayscale PNG. pixel(x, y) -> 0..255."""
    raw = bytearray()
    for y in range(height):
        raw.append(0)
        raw.extend(pixel(x, y) for x in range(width))

    def chunk(kind, data):
        body = kind + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body) & 0xFFFFFFFF)

    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 0, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(bytes(raw), 9))
        + chunk(b"IEND", b"")
    )


def page_pixel(seed):
    """Fake 'scanned text': horizontal dark bars on light paper."""
    def px(x, y):
        if 20 <= x < 180 and 30 <= y < 250 and (y // 10 + seed) % 2 == 0 and (x * 7 + y * 3 + seed) % 11 > 2:
            return 40
        return 235
    return px


def add_zip(zf, name, data, stored=False):
    info = zipfile.ZipInfo(name, FIXED_TIME)
    info.compress_type = zipfile.ZIP_STORED if stored else zipfile.ZIP_DEFLATED
    zf.writestr(info, data)


# ------------------------------------------------------------------- EPUB
def build_epub(path, title, author, lang, isbn, chapters, fixed_layout=False, publisher="Editora Sintética", date="2020-05-01"):
    extra_meta = '<meta property="rendition:layout">pre-paginated</meta>' if fixed_layout else ""
    manifest = "".join(
        f'<item id="c{i}" href="c{i}.xhtml" media-type="application/xhtml+xml"/>' for i in range(len(chapters))
    )
    spine = "".join(f'<itemref idref="c{i}"/>' for i in range(len(chapters)))
    opf = f"""<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bid">urn:isbn:{isbn}</dc:identifier>
    <dc:title>{title}</dc:title>
    <dc:creator>{author}</dc:creator>
    <dc:language>{lang}</dc:language>
    <dc:publisher>{publisher}</dc:publisher>
    <dc:date>{date}</dc:date>
    <meta property="dcterms:modified">2026-01-01T00:00:00Z</meta>
    {extra_meta}
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    {manifest}
  </manifest>
  <spine>{spine}</spine>
</package>"""
    nav_items = "".join(f'<li><a href="c{i}.xhtml">{t}</a></li>' for i, (t, _) in enumerate(chapters))
    nav = (
        '<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" '
        'xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Sumário</title></head><body>'
        f'<nav epub:type="toc"><ol>{nav_items}</ol></nav></body></html>'
    )
    container = (
        '<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">'
        '<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>'
    )
    with zipfile.ZipFile(path, "w") as zf:
        add_zip(zf, "mimetype", b"application/epub+zip", stored=True)
        add_zip(zf, "META-INF/container.xml", container.encode())
        add_zip(zf, "OEBPS/content.opf", opf.encode())
        add_zip(zf, "OEBPS/nav.xhtml", nav.encode())
        for i, (t, body) in enumerate(chapters):
            xhtml = (
                '<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml">'
                f"<head><title>{t}</title></head><body><h1>{t}</h1><p>{body}</p></body></html>"
            )
            add_zip(zf, f"OEBPS/c{i}.xhtml", xhtml.encode())


# -------------------------------------------------------------------- CBZ
def build_cbz(path, images, comicinfo):
    with zipfile.ZipFile(path, "w") as zf:
        add_zip(zf, "ComicInfo.xml", comicinfo.encode())
        for name, data in images:
            add_zip(zf, name, data, stored=True)


def comicinfo(series, number, title, manga=None):
    m = f"<Manga>{manga}</Manga>" if manga else ""
    return (
        '<?xml version="1.0"?><ComicInfo><Title>%s</Title><Series>%s</Series><Number>%s</Number>'
        "<Writer>Autora Sintética</Writer><LanguageISO>pt</LanguageISO>%s</ComicInfo>" % (title, series, number, m)
    )


# -------------------------------------------------------------------- PDF
def build_pdf(path, pages):
    """pages: list of ('text', str) or ('image', (w, h, bytes))."""
    objs = {}  # num -> bytes
    next_num = [3]

    def new():
        n = next_num[0]
        next_num[0] += 1
        return n

    kids = []
    font = new()
    objs[font] = b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"
    for kind, payload in pages:
        page = new()
        content = new()
        if kind == "text":
            lines = payload.split("\n")
            ops = ["BT", "/F1 14 Tf", "72 760 Td", "18 TL"]
            for ln in lines:
                esc = ln.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")
                ops.append("(" + esc + ") Tj T*")
            ops.append("ET")
            stream = "\n".join(ops).encode("cp1252")
            objs[content] = b"<< /Length %d >>\nstream\n" % len(stream) + stream + b"\nendstream"
            res = f"<< /Font << /F1 {font} 0 R >> >>"
        else:
            w, h, data = payload
            img = new()
            comp = zlib.compress(data, 9)
            objs[img] = (
                b"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray "
                b"/BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n" % (w, h, len(comp))
                + comp
                + b"\nendstream"
            )
            stream = b"q 595 0 0 842 0 0 cm /Im0 Do Q"
            objs[content] = b"<< /Length %d >>\nstream\n" % len(stream) + stream + b"\nendstream"
            res = f"<< /XObject << /Im0 {img} 0 R >> >>"
        objs[page] = (
            f"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources {res} /Contents {content} 0 R >>"
        ).encode()
        kids.append(page)
    objs[2] = f"<< /Type /Pages /Kids [{' '.join(f'{k} 0 R' for k in kids)}] /Count {len(kids)} >>".encode()
    objs[1] = b"<< /Type /Catalog /Pages 2 0 R >>"

    out = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = {}
    for num in sorted(objs):
        offsets[num] = len(out)
        out += b"%d 0 obj\n" % num + objs[num] + b"\nendobj\n"
    xref = len(out)
    size = max(objs) + 1
    out += b"xref\n0 %d\n0000000000 65535 f \n" % size
    for num in range(1, size):
        out += b"%010d 00000 n \n" % offsets[num]
    out += b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (size, xref)
    path.write_bytes(bytes(out))


def scanned_page(seed):
    w, h = 200, 280
    px = page_pixel(seed)
    return (w, h, bytes(px(x, y) for y in range(h) for x in range(w)))


# ------------------------------------------------------------------- main
def main():
    OUT.mkdir(parents=True, exist_ok=True)
    manifest = []

    def record(name, expect, note, **extra):
        data = (OUT / name).read_bytes()
        manifest.append({
            "file": name, "expect": expect, "note": note,
            "sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data), **extra,
        })

    lorem = "Texto sintético para testes automatizados. " * 12

    build_epub(OUT / "epub_acentos.epub", "Códice: Ação e Reação", "Antônio da Conceição", "pt-BR",
               "9780000000011", [("Capítulo um: Início", "Não há coração sem ação. " + lorem),
                                 ("Capítulo dois: Vovó e o pão", "Órgão, ímã, lição e pântano. " + lorem)])
    record("epub_acentos.epub", "valid", "EPUB refluível com acentos (título, autor, capítulos)", format="epub", language="pt-BR")

    build_epub(OUT / "epub_fixed_layout.epub", "Layout Fixo de Teste", "Bianca Sintética", "pt-BR",
               "9780000000028", [("Página 1", lorem), ("Página 2", lorem)], fixed_layout=True)
    record("epub_fixed_layout.epub", "valid", "EPUB fixed-layout (rendition:layout pre-paginated) para avaliar escopo", format="epub")

    build_epub(OUT / "epub_duplicata.epub", "Códice: Ação e Reação", "Antônio da Conceição", "pt-BR",
               "9780000000011", [("Capítulo um: Início", "Não há coração sem ação. " + lorem),
                                 ("Capítulo dois: Vovó e o pão", "Órgão, ímã, lição e pântano. " + lorem)])
    record("epub_duplicata.epub", "duplicate", "Bytes idênticos a epub_acentos.epub com outro nome (hash igual)", duplicate_of="epub_acentos.epub")

    build_epub(OUT / "epub_alterado.epub", "Códice: Ação e Reação", "Antônio da Conceição", "pt-BR",
               "9780000000011", [("Capítulo um: Início", "Não há coração sem ação. " + lorem),
                                 ("Capítulo dois: Vovó e o pão", "Conteúdo alterado depois da importação. " + lorem)])
    record("epub_alterado.epub", "valid", "Mesma obra e mesmos metadados com conteúdo diferente (hash diferente): nova versão, não duplicata", format="epub", same_work_as="epub_acentos.epub")

    build_epub(OUT / "epub_outra_edicao_en.epub", "Codex: Action and Reaction", "Antonio da Conceicao", "en",
               "9780000000035", [("Chapter one", "There is no heart without action. " + lorem)], publisher="Synthetic Press")
    record("epub_outra_edicao_en.epub", "valid", "Outro idioma e ISBN, autor com grafia parecida: candidato à revisão de vínculo, sem fusão automática", format="epub", language="en", candidate_of="epub_acentos.epub")

    data = (OUT / "epub_acentos.epub").read_bytes()
    (OUT / "epub_corrompido.epub").write_bytes(data[: len(data) // 2])
    record("epub_corrompido.epub", "invalid", "EPUB truncado ao meio (zip inválido)")

    (OUT / "falso.pdf").write_bytes("Isto é apenas texto, não um PDF.\n".encode("utf-8"))
    record("falso.pdf", "invalid", "Extensão .pdf com conteúdo de texto: valida conteúdo, não só extensão")

    build_pdf(OUT / "pdf_digital.pdf", [("text", "Documento digital de teste\nPágina 1: texto nativo selecionável.\nAção, coração e emoção."),
                                       ("text", "Página 2: mais texto nativo para extração.")])
    record("pdf_digital.pdf", "valid", "PDF com texto nativo em todas as páginas", format="pdf", pages=2, text_pages=[1, 2])

    build_pdf(OUT / "pdf_escaneado.pdf", [("image", scanned_page(0)), ("image", scanned_page(1))])
    record("pdf_escaneado.pdf", "valid", "PDF só de imagens, sem camada de texto (candidato a OCR)", format="pdf", pages=2, text_pages=[])

    build_pdf(OUT / "pdf_misto.pdf", [("text", "Página 1 com texto nativo.\nAcentuação: ção, ã, é."), ("image", scanned_page(2))])
    record("pdf_misto.pdf", "valid", "PDF misto: página 1 com texto, página 2 escaneada (OCR só na 2)", format="pdf", pages=2, text_pages=[1])

    def pages(n, seed):
        return [(f"{i:03d}.png", png_gray(200, 280, page_pixel(seed + i))) for i in range(1, n + 1)]

    build_cbz(OUT / "cbz_ltr.cbz", pages(4, 0), comicinfo("Série Sintética", "1", "Volume Um", None))
    record("cbz_ltr.cbz", "valid", "CBZ esquerda para direita, série com posição 1", format="cbz", direction="ltr", series="Série Sintética", series_index=1)

    build_cbz(OUT / "cbz_rtl.cbz", pages(4, 5), comicinfo("Mangá Sintético", "2", "Volume Dois", "YesAndRightToLeft"))
    record("cbz_rtl.cbz", "valid", "CBZ direita para esquerda (ComicInfo Manga=YesAndRightToLeft)", format="cbz", direction="rtl", series="Mangá Sintético", series_index=2)

    tall = png_gray(400, 6000, lambda x, y: 40 if (y // 25) % 2 == 0 and 30 < x < 370 else 235)
    build_cbz(OUT / "cbz_webtoon_imagem_longa.cbz", [("001.png", tall)], comicinfo("Webtoon Sintético", "1", "Capítulo 1", None))
    record("cbz_webtoon_imagem_longa.cbz", "valid", "CBZ com uma imagem muito alta (400x6000), estilo webtoon", format="cbz")

    # write_bytes: no platform newline translation, so hashes are identical on Windows and macOS
    (OUT / "notas.txt").write_bytes("Arquivo de texto simples para testes.\nSegunda linha com acentuação: coração.\n".encode("utf-8"))
    record("notas.txt", "valid", "Texto simples UTF-8", format="txt")

    (OUT / "manifest.json").write_bytes(
        json.dumps({"generated_by": "testdata/generate_corpus.py", "files": manifest}, ensure_ascii=False, indent=2).encode("utf-8") + b"\n"
    )
    print(f"{len(manifest)} files written to {OUT}")


if __name__ == "__main__":
    main()
