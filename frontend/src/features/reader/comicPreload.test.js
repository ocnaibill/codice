import { describe, expect, it } from 'vitest';
import { preloadOrder } from './comicPreload';

describe('which pages of a comic are asked for, and in what order (#181)', () => {
  it('asks for the page on the screen first, then the ones ahead nearest first, then the ones behind nearest first', () => {
    expect(preloadOrder({ current: 5, shown: 1, count: 2, total: 20 })).toEqual([5, 6, 7, 4, 3]);
  });

  it('asks for the two pages that are shown before any other, in the double view', () => {
    const order = preloadOrder({ current: 6, shown: 2, count: 2, total: 20 });
    expect(order.slice(0, 2)).toEqual([6, 7]);
    // the pages behind come after the ones ahead: a person reads forward
    expect(order).toEqual([6, 7, 8, 9, 10, 11, 5, 4]);
  });

  it('brings, in the double view, as far ahead as `count` turns of the page', () => {
    const order = preloadOrder({ current: 0, shown: 2, count: 3, total: 50 });
    expect(order).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);
  });

  it('stops at the first and the last page, and has no page twice', () => {
    expect(preloadOrder({ current: 0, shown: 1, count: 2, total: 3 })).toEqual([0, 1, 2]);
    expect(preloadOrder({ current: 2, shown: 1, count: 2, total: 3 })).toEqual([2, 1, 0]);
    const order = preloadOrder({ current: 9, shown: 2, count: 2, total: 10 });
    expect(order).toEqual([9, 8, 7]);
    expect(new Set(order).size).toBe(order.length);
  });

  it('a comic of one page has one page, and an empty one has none', () => {
    expect(preloadOrder({ current: 0, shown: 2, count: 2, total: 1 })).toEqual([0]);
    expect(preloadOrder({ current: 0, shown: 1, count: 2, total: 0 })).toEqual([]);
  });

  it('has the usual values when it is only given where it is', () => {
    expect(preloadOrder({ current: 3, total: 10 })).toEqual([3, 4, 5, 2, 1]);
  });
});
