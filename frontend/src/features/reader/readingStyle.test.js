import { describe, it, expect } from 'vitest';
import { readingStyle, BASE_SIZE, DEFAULT_LINE_HEIGHT } from './readingStyle';
import { DEFAULT_SETTINGS } from './epubThemes';

describe('readingStyle', () => {
  it('draws the default page as the paper, in the usual letter and the app font', () => {
    const s = readingStyle(DEFAULT_SETTINGS);
    expect(s.page).toEqual({ backgroundColor: '#faf8f4' });
    expect(s.text.color).toBe('#18181b');
    expect(s.text.fontSize).toBe(`${BASE_SIZE}px`);
    expect(s.text.lineHeight).toBe(DEFAULT_LINE_HEIGHT);
    expect('fontFamily' in s.text).toBe(false);
    expect(s.theme.link).toBe('#1d4ed8');
  });

  it('follows the page color chosen', () => {
    const s = readingStyle({ ...DEFAULT_SETTINGS, theme: 'escuro' });
    expect(s.page.backgroundColor).toBe('#18181b');
    expect(s.text.color).toBe('#e4e4e7');
    expect(s.theme.id).toBe('escuro');
  });

  it('takes the size as a percentage of the usual letter', () => {
    expect(readingStyle({ ...DEFAULT_SETTINGS, size: 150 }).text.fontSize).toBe('24px');
    expect(readingStyle({ ...DEFAULT_SETTINGS, size: 80 }).text.fontSize).toBe('12.8px');
    expect(readingStyle({ ...DEFAULT_SETTINGS, size: 200 }).text.fontSize).toBe('32px');
  });

  it("uses the app's own fonts when one is chosen, not the ones of the page of a book", () => {
    expect(readingStyle({ ...DEFAULT_SETTINGS, font: 'serifada' }).text.fontFamily).toBe('"Newsreader Variable", Georgia, serif');
    expect(readingStyle({ ...DEFAULT_SETTINGS, font: 'sem-serifa' }).text.fontFamily).toBe('"Plus Jakarta Sans Variable", system-ui, sans-serif');
  });

  it('uses the space between lines chosen', () => {
    expect(readingStyle({ ...DEFAULT_SETTINGS, spacing: 'media' }).text.lineHeight).toBe(1.5);
    expect(readingStyle({ ...DEFAULT_SETTINGS, spacing: 'ampla' }).text.lineHeight).toBe(1.8);
  });

  it('reads what is not valid as the default', () => {
    const s = readingStyle({ theme: 'x', font: 'y', size: 5, spacing: 'z' });
    expect(s.page.backgroundColor).toBe('#faf8f4');
    expect(s.text.fontSize).toBe('16px');
  });
});
