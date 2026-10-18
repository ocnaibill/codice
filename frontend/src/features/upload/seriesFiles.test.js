import { describe, expect, it } from 'vitest';
import {
  MAX_SERIES_FILES, deriveRows, folderOf, gaps, guessSeries, listNumbers, numberFor, parseTyped, pickSeriesFiles, readNumber, runPool, sortRows, summarize, titleFor,
} from './seriesFiles';

const file = (name, size = 10, lastModified = 1) => ({ name, size, lastModified });
const rowsOf = (names, extra = {}) => names.map((name, i) => ({ id: i + 1, name, read: readNumber(name), typedNumber: null, typedTitle: null, ...extra }));

describe('readNumber', () => {
  it('reads the number a name gives to a chapter, in the ways people write it', () => {
    const chapter = (name) => readNumber(name).chapter;
    expect(chapter('Jujutsu Kaisen Modelo - Cap 1.cbz')).toBe(1);
    expect(chapter('Jujutsu Kaisen Modelo - Cap. 02.cbz')).toBe(2);
    expect(chapter('Capítulo 033.cbz')).toBe(33);
    expect(chapter('Berserk Chapter 12.cbr')).toBe(12);
    expect(chapter('Berserk ch 12.cbr')).toBe(12);
    expect(chapter('Berserk c012.cbz')).toBe(12);
    expect(chapter('Berserk - Cap 10.5.cbz')).toBe(10.5);
    expect(chapter('Berserk - Cap 10,5.cbz')).toBe(10.5);
    expect(chapter('Berserk_Cap_7.cbz')).toBe(7);
  });

  it('reads volumes, and both when a name has both', () => {
    expect(readNumber('Akira Vol 3.cbz').volume).toBe(3);
    expect(readNumber('Akira Volume 03.cbz').volume).toBe(3);
    expect(readNumber('Akira v03.cbz').volume).toBe(3);
    const both = readNumber('Berserk v03 c012.cbz');
    expect([both.volume, both.chapter]).toEqual([3, 12]);
  });

  it('reads a bare number: after a hash, or the last one in the name', () => {
    expect(readNumber('Batman #7.cbz').bare).toBe(7);
    expect(readNumber('One Piece 1001.cbz').bare).toBe(1001);
    expect(readNumber('Dr Stone 2 - 15.cbz').bare).toBe(15);
  });

  it('does not take a word for a marker', () => {
    expect(readNumber('Beach 3.cbz').chapter).toBeNull();
    expect(readNumber('Capitão América.cbz')).toMatchObject({ chapter: null, volume: null, bare: null });
    expect(readNumber('Voice.cbz').volume).toBeNull();
  });

  it('has no number when the name has none', () => {
    expect(readNumber('Sem numero.cbz')).toMatchObject({ chapter: null, volume: null, bare: null });
  });
});

describe('numberFor', () => {
  it('follows the unit the person chose, and reads each file by its name when they did not', () => {
    const both = readNumber('Berserk v03 c012.cbz');
    expect(numberFor(both, 'chapter')).toEqual({ number: 12, unit: 'chapter', volumeNumber: 3, covers: null });
    expect(numberFor(both, 'volume')).toEqual({ number: 3, unit: 'volume', volumeNumber: null, covers: null });
    expect(numberFor(readNumber('Akira Vol 3.cbz'), 'auto')).toEqual({ number: 3, unit: 'volume', volumeNumber: null, covers: null });
    expect(numberFor(readNumber('Akira 7.cbz'), 'auto')).toEqual({ number: 7, unit: 'chapter', volumeNumber: null, covers: null });
    expect(numberFor(readNumber('Akira 7.cbz'), 'volume')).toMatchObject({ number: 7, unit: 'volume' });
  });

  it('says the volume a chapter is in, when the name says it (DEC-202)', () => {
    expect(numberFor(readNumber('Berserk v03 c012.cbz'), 'auto')).toMatchObject({ number: 12, unit: 'chapter', volumeNumber: 3 });
    expect(numberFor(readNumber('Berserk - Cap 12.cbz'), 'auto').volumeNumber).toBeNull();
  });

  it('says the chapters a file holds: a volume with its range, a chapter that joins two', () => {
    const volume = numberFor(readNumber('Berserk Vol 3 Caps 18-26.cbz'), 'auto');
    expect(volume).toEqual({ number: 3, unit: 'volume', volumeNumber: null, covers: { from: 18, to: 26 } });
    expect(numberFor(readNumber('Berserk Vol 3 (18-26).cbz'), 'auto').covers).toEqual({ from: 18, to: 26 });
    expect(numberFor(readNumber('Berserk Cap 12-13.cbz'), 'auto')).toMatchObject({ number: 12, unit: 'chapter', covers: { from: 12, to: 13 } });
    expect(numberFor(readNumber('Berserk Chapters 1 a 9.cbz'), 'volume').covers).toEqual({ from: 1, to: 9 });
  });

  it('takes no range from a name that has none, or from one that goes backwards', () => {
    expect(readNumber('Berserk Cap 12.cbz').range).toBeNull();
    expect(readNumber('Berserk Cap 13-12.cbz').range).toBeNull();
    expect(readNumber('Berserk 2020-2021 Cap 5.cbz').range).toBeNull();
  });
});

describe('guessSeries', () => {
  it('is what comes before the number in most of the names', () => {
    expect(guessSeries(['Jujutsu Kaisen Modelo - Cap 1.cbz', 'Jujutsu Kaisen Modelo - Cap 2.cbz', 'Jujutsu Kaisen Modelo - Cap 3.cbz'])).toBe('Jujutsu Kaisen Modelo');
    expect(guessSeries(['Berserk_Cap_1.cbz', 'Berserk_Cap_2.cbz', 'Outro Cap 3.cbz'])).toBe('Berserk');
  });

  it('is the folder when the names say nothing, and nothing when there is no folder', () => {
    expect(guessSeries(['Cap 1.cbz', 'Cap 2.cbz'], 'Jujutsu Kaisen')).toBe('Jujutsu Kaisen');
    expect(guessSeries(['001.cbz', '002.cbz'])).toBe('');
  });
});

describe('titles and typed numbers', () => {
  it('writes "Série - Cap. N" by default', () => {
    expect(titleFor('Jujutsu', 'chapter', 1)).toBe('Jujutsu - Cap. 1');
    expect(titleFor('Jujutsu', 'chapter', 10.5)).toBe('Jujutsu - Cap. 10.5');
    expect(titleFor('Akira', 'volume', 3)).toBe('Akira - Vol. 3');
    expect(titleFor('', 'chapter', 3)).toBe('Cap. 3');
    expect(titleFor('X', 'chapter', null, 'Sem número_final.cbz')).toBe('Sem número final');
  });

  it('takes a typed number only when it is above zero, with a comma or a point', () => {
    expect(parseTyped('12')).toBe(12);
    expect(parseTyped(' 10,5 ')).toBe(10.5);
    for (const bad of ['', '0', 'a', '-1', '1e3', '1.234']) expect(parseTyped(bad), bad).toBeNull();
  });
});

describe('titles of what holds many chapters', () => {
  it('writes "Cap. 12-13" for a chapter that joins two and keeps a volume as its number', () => {
    expect(titleFor('S', 'chapter', 12, 'x.cbz', { from: 12, to: 13 })).toBe('S - Cap. 12-13');
    expect(titleFor('S', 'volume', 3, 'x.cbz', { from: 18, to: 26 })).toBe('S - Vol. 3');
    expect(titleFor('S', 'chapter', 12, 'x.cbz')).toBe('S - Cap. 12');
  });

  it('gives each row its volume and its range, and the same number in two units is still two works', () => {
    const rows = deriveRows(rowsOf(['S v03 c012.cbz', 'S Vol 3 Caps 18-26.cbz']), { series: 'S', unit: 'auto' });
    expect(rows[0]).toMatchObject({ unit: 'chapter', number: 12, volumeNumber: 3, situation: 'new' });
    expect(rows[1]).toMatchObject({ unit: 'volume', number: 3, covers: { from: 18, to: 26 }, situation: 'new', title: 'S - Vol. 3' });
  });
});

describe('deriveRows and summarize', () => {
  const names = ['S - Cap 1.cbz', 'S - Cap 2.cbz', 'S - Cap 2 (b).cbz', 'S - Cap 4.cbz', 'S - extra.cbz'];

  it('marks the repeated number, the missing one and the new ones', () => {
    const rows = deriveRows(rowsOf(names), { series: 'S', unit: 'chapter' });
    expect(rows.map((r) => r.situation)).toEqual(['new', 'repeated', 'repeated', 'new', 'missing']);
    expect(rows[0].title).toBe('S - Cap. 1');
    const s = summarize(rows);
    expect(s).toMatchObject({ total: 5, withNumber: 4, withoutNumber: 1, repeated: [2], repeatedRows: 2, missing: [3], first: 1, last: 4, sendable: 2 });
  });

  it('lets a typed number fix a repeat, and a typed title stay when the series changes', () => {
    const base = rowsOf(names);
    base[2] = { ...base[2], typedNumber: '3' };
    base[0] = { ...base[0], typedTitle: 'Primeiro' };
    const rows = deriveRows(base, { series: 'Outra', unit: 'chapter' });
    expect(rows[2]).toMatchObject({ number: 3, situation: 'new', title: 'Outra - Cap. 3' });
    expect(rows[0].title).toBe('Primeiro');
    expect(summarize(rows).repeated).toEqual([]);
  });

  it('does not call a chapter 2 and a volume 2 the same number', () => {
    const rows = deriveRows(rowsOf(['S Vol 2.cbz', 'S Cap 2.cbz']), { series: 'S', unit: 'auto' });
    expect(rows.map((r) => r.situation)).toEqual(['new', 'new']);
  });
});

describe('gaps and listNumbers', () => {
  it('lists the whole numbers skipped between the first and the last', () => {
    expect(gaps([{ number: 1, unit: 'chapter' }, { number: 4, unit: 'chapter' }, { number: 2.5, unit: 'chapter' }])).toEqual([2, 3]);
    expect(gaps([])).toEqual([]);
  });

  it('says them as a sentence would', () => {
    expect(listNumbers([57])).toBe('57');
    expect(listNumbers([57, 58])).toBe('57 e 58');
    expect(listNumbers([1, 2, 3])).toBe('1, 2 e 3');
    expect(listNumbers([1, 2, 3, 4, 5, 6, 7, 8, 9])).toBe('1, 2, 3, 4, 5 e mais 4');
  });
});

describe('sortRows', () => {
  it('reads in the order of the series: by number, the ones with none last', () => {
    const rows = deriveRows(rowsOf(['S Cap 10.cbz', 'S Cap 2.cbz', 'S extra.cbz', 'S Cap 1.cbz']), { series: 'S', unit: 'chapter' });
    expect(sortRows(rows).map((r) => r.name)).toEqual(['S Cap 1.cbz', 'S Cap 2.cbz', 'S Cap 10.cbz', 'S extra.cbz']);
  });
});

describe('pickSeriesFiles and folderOf', () => {
  it('keeps the formats of a series, drops the repeated, and stops at the limit', () => {
    const files = [file('a.cbz'), file('b.epub'), file('a.cbz'), file('c.PDF')];
    const picked = pickSeriesFiles(files);
    expect(picked.kept.map((f) => f.name)).toEqual(['a.cbz', 'c.PDF']);
    expect([picked.other, picked.repeated, picked.over]).toEqual([1, 1, 0]);
    const many = Array.from({ length: MAX_SERIES_FILES + 3 }, (_, i) => file(`c${i}.cbz`, 10, i));
    expect(pickSeriesFiles(many)).toMatchObject({ over: 3 });
    expect(pickSeriesFiles(many).kept).toHaveLength(MAX_SERIES_FILES);
    expect(pickSeriesFiles([file('z.cbz', 1, 99)], many.slice(0, MAX_SERIES_FILES)).over).toBe(1);
  });

  it('names the folder a file came from', () => {
    expect(folderOf({ webkitRelativePath: 'Jujutsu Kaisen/Cap 1.cbz' })).toBe('Jujutsu Kaisen');
    expect(folderOf({ webkitRelativePath: '' })).toBe('');
    expect(folderOf({})).toBe('');
  });
});

describe('runPool', () => {
  it('runs at most `limit` at once, in the order of the list', async () => {
    let going = 0;
    let most = 0;
    const started = [];
    await runPool([1, 2, 3, 4, 5, 6, 7], async (n) => {
      started.push(n);
      going += 1;
      most = Math.max(most, going);
      await new Promise((r) => setTimeout(r, 5));
      going -= 1;
    }, 3);
    expect(most).toBe(3);
    expect(started).toEqual([1, 2, 3, 4, 5, 6, 7]);
  });

  it('stops taking new ones when told, lets the ones going finish, and returns what never started', async () => {
    let stop = false;
    const done = [];
    const left = await runPool([1, 2, 3, 4, 5], async (n) => {
      if (n === 2) stop = true;
      await new Promise((r) => setTimeout(r, 5));
      done.push(n);
    }, 2, () => stop);
    expect(done.sort()).toEqual([1, 2]);
    expect(left).toEqual([3, 4, 5]);
  });

  it('goes on after one that fails to be handled by the worker itself', async () => {
    const seen = [];
    await runPool([1, 2, 3], async (n) => { seen.push(n); }, 5);
    expect(seen).toEqual([1, 2, 3]);
  });
});
