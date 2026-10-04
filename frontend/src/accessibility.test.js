import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, it, expect } from 'vitest';

// The accessibility of what is decided in the source and not on one screen: the language and name of the page, the colors the
// theme lets a text sit on, where the keyboard is, and the system's request for less motion. (What is on a screen is checked by
// the tests of that screen.)
const css = readFileSync(join('src', 'index.css'), 'utf8');
const html = readFileSync('index.html', 'utf8');

const token = (name) => {
  const m = css.match(new RegExp(`--color-${name}:\\s*(#[0-9a-fA-F]{6})`));
  if (!m) throw new Error(`no token --color-${name}`);
  return m[1];
};
const WHITE = '#ffffff';

const luminance = (hex) => {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
    .map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const contrast = (a, b) => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
};
const colorOf = (name) => (name === 'white' ? WHITE : token(name));

describe('the page', () => {
  it('says its language and has a name, not the defaults of the template', () => {
    expect(html).toMatch(/<html[^>]*\blang="pt-BR"/);
    expect(html).toMatch(/<title>Códice<\/title>/);
  });
});

describe('the contrast of the colors of the theme (WCAG 2.2 AA)', () => {
  // [text, background]: 4.5:1 for text.
  const TEXT_ON = [
    ...['ink', 'ink-soft', 'ink-faint', 'ink-warm', 'brand', 'success', 'danger', 'warning'].flatMap((ink) =>
      ['white', 'surface', 'surface-alt'].map((bg) => [ink, bg])),
    ['success', 'success-soft'], ['danger', 'danger-soft'], ['warning', 'warning-soft'],
    ['ink', 'info-soft'], ['brand', 'info-soft'], ['ink-faint', 'info-soft'], ['ink-faint', 'success-soft'],
    // the labels of the buttons
    ['white', 'brand'], ['white', 'brand-light'], ['white', 'success'], ['white', 'danger'],
  ];
  it.each(TEXT_ON)('%s text on %s is at least 4.5:1', (ink, bg) => {
    expect(contrast(colorOf(ink), colorOf(bg)), `${ink} on ${bg}`).toBeGreaterThanOrEqual(4.5);
  });

  it('keeps the faint ink lighter than the soft ink (the hierarchy of the texts survives the contrast)', () => {
    expect(luminance(token('ink-faint'))).toBeGreaterThan(luminance(token('ink-soft')));
    expect(luminance(token('ink-soft'))).toBeGreaterThan(luminance(token('ink')));
  });
});

describe('where the keyboard is', () => {
  // The rule has to be outside of any @layer: the utilities of the framework are in a layer (outline-none among them), and a
  // rule outside of every layer wins over a layer whatever the specificity.
  const outsideLayers = css.replace(/@theme\s*\{(?:[^{}]|\{[^{}]*\})*\}/g, '').replace(/@layer[^{]*\{(?:[^{}]|\{[^{}]*\})*\}/g, '');

  it('draws a ring on whatever the keyboard focuses, over the fields that turned the outline off', () => {
    expect(outsideLayers).toMatch(/:where\([^)]*\bbutton\b[^)]*\bselect\b[^)]*\):focus-visible\s*\{[^}]*outline:\s*2px solid var\(--color-brand\)/);
    expect(outsideLayers).toMatch(/:focus-visible\s*\{[^}]*outline-offset:\s*2px/);
  });

  it('covers links, buttons, fields, summaries and anything with a tabindex', () => {
    const rule = outsideLayers.match(/:where\(([^)]*)\):focus-visible/)?.[1] ?? '';
    for (const el of ['a', 'button', 'input', 'select', 'textarea', 'summary', '[tabindex]']) {
      expect(rule.split(',').map((s) => s.trim()), el).toContain(el);
    }
  });

  it('paints the placeholder with the faint ink, solid, and not with the half-transparent default', () => {
    expect(outsideLayers).toMatch(/::placeholder\s*\{[^}]*color:\s*var\(--color-ink-faint\)[^}]*opacity:\s*1/);
  });
});

describe('motion', () => {
  it('stops every animation and transition for who asks the system for less motion', () => {
    const block = css.match(/@media \(prefers-reduced-motion: reduce\)\s*\{([\s\S]*?)\n\}/)?.[1] ?? '';
    expect(block).toMatch(/animation-duration:\s*0\.01ms\s*!important/);
    expect(block).toMatch(/transition-duration:\s*0\.01ms\s*!important/);
    expect(block).toMatch(/scroll-behavior:\s*auto\s*!important/);
  });
});
