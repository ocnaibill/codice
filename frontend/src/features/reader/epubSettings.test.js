import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getEpubSettings, saveEpubSettings, setPreferenceOwner } from './preferences';
import { toScreen, acrossPage } from './epubGestures';
import { flattenToc } from './epubToc';

describe('how the EPUB reader looks, remembered', () => {
  beforeEach(() => {
    localStorage.clear();
    setPreferenceOwner('ana');
  });
  afterEach(() => vi.restoreAllMocks());

  it('starts with the paper page and the book as it is, and remembers what was chosen', () => {
    expect(getEpubSettings()).toEqual({ theme: 'papel', font: 'livro', size: 100, spacing: 'livro' });
    const chosen = { theme: 'preto', font: 'serifada', size: 140, spacing: 'ampla' };
    saveEpubSettings(chosen);
    expect(getEpubSettings()).toEqual(chosen);
  });

  it('belongs to the account on this device', () => {
    saveEpubSettings({ theme: 'sepia', font: 'livro', size: 120, spacing: 'livro' });
    setPreferenceOwner('bob');
    expect(getEpubSettings().theme).toBe('papel');
    saveEpubSettings({ theme: 'cinza', font: 'livro', size: 90, spacing: 'livro' });
    setPreferenceOwner('ana');
    expect(getEpubSettings()).toMatchObject({ theme: 'sepia', size: 120 });
    setPreferenceOwner('bob');
    expect(getEpubSettings()).toMatchObject({ theme: 'cinza', size: 90 });
  });

  it('is kept apart from the mode of the comic reader', () => {
    saveEpubSettings({ theme: 'sepia', font: 'livro', size: 120, spacing: 'livro' });
    expect(localStorage.getItem('codice:comic-mode:ana')).toBeNull();
    expect(localStorage.getItem('codice:epub-settings:ana')).not.toBeNull();
  });

  it('remembers nothing when nobody is signed in', () => {
    setPreferenceOwner(null);
    saveEpubSettings({ theme: 'preto', font: 'livro', size: 120, spacing: 'livro' });
    expect(localStorage.length).toBe(0);
    expect(getEpubSettings().theme).toBe('papel');
  });

  it('does not keep what is not in the lists, and reads damaged storage as the default', () => {
    saveEpubSettings({ theme: 'rosa', font: 'livro', size: 999, spacing: 'livro' });
    expect(JSON.parse(localStorage.getItem('codice:epub-settings:ana'))).toEqual({ theme: 'papel', font: 'livro', size: 100, spacing: 'livro' });
    localStorage.setItem('codice:epub-settings:ana', '{not json');
    expect(getEpubSettings().theme).toBe('papel');
    localStorage.setItem('codice:epub-settings:ana', JSON.stringify({ theme: 'x', size: 130 }));
    expect(getEpubSettings()).toMatchObject({ theme: 'papel', size: 130 });
  });

  it('copes with storage that cannot be read or written', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('full'); });
    expect(getEpubSettings().theme).toBe('papel');
    expect(() => saveEpubSettings({ theme: 'preto' })).not.toThrow();
  });
});

describe('where a touch on the book is', () => {
  it('brings a point of the iframe to the screen: the iframe moves as the pages turn', () => {
    expect(toScreen({ frameLeft: -800, frameTop: 40, clientX: 900, clientY: 100 })).toEqual({ x: 100, y: 140 });
    expect(toScreen({ frameLeft: 12, frameTop: 0, clientX: 5, clientY: 7 })).toEqual({ x: 17, y: 7 });
  });

  it('says how far across the page it is, from 0 to 1, and does not divide by nothing', () => {
    expect(acrossPage({ x: 150, left: 100, width: 200 })).toBe(0.25);
    expect(acrossPage({ x: 300, left: 100, width: 200 })).toBe(1);
    expect(acrossPage({ x: 100, left: 100, width: 200 })).toBe(0);
    expect(acrossPage({ x: 50, left: 100, width: 0 })).toBe(-50);
  });
});

describe('the table of contents as a list', () => {
  it('goes through the entries in order, each with how deep it is, and trims the names', () => {
    const toc = [
      { id: 'a', href: 'a.xhtml', label: ' Parte I ', subitems: [{ id: 'a1', href: 'a1.xhtml', label: 'Capítulo 1', subitems: [{ id: 'a1x', href: 'a1x.xhtml', label: 'Seção' }] }, { id: 'a2', href: 'a2.xhtml', label: 'Capítulo 2' }] },
      { id: 'b', href: 'b.xhtml', label: 'Parte II' },
    ];
    expect(flattenToc(toc)).toEqual([
      { id: 'a', href: 'a.xhtml', label: 'Parte I', depth: 0 },
      { id: 'a1', href: 'a1.xhtml', label: 'Capítulo 1', depth: 1 },
      { id: 'a1x', href: 'a1x.xhtml', label: 'Seção', depth: 2 },
      { id: 'a2', href: 'a2.xhtml', label: 'Capítulo 2', depth: 1 },
      { id: 'b', href: 'b.xhtml', label: 'Parte II', depth: 0 },
    ]);
  });

  it('is empty for no book or a book without one, and an entry without a name has an empty one', () => {
    expect(flattenToc(undefined)).toEqual([]);
    expect(flattenToc([])).toEqual([]);
    expect(flattenToc([{ id: 'x', href: 'x.xhtml' }])).toEqual([{ id: 'x', href: 'x.xhtml', label: '', depth: 0 }]);
  });
});
