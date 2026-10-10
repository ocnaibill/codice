import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, it, expect } from 'vitest';

// What the CSS of the home says, which a test in jsdom cannot lay out: the places are in the file, and a change of them is a change of the page.
const css = readFileSync(resolve(process.cwd(), 'src/components/layout/library-shell.css'), 'utf8');
const rule = (selector) => {
  const start = css.indexOf(`${selector} {`);
  return start < 0 ? '' : css.slice(start, css.indexOf('}', start));
};

describe('the layout of the home', () => {
  it('puts the reading and the favorites side by side, the catalog across the whole width, and the notes under it', () => {
    const columns = rule('.home-columns');
    expect(columns).toContain("'continue favorites'");
    expect(columns).toContain("'catalog catalog'");
    expect(columns).toContain("'notes notes'");
  });

  it('makes the cards of a shelf as tall as the tallest of them', () => {
    expect(rule('.library-carousel-item')).toContain('display: flex');
    expect(rule('.library-carousel-item > *')).toContain('flex: 1 1 auto');
  });

  it('has a greeting that does not take the width of the cards beside it', () => {
    const size = rule('.home-greeting h1').match(/clamp\((\d+)px, [\d.]+vw, (\d+)px\)/);
    expect(Number(size[2])).toBeLessThanOrEqual(32);
    expect(rule('.home-greeting')).toContain('min-width: 0');
  });
});
