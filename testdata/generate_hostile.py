#!/usr/bin/env python3
"""Generate hostile, synthetic files for the beta's "refuse or fail cleanly" checks (issue #89, section 1).

These files look right at the door (the extension, the first bytes) and are wrong inside, or are legitimate files that a
careless filter would refuse. The point is what Codice does with each: it must either refuse with a clear message, or accept
and end in a clean `failed`/`empty` state with a reason, never take the worker down or leave a half-made work behind, and
never refuse a legitimate file.

Everything is invented. Output goes to --out (default: testdata/hostile/, not committed). Needs the standard library, and
PyMuPDF (`worker/venv`) for the two password-protected PDFs; without it those two are skipped.
Run it with `worker/venv/bin/python testdata/generate_hostile.py --out DIR`, then `testdata/run_hostile.py`.

`expect` in manifest.json is what a healthy Codice does:
  refuse   the upload is refused with 4xx and a message, and nothing is left behind;
  clean    the upload is accepted and every job ends, text extraction is `failed` or `empty` with a reason (the work may exist);
  ok       the upload is accepted and the text is read (a legitimate file).
"""
import argparse
import json
import random
import struct
import zipfile
import zlib
from pathlib import Path

import generate_corpus as corpus

HERE = Path(__file__).resolve().parent
CORPUS = HERE / "corpus"
manifest = []


def record(out, name, expect, note, data=None, upload_as=None):
    if data is not None:
        (out / name).write_bytes(data)
    manifest.append({"file": name, "expect": expect, "note": note, **({"upload_as": upload_as} if upload_as else {})})


def zip_bytes(entries, stored=False):
    import io
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        for name, data in entries:
            corpus.add_zip(zf, name, data, stored=stored)
    return buf.getvalue()


def epub_entries(title="Livro", author="Autora Sintética", chapters=None, container=None, extra=()):
    """The entries of a small, valid EPUB, to be broken one way or another."""
    chapters = chapters or [("Capítulo 1", "Texto de um capítulo qualquer, com palavras suficientes para virar um trecho.")]
    tmp = HERE / ".hostile-tmp.epub"
    corpus.build_epub(tmp, title, author, "pt", "9780000000002", chapters)
    with zipfile.ZipFile(tmp) as zf:
        entries = [(i.filename, zf.read(i.filename)) for i in zf.infolist()]
    tmp.unlink()
    if container is not None:
        entries = [(n, container if n == "META-INF/container.xml" else d) for n, d in entries]
    return entries + list(extra)


# ------------------------------------------------------------------ PDFs
def pdfs(out):
    text = [("text", "Página %d. Um texto qualquer para a extração achar o que ler, com acentuação: coração." % i) for i in range(1, 6)]
    good = out / ".good.pdf"
    corpus.build_pdf(good, text)
    raw = good.read_bytes()
    good.unlink()

    record(out, "pdf_truncado.pdf", "clean", "Cabeçalho certo, termina no meio (sem xref nem %%EOF)", raw[: len(raw) * 6 // 10])
    broken = raw.replace(b"startxref\n", b"startxref\n9999")
    record(out, "pdf_xref_quebrada.pdf", "ok", "Tabela xref apontando para fora: o MuPDF costuma reconstruir e ler o texto", broken)
    record(out, "pdf_so_cabecalho.pdf", "clean", "%PDF-1.4 e depois bytes sem sentido",
           b"%PDF-1.4\n" + bytes(random.Random(1).randrange(256) for _ in range(2000)))
    nopages = raw.replace(b"/Count 5", b"/Count 0")
    nopages = nopages.replace(b"/Kids [", b"/Kids [] /Ignored [")
    record(out, "pdf_zero_paginas.pdf", "clean", "Árvore de páginas vazia", nopages)
    huge = raw.replace(b"/MediaBox [0 0 595 842]", b"/MediaBox [0 0 14400 14400]")
    record(out, "pdf_pagina_gigante.pdf", "ok", "Página de 14.400 x 14.400 pt (o máximo do formato): ler o texto não pode estourar a memória", huge)
    record(out, "pdf_com_extensao_epub.epub", "refuse", "Um PDF de verdade chamado .epub", raw)

    try:
        import fitz  # PyMuPDF
    except ImportError:
        print("PyMuPDF missing: the two password-protected PDFs were skipped")
        return
    doc = fitz.open(str(CORPUS / "pdf_digital.pdf"))
    for name, user, expect, note in (
        ("pdf_protegido_senha_para_abrir.pdf", "segredo", "clean", "AES-256, pede senha para abrir: sem a senha nada se lê"),
        ("pdf_protegido_so_dono.pdf", "", "ok", "AES-256 só com senha de dono (copiar proibido): abre sem senha, e o texto é lido"),
    ):
        path = out / name
        doc.save(str(path), encryption=fitz.PDF_ENCRYPT_AES_256, owner_pw="dono-secreto", user_pw=user,
                 permissions=fitz.PDF_PERM_PRINT)
        record(out, name, expect, note)
    doc.close()


# ----------------------------------------------------------------- EPUBs
def epubs(out):
    adept = ('<?xml version="1.0"?><encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container">'
             '<EncryptedData xmlns="http://www.w3.org/2001/04/xmlenc#"><EncryptionMethod Algorithm="http://www.w3.org/2001/04/xmlenc#aes128-cbc"/>'
             '<KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#"><resource xmlns="http://ns.adobe.com/adept">urn:uuid:1</resource></KeyInfo>'
             '<CipherData><CipherReference URI="OEBPS/c0.xhtml"/></CipherData></EncryptedData></encryption>')
    drm = epub_entries(extra=[("META-INF/encryption.xml", adept.encode()), ("META-INF/rights.xml", b"<rights xmlns='http://ns.adobe.com/adept'/>")])
    drm = [(n, (random.Random(2).randbytes(400) if n == "OEBPS/c0.xhtml" else d)) for n, d in drm]
    record(out, "epub_com_drm_adobe.epub", "clean", "Capítulos cifrados (Adobe ADEPT) e encryption.xml: sem texto legível, sem tentar quebrar", zip_bytes(drm))
    apple = epub_entries(extra=[("META-INF/sinf.xml", b"<fairplay/>")])
    record(out, "epub_com_drm_apple.epub", "ok", "Só um sinf.xml sem capítulos cifrados: não é prova de DRM, o texto é lido", zip_bytes(apple))

    fonts = ('<?xml version="1.0"?><encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container">'
             '<EncryptedData xmlns="http://www.w3.org/2001/04/xmlenc#"><EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/>'
             '<CipherData><CipherReference URI="OEBPS/fonte.otf"/></CipherData></EncryptedData></encryption>')
    record(out, "epub_fontes_ofuscadas.epub", "ok", "encryption.xml só da ofuscação de fontes (comum em livros de editoras): NÃO é DRM",
           zip_bytes(epub_entries(extra=[("META-INF/encryption.xml", fonts.encode()), ("OEBPS/fonte.otf", b"OTTO" + bytes(64))])))

    record(out, "epub_sem_opf.epub", "clean", "container.xml aponta para um OPF que não existe",
           zip_bytes(epub_entries(container=(
               b'<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">'
               b'<rootfiles><rootfile full-path="OEBPS/nao-existe.opf" media-type="application/oebps-package+xml"/></rootfiles></container>'))))

    ents = epub_entries()
    ents = [(n, (b"<html><body><h1>Sem fechar<p>Texto com <b>tag aberta & e&nbsp;entidades <i>quebradas" if n == "OEBPS/c0.xhtml" else d)) for n, d in ents]
    record(out, "epub_xhtml_quebrado.epub", "ok", "Capítulo com XML malformado (tags abertas, & solto): leitores reais toleram, o texto deve sair", zip_bytes(ents))

    record(out, "epub_zip_slip.epub", "ok", "Entradas com ../ e caminho absoluto: o texto é lido e nada pode ser escrito fora da pasta (conferir com find / -name 'escapou*' nos contêineres)",
           zip_bytes(epub_entries(extra=[("../../escapou.txt", b"escapou"), ("/tmp/escapou-abs.txt", b"escapou")])))

    # Decompression bomb: ~512 MiB of text in one chapter, a few hundred KB on disk.
    import io
    buf = io.BytesIO()
    entries = epub_entries()
    with zipfile.ZipFile(buf, "w") as zf:
        for n, d in entries:
            if n != "OEBPS/c0.xhtml":
                corpus.add_zip(zf, n, d, stored=(n == "mimetype"))
        info = zipfile.ZipInfo("OEBPS/c0.xhtml", corpus.FIXED_TIME)
        info.compress_type = zipfile.ZIP_DEFLATED
        chunk = (b"palavra " * 131072)  # 1 MiB
        with zf.open(info, "w", force_zip64=True) as w:
            w.write(b'<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body><p>')
            for _ in range(512):
                w.write(chunk)
            w.write(b"</p></body></html>")
    record(out, "epub_bomba_de_descompressao.epub", "clean",
           "Um capítulo de 512 MiB de texto em poucas centenas de KB: a extração tem que ter teto e não derrubar o worker", buf.getvalue())

    record(out, "epub_sem_titulo_nem_autor.epub", "ok", "Metadados vazios: a obra existe com um título derivado do arquivo",
           zip_bytes(epub_entries(title="", author="")))
    odd = "Título " + "ñ\u202eRTL\u0000\x07🙂" * 3000
    record(out, "epub_titulo_enorme_e_estranho.epub", "ok", "Título de ~30 mil caracteres com RTL, NUL, controle e emoji: o banco e a tela não podem quebrar",
           zip_bytes(epub_entries(title=odd.replace("\x00", "").replace("\x07", ""), author="A" * 5000)))
    real = zip_bytes(epub_entries())
    record(out, "epub_com_extensao_pdf.pdf", "refuse", "Um EPUB de verdade chamado .pdf", real)


# ------------------------------------------------------------------ CBZ
def png_header_only(width, height):
    def chunk(kind, data):
        body = kind + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body) & 0xFFFFFFFF)
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 0, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(b"\x00" * 1000, 9)) + chunk(b"IEND", b""))


def cbzs(out):
    page = corpus.png_gray(200, 280, corpus.page_pixel(1))
    info = corpus.comicinfo("Série Hostil", "1", "Edição de teste")
    cut = page[: len(page) // 2]
    record(out, "cbz_imagem_corrompida.cbz", "ok", "Uma página truncada no meio de três boas: a leitura segue, o teste é que a obra abra",
           zip_bytes([("ComicInfo.xml", info.encode()), ("001.png", page), ("002.png", cut), ("003.png", page)], stored=True))
    record(out, "cbz_imagem_bomba.cbz", "ok", "PNG que declara 60.000 x 60.000 px (3,6 bilhões): a capa e a miniatura não podem alocar isso",
           zip_bytes([("ComicInfo.xml", info.encode()), ("001.png", png_header_only(60000, 60000)), ("002.png", page)], stored=True))
    record(out, "cbz_zip_slip.cbz", "ok", "Entradas com ../ junto de páginas boas: nada escapa da pasta",
           zip_bytes([("001.png", page), ("../../escapou.png", page), ("/tmp/escapou-abs.png", page)], stored=True))
    record(out, "cbz_nomes_unicode.cbz", "ok", "Páginas com nomes em japonês, acentos e espaços, fora de ordem lexical",
           zip_bytes([("ページ 10.png", page), ("ページ 2.png", page), ("pagina ção 1.png", page)], stored=True))
    record(out, "cbz_com_um_pdf_dentro.cbz", "refuse", "ZIP sem nenhuma imagem, só um PDF",
           zip_bytes([("livro.pdf", (CORPUS / "pdf_digital.pdf").read_bytes())]))


# ---------------------------------------------------------------- áudio
def audios(out):
    record(out, "mp3_so_cabecalho_id3.mp3", "clean", "ID3v2 que declara um tag de 200 MB e o arquivo acaba",
           b"ID3\x03\x00\x00\x00\x7f\x7f\x7f" + bytes(100))
    record(out, "mp3_truncado.mp3", "clean", "Quadro MPEG válido e depois nada: sem duração nem capítulos",
           b"\xff\xfb\x90\x00" + bytes(413))
    record(out, "m4b_so_ftyp.m4b", "clean", "Só a caixa ftyp: sem moov, sem áudio",
           struct.pack(">I", 24) + b"ftypM4B \x00\x00\x00\x00M4B isom")


# --------------------------------------------------------------- texto
def texts(out):
    sample = "Coração, ação e emoção: acentuação em português. “Aspas” e travessão — também.\n" * 40
    record(out, "txt_cp1252.txt", "ok", "Texto Windows-1252 de verdade (acentos em um byte): é a codificação do TXT antigo", sample.encode("cp1252"))
    # Another text than the Windows-1252 one: the same text in two encodings is the same file once converted (DEC-126), a duplicate.
    other = "Um texto em UTF-16, com acentuação: ação, coração e emoção — salvo pelo Bloco de Notas.\r\n" * 30
    record(out, "txt_utf16le_bom.txt", "ok", "UTF-16 LE com BOM (o 'Unicode' do Bloco de Notas)", b"\xff\xfe" + other.encode("utf-16-le"))
    record(out, "txt_utf8_bom.txt", "ok", "UTF-8 com BOM", b"\xef\xbb\xbf" + sample.encode("utf-8"))
    record(out, "md_cp1252.md", "ok", "Markdown em Windows-1252", ("# Título\n\n" + sample).encode("cp1252"))
    record(out, "txt_binario_com_extensao_txt.txt", "refuse", "Bytes aleatórios chamados .txt", random.Random(3).randbytes(4096))


# --------------------------------------------------------------- nomes
def names(out):
    """Same good file under hostile names. `upload_as` is the name sent in the form; the file on disk has a plain one."""
    good = (CORPUS / "notas.txt").read_bytes()
    for slug, sent, expect, note in (
        ("nome_com_barras.txt", "../../../etc/passwd.txt", "ok", "O nome enviado tenta sair da pasta: o arquivo guardado fica dentro dela, com o nome limpo"),
        ("nome_comprido.txt", ("n" * 300) + ".txt", "ok", "Nome de 300 caracteres (o limite comum do sistema de arquivos é 255)"),
        ("nome_com_quebra.txt", "linha1\nlinha2\r\n.txt", "refuse", "Nome com quebras de linha cruas no cabeçalho: não é um formulário válido, 400 com mensagem (os nomes que vêm de um disco são limpos pela importação em lote)"),
        ("nome_rtl_e_nul.txt", "livro\u202etxt.exe\u0000.txt", "refuse", "Nome com inversão de direção (RTL) e um NUL cru no cabeçalho: 400 com mensagem (a limpeza do nome cobre os que vêm de um disco)"),
        ("nome_so_extensao.txt", ".txt", "ok", "Nome que é só a extensão"),
    ):
        record(out, slug, expect, note, good + slug.encode() + b"\n", upload_as=sent)  # distinct bytes: the same content is refused as a duplicate


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--out", type=Path, default=HERE / "hostile")
    out = ap.parse_args().out
    out.mkdir(parents=True, exist_ok=True)
    for build in (pdfs, epubs, cbzs, audios, texts, names):
        build(out)
    (out / "manifest.json").write_text(json.dumps({"generated_by": "testdata/generate_hostile.py", "files": manifest}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"{len(manifest)} files written to {out}")


if __name__ == "__main__":
    main()
