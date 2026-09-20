import { describe, it, expect } from 'vitest';
import { findFile, languageName, formatSize, positionFromLocator, placeLabel, parseTags, completionText, whereYouAre, candidateLabel } from './files';

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

describe('locators', () => {
  it('turns a locator into the position each viewer opens from', () => {
    expect(positionFromLocator({ type: 'epub', cfi: 'epubcfi(/6/2)', href: 'a.xhtml' })).toBe('epubcfi(/6/2)');
    expect(positionFromLocator({ type: 'epub', href: 'a.xhtml' })).toBe('a.xhtml');
    expect(positionFromLocator({ type: 'pdf', page: 11 })).toBe('12'); // the PDF viewer counts from 1
    expect(positionFromLocator({ type: 'image', index: 7 })).toBe('7'); // the comic viewer from 0
    expect(positionFromLocator({ type: 'audio', track: 0, ms: 93500 })).toBe('93.5');
    expect(positionFromLocator({ type: 'text', offset: 40 })).toBe('40');
    expect(positionFromLocator(null)).toBeUndefined();
    expect(positionFromLocator({ type: 'other' })).toBeUndefined();
  });

  it('says where a locator points', () => {
    expect(placeLabel({ type: 'pdf', page: 11 })).toBe('PDF, página 12');
    expect(placeLabel({ type: 'pdf', page: 11, label: 'xii' })).toBe('PDF, página xii');
    expect(placeLabel({ type: 'audio', track: 1, ms: 125000 })).toBe('Áudio, faixa 2, 2:05');
    expect(placeLabel({ type: 'image', index: 0 })).toBe('Imagem 1');
    expect(placeLabel({ type: 'epub', cfi: 'x' })).toBe('EPUB, posição salva');
    expect(placeLabel(null)).toBeNull();
  });

  it('splits typed tags', () => {
    expect(parseTags(' a, b ,, c ')).toEqual(['a', 'b', 'c']);
    expect(parseTags('')).toEqual([]);
    expect(parseTags(undefined)).toEqual([]);
  });
});

describe('reading summary texts', () => {
  it('counts the times a work was finished, by format, most first', () => {
    expect(completionText({ total: 3, byFormat: { pdf: 1, epub: 2 } })).toBe('Terminada 3 vezes: 2 em EPUB, 1 em PDF');
    expect(completionText({ total: 1, byFormat: { epub: 1 } })).toBe('Terminada 1 vez: 1 em EPUB');
    expect(completionText({ total: 0, byFormat: {} })).toBeNull();
    expect(completionText(undefined)).toBeNull();
  });

  it('says where the person is', () => {
    expect(whereYouAre({ format: 'pdf', language: 'pt', percentComplete: 41.6 })).toBe('Você está em 42% no PDF (Português)');
    expect(whereYouAre({ format: 'epub', percentComplete: 0 })).toBe('Você está lendo o EPUB');
    expect(whereYouAre(null)).toBeNull();
  });

  it('names how a candidate position was found', () => {
    expect(candidateLabel({ method: 'text', confidence: 'high' })).toBe('mesmo trecho, alta confiança');
    expect(candidateLabel({ method: 'anchors', confidence: 'low' })).toBe('mesmos nomes e números, confiança baixa');
    expect(candidateLabel({ method: 'structure', confidence: 'medium' })).toBe('mesmo capítulo, confiança média');
    expect(candidateLabel({ method: 'unknown' })).toBe('correspondência aproximada');
    expect(candidateLabel(undefined)).toBe('correspondência aproximada');
  });
});
