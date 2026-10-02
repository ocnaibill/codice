import { describe, expect, it, vi } from 'vitest';
import {
  READING_THEMES, READING_FONTS, READING_SPACING, DEFAULT_SETTINGS, SIZE_MIN, SIZE_MAX, SIZE_STEP,
  sanitizeSettings, contrastRatio, epubRules, themeName, applyEpubSettings, fontFaceCss,
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
    expect(DEFAULT_SETTINGS).toEqual({ theme: 'papel', font: 'livro', size: 100, spacing: 'livro' });
    expect(READING_THEMES.find((t) => t.id === 'papel')).toMatchObject({ background: '#faf8f4', text: '#18181b' });
  });
});

describe('sanitizeSettings', () => {
  it('keeps what is in the lists', () => {
    const all = { theme: 'sepia', font: 'serifada', size: 130, spacing: 'ampla' };
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
    expect(Object.keys(sanitizeSettings({ theme: 'preto', extra: '<script>', __proto__: { a: 1 } }))).toEqual(['theme', 'font', 'size', 'spacing']);
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
    expect((css.match(/@font-face/g) || []).length).toBe(4);
    expect(css).toContain('font-family:"Codice Serif";font-style:normal');
    expect(css).toContain('font-family:"Codice Serif";font-style:italic');
    expect(css).toContain('font-family:"Codice Sans";font-style:normal');
    expect(css).toContain('font-family:"Codice Sans";font-style:italic');
    expect(css).toContain('url("http://app.test/');
    expect(css).toContain('font-weight:200 800');
    expect(css).toContain('format("woff2")');
  });
});
