#!/usr/bin/env python3
"""Reproducible public-domain benchmark for equivalent-position matching (#32)."""
from __future__ import annotations

import argparse
import collections
import hashlib
import json
import re
import resource
import subprocess
import sys
import time
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'worker'))

from textindex.epub import epub_segments  # noqa: E402


BOOKS = {
    'verne-pt': {
        'url': 'https://www.gutenberg.org/cache/epub/28341/pg28341.epub',
        'sha256': 'd97040e38390882837f922464dc96c843a5a9ec6b6eaefd25b01a86cf1cf49db',
        'language': 'pt',
    },
    'verne-en-illustrated': {
        'url': 'https://www.gutenberg.org/cache/epub/44278/pg44278.epub',
        'sha256': '7e8b9951938ea099d3999a2ff4dda9b86bb36b0080fb8f1e35c86c299f34d707',
        'language': 'en',
    },
    'verne-en': {
        'url': 'https://www.gutenberg.org/cache/epub/83/pg83.epub',
        'sha256': '558c7ab736bc5f6d698c65c1d9a7efd6404f22bc5883615233aa7190ebf71a53',
        'language': 'en',
    },
    'holmes-adventures': {
        'url': 'https://www.gutenberg.org/cache/epub/1661/pg1661.epub',
        'sha256': '209a8fd827fa88a11f04253d5af10d47967eaa1df3a5c74120ba9aa5e855bba6',
        'language': 'en',
    },
    'holmes-study': {
        'url': 'https://www.gutenberg.org/cache/epub/244/pg244.epub',
        'sha256': '76df519d3ce04fb50e2d2502328c62decb945e7c20a99d8cbd69e97fe707b252',
        'language': 'en',
    },
}

MODELS = {
    'e5-small': 'intfloat/multilingual-e5-small',
    'labse': 'sentence-transformers/LaBSE',
}
NUMBER = re.compile(r'(?<!\w)\d[\d.,:]*\d(?!\w)')
ROMAN = re.compile(r'\b(?:CAPITULO|CHAPTER)\s+([IVXLCDM]+|\d+)\b', re.I)
ROMAN_VALUES = {'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
MAX_DOWNLOAD_BYTES = 10 * 1024 * 1024


def roman_number(value: str) -> int:
    if value.isdigit():
        return int(value)
    total = previous = 0
    for char in reversed(value.upper()):
        current = ROMAN_VALUES[char]
        total += -current if current < previous else current
        previous = max(previous, current)
    return total


def download(cache: Path, name: str, spec: dict) -> Path:
    target = cache / f'{name}.epub'
    if not target.exists() or digest(target) != spec['sha256']:
        print(f'baixando {name}...', file=sys.stderr)
        temporary = target.with_suffix('.part')
        request = urllib.request.Request(spec['url'], headers={'User-Agent': 'Codice-equivalence-benchmark/1'})
        total, too_large = 0, False
        with urllib.request.urlopen(request, timeout=60) as response, temporary.open('wb') as output:
            while block := response.read(1024 * 1024):
                total += len(block)
                if total > MAX_DOWNLOAD_BYTES:
                    too_large = True
                    break
                output.write(block)
        if too_large:
            temporary.unlink(missing_ok=True)
            raise SystemExit(f'{name}: o download ultrapassou 10 MiB')
        if digest(temporary) != spec['sha256']:
            temporary.unlink(missing_ok=True)
            raise SystemExit(f'{name}: o SHA-256 baixado não corresponde ao manifesto')
        temporary.replace(target)
    return target


def digest(path: Path) -> str:
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()


def chapter_by_node(structure: list[dict]) -> dict[int, int]:
    chapter = None
    result = {}
    seen = set()
    for index, node in enumerate(structure):
        match = ROMAN.search(node['title'])
        if match:
            candidate = roman_number(match.group(1))
            # The two English files also contain the sequel, whose numbering restarts at I.
            if candidate == 1 and seen:
                chapter = None
            elif candidate not in seen and 1 <= candidate <= 28:
                chapter = candidate
                seen.add(candidate)
        if chapter is not None:
            result[index] = chapter
    return result


def extract(path: Path) -> dict:
    metadata = {}
    raw = list(epub_segments(path, out=metadata))
    structure = metadata.get('structure', [])
    chapters = chapter_by_node(structure)
    segments = []
    for sequence, item in enumerate(raw):
        part = structure[item.node]['part'] if item.node is not None and structure else ''
        segments.append({
            'ID': sequence + 1,
            'Sequence': sequence,
            'Section': item.section or '',
            'Text': item.text,
            'Locator': {'type': 'text', 'offset': sequence},
            'Node': item.node if item.node is not None else -1,
            'Part': part,
            '_chapter': chapters.get(item.node),
        })
    return {'Segments': segments, 'Nodes': structure}


def number_index(book: dict) -> dict[tuple[int, str], list[int]]:
    found = collections.defaultdict(list)
    for segment in book['Segments']:
        chapter = segment['_chapter']
        if chapter is None:
            continue
        for token in set(NUMBER.findall(segment['Text'])):
            digits = re.sub(r'\D', '', token)
            if len(digits) >= 2:
                found[(chapter, digits)].append(segment['Sequence'])
    return found


def positive_cases(files: dict) -> list[dict]:
    cases = []
    directions = [
        ('verne-pt', 'verne-en-illustrated', 'idiomas diferentes'),
        ('verne-en-illustrated', 'verne-pt', 'idiomas diferentes'),
        ('verne-pt', 'verne-en', 'idiomas diferentes'),
        ('verne-en', 'verne-pt', 'idiomas diferentes'),
        ('verne-en', 'verne-en-illustrated', 'mesmo idioma'),
        ('verne-en-illustrated', 'verne-en', 'mesmo idioma'),
    ]
    indexes = {name: number_index(files[name]) for name in {item for pair in directions for item in pair[:2]}}
    for source, destination, suite in directions:
        pairs = set()
        for key, left in indexes[source].items():
            right = indexes[destination].get(key, [])
            if len(left) == len(right) == 1:
                pairs.add((left[0], right[0]))
        for source_sequence, expected in sorted(pairs):
            cases.append({'suite': suite, 'source': source, 'destination': destination,
                          'sourceSequence': source_sequence, 'expectedSequence': expected})
    return cases


def negative_cases(files: dict) -> list[dict]:
    cases = []
    for source, destination in [('holmes-adventures', 'holmes-study'), ('holmes-study', 'holmes-adventures')]:
        body = [segment for segment in files[source]['Segments'] if segment['Part'] == 'body' and len(segment['Text']) >= 500]
        for segment in body[::max(1, len(body) // 30)][:30]:
            cases.append({'suite': 'livros irmãos negativos', 'source': source, 'destination': destination,
                          'sourceSequence': segment['Sequence'], 'expectedSequence': None})
    return cases


def mask_truth(files: dict) -> None:
    for book in files.values():
        for segment in book['Segments']:
            segment['Text'] = NUMBER.sub('', segment['Text'])
            segment.pop('_chapter', None)


def add_embeddings(files: dict, model_key: str) -> None:
    try:
        from sentence_transformers import SentenceTransformer
    except ImportError as error:
        raise SystemExit('instale worker/requirements-embeddings.txt para medir modelos') from error
    model_name = MODELS[model_key]
    model = SentenceTransformer(model_name, device='cpu')
    for name, book in files.items():
        texts = [segment['Text'] for segment in book['Segments']]
        if model_key == 'e5-small':
            texts = [f'query: {text}' for text in texts]
        vectors = model.encode(texts, batch_size=32, normalize_embeddings=True, show_progress_bar=False)
        for segment, vector in zip(book['Segments'], vectors):
            segment.update({'Embedding': vector.tolist(), 'EmbeddingProvider': 'sentence-transformers',
                            'EmbeddingModel': model_name, 'EmbeddingRevision': 'default',
                            'EmbeddingPreprocessing': 1})


def run_engine(files: dict, cases: list[dict]) -> list[dict]:
    payload = json.dumps({'files': files, 'cases': cases}, ensure_ascii=False).encode()
    process = subprocess.run(['go', 'run', './cmd/equivalence-benchmark'], cwd=ROOT / 'backend', input=payload,
                             stdout=subprocess.PIPE, check=True)
    return [json.loads(line) for line in process.stdout.splitlines()]


def report(profile: str, cases: list[dict], rows: list[dict], elapsed: float) -> None:
    rss = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    # macOS reports bytes; Linux and the other supported Unix hosts report KiB.
    rss_mib = rss / (1024 * 1024) if sys.platform == 'darwin' else rss / 1024
    print(f'\n## {profile} ({elapsed:.1f} s; pico do processo Python {rss_mib:.0f} MiB)')
    print('| conjunto | tentativas | ofertas | cobertura | precisão ≤1 | efetivo ≤1 | erros/ofertas indevidas | latência p50 |')
    print('| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |')
    case_by_suite = collections.Counter(case['suite'] for case in cases)
    for suite in case_by_suite:
        sample = [row for row in rows if row['suite'] == suite]
        total = len(sample)
        offered = [row for row in sample if row['status'] != 'not_found']
        if suite == 'livros irmãos negativos':
            correct = []
            wrong = len(offered)
        else:
            correct = [row for row in offered if row.get('distance', 10**9) <= 1]
            wrong = len(offered) - len(correct)
        latency = sorted(row['latencyMicros'] for row in sample)[total // 2] if total else 0
        precision = f'{len(correct) / len(offered):.1%}' if offered and suite != 'livros irmãos negativos' else '—'
        effective = f'{len(correct) / total:.1%}' if suite != 'livros irmãos negativos' else '—'
        print(f'| {suite} | {total} | {len(offered)} | {len(offered)/total:.1%} | '
              f'{precision} | {effective} | {wrong} | {latency / 1000:.2f} ms |')

    print('\n| método | confiança | ofertas | precisão ≤1 | erradas |')
    print('| --- | --- | ---: | ---: | ---: |')
    positive_suites = {'idiomas diferentes', 'mesmo idioma'}
    offered = [row for row in rows if row['suite'] in positive_suites and row['status'] != 'not_found']
    groups = collections.defaultdict(list)
    for row in offered:
        groups[(row.get('method', 'unknown'), row.get('confidence', 'unknown'))].append(row)
    for (method, confidence), sample in sorted(groups.items()):
        correct = sum(row.get('distance', 10**9) <= 1 for row in sample)
        print(f'| {method} | {confidence} | {len(sample)} | {correct / len(sample):.1%} | {len(sample) - correct} |')


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument('--profile', choices=['deterministic', *MODELS], action='append',
                        help='perfil a medir; pode ser repetido (padrão: deterministic)')
    parser.add_argument('--cache', type=Path, default=ROOT / 'tmp' / 'equivalence-benchmark')
    parser.add_argument('--json', type=Path, help='também grava os resultados completos em JSON')
    args = parser.parse_args()
    profiles = args.profile or ['deterministic']
    args.cache.mkdir(parents=True, exist_ok=True)
    paths = {name: download(args.cache, name, spec) for name, spec in BOOKS.items()}
    pristine = {name: extract(path) for name, path in paths.items()}
    cases = positive_cases(pristine) + negative_cases(pristine)
    mask_truth(pristine)
    complete = {}
    for profile in profiles:
        files = json.loads(json.dumps(pristine))
        started = time.monotonic()
        if profile != 'deterministic':
            add_embeddings(files, profile)
        rows = run_engine(files, cases)
        elapsed = time.monotonic() - started
        report(profile, cases, rows, elapsed)
        complete[profile] = rows
    if args.json:
        args.json.parent.mkdir(parents=True, exist_ok=True)
        args.json.write_text(json.dumps({'cases': cases, 'profiles': complete}, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    main()
