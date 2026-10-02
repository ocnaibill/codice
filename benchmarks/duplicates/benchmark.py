#!/usr/bin/env python3
"""Reproducible public-domain benchmark for finding the same text in two files (#38).

It measures the production fingerprint (backend/internal/fingerprint) on books that can be redistributed, the texts of
which come from the set of benchmarks/equivalence (five Project Gutenberg EPUBs, SHA-256 pinned, downloaded to tmp/).
Nothing here approves a threshold: it says, pair by pair, what the fingerprint says, and where that is not what a
person would say.

The pairs: the same text as an EPUB and as a PDF made from it; two editions of one text; a book in a collection; a
part of a book; a translation; two books of the same author that share characters; two unrelated books.
"""
from __future__ import annotations

import argparse
import json
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'worker'))
sys.path.insert(0, str(ROOT / 'benchmarks' / 'equivalence'))

import fitz  # noqa: E402

import benchmark as books  # noqa: E402  (the equivalence benchmark: the downloads and the EPUB reader)
from textindex.pdf import pdf_segments  # noqa: E402
from textindex.normalize import clean  # noqa: E402


def words(segments, body_only=True):
    """The words of a book, as the backend takes them (see equivalence.Words): lower case, no accents, letters and digits."""
    import re
    import unicodedata
    out = []
    for s in segments:
        if body_only and s.get('Part') not in ('body', ''):
            continue
        text = unicodedata.normalize('NFD', s['Text'].lower())
        out.extend(re.findall(r'[^\W_]+', ''.join(c for c in text if not unicodedata.combining(c))))
    return out


def make_pdf(path: Path, paragraphs: list[str]) -> None:
    """A PDF of the text, laid out in pages as a typesetter would: the paragraphs flow from page to page."""
    doc = fitz.open()
    font = fitz.Font('tiro')
    rect = fitz.Rect(40, 50, 380, 590)
    queue = [word for paragraph in paragraphs for word in paragraph.split()]
    at = 0
    while at < len(queue):
        page = doc.new_page(width=420, height=640)
        writer = fitz.TextWriter(page.rect)
        chunk = queue[at:at + 900]  # more than a page holds
        left = writer.fill_textbox(rect, ' '.join(chunk), font=font, fontsize=10, lineheight=1.3)
        writer.write_text(page)
        unplaced = sum(len((line if isinstance(line, str) else line[0]).split()) for line in left)
        at += len(chunk) - unplaced
    doc.save(str(path))
    doc.close()


def corpus(cache: Path) -> dict:
    files = {}
    raw = {}
    for key in ('verne-en', 'verne-en-illustrated', 'verne-pt', 'holmes-adventures', 'holmes-study'):
        raw[key] = books.extract(books.download(cache, key, books.BOOKS[key]))
        files[key] = words(raw[key]['Segments'])
    en = raw['verne-en']['Segments']
    # The same text as a PDF: the paragraphs of the body, laid out in pages and read back by the worker's PDF reader.
    pdf_path = cache / 'verne-en.pdf'
    if not pdf_path.exists():
        print('desenhando o PDF...', file=sys.stderr)
        make_pdf(pdf_path, [s['Text'].replace('\n', ' ') for s in en if s.get('Part') in ('body', '')])
    files['verne-en-pdf'] = words([{'Text': s.text} for s in pdf_segments(str(pdf_path))], body_only=False)
    # The text with its licence and everything: a file with no outline, read whole.
    files['verne-en-whole'] = words(en, body_only=False)
    files['verne-en-first-third'] = files['verne-en'][:len(files['verne-en']) // 3]
    files['collection'] = files['verne-en'] + files['holmes-adventures']
    return files


PAIRS = [
    ('verne-en', 'verne-en-illustrated', 'duas edições do mesmo texto', 'same'),
    ('verne-en', 'verne-en-pdf', 'o EPUB e o PDF feito dele', 'same'),
    ('verne-en-illustrated', 'verne-en-pdf', 'a outra edição e o PDF', 'same'),
    ('verne-en-whole', 'verne-en-illustrated', 'um arquivo sem sumário (com a licença) e a outra edição', 'same'),
    ('verne-en', 'verne-en-first-third', 'um terço do livro e o livro', 'contains'),
    ('verne-en', 'collection', 'o livro e uma coletânea que o traz', 'contains'),
    ('verne-pt', 'verne-en', 'a tradução (nenhuma sequência de palavras em comum)', 'none'),
    ('holmes-adventures', 'holmes-study', 'dois livros do mesmo autor e dos mesmos personagens', 'none'),
    ('verne-en', 'holmes-adventures', 'livros sem relação', 'none'),
    ('verne-en-pdf', 'holmes-study', 'livros sem relação (um deles em PDF)', 'none'),
    ('verne-pt', 'holmes-adventures', 'livros sem relação em idiomas diferentes', 'none'),
]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__.split('\n')[0])
    parser.add_argument('--cache', type=Path, default=ROOT / 'tmp' / 'equivalence-benchmark')
    parser.add_argument('--json', type=Path, help='guarda cada par')
    args = parser.parse_args()
    args.cache.mkdir(parents=True, exist_ok=True)
    started = time.monotonic()
    files = corpus(args.cache)
    payload = json.dumps({'files': files, 'pairs': [{'a': a, 'b': b, 'kind': kind, 'expect': expect} for a, b, kind, expect in PAIRS]}).encode()
    process = subprocess.run(['go', 'run', './cmd/duplicates-benchmark'], cwd=ROOT / 'backend', input=payload, stdout=subprocess.PIPE, check=True)
    rows = [json.loads(line) for line in process.stdout.splitlines()]
    print(f'| par | o que é | amostra (A / B) | em comum | de A | de B | resultado | esperado |')
    print('| --- | --- | ---: | ---: | ---: | ---: | --- | --- |')
    wrong = 0
    for row in rows:
        ok = row['Verdict'] == row['Expect']
        wrong += not ok
        print(f"| {row['A']} × {row['B']} | {row['Kind']} | {row['hashesA']} / {row['hashesB']} | {row['shared']} | {100 * row['ofA']:.1f}% | "
              f"{100 * row['ofB']:.1f}% | {row['Verdict']}{'' if ok else ' ⚠'} | {row['Expect']} |")
    print(f"\n{len(rows)} pares, {wrong} fora do esperado ({time.monotonic() - started:.0f} s)")
    if args.json:
        args.json.write_text(json.dumps(rows, ensure_ascii=False, indent=1), encoding='utf-8')


if __name__ == '__main__':
    main()
