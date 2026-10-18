import { describe, expect, it } from 'vitest';
import { endFromCount, rangeCount, suggestRange } from './coversRange';

describe('rangeCount', () => {
  it('counts the chapters of a whole range, with a comma or a point', () => {
    expect(rangeCount('18', '26')).toBe(9);
    expect(rangeCount('1', '1')).toBe(1);
    expect(rangeCount('10,5', '12')).toBe(2);
  });

  it('has no count for what is not a range', () => {
    for (const [from, to] of [['', '5'], ['5', ''], ['9', '3'], ['a', '3'], ['0', '3']]) expect(rangeCount(from, to), `${from}-${to}`).toBeNull();
  });
});

describe('endFromCount', () => {
  it('gives the last chapter from the first and the quantity', () => {
    expect(endFromCount('18', '9')).toBe('26');
    expect(endFromCount('1', '1')).toBe('1');
    expect(endFromCount('', '9')).toBe('');
    expect(endFromCount('18', '0')).toBe('');
    expect(endFromCount('18', 'x')).toBe('');
  });
});

describe('suggestRange', () => {
  const works = [
    { id: 1, unit: 'chapter', position: 18, volumeNumber: 3 },
    { id: 2, unit: 'chapter', position: 26, volumeNumber: 3 },
    { id: 3, unit: 'chapter', position: 27, volumeNumber: 4 },
    { id: 4, unit: 'volume', position: 3, volumeNumber: 3 },
  ];

  it('is from the first to the last chapter that says it is in the volume', () => {
    expect(suggestRange(works, 3)).toEqual({ from: 18, to: 26, chapters: 2 });
  });

  it('is nothing when no chapter says it, or the volume has no number', () => {
    expect(suggestRange(works, 9)).toBeNull();
    expect(suggestRange(works, null)).toBeNull();
    expect(suggestRange(undefined, 3)).toBeNull();
  });
});
