#!/usr/bin/env python3
"""Generate a large, deterministic, fully synthetic library for the scale test.

    python3 testdata/generate_scale.py --count 10000 --out /some/folder [--seed 1] [--start 0]

Standard library only. Every file is different (so none is a duplicate), has its own
title, an author and a series drawn from pools of realistic size, a cover (EPUB) or
pages (CBZ), and a body of text from a fixed vocabulary, so a search for a common
word finds thousands of works and a search for the marker of one finds exactly one.
Mix: 65% EPUB, 20% CBZ, 10% TXT, 5% PDF. Everything is invented.

The same --seed and --count give byte-identical files; --start continues a library
(generate 1000, then 9000 more with --start 1000 to reach 10000).

Each work's marker is printed in the manifest (manifest.jsonl in --out): that file
tells the measuring script which words to search for.
"""
import argparse
import hashlib
import json
import random
import struct
import sys
import zipfile
import zlib
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import generate_corpus as gc  # noqa: E402  (builders for CBZ and PDF)

SYLLABLES = ["ma", "ri", "ne", "so", "lu", "ca", "do", "ve", "ta", "pe", "gi", "bo", "ar", "el", "in", "or", "un", "ça", "ão", "ém"]
WORDS_COMMON = (
    "casa tempo vida mundo noite dia caminho rio mar cidade história amor guerra paz sonho livro "
    "palavra silêncio memória viagem janela porta jardim estrada ponte montanha floresta ilha "
    "verdade segredo promessa lembrança coração mão olhar voz sombra luz fogo água terra vento"
).split()
LANGS = [("pt", 0.55), ("en", 0.2), ("es", 0.1), ("fr", 0.08), ("it", 0.07)]
FORMATS = [("epub", 0.65), ("cbz", 0.20), ("txt", 0.10), ("pdf", 0.05)]


def pick(rng, weighted):
    r, acc = rng.random(), 0.0
    for value, w in weighted:
        acc += w
        if r < acc:
            return value
    return weighted[-1][0]


def name(rng, parts):
    return "".join(rng.choice(SYLLABLES) for _ in range(parts)).capitalize()


def vocabulary(rng, size=3000):
    return [name(rng, rng.randint(2, 4)).lower() for _ in range(size)]


def cover_png(seed, w=96, h=144):
    """A small cover whose bytes depend on the work (so every cover is different)."""
    r = random.Random(seed)
    a, b, c = r.randrange(40, 215), r.randrange(40, 215), r.randrange(0, 4)

    def px(x, y):
        return (a + (x * (c + 1) + y) // 2 + (b if (x // 12 + y // 12) % 2 else 0)) % 256

    return gc.png_gray(w, h, px)


def body(rng, vocab, marker, words):
    out = []
    for i in range(words):
        out.append(rng.choice(WORDS_COMMON) if rng.random() < 0.12 else rng.choice(vocab))
    if marker:
        out.insert(len(out) // 2, marker)
    return " ".join(out) + "."


def build_epub(path, meta, rng, vocab):
    cover = cover_png(meta["n"])
    chapters = [(f"Capítulo {i + 1}", body(rng, vocab, meta["marker"] if i == 1 else "", 450)) for i in range(3)]
    manifest = "".join(f'<item id="c{i}" href="c{i}.xhtml" media-type="application/xhtml+xml"/>' for i in range(3))
    spine = "".join(f'<itemref idref="c{i}"/>' for i in range(3))
    series = (
        f'<meta property="belongs-to-collection" id="s">{meta["series"]}</meta>'
        f'<meta refines="#s" property="group-position">{meta["number"]}</meta>' if meta["series"] else ""
    )
    opf = f"""<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bid">urn:uuid:{meta["uuid"]}</dc:identifier>
    <dc:title>{meta["title"]}</dc:title>
    <dc:creator>{meta["author"]}</dc:creator>
    <dc:language>{meta["lang"]}</dc:language>
    <dc:publisher>{meta["publisher"]}</dc:publisher>
    <dc:date>{meta["year"]}-01-01</dc:date>
    <meta property="dcterms:modified">2026-01-01T00:00:00Z</meta>
    {series}
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="cover" href="cover.png" media-type="image/png" properties="cover-image"/>
    {manifest}
  </manifest>
  <spine>{spine}</spine>
</package>"""
    nav = (
        '<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" '
        'xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Sumário</title></head><body>'
        '<nav epub:type="toc"><ol>' + "".join(f'<li><a href="c{i}.xhtml">{t}</a></li>' for i, (t, _) in enumerate(chapters))
        + "</ol></nav></body></html>"
    )
    container = (
        '<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">'
        '<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>'
    )
    with zipfile.ZipFile(path, "w") as zf:
        gc.add_zip(zf, "mimetype", b"application/epub+zip", stored=True)
        gc.add_zip(zf, "META-INF/container.xml", container.encode())
        gc.add_zip(zf, "OEBPS/content.opf", opf.encode())
        gc.add_zip(zf, "OEBPS/nav.xhtml", nav.encode())
        gc.add_zip(zf, "OEBPS/cover.png", cover, stored=True)
        for i, (t, text) in enumerate(chapters):
            xhtml = (
                '<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml">'
                f"<head><title>{t}</title></head><body><h1>{t}</h1><p>{text}</p></body></html>"
            )
            gc.add_zip(zf, f"OEBPS/c{i}.xhtml", xhtml.encode())


def build_cbz(path, meta, rng, vocab):
    pages = [(f"{i + 1:03d}.png", cover_png(meta["n"] * 10 + i, 120, 180)) for i in range(4)]
    info = gc.comicinfo(meta["series"] or meta["title"], meta["number"], meta["title"])
    gc.build_cbz(path, pages, info)


def build_txt(path, meta, rng, vocab):
    path.write_text(f'{meta["title"]}\n\n{body(rng, vocab, meta["marker"], 900)}\n', encoding="utf-8")


def build_pdf(path, meta, rng, vocab):
    gc.build_pdf(path, [("text", body(rng, vocab, meta["marker"], 120)[:600].encode("latin-1", "replace").decode("latin-1"))])


BUILD = {"epub": build_epub, "cbz": build_cbz, "txt": build_txt, "pdf": build_pdf}


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--count", type=int, required=True)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--start", type=int, default=0)
    ap.add_argument("--universe", type=int, default=10000, help="size of the whole library the pools of authors and series are made for; keep it the same when continuing with --start")
    a = ap.parse_args()

    a.out.mkdir(parents=True, exist_ok=True)
    pool = random.Random(a.seed)
    vocab = vocabulary(pool)
    n_authors = max(20, a.universe) // 4
    authors = [f"{name(pool, 2)} {name(pool, 3)}" for _ in range(n_authors)]
    publishers = [f"Editora {name(pool, 3)}" for _ in range(60)]
    series_pool = [f"{name(pool, 3)} {pool.choice(WORDS_COMMON)}" for _ in range(max(10, a.universe // 12))]

    manifest = (a.out / "manifest.jsonl").open("a", encoding="utf-8")
    for n in range(a.start, a.start + a.count):
        rng = random.Random(a.seed * 1_000_003 + n)
        fmt = pick(rng, FORMATS)
        in_series = rng.random() < 0.18
        meta = {
            "n": n,
            "format": fmt,
            "title": f'{rng.choice(WORDS_COMMON).capitalize()} {rng.choice(vocab)} {rng.choice(WORDS_COMMON)} {n:05d}',
            "author": rng.choice(authors),
            "lang": pick(rng, LANGS),
            "publisher": rng.choice(publishers),
            "year": rng.randint(1950, 2025),
            "series": rng.choice(series_pool) if in_series else "",
            "number": rng.randint(1, 30),
            "marker": f"zq{n:06d}x",
            "uuid": str(__import__("uuid").UUID(bytes=hashlib.sha256(f"{a.seed}-{n}".encode()).digest()[:16])),
        }
        path = a.out / f"obra-{n:06d}.{fmt}"
        BUILD[fmt](path, meta, rng, vocab)
        manifest.write(json.dumps({k: meta[k] for k in ("n", "format", "title", "author", "lang", "series", "marker")}, ensure_ascii=False) + "\n")
    manifest.close()
    print(f"{a.count} files written to {a.out} (from #{a.start})")


if __name__ == "__main__":
    main()
