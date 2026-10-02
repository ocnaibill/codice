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
import hashlib
import json
import re
import subprocess
import urllib.request
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'worker'))
sys.path.insert(0, str(ROOT / 'benchmarks' / 'equivalence'))

import fitz  # noqa: E402

import benchmark as books  # noqa: E402  (the equivalence benchmark: the downloads and the EPUB reader)
from textindex.pdf import pdf_segments  # noqa: E402
from textindex.plain import plain_segments  # noqa: E402
from textindex.normalize import clean  # noqa: E402


# Texts for the pairs that are translations of one another, read against each other. Fixed by hash, like the rest.
TEXTS = {
    'candide-fr': ('https://www.gutenberg.org/cache/epub/4650/pg4650.txt', '1cc10ab3268a76aa01bd3f9722ae2ae0e5e36b869a18b0bb0832b75bc0327806'),
    'candide-en': ('https://www.gutenberg.org/cache/epub/19942/pg19942.txt', 'fb3943df826f375c4ab78d21f2d30bc95a0b2cb448e8812447a807d678eb0b5c'),
    'pinocchio-it': ('https://www.gutenberg.org/cache/epub/52484/pg52484.txt', 'b51a398da257709bfd2c0007e42980935751276cdca4178d738fa4c92f799ae6'),
    'pinocchio-en': ('https://www.gutenberg.org/cache/epub/500/pg500.txt', '072a84b3da860036ebf81dbd305149ef0d908064c30d513c7f640599fabeb99b'),
    'quijote-es': ('https://www.gutenberg.org/cache/epub/2000/pg2000.txt', '534f41d59f7142163fa0964076ac6351845c006ea6433778be637fac6d5b04d7'),
    'quijote-en': ('https://www.gutenberg.org/cache/epub/996/pg996.txt', '2864143e9addf498c1c5fd585db4a1ff3e06651c344cddf0931342f1a0ce24d4'),
}
MAX_DOWNLOAD_BYTES = 10 * 1024 * 1024


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def download_text(cache: Path, key: str) -> Path:
    url, sha = TEXTS[key]
    target = cache / 'txt' / f'{key}.txt'
    target.parent.mkdir(parents=True, exist_ok=True)
    if not target.exists() or digest(target) != sha:
        print(f'baixando {key}...', file=sys.stderr)
        request = urllib.request.Request(url, headers={'User-Agent': 'Codice-duplicates-benchmark/1'})
        with urllib.request.urlopen(request, timeout=60) as response:
            data = response.read(MAX_DOWNLOAD_BYTES + 1)
        if len(data) > MAX_DOWNLOAD_BYTES:
            raise SystemExit(f'{key}: o download ultrapassou 10 MiB')
        if hashlib.sha256(data).hexdigest() != sha:
            raise SystemExit(f'{key}: o SHA-256 baixado não corresponde ao manifesto')
        target.write_bytes(data)
    return target


def book_of(path: Path, name: str, cache: Path) -> dict:
    """A Project Gutenberg text, without its licence, as the segments the worker would make of it (no outline)."""
    text = path.read_text(encoding='utf-8-sig')
    start, end = re.search(r'\*\*\* ?START OF (?:THE|THIS) PROJECT GUTENBERG EBOOK[^\n]*\n', text), re.search(r'\*\*\* ?END OF (?:THE|THIS) PROJECT GUTENBERG EBOOK', text)
    body = text[start.end() if start else 0:end.start() if end else len(text)]
    clean_path = cache / 'txt' / f'{name}.body.txt'
    clean_path.write_text(body, encoding='utf-8')
    return {'Segments': [{'ID': i + 1, 'Sequence': i, 'Section': '', 'Text': seg.text, 'Locator': {'type': 'text', 'offset': i}, 'Node': -1, 'Part': ''}
                         for i, seg in enumerate(plain_segments(str(clean_path)))], 'Nodes': []}


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
    # The files that are read against each other: the same segments the worker would publish.
    segmented = {key: {'Segments': [{k: v for k, v in seg.items() if not k.startswith('_')} for seg in raw[key]['Segments']], 'Nodes': raw[key]['Nodes']}
                 for key in ('verne-pt', 'verne-en', 'verne-en-illustrated', 'holmes-adventures', 'holmes-study')}
    for key in TEXTS:
        segmented[key] = book_of(download_text(cache, key), key, cache)
    return files, segmented


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


PARALLEL = [
    ('verne-pt', 'verne-en', 'Da Terra à Lua: português e inglês', 'translation'),
    ('verne-pt', 'verne-en-illustrated', 'Da Terra à Lua: português e a edição que traz também a continuação', 'translation'),
    ('candide-fr', 'candide-en', 'Candide: francês e inglês', 'translation'),
    ('pinocchio-it', 'pinocchio-en', 'Pinóquio: italiano e inglês (poucos nomes, e os que há são traduzidos)', 'translation', 'none'),
    ('quijote-es', 'quijote-en', 'Dom Quixote: espanhol e inglês (dois milhões de caracteres)', 'translation'),
    ('holmes-adventures', 'holmes-study', 'dois livros do mesmo autor e dos mesmos personagens', 'none'),
    ('verne-pt', 'candide-fr', 'livros sem relação (português e francês)', 'none'),
    ('verne-en', 'candide-en', 'livros sem relação (os dois em inglês)', 'none'),
    ('pinocchio-it', 'quijote-es', 'livros sem relação (italiano e espanhol)', 'none'),
    ('candide-en', 'pinocchio-en', 'livros sem relação (os dois em inglês, curtos)', 'none'),
    ('quijote-en', 'holmes-adventures', 'livros sem relação (um deles enorme)', 'none'),
    ('verne-pt', 'holmes-adventures', 'livros sem relação em idiomas diferentes', 'none'),
    ('pinocchio-en', 'candide-fr', 'livros sem relação em idiomas diferentes', 'none'),
]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__.split('\n')[0])
    parser.add_argument('--cache', type=Path, default=ROOT / 'tmp' / 'equivalence-benchmark')
    parser.add_argument('--json', type=Path, help='guarda cada par')
    args = parser.parse_args()
    args.cache.mkdir(parents=True, exist_ok=True)
    started = time.monotonic()
    files, books_ = corpus(args.cache)
    payload = json.dumps({'files': files, 'pairs': [{'a': a, 'b': b, 'kind': kind, 'expect': expect} for a, b, kind, expect in PAIRS],
                          'books': books_, 'parallel': [{'a': a, 'b': b, 'kind': kind, 'expect': expect} for a, b, kind, expect, *_ in PARALLEL]}).encode()
    process = subprocess.run(['go', 'run', './cmd/duplicates-benchmark'], cwd=ROOT / 'backend', input=payload, stdout=subprocess.PIPE, check=True)
    rows = [json.loads(line) for line in process.stdout.splitlines()]
    text_rows = [r for r in rows if r['mode'] == 'text']
    read_rows = [r for r in rows if r['mode'] == 'parallel']
    print('### O mesmo texto (impressão digital)\n')
    print('| par | o que é | amostra (A / B) | em comum | de A | de B | resultado | esperado |')
    print('| --- | --- | ---: | ---: | ---: | ---: | --- | --- |')
    wrong = 0
    for row in text_rows:
        ok = row['Verdict'] == row['Expect']
        wrong += not ok
        print(f"| {row['A']} × {row['B']} | {row['Kind']} | {row['hashesA']} / {row['hashesB']} | {row['shared']} | {100 * row['ofA']:.1f}% | "
              f"{100 * row['ofB']:.1f}% | {row['Verdict']}{'' if ok else ' ⚠'} | {row['Expect']} |")
    print('\n### Tradução (um lido contra o outro, de ponta a ponta)\n')
    print('| par | o que é | trechos achados | em ordem | resultado | esperado | ms |')
    print('| --- | --- | ---: | ---: | --- | --- | ---: |')
    known = {(a, b): result for a, b, _kind, _expect, *rest in PARALLEL for result in rest}  # what a known limit is read as
    for row in read_rows:
        limit = known.get((row['A'], row['B']))
        ok = row['Verdict'] == row['Expect'] or row['Verdict'] == limit
        wrong += not ok
        order = '—' if row['order'] < 0 else f"{100 * row['order']:.0f}%"
        mark = '' if row['Verdict'] == row['Expect'] else (' (limite conhecido)' if ok else ' ⚠')
        print(f"| {row['A']} × {row['B']} | {row['Kind']} | {row['hits']} de {row['samples']} ({100 * row['share']:.0f}%) | {order} | "
              f"{row['Verdict']}{mark} | {row['Expect']} | {row['latencyMillis']} |")
    print(f"\n{len(rows)} pares, {wrong} fora do esperado ({time.monotonic() - started:.0f} s)")
    if args.json:
        args.json.write_text(json.dumps(rows, ensure_ascii=False, indent=1), encoding='utf-8')


if __name__ == '__main__':
    main()
