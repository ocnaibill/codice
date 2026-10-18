import { describe, expect, it } from 'vitest';
import { MAX_RULER, readThrough, rulerChips, rulerLabel, rulerOf, spanText } from './ruler';

const vol = (id, n, from, to, completed = false) => ({ id, title: `Vol ${n}`, unit: 'volume', position: n, coversFrom: from, coversTo: to, completed });
const chap = (id, n, completed = false) => ({ id, title: `Cap ${n}`, unit: 'chapter', position: n, completed });
const story = (extra = {}) => ({ ranged: true, chapters: 20, read: 9, gaps: [{ from: 10, to: 17 }], readSpans: [{ from: 1, to: 9 }], ...extra });
const works = [vol(1, 1, 1, 9, true), vol(2, 3, 18, 26), chap(31, 31), chap(32, 32)];

describe('rulerOf', () => {
  it('has nothing to draw when no work says what it contains, or there is nothing in the story', () => {
    expect(rulerOf(works, { ranged: false, chapters: 20 })).toBeNull();
    expect(rulerOf(works, null)).toBeNull();
    expect(rulerOf([], story())).toBeNull();
    expect(rulerOf(works, story({ chapters: 0 }))).toBeNull();
  });

  it('has a cell for each whole chapter, from the first to the last, read, in the library or in no file', () => {
    const r = rulerOf(works, story({ gaps: [{ from: 10, to: 17 }, { from: 27, to: 30 }] }));
    expect([r.first, r.last, r.cells.length]).toEqual([1, 32, 32]);
    const state = (n) => r.cells.find((c) => c.n === n).state;
    expect([state(1), state(9), state(10), state(17), state(18), state(27), state(31)]).toEqual(['read', 'read', 'gap', 'gap', 'have', 'gap', 'have']);
  });

  it('tells the files that hold many chapters from the chapters that are a file of their own', () => {
    const r = rulerOf(works, story());
    expect(r.volumes.map((v) => [v.id, v.from, v.to, v.done])).toEqual([[1, 1, 9, true], [2, 18, 26, false]]);
    expect(r.loose.map((c) => c.id)).toEqual([31, 32]);
  });

  it('leaves the complementary works out, and draws no ruler for a story too long to read as one', () => {
    expect(rulerOf([...works, { id: 9, unit: 'extra', position: 99, coversFrom: 1, coversTo: 99 }], story()).last).toBe(32);
    expect(rulerOf([vol(1, 1, 1, MAX_RULER + 5)], story())).toBeNull();
  });
});

describe('the words of the ruler', () => {
  it('says what was read, what the library has and what no file contains', () => {
    const r = rulerOf(works, story({ gaps: [{ from: 10, to: 17 }, { from: 27, to: 30 }] }));
    expect(rulerChips(r).map((c) => c.text)).toEqual([
      'Lido: caps. 1 a 9',
      'No acervo: 20 de 32 capítulos',
      'Sem arquivo: caps. 10 a 17 · caps. 27 a 30',
      '2 arquivos que reúnem capítulos · 2 capítulos avulsos',
    ]);
  });

  it('says nothing was read when nothing was', () => {
    const r = rulerOf(works, story({ readSpans: [], read: 0 }));
    expect(rulerChips(r).some((c) => c.text.startsWith('Lido'))).toBe(false);
  });

  it('puts the mark after the last chapter read from the beginning, and nowhere when the reading did not start there', () => {
    expect(readThrough(rulerOf(works, story()))).toBe(9);
    expect(readThrough(rulerOf(works, story({ readSpans: [{ from: 18, to: 26 }] })))).toBeNull();
    expect(readThrough(rulerOf(works, story({ readSpans: [] })))).toBeNull();
  });

  it('names a stretch and a work', () => {
    expect(spanText({ from: 5, to: 5 })).toBe('cap. 5');
    expect(spanText({ from: 5, to: 9 })).toBe('caps. 5 a 9');
    expect(rulerLabel(vol(1, 3, 1, 2))).toBe('Vol. 3');
    expect(rulerLabel(chap(1, 12))).toBe('Cap. 12');
    expect(rulerLabel({ title: 'Watchmen', unit: 'oneshot' })).toBe('Watchmen');
  });
});
