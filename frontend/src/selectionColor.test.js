import { readFileSync } from 'node:fs';
import { describe, it, expect } from 'vitest';

const css = readFileSync('src/index.css', 'utf8');

describe('the color of a selected text', () => {
  it('is the brand terracotta, in the whole app, and not the blue of the browser', () => {
    const rule = css.match(/(^|\n)::selection\s*\{([^}]*)\}/);
    expect(rule, 'a ::selection rule for the whole page').not.toBeNull();
    expect(rule[2]).toContain('var(--color-brand)');
    expect(rule[2]).toMatch(/background-color:\s*color-mix\(in srgb, var\(--color-brand\) 30%, transparent\)/);
    expect(css).toContain('--color-brand: #944516;');
  });

  it('leaves the color of the text alone', () => {
    const rule = css.match(/(^|\n)::selection\s*\{([^}]*)\}/)[2];
    expect(rule).not.toMatch(/(^|[;\s])color\s*:/);
  });
});
