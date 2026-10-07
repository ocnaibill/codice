import { describe, it, expect } from 'vitest';
import { collectionLine, worksText } from './text';

describe('the text of a collection', () => {
  it('counts the works, in the singular too', () => {
    expect(worksText(1)).toBe('1 obra');
    expect(worksText(2)).toBe('2 obras');
    expect(worksText(0)).toBe('0 obras');
  });

  it('says what is in it and how much was read, and only when something was', () => {
    expect(collectionLine({ workCount: 12, completedCount: 3 })).toBe('12 obras · Leu 3 de 12');
    expect(collectionLine({ workCount: 12, completedCount: 0 })).toBe('12 obras');
    expect(collectionLine({ workCount: 1, completedCount: 1 })).toBe('1 obra · Leu 1 de 1');
    expect(collectionLine({ workCount: 0, completedCount: 0 })).toBe('Nenhuma obra ainda');
    expect(collectionLine({})).toBe('Nenhuma obra ainda');
  });
});
