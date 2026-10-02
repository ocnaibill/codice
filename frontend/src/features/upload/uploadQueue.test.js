import { describe, expect, it } from 'vitest';
import { MAX_FILES, STATUS, placesTaken, planAdd, queueReducer } from './uploadQueue';

let stamp = 0;
const file = (name, size = 100, lastModified = ++stamp) => ({ name, size, lastModified });
const item = (id, name, status = STATUS.QUEUED, extra = {}) => ({ id, file: file(name), status, progress: 0, message: '', ...extra });

describe('how many files the list takes', () => {
  it('is five', () => {
    expect(MAX_FILES).toBe(5);
  });
});

describe('what adding files to the list does', () => {
  it('makes a queued row of each file that is taken, numbered from the id it is given, in order', () => {
    const { added, overCap, repeated } = planAdd([], [file('a.epub'), file('b.pdf')], 7);
    expect(added.map((r) => [r.id, r.file.name, r.status, r.progress, r.message])).toEqual([[7, 'a.epub', 'queued', 0, ''], [8, 'b.pdf', 'queued', 0, '']]);
    expect([overCap, repeated]).toEqual([0, 0]);
  });

  it('makes a refused row of a type that is not taken, with the reason, and queues the rest', () => {
    const { added } = planAdd([], [file('programa.exe'), file('semextensao'), file('Duna.EPUB')], 1);
    expect(added.map((r) => r.status)).toEqual(['refused', 'refused', 'queued']);
    expect(added[0].message).toBe('Esse tipo de arquivo não é aceito (.exe).');
    expect(added[1].message).toBe('Esse tipo de arquivo não é aceito.');
    expect(added[2].message).toBe('');
  });

  it('keeps the first ones when there are more than the list takes, and counts the rest', () => {
    const files = [1, 2, 3, 4, 5, 6, 7].map((n) => file(`${n}.epub`));
    const { added, overCap } = planAdd([], files, 1);
    expect(added.map((r) => r.file.name)).toEqual(['1.epub', '2.epub', '3.epub', '4.epub', '5.epub']);
    expect(overCap).toBe(2);
  });

  it('counts the rows already on the list against the five', () => {
    const items = [item(1, 'a.epub'), item(2, 'b.epub'), item(3, 'c.epub'), item(4, 'd.epub')];
    const { added, overCap } = planAdd(items, [file('e.epub'), file('f.epub')], 5);
    expect(added.map((r) => r.file.name)).toEqual(['e.epub']);
    expect(overCap).toBe(1);
    expect(planAdd([...items, item(5, 'e.epub')], [file('f.epub')], 6)).toMatchObject({ added: [], overCap: 1 });
  });

  it('counts a refused row against the five too', () => {
    const { added, overCap } = planAdd([], [file('1.exe'), file('2.exe'), file('3.exe'), file('4.exe'), file('5.exe'), file('6.epub')], 1);
    expect(added).toHaveLength(5);
    expect(overCap).toBe(1);
  });

  it('leaves out a file that is already there (same name, size and date) and counts it', () => {
    const duna = file('Duna.epub', 100, 5);
    const { added, repeated, overCap } = planAdd([{ id: 1, file: duna, status: STATUS.QUEUED, progress: 0, message: '' }], [{ ...duna }, file('Outro.epub')], 2);
    expect(added.map((r) => r.file.name)).toEqual(['Outro.epub']);
    expect(repeated).toBe(1);
    expect(overCap).toBe(0);
  });

  it('takes a file with the same name when its size or its date is another', () => {
    const base = file('Duna.epub', 100, 5);
    const rows = [{ id: 1, file: base, status: STATUS.QUEUED, progress: 0, message: '' }];
    expect(planAdd(rows, [file('Duna.epub', 101, 5)], 2).added).toHaveLength(1);
    expect(planAdd(rows, [file('Duna.epub', 100, 6)], 2).added).toHaveLength(1);
    expect(planAdd(rows, [file('Outro.epub', 100, 5)], 2).added).toHaveLength(1);
  });

  it('counts a file that comes twice in the same choice once', () => {
    const duna = file('Duna.epub', 100, 5);
    const { added, repeated } = planAdd([], [duna, { ...duna }, { ...duna }], 1);
    expect(added).toHaveLength(1);
    expect(repeated).toBe(2);
  });

  it('is nothing for no files', () => {
    expect(planAdd([item(1, 'a.epub')], [], 2)).toEqual({ added: [], overCap: 0, repeated: 0 });
  });

  it('does not change the list it is given', () => {
    const items = [item(1, 'a.epub')];
    planAdd(items, [file('b.epub')], 2);
    expect(items).toHaveLength(1);
  });
});

describe('what happens to the rows', () => {
  const rows = () => [item(1, 'a.epub'), item(2, 'b.epub', STATUS.DONE), item(3, 'c.epub')];

  it('adds rows at the end', () => {
    expect(queueReducer(rows(), { type: 'append', items: [item(4, 'd.epub')] }).map((r) => r.id)).toEqual([1, 2, 3, 4]);
  });
  it('changes some fields of one row and leaves the rest as they are', () => {
    const before = rows();
    const next = queueReducer(before, { type: 'patch', id: 3, patch: { status: STATUS.UPLOADING, progress: 40 } });
    expect(next[2]).toMatchObject({ id: 3, status: 'uploading', progress: 40, message: '' });
    expect(next[0]).toBe(before[0]);
    expect(next[1]).toBe(before[1]);
    expect(queueReducer(before, { type: 'patch', id: 99, patch: { status: 'error' } })).toEqual(before);
  });
  it('takes one row away', () => {
    expect(queueReducer(rows(), { type: 'remove', id: 2 }).map((r) => r.id)).toEqual([1, 3]);
  });
  it('cancels the rows that are waiting, and only them', () => {
    const next = queueReducer([...rows(), item(4, 'd.epub', STATUS.UPLOADING), item(5, 'e.epub', STATUS.ERROR)], { type: 'cancelQueued' });
    expect(next.map((r) => r.status)).toEqual(['cancelled', 'done', 'cancelled', 'uploading', 'error']);
  });
  it('drops the rows that were sent, and only them', () => {
    const next = queueReducer([item(1, 'a.epub', STATUS.DONE), item(2, 'b.epub', STATUS.ERROR), item(3, 'c.epub', STATUS.DONE), item(4, 'd.epub')], { type: 'dropDone' });
    expect(next.map((r) => r.id)).toEqual([2, 4]);
  });
  it('counts the places that are taken: every row but the ones that were sent', () => {
    expect(placesTaken([])).toBe(0);
    expect(placesTaken([item(1, 'a.epub', STATUS.DONE), item(2, 'b.epub', STATUS.ERROR), item(3, 'c.epub', STATUS.REFUSED), item(4, 'd.epub'), item(5, 'e.epub', STATUS.DUPLICATE)])).toBe(4);
  });
  it('empties the list', () => {
    expect(queueReducer(rows(), { type: 'reset' })).toEqual([]);
  });
  it('does not know what it was not told', () => {
    const before = rows();
    expect(queueReducer(before, { type: 'what' })).toBe(before);
  });
});
