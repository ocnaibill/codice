#!/usr/bin/env python3
"""Reproducible evaluation of the OCR that reads scanned PDFs (#24, QA-007).

Measures the engine the worker uses, through the code the worker uses to turn a page into a picture, on pages made from
books of the public domain (see pages.py) with the text on each page known exactly. It answers what the specification
(15.3) asks, and approves no threshold: accuracy of the transcription, reading order, usefulness for search, and, for the
choices the owner is asked to make, what the language the engine is told costs and how well the language is told by
itself (DEC-104).

It does not run with `make test`. Run it where the engine is, which is the `ocr` image: `make benchmark-ocr`.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import random
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'worker'))
sys.path.insert(0, str(Path(__file__).resolve().parent))

import fitz  # noqa: E402

import metrics  # noqa: E402
import pages  # noqa: E402
from pages import Look  # noqa: E402

MAX_DOWNLOAD_BYTES = 10 * 1024 * 1024

# What the pages are made from. Fixed by hash: if Gutenberg changes a file the benchmark stops instead of quietly
# measuring something else. `code` is the language as the engine names it.
SOURCES = {
    'pt': {'url': 'https://www.gutenberg.org/cache/epub/55752/pg55752.txt', 'code': 'por',
           'sha256': '0fc3dbf384544d81d87e5a731e67b7976ac3a57acb378f0f354d12a3b52bd0c7', 'what': 'Dom Casmurro (Machado de Assis)'},
    'en': {'url': 'https://www.gutenberg.org/cache/epub/1661/pg1661.txt', 'code': 'eng',
           'sha256': '922e2a12ccb43a4c9544c260b2166c6ad2097aeb5957faeee113f173bb857cd0', 'what': 'The Adventures of Sherlock Holmes'},
    'es': {'url': 'https://www.gutenberg.org/cache/epub/2000/pg2000.txt', 'code': 'spa',
           'sha256': '534f41d59f7142163fa0964076ac6351845c006ea6433778be637fac6d5b04d7', 'what': 'Don Quijote'},
    'fr': {'url': 'https://www.gutenberg.org/cache/epub/4650/pg4650.txt', 'code': 'fra',
           'sha256': '1cc10ab3268a76aa01bd3f9722ae2ae0e5e36b869a18b0bb0832b75bc0327806', 'what': "Candide, ou l'optimisme"},
    'it': {'url': 'https://www.gutenberg.org/cache/epub/52484/pg52484.txt', 'code': 'ita',
           'sha256': 'b51a398da257709bfd2c0007e42980935751276cdca4178d738fa4c92f799ae6', 'what': 'Le avventure di Pinocchio'},
}

CLEAN = Look('limpa')
DIRTY = Look('suja', dpi=100, jpeg=30, speckles=500, paper=0.82, ink=0.4, skew=1.5)
# What changes about a page, one thing at a time, on the Portuguese book read with the owner's default and with
# Portuguese alone (what the automatic choice would pick).
LOOKS = [
    CLEAN,
    CLEAN.but(name='duas colunas', columns=2),
    CLEAN.but(name='sem serifa', font='sans'),
    CLEAN.but(name='itálico', font='italic'),
    CLEAN.but(name='fonte pequena (9 pt)', size=9),
    CLEAN.but(name='fonte grande (16 pt)', size=16),
    CLEAN.but(name='100 dpi', dpi=100),
    CLEAN.but(name='150 dpi', dpi=150),
    CLEAN.but(name='300 dpi', dpi=300),
    CLEAN.but(name='girada 90°', rotate=90),
    CLEAN.but(name='girada 180°', rotate=180),
    CLEAN.but(name='inclinada 3°', skew=3.0),
    CLEAN.but(name='manchas', speckles=600),
    CLEAN.but(name='papel e tinta fracos', paper=0.8, ink=0.45),
    CLEAN.but(name='JPEG 25', jpeg=25),
    CLEAN.but(name='cabeçalho e número', header=True),
    DIRTY,
    DIRTY.but(name='suja, duas colunas', columns=2),
]
DEFAULT, ALONE = 'por+eng', 'por'
# The language the engine is told, on the Portuguese, English and mixed books (clean and dirty).
LANGUAGES = ['por', 'por+eng', 'eng']
DETECT_LENGTHS = [40, 100, 300, 1000]
DETECT_SAMPLES = 25
OTHER = ['es', 'fr', 'it']


def digest(path: Path) -> str:
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()


def download(cache: Path, key: str, spec: dict) -> Path:
    target = cache / 'src' / f'{key}.txt'
    target.parent.mkdir(parents=True, exist_ok=True)
    if not target.exists() or digest(target) != spec['sha256']:
        print(f'baixando {key}...', file=sys.stderr)
        temporary = target.with_suffix('.part')
        request = urllib.request.Request(spec['url'], headers={'User-Agent': 'Codice-ocr-benchmark/1'})
        total = 0
        with urllib.request.urlopen(request, timeout=60) as response, temporary.open('wb') as out:
            while block := response.read(1024 * 1024):
                total += len(block)
                if total > MAX_DOWNLOAD_BYTES:
                    temporary.unlink(missing_ok=True)
                    raise SystemExit(f'{key}: o download ultrapassou 10 MiB')
                out.write(block)
        if digest(temporary) != spec['sha256']:
            temporary.unlink(missing_ok=True)
            raise SystemExit(f'{key}: o SHA-256 baixado não corresponde ao manifesto')
        temporary.replace(target)
    return target


def load_books(cache: Path) -> dict[str, list[str]]:
    books = {}
    for key, spec in SOURCES.items():
        text = download(cache, key, spec).read_text(encoding='utf-8-sig')
        books[key] = pages.paragraphs(pages.clean_source(text))
        if len(books[key]) < 100:
            raise SystemExit(f'{key}: poucos parágrafos ({len(books[key])}); o texto mudou?')
    return books


def seed_of(*parts) -> int:
    return int(hashlib.sha256('|'.join(map(str, parts)).encode()).hexdigest()[:12], 16)


def source_text(books: dict, lang: str, rng: random.Random) -> str:
    if lang == 'misto':
        return pages.alternate(books['pt'], books['en'], rng)
    return pages.excerpt(books[lang], rng)


def make_pages(books: dict, lang: str, look: Look, count: int, seed: int) -> list[tuple[fitz.Document, str, bytes]]:
    """`count` pages of the book in `lang` drawn with `look`: (the scan as the worker will open it, the truth, the pdf).
    The excerpt of page i is the same for every look, so that looks can be compared on the very same text."""
    out = []
    for i in range(count):
        rng = random.Random(seed_of(seed, lang, i))
        text = source_text(books, lang, rng)
        title = SOURCES.get(lang, SOURCES['pt'])['what']
        picture, truth = pages.draw(look, text, random.Random(seed_of(seed, lang, look.name, i)), title, i + 1)
        pdf = pages.scan([picture])
        out.append((fitz.open(stream=pdf, filetype='pdf'), truth, pdf))
    return out


def measure(engine, ocr, doc, truth: str, config: str) -> dict:
    image, dpi = ocr.render(doc[0])
    started = time.monotonic()
    got = engine.recognize(image, config, dpi)
    seconds = time.monotonic() - started
    coverage, order = metrics.reading_order(truth, got)
    return {
        'cer': metrics.cer(truth, got), 'wer': metrics.wer(truth, got),
        'search': metrics.search_recall(truth, got), 'search_accents': metrics.search_recall(truth, got, True),
        'order': order, 'coverage': coverage, 'seconds': seconds, 'dpi': dpi, 'read': got, 'truth_chars': len(truth),
    }


def run_matrix(engine, ocr, books, plan, count, seed, log=print) -> list[dict]:
    """plan: [(text language, look, [configs])]. Each page is drawn once and read with every config asked for."""
    rows, done = [], set()
    for lang, look, configs in plan:
        todo = [c for c in configs if (lang, look.name, c) not in done]
        if not todo:
            continue
        drawn = make_pages(books, lang, look, count, seed)
        for config in todo:
            done.add((lang, look.name, config))
            for i, (doc, truth, _pdf) in enumerate(drawn):
                row = measure(engine, ocr, doc, truth, config)
                row.update(text=lang, look=look.name, config=config, page=i)
                rows.append(row)
            log(f'  {lang:6} {look.name:24} {config:8} CER {100 * metrics.mean(r["cer"] for r in rows[-count:]):5.1f}%')
        for doc, _t, _p in drawn:
            doc.close()
    return rows


def default_plan(only: set[str] | None) -> list:
    plan = []
    for look in LOOKS:
        plan.append(('pt', look, [ALONE, DEFAULT]))
    for lang in ('en', 'misto'):
        for look in (CLEAN, DIRTY):
            plan.append((lang, look, LANGUAGES))
    for look in (CLEAN, DIRTY):
        plan.append(('pt', look, LANGUAGES))
    for lang in OTHER:
        for look in (CLEAN, DIRTY):
            plan.append((lang, look, [SOURCES[lang]['code'], DEFAULT]))
    if only:
        plan = [p for p in plan if p[1].name in only or p[0] in only]
    return plan


# --- telling the language ---

def detect_clean(books: dict, seed: int) -> list[dict]:
    """How often the detector of the library names the language of clean text of each length, with no OCR in between."""
    from textindex.language import detect
    out = []
    for lang in SOURCES:
        for length in DETECT_LENGTHS:
            right = wrong = unsure = 0
            for i in range(DETECT_SAMPLES):
                rng = random.Random(seed_of(seed, 'detect', lang, length, i))
                text = ' '.join(rng.choice(books[lang]).split())
                start = rng.randrange(0, max(1, len(text) - length))
                got = detect(text[start:start + length])
                if got is None:
                    unsure += 1
                elif got == lang:
                    right += 1
                else:
                    wrong += 1
            out.append({'language': lang, 'chars': length, 'right': right, 'wrong': wrong, 'unsure': unsure})
    return out


def detect_read(rows: list[dict]) -> list[dict]:
    """The same on what was read by OCR with the default language, from all the pages of a book and look (the worker
    reads three pages and tells the language from the text of the three together)."""
    from textindex.language import detect
    groups: dict[tuple, list[str]] = {}
    for r in rows:
        if r['config'] == DEFAULT and r['text'] in SOURCES:
            groups.setdefault((r['text'], r['look']), []).append(r['read'])
    out = []
    for (lang, look), texts in groups.items():
        out.append({'language': lang, 'look': look, 'detected': detect('\n'.join(texts))})
    return out


# --- pages of real scans, given by the maintainer ---

def load_real(folder: Path) -> list[tuple[str, bytes, str, int]]:
    """(name, picture, truth, dpi) for each image or one-page PDF in the folder that has a .txt of the same name."""
    out = []
    for path in sorted(folder.iterdir()):
        truth = path.with_suffix('.txt')
        if path.suffix.lower() in ('.png', '.jpg', '.jpeg', '.pdf') and truth.is_file():
            out.append((path.stem, path.read_bytes(), truth.read_text(encoding='utf-8'), path.suffix.lower()))
    return out


def run_real(engine, ocr, folder: Path, configs: list[str], dpi: int) -> list[dict]:
    rows = []
    for name, data, truth, suffix in load_real(folder):
        if suffix == '.pdf':
            doc = fitz.open(stream=data, filetype='pdf')
            image, real_dpi = ocr.render(doc[0])
            doc.close()
        else:
            image, real_dpi = data, dpi
        for config in configs:
            started = time.monotonic()
            got = engine.recognize(image, config, real_dpi)
            seconds = time.monotonic() - started
            coverage, order = metrics.reading_order(truth, got)
            rows.append({'text': 'real', 'look': name, 'config': config, 'page': 0, 'cer': metrics.cer(truth, got),
                         'wer': metrics.wer(truth, got), 'search': metrics.search_recall(truth, got),
                         'search_accents': metrics.search_recall(truth, got, True), 'order': order, 'coverage': coverage,
                         'seconds': seconds, 'dpi': real_dpi, 'read': got, 'truth_chars': len(truth)})
    return rows


# --- report ---

def pct(value, digits=1) -> str:
    return '—' if value is None else f'{100 * value:.{digits}f}'


def aggregate(rows: list[dict]) -> dict[tuple, dict]:
    groups: dict[tuple, list[dict]] = {}
    for r in rows:
        groups.setdefault((r['text'], r['look'], r['config']), []).append(r)
    return {
        key: {name: metrics.mean(r[name] for r in group)
              for name in ('cer', 'wer', 'search', 'search_accents', 'order', 'coverage', 'seconds')} | {'pages': len(group)}
        for key, group in groups.items()
    }


HEADER = '| {} | idioma lido | erro de letra % | erro de palavra % | busca sem acento % | busca com acento % | ordem % (achada %) | s/página |\n|---|---|---|---|---|---|---|---|'


def table(title: str, agg: dict, keys: list[tuple], first: str) -> str:
    lines = [f'### {title}', '', HEADER.format(first)]
    for key in keys:
        a = agg[key]
        lines.append(f'| {key[1] if first == "página" else key[0]} | `{key[2]}` | {pct(a["cer"])} | {pct(a["wer"])} | {pct(a["search"])} | '
                     f'{pct(a["search_accents"])} | {pct(a["order"], 0)} ({pct(a["coverage"], 0)}) | {a["seconds"]:.1f} |')
    return '\n'.join(lines) + '\n'


def report(engine, rows, clean_detect, read_detect, args) -> str:
    agg = aggregate(rows)
    out = [f'# Avaliação do OCR', '',
           f'Motor: {engine.name} {engine.version()} · {args.pages} página(s) por condição · semente {args.seed}. '
           'Texto de livros de domínio público desenhado em páginas só de imagem; a verdade é o que foi desenhado. '
           '**Nenhum limiar foi aprovado: os números descrevem, não aprovam.** '
           '`busca sem acento` é a fração das palavras distintas (4 letras ou mais) da página que estão no texto lido, '
           'sem diferenciar caixa nem acento (como a nossa busca); `com acento` exige a palavra escrita igual. '
           '`ordem` é a fração dos pares de trechos lidos na ordem certa, e entre parênteses quantos trechos foram achados.', '']
    pt = [k for k in agg if k[0] == 'pt' and k[1] in {l.name for l in LOOKS} and k[2] in (ALONE, DEFAULT)]
    order = {l.name: i for i, l in enumerate(LOOKS)}
    pt.sort(key=lambda k: (order[k[1]], k[2] != ALONE))
    if pt:
        out += [table('Como a página é (português, lido em `por` e em `por+eng`)', agg, pt, 'página'), '']
    mixes = sorted([k for k in agg if k[0] in ('pt', 'en', 'misto') and k[1] in ('limpa', 'suja') and k[2] in LANGUAGES],
                   key=lambda k: (['pt', 'en', 'misto'].index(k[0]), k[1] != 'limpa', LANGUAGES.index(k[2])))
    if mixes:
        out += ['### O idioma dito ao motor (livro em português, em inglês e misto; página limpa e suja)', '',
                HEADER.format('livro').replace('| livro |', '| livro / página |')]
        for k in mixes:
            a = agg[k]
            out.append(f'| {k[0]} / {k[1]} | `{k[2]}` | {pct(a["cer"])} | {pct(a["wer"])} | {pct(a["search"])} | {pct(a["search_accents"])} | '
                       f'{pct(a["order"], 0)} ({pct(a["coverage"], 0)}) | {a["seconds"]:.1f} |')
        out.append('')
    others = sorted([k for k in agg if k[0] in OTHER], key=lambda k: (OTHER.index(k[0]), k[1] != 'limpa', k[2] != SOURCES[k[0]]['code']))
    if others:
        out += ['### Outros idiomas: o idioma certo contra o padrão `por+eng`', '', HEADER.format('livro').replace('| livro |', '| livro / página |')]
        for k in others:
            a = agg[k]
            out.append(f'| {k[0]} / {k[1]} | `{k[2]}` | {pct(a["cer"])} | {pct(a["wer"])} | {pct(a["search"])} | {pct(a["search_accents"])} | '
                       f'{pct(a["order"], 0)} ({pct(a["coverage"], 0)}) | {a["seconds"]:.1f} |')
        out.append('')
    real = [k for k in agg if k[0] == 'real']
    if real:
        out += [table('Páginas reais (da pasta dada)', agg, real, 'página'), '']
    if clean_detect:
        out += ['### Descobrir o idioma pelo texto limpo (sem OCR)', '',
                f'{DETECT_SAMPLES} trechos de cada tamanho, de lugares sortidos do livro. Cada célula: certo / errado / sem resposta.', '',
                '| idioma | ' + ' | '.join(f'{n} caracteres' for n in DETECT_LENGTHS) + ' |', '|---|' + '---|' * len(DETECT_LENGTHS)]
        for lang in SOURCES:
            cells = []
            for n in DETECT_LENGTHS:
                r = next(x for x in clean_detect if x['language'] == lang and x['chars'] == n)
                cells.append(f'{r["right"]} / {r["wrong"]} / {r["unsure"]}')
            out.append(f'| {lang} | ' + ' | '.join(cells) + ' |')
        out.append('')
    if read_detect:
        out += ['### Descobrir o idioma pelo texto que o OCR leu com o padrão `por+eng`', '',
                'O que o worker faz: lê páginas com o padrão e diz o idioma do texto lido.', '', '| livro | página | descoberto | acertou |', '|---|---|---|---|']
        for r in sorted(read_detect, key=lambda r: (r['language'], r['look'])):
            ok = r['detected'] == r['language']
            out.append(f'| {r["language"]} | {r["look"]} | {r["detected"] or "sem resposta"} | {"sim" if ok else "não"} |')
        out.append('')
    return '\n'.join(out)


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split('\n')[0])
    ap.add_argument('--cache', type=Path, default=ROOT / 'tmp' / 'ocr-benchmark', help='where the texts are kept')
    ap.add_argument('--out', type=Path, help='where the report goes (default: the cache folder)')
    ap.add_argument('--pages', type=int, default=3, help='pages per condition')
    ap.add_argument('--seed', type=int, default=7)
    ap.add_argument('--only', help='comma-separated look names or text languages, to run a part')
    ap.add_argument('--real', type=Path, help='folder with pages of real scans: name.png|jpg|pdf and name.txt (the truth)')
    ap.add_argument('--real-dpi', type=int, default=200, help='resolution to tell the engine for real images')
    ap.add_argument('--real-languages', default=','.join(LANGUAGES))
    ap.add_argument('--no-detect', action='store_true')
    args = ap.parse_args(argv)

    import ocr
    engine = ocr.Tesseract()
    if not engine.installed():
        raise SystemExit('o motor (tesseract) não está instalado: rode na imagem `ocr` (make benchmark-ocr)')
    have = set(engine.languages())
    need = {p for c in LANGUAGES + [s['code'] for s in SOURCES.values()] for p in c.split('+')}
    if need - have:
        print(f'aviso: o motor não tem {sorted(need - have)}; as condições com esses idiomas serão puladas', file=sys.stderr)

    args.cache.mkdir(parents=True, exist_ok=True)
    out_dir = args.out or args.cache
    books = load_books(args.cache)
    plan = [p for p in default_plan(set(args.only.split(',')) if args.only else None)
            if all(set(c.split('+')) <= have for c in p[2])]
    print(f'{len(plan)} grupos de páginas, {args.pages} página(s) cada', file=sys.stderr)
    rows = run_matrix(engine, ocr, books, plan, args.pages, args.seed, log=lambda m: print(m, file=sys.stderr))
    if args.real:
        rows += run_real(engine, ocr, args.real, [c for c in args.real_languages.split(',') if set(c.split('+')) <= have], args.real_dpi)

    clean = [] if args.no_detect else detect_clean(books, args.seed)
    read = [] if args.no_detect else detect_read(rows)
    text = report(engine, rows, clean, read, args)
    out_dir.mkdir(parents=True, exist_ok=True)
    (out_dir / 'report.md').write_text(text, encoding='utf-8')
    slim = [{k: v for k, v in r.items() if k != 'read'} for r in rows]
    (out_dir / 'results.json').write_text(json.dumps({'engine': engine.version(), 'rows': slim, 'detect_clean': clean,
                                                      'detect_read': read}, ensure_ascii=False, indent=1), encoding='utf-8')
    print(text)
    print(f'\nrelatório em {out_dir / "report.md"}', file=sys.stderr)
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
