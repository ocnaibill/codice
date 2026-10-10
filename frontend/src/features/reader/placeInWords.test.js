import { describe, it, expect } from 'vitest';
import { chapterAt, epubChapterEntries, epubUnit, pageUnit, placeLabel, whereExtras } from './placeInWords';

describe('chapterAt', () => {
  const entries = [
    { title: 'Capa', at: 1 },
    { title: 'Parte I', at: 5 },
    { title: 'Capítulo 1', at: 5 },
    { title: 'Capítulo 2', at: 20 },
  ];

  it('is the entry that started last at or before the place', () => {
    expect(chapterAt(entries, 4)).toBe('Capa');
    expect(chapterAt(entries, 19)).toBe('Capítulo 1');
    expect(chapterAt(entries, 20)).toBe('Capítulo 2');
    expect(chapterAt(entries, 900)).toBe('Capítulo 2');
  });

  it('says nothing before the first entry, or when the place is not known', () => {
    expect(chapterAt(entries, 0)).toBeUndefined();
    expect(chapterAt(entries, undefined)).toBeUndefined();
    expect(chapterAt(entries, NaN)).toBeUndefined();
    expect(chapterAt([], 3)).toBeUndefined();
    expect(chapterAt(undefined, 3)).toBeUndefined();
  });

  it('chooses between entries that start together by what the place can tell', () => {
    expect(chapterAt(entries, 7, 'last')).toBe('Capítulo 1'); // a PDF page that is there has begun them both
    expect(chapterAt(entries, 7, 'first')).toBe('Parte I'); // an EPUB file may hold anchors the person has not reached
  });

  it('skips entries without a title or a place', () => {
    expect(chapterAt([{ title: '', at: 1 }, { title: 'Sem lugar', at: undefined }, { title: 'Um', at: 2 }], 9)).toBe('Um');
  });
});

describe('epubChapterEntries', () => {
  const items = [
    { href: 'OEBPS/cap1.xhtml', index: 0 },
    { href: 'OEBPS/cap2.xhtml', index: 1 },
  ];
  const spine = { spineItems: items, get: (href) => items.find((i) => i.href === href) };

  it('puts each entry of the table of contents at the index of its file', () => {
    const toc = [
      { label: 'Um', href: 'OEBPS/cap1.xhtml' },
      { label: 'Dois', href: 'OEBPS/cap2.xhtml#inicio' },
    ];
    expect(epubChapterEntries(toc, spine)).toEqual([{ title: 'Um', at: 0 }, { title: 'Dois', at: 1 }]);
  });

  it('finds a file the contents give relative to themselves', () => {
    expect(epubChapterEntries([{ label: 'Dois', href: 'cap2.xhtml' }], spine)).toEqual([{ title: 'Dois', at: 1 }]);
  });

  it('finds a file the package gives relative to the contents, which sit deeper', () => {
    expect(epubChapterEntries([{ label: 'Um', href: 'OEBPS/Text/cap1.xhtml' }], { spineItems: [{ href: 'Text/cap1.xhtml', index: 4 }], get: () => null })).toEqual([
      { title: 'Um', at: 4 },
    ]);
  });

  it('leaves out what it cannot place or has no name', () => {
    const toc = [{ label: 'Perdido', href: 'outro.xhtml' }, { label: '', href: 'OEBPS/cap1.xhtml' }, { label: 'Sem href' }];
    expect(epubChapterEntries(toc, spine)).toEqual([]);
    expect(epubChapterEntries(undefined, undefined)).toEqual([]);
  });
});

describe('epubUnit', () => {
  const locations = (length, at) => ({ length: () => length, locationFromCfi: () => at });

  it('counts the positions from 1', () => {
    expect(epubUnit(locations(840, 0), 'epubcfi(/6/2)')).toEqual({ index: 1, total: 840 });
    expect(epubUnit(locations(840, 119), 'epubcfi(/6/2)')).toEqual({ index: 120, total: 840 });
  });

  it('never goes past the end', () => {
    expect(epubUnit(locations(840, 900), 'epubcfi(/6/2)')).toEqual({ index: 840, total: 840 });
  });

  it('says nothing while the positions are not known', () => {
    expect(epubUnit(locations(0, 0), 'epubcfi(/6/2)')).toBeUndefined();
    expect(epubUnit(locations(840, -1), 'epubcfi(/6/2)')).toBeUndefined();
    expect(epubUnit(locations(840, 3), '')).toBeUndefined();
    expect(epubUnit(undefined, 'epubcfi(/6/2)')).toBeUndefined();
  });
});

describe('pageUnit', () => {
  it('is the page of the total, from 1', () => {
    expect(pageUnit(42, 310)).toEqual({ index: 42, total: 310 });
    expect(pageUnit(1, 1)).toEqual({ index: 1, total: 1 });
  });

  it('says nothing for a page that is not one of the file', () => {
    expect(pageUnit(0, 10)).toBeUndefined();
    expect(pageUnit(11, 10)).toBeUndefined();
    expect(pageUnit(3, 0)).toBeUndefined();
    expect(pageUnit(3, undefined)).toBeUndefined();
    expect(pageUnit(1.5, 10)).toBeUndefined();
  });
});

describe('whereExtras', () => {
  it('is what the server takes: the chapter in one line and the unit', () => {
    expect(whereExtras({ chapter: '  Parte II:\n  O  Deserto ', unit: { index: 3, total: 9 } })).toEqual({
      chapter: 'Parte II: O Deserto',
      unitIndex: 3,
      unitTotal: 9,
    });
  });

  it('is nothing for what is not known, so that the server does not keep what the place no longer is', () => {
    expect(whereExtras()).toEqual({});
    expect(whereExtras({ chapter: '   ', unit: undefined })).toEqual({});
  });

  it('cuts a title at the length the server keeps', () => {
    expect(whereExtras({ chapter: 'x'.repeat(500) }).chapter).toHaveLength(200);
  });
});

describe('placeLabel', () => {
  it('says the page of how many for a PDF and a comic, and the position of how many for the rest', () => {
    expect(placeLabel('pdf', { unitIndex: 42, unitTotal: 310 })).toBe('Página 42 de 310');
    expect(placeLabel('cbz', { unitIndex: 5, unitTotal: 32 })).toBe('Página 5 de 32');
    expect(placeLabel('epub', { unitIndex: 3412, unitTotal: 5018 })).toBe('Pos. 3.412 de 5.018');
  });

  it('falls back to the page of an older save for a PDF (from 1) and a comic (from 0), and to nothing for the rest', () => {
    expect(placeLabel('pdf', {}, '42')).toBe('Página 42');
    expect(placeLabel('cbr', undefined, '4')).toBe('Página 5');
    expect(placeLabel('epub', {}, 'epubcfi(/6/2)')).toBeNull();
    expect(placeLabel('txt', {}, '1534')).toBeNull();
    expect(placeLabel('pdf', {}, 'abc')).toBeNull();
  });

  it('does not say a place that is past the end', () => {
    expect(placeLabel('pdf', { unitIndex: 400, unitTotal: 310 }, '42')).toBe('Página 42');
  });
});
