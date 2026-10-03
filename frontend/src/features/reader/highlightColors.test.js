import { describe, it, expect } from 'vitest';
import { DEFAULT_HIGHLIGHT_COLOR, HIGHLIGHT_COLORS, highlightColor, isHighlightColor } from './highlightColors';

describe('the colors of a highlight', () => {
  it('are four, with names in Portuguese and a color each, terracotta first', () => {
    expect(HIGHLIGHT_COLORS.map((c) => c.id)).toEqual(['terracotta', 'sepia', 'sage', 'indigo']);
    expect(HIGHLIGHT_COLORS.map((c) => c.name)).toEqual(['Terracota', 'Sépia', 'Sálvia', 'Índigo']);
    for (const c of HIGHLIGHT_COLORS) expect(c.hex).toMatch(/^#[0-9a-f]{6}$/);
    expect(new Set(HIGHLIGHT_COLORS.map((c) => c.hex)).size).toBe(4);
    expect(DEFAULT_HIGHLIGHT_COLOR).toBe('terracotta');
  });

  it('know which ids are colors', () => {
    expect(isHighlightColor('sage')).toBe(true);
    for (const id of ['red', 'Sage', '', null, undefined, '#944516']) expect(isHighlightColor(id), String(id)).toBe(false);
  });

  it('give the default for what is not one', () => {
    expect(highlightColor('indigo').name).toBe('Índigo');
    expect(highlightColor('red').id).toBe('terracotta');
    expect(highlightColor(undefined).id).toBe('terracotta');
  });
});
