import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, it, expect } from 'vitest';

// The accessibility of what is decided in the source and not on one screen: the language and name of the page, the colors the
// theme lets a text sit on, where the keyboard is, and the system's request for less motion. (What is on a screen is checked by
// the tests of that screen.)
const css = readFileSync(join('src', 'index.css'), 'utf8');
const shellCss = readFileSync(join('src', 'components', 'layout', 'library-shell.css'), 'utf8');
const html = readFileSync('index.html', 'utf8');

// The app has two palettes: the one of the theme (index.css), and the warmer one that the shell of the library puts over
// it (library-shell.css, `.library-shell { --color-...: ... }`): it overrides some of the tokens, and the rest it inherits.
const tokensOf = (source) => Object.fromEntries([...source.matchAll(/--(color-[a-z-]+|library-paper):\s*(#[0-9a-fA-F]{6})/g)].map((m) => [m[1], m[2]]));
const THEME = tokensOf(css);
const SHELL_BLOCK = shellCss.match(/\.library-shell\s*\{([^}]*)\}/)?.[1] ?? '';
const SHELL = { ...THEME, ...tokensOf(SHELL_BLOCK) };
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
const colorOf = (palette, name) => {
  if (name === 'white') return WHITE;
  const value = palette[`color-${name}`] ?? palette[name];
  if (!value) throw new Error(`no token ${name}`);
  return value;
};
const token = (name) => colorOf(THEME, name);

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
  const SHELL_EXTRA = [['ink-faint', 'library-paper'], ['ink-soft', 'library-paper'], ['ink', 'library-paper'], ['brand', 'library-paper']];

  for (const [name, palette, pairs] of [['theme', THEME, TEXT_ON], ['library shell', SHELL, [...TEXT_ON, ...SHELL_EXTRA]]]) {
    it.each(pairs)(`${name}: %s text on %s is at least 4.5:1`, (ink, bg) => {
      expect(contrast(colorOf(palette, ink), colorOf(palette, bg)), `${ink} on ${bg}`).toBeGreaterThanOrEqual(4.5);
    });
  }

  it('reads the palette of the shell from the stylesheet (it overrides the faint ink and the surfaces)', () => {
    expect(SHELL['color-ink-faint']).toMatch(/^#[0-9a-f]{6}$/i);
    expect(SHELL['color-surface-alt']).not.toBe(THEME['color-surface-alt']);
  });

  it('keeps the faint ink lighter than the soft ink in both palettes (the hierarchy of the texts survives the contrast)', () => {
    for (const palette of [THEME, SHELL]) {
      expect(luminance(colorOf(palette, 'ink-faint'))).toBeGreaterThan(luminance(colorOf(palette, 'ink-soft')));
      expect(luminance(colorOf(palette, 'ink-soft'))).toBeGreaterThan(luminance(colorOf(palette, 'ink')));
    }
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

describe('the dialogs', () => {
  const walk = (dir) => readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? walk(path) : [path];
  });
  const sources = walk('src').filter((f) => /\.jsx$/.test(f) && !/\.test\.jsx$/.test(f) && !/testUtils\.jsx$/.test(f));

  // What a dialog owes the keyboard (focus in, Tab inside, Escape on the top one, focus back) is done by one hook, and a dialog
  // that does it by hand is a dialog that forgets one of the four.
  it('use the hook of the dialogs when they are modal', () => {
    const modal = sources.filter((f) => /aria-modal="true"/.test(readFileSync(f, 'utf8')));
    expect(modal.length).toBeGreaterThanOrEqual(15);
    const without = modal.filter((f) => !/\buseDialog\(/.test(readFileSync(f, 'utf8')));
    expect(without).toEqual([]);
  });

  it('and the ones that are not modal either use it or are the settings panel of the reader, which the reader manages with its own Escape and immersive mode', () => {
    const popovers = ['ReadingSettingsPanel.jsx']; // an open panel of the reader: closed by the reader (Escape, immersive mode)
    const plain = sources.filter((f) => /role="dialog"/.test(readFileSync(f, 'utf8')) && !/aria-modal="true"/.test(readFileSync(f, 'utf8')));
    const without = plain.filter((f) => !/\buseDialog\(/.test(readFileSync(f, 'utf8')) && !popovers.some((p) => f.endsWith(p)));
    expect(without).toEqual([]);
  });

  it('never close on Escape by their own listener (the one on top is the only one that answers)', () => {
    const own = sources.filter((f) => /role="dialog"/.test(readFileSync(f, 'utf8')))
      .filter((f) => /key === 'Escape'/.test(readFileSync(f, 'utf8')));
    expect(own).toEqual([]);
  });
});
