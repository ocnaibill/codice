import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, it, expect } from 'vitest';

// The tests run from the folder of the frontend.
const read = (path) => readFileSync(resolve(process.cwd(), path), 'utf8');
const favicon = read('public/favicon.svg');
const icons = read('src/components/ui/LibraryIcon.jsx');
const index = read('index.html');

// The path of the book of the library icon, as the menu draws it.
const bookPath = icons.match(/book: \(\s*<>\s*<path d="([^"]+)"/)?.[1];

describe('the favicon of the app', () => {
  it('draws the book of the library icon, the same path as the menu', () => {
    expect(bookPath).toMatch(/^M12 5v15/);
    expect(favicon).toContain(`d="${bookPath}"`);
  });

  it('is the icon the page points to', () => {
    expect(index).toContain('<link rel="icon" type="image/svg+xml" href="/favicon.svg" />');
  });

  it('is a brand tile with a cream book, and swaps them when the browser is dark', () => {
    const [light, dark] = favicon.split('@media (prefers-color-scheme: dark)');
    expect(dark).toBeTruthy();
    expect(light).toContain('.tile { fill: #944516; }');
    expect(light).toContain('.book { stroke: #fdf6ec; }');
    expect(dark).toContain('.tile { fill: #fdf6ec; }');
    expect(dark).toContain('.book { stroke: #944516; }');
  });

  it('keeps the colors in the style, not on the shapes, so that the dark mode can change them', () => {
    expect(favicon).not.toMatch(/<rect[^>]*\sfill=/);
    expect(favicon).not.toMatch(/<g[^>]*\sstroke="/);
    expect(favicon).toContain('class="tile"');
    expect(favicon).toContain('class="book"');
  });
});
