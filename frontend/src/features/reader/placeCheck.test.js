import { describe, it, expect } from 'vitest';
import { audioPlaceProblem, epubPlaceProblem, imagePlaceProblem, pdfPlaceProblem } from './placeCheck';

describe('pdfPlaceProblem', () => {
  it('has nothing to say when no place was asked for or the page exists', () => {
    expect(pdfPlaceProblem(undefined, 10)).toBeNull();
    expect(pdfPlaceProblem(null, 10)).toBeNull();
    expect(pdfPlaceProblem('', 10)).toBeNull();
    expect(pdfPlaceProblem('1', 10)).toBeNull();
    expect(pdfPlaceProblem('10', 10)).toBeNull();
    expect(pdfPlaceProblem('12', null)).toBeNull(); // the size of the file is not known yet
  });

  it('says a page past the end does not exist, and how many the file has', () => {
    expect(pdfPlaceProblem('12', 10)).toBe('A página 12 não existe: o arquivo tem 10 páginas.');
    expect(pdfPlaceProblem('2', 1)).toBe('A página 2 não existe: o arquivo tem 1 página.');
  });

  it('says a page that is not a page is not valid', () => {
    for (const asked of ['0', '-3', 'abc', '2.5', 'NaN']) {
      expect(pdfPlaceProblem(asked, 10), asked).toBe('A página pedida não é válida.');
    }
  });
});

describe('imagePlaceProblem', () => {
  it('counts images from 0 and says the one asked for is not there', () => {
    expect(imagePlaceProblem(undefined, 3)).toBeNull();
    expect(imagePlaceProblem('0', 3)).toBeNull();
    expect(imagePlaceProblem('2', 3)).toBeNull();
    expect(imagePlaceProblem('3', 3)).toBe('A imagem 4 não existe: o arquivo tem 3 imagens.');
    expect(imagePlaceProblem('1', 1)).toBe('A imagem 2 não existe: o arquivo tem 1 imagem.');
    expect(imagePlaceProblem('5', 0)).toBeNull(); // nothing is known about the file
  });

  it('says an image that is not an image is not valid', () => {
    for (const asked of ['-1', 'x', '1.5']) expect(imagePlaceProblem(asked, 3), asked).toBe('A imagem pedida não é válida.');
  });
});

describe('audioPlaceProblem', () => {
  it('accepts a time inside the audio, and the very end', () => {
    expect(audioPlaceProblem(undefined, 100)).toBeNull();
    expect(audioPlaceProblem('0', 100)).toBeNull();
    expect(audioPlaceProblem('99.5', 100)).toBeNull();
    expect(audioPlaceProblem('100', 100)).toBeNull();
    expect(audioPlaceProblem('500', NaN)).toBeNull(); // duration not known
    expect(audioPlaceProblem('500', 0)).toBeNull();
  });

  it('says a time past the end, or one that is not a time', () => {
    expect(audioPlaceProblem('101', 100)).toBe('O ponto passa do fim do áudio.');
    for (const asked of ['-2', 'x', 'Infinity']) expect(audioPlaceProblem(asked, 100), asked).toBe('O ponto pedido não é válido.');
  });
});

describe('epubPlaceProblem', () => {
  it('says the chapter is gone when the book has none at that position', () => {
    expect(epubPlaceProblem({ section: null, locator: { href: 'c1.xhtml' } })).toContain('não existe mais neste EPUB');
    expect(epubPlaceProblem({ section: undefined })).toContain('não existe mais neste EPUB');
  });

  it('accepts the chapter the place was saved in, whichever way the folder is written', () => {
    expect(epubPlaceProblem({ section: { href: 'Text/c1.xhtml' }, locator: { href: 'c1.xhtml' } })).toBeNull();
    expect(epubPlaceProblem({ section: { href: 'c1.xhtml' }, locator: { href: 'Text/c1.xhtml#p2' } })).toBeNull();
    expect(epubPlaceProblem({ section: { href: 'c1.xhtml' }, locator: { href: 'c1.xhtml' } })).toBeNull();
  });

  it('says the book changed when that position is now another chapter', () => {
    expect(epubPlaceProblem({ section: { href: 'Text/c9.xhtml' }, locator: { href: 'c1.xhtml' } })).toContain('não é mais o mesmo');
    expect(epubPlaceProblem({ section: { href: 'Text/xc1.xhtml' }, locator: { href: 'c1.xhtml' } })).toContain('não é mais o mesmo'); // a suffix of the name is not the same file
  });

  it('has nothing to compare without a saved chapter', () => {
    expect(epubPlaceProblem({ section: { href: 'c9.xhtml' }, locator: {} })).toBeNull();
    expect(epubPlaceProblem({ section: { href: 'c9.xhtml' } })).toBeNull();
    expect(epubPlaceProblem({ section: { href: '' }, locator: { href: 'c1.xhtml' } })).toContain('não é mais o mesmo');
  });
});
