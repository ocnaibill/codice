import { readFileSync } from 'node:fs';
import { describe, expect, it, vi } from 'vitest';
import {
  READING_THEMES, READING_FONTS, READING_SPACING, READING_MARGINS, DEFAULT_SETTINGS, marginStyle, SIZE_MIN, SIZE_MAX, SIZE_STEP,
  sanitizeSettings, contrastRatio, epubRules, SELECTION_ON_LIGHT, SELECTION_ON_DARK, themeName, applyEpubSettings, fontFaceCss,
} from './epubThemes';

describe('contrastRatio', () => {
  it('is 21 for black on white, 1 for a color on itself, and the same either way round', () => {
    expect(contrastRatio('#000000', '#ffffff')).toBeCloseTo(21, 5);
    expect(contrastRatio('#ffffff', '#000000')).toBeCloseTo(21, 5);
    expect(contrastRatio('#777777', '#777777')).toBeCloseTo(1, 5);
  });

  it('weighs green more than red, and red more than blue', () => {
    expect(contrastRatio('#00ff00', '#000000')).toBeGreaterThan(contrastRatio('#ff0000', '#000000'));
    expect(contrastRatio('#ff0000', '#000000')).toBeGreaterThan(contrastRatio('#0000ff', '#000000'));
  });

  it('gives the known value of grey #767676 on white (4.54, the least that passes)', () => {
    expect(contrastRatio('#767676', '#ffffff')).toBeCloseTo(4.54, 2);
  });
});

describe('the pages of the reader', () => {
  it('are the six that were asked for, each with a name and its own colors', () => {
    expect(READING_THEMES.map((t) => t.label)).toEqual(['Branco', 'Papel', 'Sépia', 'Cinza', 'Escuro', 'Preto']);
    expect(new Set(READING_THEMES.map((t) => t.id)).size).toBe(6);
    expect(new Set(READING_THEMES.map((t) => t.background)).size).toBe(6);
  });

  it.each(READING_THEMES.map((t) => [t.id, t]))('%s: the text and the links can be read on the page (4.5 or more)', (_id, t) => {
    expect(contrastRatio(t.text, t.background)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(t.link, t.background)).toBeGreaterThanOrEqual(4.5);
  });

  it('keep the paper of before as the default', () => {
    expect(DEFAULT_SETTINGS).toEqual({ theme: 'papel', font: 'livro', size: 100, spacing: 'livro', margins: 'livro', justify: false });
    expect(READING_THEMES.find((t) => t.id === 'papel')).toMatchObject({ background: '#faf8f4', text: '#18181b' });
  });
});

describe('sanitizeSettings', () => {
  it('keeps what is in the lists', () => {
    const all = { theme: 'sepia', font: 'dislexia', size: 130, spacing: 'ampla', margins: 'media', justify: true };
    expect(sanitizeSettings(all)).toEqual(all);
  });

  it('puts the default where a value is not in the list', () => {
    expect(sanitizeSettings({ theme: 'rosa', font: 'comic', size: 100, spacing: 'enorme' })).toEqual(DEFAULT_SETTINGS);
    expect(sanitizeSettings({ theme: '#ff0000' }).theme).toBe('papel');
  });

  it('accepts only a size on one of the steps, from the least to the most', () => {
    expect(sanitizeSettings({ size: SIZE_MIN }).size).toBe(SIZE_MIN);
    expect(sanitizeSettings({ size: SIZE_MAX }).size).toBe(SIZE_MAX);
    for (const size of [SIZE_MIN - SIZE_STEP, SIZE_MAX + SIZE_STEP, 85, 100.5, '110', null, NaN, 'grande', -100, 0]) {
      expect(sanitizeSettings({ size }).size, String(size)).toBe(size === '110' ? 110 : 100);
    }
  });

  it('gives the default for anything that is not an object', () => {
    for (const raw of [null, undefined, 'x', 7, true, []]) expect(sanitizeSettings(raw)).toEqual(DEFAULT_SETTINGS);
  });

  it('returns only the four choices, whatever else came with them', () => {
    expect(Object.keys(sanitizeSettings({ theme: 'preto', extra: '<script>', __proto__: { a: 1 } }))).toEqual(['theme', 'font', 'size', 'spacing', 'margins', 'justify']);
  });
});

describe('the rules of the page of a book', () => {
  const rules = (settings) => epubRules({ ...DEFAULT_SETTINGS, ...settings });

  it('paint the page and the text, and win over what the book forces', () => {
    const n = `.${themeName(DEFAULT_SETTINGS)}`;
    const r = rules({});
    expect(r[n].background).toBe('#faf8f4 !important');
    expect(r[`${n} body`].background).toBe('#faf8f4 !important');
    expect(r[`${n}, ${n} *`].color).toBe('#18181b !important');
    expect(r[`${n} a, ${n} a *`].color).toBe('#1d4ed8 !important');
  });

  it('use the colors of the chosen page', () => {
    const r = rules({ theme: 'escuro' });
    const n = `.${themeName({ ...DEFAULT_SETTINGS, theme: 'escuro' })}`;
    expect(r[n].background).toBe('#18181b !important');
    expect(r[`${n}, ${n} *`].color).toBe('#e4e4e7 !important');
    expect(r[`${n} a, ${n} a *`].color).toBe('#93c5fd !important');
  });

  it('paint a selected text terracotta, lighter and stronger on the dark pages, and win over the book', () => {
    for (const theme of ['branco', 'papel', 'sepia', 'cinza']) {
      const n = `.${themeName({ ...DEFAULT_SETTINGS, theme })}`;
      expect(rules({ theme })[`${n}::selection, ${n} *::selection`].background, theme).toBe(`${SELECTION_ON_LIGHT} !important`);
    }
    for (const theme of ['escuro', 'preto']) {
      const n = `.${themeName({ ...DEFAULT_SETTINGS, theme })}`;
      expect(rules({ theme })[`${n}::selection, ${n} *::selection`].background, theme).toBe(`${SELECTION_ON_DARK} !important`);
    }
    expect(SELECTION_ON_LIGHT).toBe('rgba(148, 69, 22, 0.3)');
    expect(SELECTION_ON_DARK).toBe('rgba(179, 93, 44, 0.55)');
  });

  it('leave the font of the book alone with "Do livro", and set the chosen one otherwise, except for code', () => {
    expect(Object.keys(rules({})).some((k) => k.includes(':not(pre)'))).toBe(false);
    const n = `.${themeName({ ...DEFAULT_SETTINGS, font: 'serifada' })}`;
    const r = rules({ font: 'serifada' });
    const key = Object.keys(r).find((k) => k.includes(':not(pre)'));
    expect(key.startsWith(`${n} `)).toBe(true);
    expect(key).toContain(':not(code)');
    expect(key).toContain(':not(pre *)');
    expect(r[key]['font-family']).toBe('"Codice Serif", Georgia, serif !important');
    const sans = rules({ font: 'sem-serifa' });
    expect(sans[Object.keys(sans).find((k) => k.includes(':not(pre)'))]['font-family']).toBe('"Codice Sans", system-ui, sans-serif !important');
  });

  it('leave the spacing of the book alone with "Do livro", and set the chosen one otherwise', () => {
    expect(Object.keys(rules({})).some((k) => k.includes('line-height') || k.includes(' p,'))).toBe(false);
    for (const [id, value] of [['media', 1.5], ['ampla', 1.8]]) {
      const r = rules({ spacing: id });
      const key = Object.keys(r).find((k) => r[k]['line-height']);
      expect(r[key]['line-height']).toBe(`${value} !important`);
      expect(key).toContain(' p,');
      expect(key).toContain(' li,');
    }
  });

  it('have a name of their own for each combination, so that changing one replaces the last', () => {
    const names = new Set();
    for (const t of READING_THEMES) for (const f of READING_FONTS) for (const p of READING_SPACING) {
      names.add(themeName({ theme: t.id, font: f.id, spacing: p.id }));
    }
    expect(names.size).toBe(READING_THEMES.length * READING_FONTS.length * READING_SPACING.length);
  });

  it('read a setting that is not valid as the default, so that nothing outside the lists reaches the page', () => {
    const r = epubRules({ theme: 'x" } body { display:none', font: 'y', size: 100, spacing: 'z' });
    expect(JSON.stringify(r)).not.toContain('display:none');
    expect(r[`.${themeName(DEFAULT_SETTINGS)}`].background).toBe('#faf8f4 !important');
  });
});

describe('applyEpubSettings', () => {
  it('registers the rules under the name, selects it and sets the size', () => {
    const themes = { register: vi.fn(), select: vi.fn(), fontSize: vi.fn() };
    const settings = { theme: 'sepia', font: 'serifada', size: 120, spacing: 'media' };
    applyEpubSettings({ themes }, settings);
    const name = themeName(settings);
    expect(themes.register).toHaveBeenCalledWith(name, epubRules(settings));
    expect(themes.select).toHaveBeenCalledWith(name);
    expect(themes.fontSize).toHaveBeenCalledWith('120%');
  });

  it('does nothing without a book, and falls back to the default for a size that is not valid', () => {
    expect(() => applyEpubSettings(null, DEFAULT_SETTINGS)).not.toThrow();
    const themes = { register: vi.fn(), select: vi.fn(), fontSize: vi.fn() };
    applyEpubSettings({ themes }, { size: 999 });
    expect(themes.fontSize).toHaveBeenCalledWith('100%');
  });
});

describe('fontFaceCss', () => {
  it('declares the four files of the two fonts, in both styles, with addresses on the app', () => {
    const css = fontFaceCss('http://app.test/leitor');
    expect((css.match(/@font-face/g) || []).length).toBe(7);
    expect(css).toContain('font-family:"Codice Serif";font-style:normal');
    expect(css).toContain('font-family:"Codice Serif";font-style:italic');
    expect(css).toContain('font-family:"Codice Sans";font-style:normal');
    expect(css).toContain('font-family:"Codice Sans";font-style:italic');
    expect(css).toContain('url("http://app.test/');
    expect(css).toContain('font-weight:200 800');
    expect(css).toContain('format("woff2")');
  });
});

describe('the font for dyslexia', () => {
  it('is one of the fonts, and has a stack for a book and one for the app, both with a fallback', () => {
    const font = READING_FONTS.find((f) => f.id === 'dislexia');
    expect(font.label).toBe('Dislexia');
    expect(font.stack).toBe('"Codice OpenDyslexic", system-ui, sans-serif');
    expect(font.appStack).toBe('"OpenDyslexic", system-ui, sans-serif');
  });

  it('is declared inside the page of a book, in its regular, bold and italic, as files of the app', () => {
    const css = fontFaceCss('http://app.test/leitor');
    expect(css).toMatch(/font-family:"Codice OpenDyslexic";font-style:normal;font-weight:400;/);
    expect(css).toMatch(/font-family:"Codice OpenDyslexic";font-style:normal;font-weight:700;/);
    expect(css).toMatch(/font-family:"Codice OpenDyslexic";font-style:italic;font-weight:400;/);
    expect(css).not.toContain('https://fonts.');
  });

  it('is the font of the book when chosen, over what the book says', () => {
    const n = `.${themeName({ ...DEFAULT_SETTINGS, font: 'dislexia' })}`;
    const rule = epubRules({ ...DEFAULT_SETTINGS, font: 'dislexia' })[`${n} *:not(pre):not(code):not(kbd):not(samp):not(pre *)`];
    expect(rule['font-family']).toBe('"Codice OpenDyslexic", system-ui, sans-serif !important');
  });
});

describe('the margins', () => {
  it('are the page\'s own, narrow, medium or wide, and "Do livro" is the first', () => {
    expect(READING_MARGINS.map((m) => m.id)).toEqual(['livro', 'estreita', 'media', 'larga']);
    expect(READING_MARGINS.map((m) => m.label)).toEqual(['Do livro', 'Estreita', 'Média', 'Larga']);
    expect(DEFAULT_SETTINGS.margins).toBe('livro');
  });

  it('are the room left on each side of the text, and nothing for the page\'s own', () => {
    expect(marginStyle({ ...DEFAULT_SETTINGS, margins: 'livro' })).toEqual({});
    expect(marginStyle({ ...DEFAULT_SETTINGS, margins: 'estreita' })).toEqual({ paddingInline: '0px' });
    expect(marginStyle({ ...DEFAULT_SETTINGS, margins: 'media' })).toEqual({ paddingInline: '6%' });
    expect(marginStyle({ ...DEFAULT_SETTINGS, margins: 'larga' })).toEqual({ paddingInline: '14%' });
    expect(marginStyle({ margins: 'enorme' })).toEqual({});
    expect(marginStyle(undefined)).toEqual({});
  });

  it('are kept when they are in the list, and are the page\'s own when they are not', () => {
    expect(sanitizeSettings({ margins: 'larga' }).margins).toBe('larga');
    for (const bad of ['enorme', '', null, 5, undefined, {}]) expect(sanitizeSettings({ margins: bad }).margins, String(bad)).toBe('livro');
  });
});

describe('justified lines', () => {
  it('are off by default, and on only for a true', () => {
    expect(DEFAULT_SETTINGS.justify).toBe(false);
    expect(sanitizeSettings({ justify: true }).justify).toBe(true);
    for (const bad of ['true', 1, 'sim', null, undefined, {}, []]) expect(sanitizeSettings({ justify: bad }).justify, String(bad)).toBe(false);
  });

  it('are made of the paragraphs and not of the headings, with hyphens, winning over the book', () => {
    const on = { ...DEFAULT_SETTINGS, justify: true };
    const n = `.${themeName(on)}`;
    const rule = epubRules(on)[`${n} p, ${n} li, ${n} blockquote, ${n} dd`];
    expect(rule).toEqual({ 'text-align': 'justify !important', hyphens: 'auto !important', '-webkit-hyphens': 'auto !important' });
  });

  it('leave the text as the book has it when off', () => {
    const rules = epubRules(DEFAULT_SETTINGS);
    expect(Object.values(rules).some((r) => 'text-align' in r || 'hyphens' in r)).toBe(false);
  });

  it('change the name of the page of the book, which is how epub.js tells one set of rules from another', () => {
    expect(themeName({ ...DEFAULT_SETTINGS, justify: true })).not.toBe(themeName(DEFAULT_SETTINGS));
    expect(themeName({ ...DEFAULT_SETTINGS, justify: true })).toMatch(/-j$/);
    expect(themeName(DEFAULT_SETTINGS)).toMatch(/-l$/);
  });
});

// The lists are the server's too (backend/internal/reading/settings.go): a value this reader offers and the server refuses
// would be lost when it is kept, so that they are the same is checked, from the source of the server.
describe('the lists, as the server has them', () => {
  const go = readFileSync('../backend/internal/reading/settings.go', 'utf8');
  const listOf = (name) => [...go.match(new RegExp(`${name}\\s*=\\s*\\[\\]string\\{([^}]*)\\}`))[1].matchAll(/"([^"]+)"/g)].map((m) => m[1]);

  it('are the same: themes, fonts, spacings and margins', () => {
    expect(listOf('Themes')).toEqual(READING_THEMES.map((t) => t.id));
    expect(listOf('Fonts')).toEqual(READING_FONTS.map((f) => f.id));
    expect(listOf('Spacings')).toEqual(READING_SPACING.map((p) => p.id));
    expect(listOf('Margins')).toEqual(READING_MARGINS.map((m) => m.id));
  });

  it('have the same size, from the least to the most, in the same steps', () => {
    const number = (name) => Number(go.match(new RegExp(`${name}\\s*=\\s*(\\d+)`))[1]);
    expect([number('SizeMin'), number('SizeMax'), number('SizeStep')]).toEqual([SIZE_MIN, SIZE_MAX, SIZE_STEP]);
  });
});
