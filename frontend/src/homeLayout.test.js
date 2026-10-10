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
  it('puts the reading and, beside it, the favorites with the notes right under them, and the catalog across the whole width under both', () => {
    const columns = rule('.home-columns');
    expect(columns).toContain("'continue rail'");
    expect(columns).toContain("'catalog catalog'");
    expect(columns).not.toContain("'notes");
    // the rail is one column that holds the favorites and the notes, in the width of the column
    const rail = rule('.home-rail');
    expect(rail).toContain('grid-area: rail');
    expect(rail).toContain('flex-direction: column');
    // the notes are one under the other, not two side by side, in a column that narrow
    expect(rule('.home-notes-list')).toContain('grid-template-columns: minmax(0, 1fr)');
    expect(css).not.toMatch(/\.home-notes-list \{\s*grid-template-columns: repeat\(2/);
  });

  it('is one column when there is no room: the reading, the favorites and the notes, then the catalog', () => {
    const narrow = css.slice(css.indexOf('@media (max-width: 1199px)'));
    const columns = narrow.slice(narrow.indexOf('.home-columns'), narrow.indexOf('}', narrow.indexOf('.home-columns')));
    expect(columns).toContain("'continue'");
    expect(columns.indexOf("'continue'")).toBeLessThan(columns.indexOf("'rail'"));
    expect(columns.indexOf("'rail'")).toBeLessThan(columns.indexOf("'catalog'"));
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
