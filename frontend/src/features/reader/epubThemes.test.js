import { describe, expect, it, vi } from 'vitest';
import { applyEpubTheme, epubThemes } from './epubThemes';

describe('EPUB themes', () => {
  it('keeps both text colors scoped to the active rendition class', () => {
    expect(epubThemes.dark['.dark, .dark *'].color).toBe('#e4e4e7 !important');
    expect(epubThemes.light['.light, .light *'].color).toBe('#18181b !important');
    expect(epubThemes.dark['body, body *']).toBeUndefined();
    expect(epubThemes.light['body, body *']).toBeUndefined();
  });

  it('registers both modes and selects the requested one', () => {
    const rendition = { themes: { register: vi.fn(), select: vi.fn() } };
    applyEpubTheme(rendition, 'dark');
    expect(rendition.themes.register).toHaveBeenCalledWith('light', epubThemes.light);
    expect(rendition.themes.register).toHaveBeenCalledWith('dark', epubThemes.dark);
    expect(rendition.themes.select).toHaveBeenCalledWith('dark');
  });
});
