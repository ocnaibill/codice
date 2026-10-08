import { describe, it, expect } from 'vitest';
import { collectionLine, COMIC_KINDS, goOnText, groupByUnit, newText, numberText, seriesCounts, stepText, UNITS, unitLabel, wordsOf, worksText } from './text';

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

describe('the units of a series (#187)', () => {
  it('lists the three units and the two kinds, in the order they are shown', () => {
    expect(UNITS.map((u) => u.key)).toEqual(['volume', 'chapter', 'oneshot']);
    expect(COMIC_KINDS.map((k) => [k.key, k.one])).toEqual([['manga', 'Mangá'], ['comic', 'Quadrinho']]);
  });

  it('writes a number the way it is read: a comma for the half, a dash for none', () => {
    expect(numberText(3)).toBe('3');
    expect(numberText(27.5)).toBe('27,5');
    expect(numberText(null)).toBe('—');
    expect(numberText(undefined)).toBe('—');
    expect(numberText(0)).toBe('0');
  });

  it('calls a work by its unit and number', () => {
    expect(unitLabel('volume', 3)).toBe('Vol. 3');
    expect(unitLabel('chapter', 27.5)).toBe('Cap. 27,5');
    expect(unitLabel('chapter', null)).toBe('Cap. —');
    expect(unitLabel('oneshot', 1)).toBe('Único');
    expect(unitLabel('', 4)).toBe('4');
    expect(unitLabel(undefined, null)).toBe('—');
    expect(unitLabel('arc', 2)).toBe('2');
  });

  it('groups the works by unit, volumes first and those with no unit last, each keeping its order', () => {
    const w = (id, unit) => ({ id, unit });
    const { groups, headings } = groupByUnit([w(1, 'chapter'), w(2, ''), w(3, 'volume'), w(4, 'chapter'), w(5, undefined), w(6, 'oneshot'), w(7, 'volume')]);
    expect(headings).toBe(true);
    expect(groups.map((g) => [g.key, g.heading, g.works.map((x) => x.id)])).toEqual([
      ['volume', 'Volumes', [3, 7]], ['chapter', 'Capítulos', [1, 4]], ['oneshot', 'Únicos', [6]], ['', 'Sem unidade', [2, 5]],
    ]);
  });

  it('leaves out the groups with no work, and has no headings when only the works with no unit are there', () => {
    expect(groupByUnit([{ id: 1 }, { id: 2, unit: '' }])).toEqual({ groups: [{ key: '', heading: 'Sem unidade', works: [{ id: 1 }, { id: 2, unit: '' }] }], headings: false });
    expect(groupByUnit([]).groups).toEqual([]);
    expect(groupByUnit([{ id: 1, unit: 'chapter' }]).groups.map((g) => g.key)).toEqual(['chapter']);
    expect(groupByUnit([{ id: 1, unit: 'chapter' }]).headings).toBe(true);
  });
});

describe('stepText, a step of a series on a button (#187)', () => {
  it('says the unit and the number, and the title when there is no number to say', () => {
    expect(stepText({ unit: 'chapter', position: 27.5, title: 'Capítulo' })).toBe('Cap. 27,5');
    expect(stepText({ unit: 'volume', position: 3, title: 'x' })).toBe('Vol. 3');
    expect(stepText({ unit: 'oneshot', position: null, title: 'Especial' })).toBe('Único');
    expect(stepText({ unit: 'chapter', position: null, title: 'Extra' })).toBe('Extra');
    expect(stepText({ unit: '', position: 4, title: 'Sem unidade' })).toBe('Sem unidade');
    expect(stepText({ unit: 'sideways', position: 4, title: 'Estranho' })).toBe('Estranho');
  });
});

describe('goOnText and seriesCounts, a series on a card (#187)', () => {
  const step = { unit: 'chapter', position: 28, title: 'x' };
  it('says whether it begins, goes on with a begun one, or reads the next', () => {
    expect(goOnText({ ...step, started: true, begun: true })).toBe('Continuar: Cap. 28');
    expect(goOnText({ ...step, started: false, begun: true })).toBe('Próximo: Cap. 28');
    expect(goOnText({ ...step, started: false, begun: false })).toBe('Começar: Cap. 28');
    expect(goOnText({ ...step, started: true, begun: false })).toBe('Continuar: Cap. 28');
    expect(goOnText({ unit: '', position: null, title: 'Extra' })).toBe('Começar: Extra');
  });

  it('counts what the series has by unit, in the singular too, and leaves out what it has none of', () => {
    expect(seriesCounts({ volumes: 30, chapters: 121, oneShots: 2 })).toBe('30 volumes · 121 capítulos · 2 únicos');
    expect(seriesCounts({ volumes: 1, chapters: 1, oneShots: 1 })).toBe('1 volume · 1 capítulo · 1 único');
    expect(seriesCounts({ volumes: 0, chapters: 3, oneShots: 0 })).toBe('3 capítulos');
    expect(seriesCounts({})).toBe('');
  });
});

describe('newText, the new ones of a series (#187)', () => {
  it('says them in the singular and in the plural', () => {
    expect(newText(1)).toBe('1 novo');
    expect(newText(2)).toBe('2 novos');
    expect(newText(121)).toBe('121 novos');
  });
});
