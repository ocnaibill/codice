import { describe, it, expect } from 'vitest';
import { collectionLine, wordsOf, worksText } from './text';

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

describe('the words of each kind', () => {
  it('speak of a list where it is the person\'s, and of a collection where it is the library\'s', () => {
    expect(wordsOf('personal').thing).toBe('lista');
    expect(wordsOf('official').thing).toBe('coleção');
    expect(wordsOf(undefined).thing).toBe('coleção');
  });

  it('say what happens to the works, which is not the same: a list leaves them alone, a collection writes their series', () => {
    expect(wordsOf('personal').removeNote).toContain('A obra continua no acervo');
    expect(wordsOf('personal').renameNote).toContain('Só o nome da lista muda');
    expect(wordsOf('official').removeNote).toContain('A série da obra é limpa e travada');
    expect(wordsOf('official').renameNote).toContain('passam a ter esse nome como série');
  });

  it('have every sentence for both', () => {
    expect(Object.keys(wordsOf('personal')).sort()).toEqual(Object.keys(wordsOf('official')).sort());
    for (const kind of ['personal', 'official']) for (const text of Object.values(wordsOf(kind))) expect(text.length).toBeGreaterThan(3);
  });
});
