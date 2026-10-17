import { describe, it, expect } from 'vitest';
import { chosenMapping, initialMapping, skippedText } from './useNotesImport';

describe('the mapping of an import', () => {
  const groups = [
    { key: 'a|x', suggested: 7, candidates: [{ workId: 7 }] },
    { key: 'b|y', suggested: 0, candidates: [{ workId: 8 }, { workId: 9 }] },
    { key: 'c|z', suggested: 0, candidates: [] },
  ];

  it('begins with the work the server was sure of, and none for the others', () => {
    expect(initialMapping(groups)).toEqual({ 'a|x': 7, 'b|y': 0, 'c|z': 0 });
    expect(initialMapping(undefined)).toEqual({});
  });

  it('sends only the books that have a work', () => {
    expect(chosenMapping({ 'a|x': 7, 'b|y': 0, 'c|z': 9 })).toEqual({ 'a|x': 7, 'c|z': 9 });
  });
});

describe('skippedText', () => {
  it('says what was left out, and nothing when nothing was', () => {
    expect(skippedText({ bookmarks: 2, limit: 1, repeated: 0 })).toBe(
      'Ficou de fora: 2 marcadores, que não têm texto; 1 trechos que o Kindle não deixou copiar (limite de recortes do livro).',
    );
    expect(skippedText({})).toBe('');
    expect(skippedText(undefined)).toBe('');
  });
});
