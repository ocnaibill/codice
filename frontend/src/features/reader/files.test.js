import { describe, it, expect } from 'vitest';
import { findFile, languageName, formatSize } from './files';

const work = {
  id: 1, fileId: 10, fileUrl: '/file/10', format: 'epub',
  editions: [
    { id: 1, language: 'en', files: [{ id: 10, format: 'epub', url: '/file/10', availability: 'available' }] },
    { id: 2, language: 'pt', files: [{ id: 20, format: 'pdf', url: '/file/20', availability: 'available' }, { id: 21, format: 'cbz', url: '/file/21', availability: 'missing' }] },
  ],
};

describe('findFile', () => {
  it('finds the file that was asked for, with its edition', () => {
    const f = findFile(work, 20);
    expect(f.format).toBe('pdf');
    expect(f.edition.language).toBe('pt');
  });

  it("falls back to the work's primary file when none is asked for", () => {
    expect(findFile(work, null).id).toBe(10);
  });

  it('does not silently read another file when the one asked for is not in the work', () => {
    expect(findFile(work, 999)).toBeNull();
  });

  it('keeps working for a card that has no editions yet', () => {
    const card = { id: 5, fileId: 50, fileUrl: '/file/50', format: 'pdf' };
    expect(findFile(card, null)).toMatchObject({ id: 50, url: '/file/50', format: 'pdf' });
    expect(findFile(card, 51)).toBeNull();
  });

  it('is null for no work', () => {
    expect(findFile(undefined, 1)).toBeNull();
  });
});

describe('labels', () => {
  it('names languages, with a region falling back to the language', () => {
    expect(languageName('pt-BR')).toBe('Português (Brasil)');
    expect(languageName('es-MX')).toBe('Espanhol');
    expect(languageName('tlh')).toBe('tlh');
    expect(languageName('')).toBeNull();
  });

  it('writes sizes', () => {
    expect(formatSize(500)).toBe('1 KB');
    expect(formatSize(5 * 1024 * 1024)).toBe('5.0 MB');
    expect(formatSize(null)).toBe('');
  });
});
