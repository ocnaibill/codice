import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, it, expect } from 'vitest';

// What is read from a file must not run as code on the origin of the app: a book that carries a script could read the token of the
// session (localStorage) or ask the API as the person. These are the rules of the source that keep it so.
const walk = (dir) => readdirSync(dir).flatMap((name) => {
  const path = join(dir, name);
  return statSync(path).isDirectory() ? walk(path) : [path];
});
const SOURCES = walk('src').filter((f) => /\.jsx?$/.test(f) && !/\.test\.jsx?$/.test(f));
const read = (file) => readFileSync(file, 'utf8');

describe('what a file can do on the origin of the app', () => {
  it('never allows scripts in the page of a book, nor in anything else that renders content of a file', () => {
    const found = SOURCES.flatMap((f) => [...read(f).matchAll(/allowScriptedContent\s*:\s*([^,}\s]+)/g)].filter((m) => m[1] !== 'false').map((m) => `${f}: ${m[0]}`));
    expect(found).toEqual([]);
  });

  it('never turns the text of a file into HTML the browser would run', () => {
    const found = SOURCES.filter((f) => /dangerouslySetInnerHTML|rehype-raw|rehypeRaw|allowDangerousHtml|\.innerHTML\s*=|srcdoc\s*=/.test(read(f)));
    expect(found).toEqual([]);
  });

  it('keeps the reader of EPUB with the scripts off, in the place where it opens the book', () => {
    const viewer = read('src/features/reader/components/viewers/EpubViewer.jsx');
    expect(viewer).toMatch(/book\.renderTo\([\s\S]*?allowScriptedContent:\s*false/);
  });
});
