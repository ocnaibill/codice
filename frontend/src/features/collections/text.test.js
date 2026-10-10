import { describe, it, expect } from 'vitest';
import { collectionLine, COMIC_KINDS, goOnText, groupByUnit, newText, numberText, seriesCounts, sequenceWord, stepText, UNITS, unitLabel, viewsOf, wordsOf, worksText, groupByVolume, groupByArc, extraGroup } from './text';

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
    expect(UNITS.map((u) => u.key)).toEqual(['volume', 'chapter', 'oneshot', 'extra']);
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
    expect(unitLabel('extra', 4)).toBe('Compl.'); // a complementary work has no number in the sequence
    expect(unitLabel('extra', null)).toBe('Compl.');
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

  it('puts the complementary works in a group of their own, after all the others (DEC-164)', () => {
    const w = (id, unit) => ({ id, unit });
    const { groups, headings } = groupByUnit([w(1, 'extra'), w(2, ''), w(3, 'volume'), w(4, 'extra')]);
    expect(headings).toBe(true);
    expect(groups.map((g) => [g.key, g.heading, g.works.map((x) => x.id)])).toEqual([
      ['volume', 'Volumes', [3]], ['', 'Sem unidade', [2]], ['extra', 'Complementares', [1, 4]],
    ]);
    // Only complementary works: the group has its heading, since it is not "no unit".
    expect(groupByUnit([w(1, 'extra')]).headings).toBe(true);
  });

  it('calls a complementary work and a one-shot by their word, with no number, on a button', () => {
    expect(stepText({ unit: 'extra', position: 3, title: 'Guia do Mundo Bruxo' })).toBe('Compl.');
    expect(stepText({ unit: 'oneshot', position: null, title: 'x' })).toBe('Único');
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

describe('sequenceWord, what the sequence of a series is called (DEC-165)', () => {
  const w = (unit, extra = {}) => ({ unit, available: true, ...extra });
  it('names the unit when every work of the sequence is of one, in the singular for one', () => {
    expect(sequenceWord([w('chapter'), w('chapter')])).toEqual({ noun: 'capítulos', read: 'lidos' });
    expect(sequenceWord([w('volume')])).toEqual({ noun: 'volume', read: 'lido' });
    expect(sequenceWord([w('oneshot'), w('oneshot')])).toEqual({ noun: 'únicos', read: 'lidos' });
  });
  it('counts the number it is given, which is the works of the sequence the server counted', () => {
    expect(sequenceWord([w('chapter'), w('chapter')], 1)).toEqual({ noun: 'capítulo', read: 'lido' });
  });
  it('leaves out the complementary works and the ones gone from the library', () => {
    expect(sequenceWord([w('volume'), w('extra'), w('chapter', { available: false })], 1)).toEqual({ noun: 'volume', read: 'lido' });
  });
  it('says nothing when the units are mixed, when there is none, or when nothing is in the sequence', () => {
    expect(sequenceWord([w('volume'), w('chapter')])).toBeNull();
    expect(sequenceWord([w(''), w(undefined)])).toBeNull();
    expect(sequenceWord([w('volume'), w('')])).toBeNull();
    expect(sequenceWord([w('extra')])).toBeNull();
    expect(sequenceWord([])).toBeNull();
  });
});

describe('the views of a series by volume and by arc (DEC-169)', () => {
  const w = (id, unit, position, extra = {}) => ({ id, unit, position, ...extra });
  const works = [
    w(1, 'chapter', 1, { volumeNumber: 1, storyArc: 'Era de Ouro' }),
    w(2, 'chapter', 2, { volumeNumber: 1, storyArc: 'Era de Ouro' }),
    w(3, 'chapter', 3, { volumeNumber: 2 }),
    w(4, 'volume', 2),
    w(5, 'chapter', 9),
    w(6, 'extra', 1, { volumeNumber: 9, storyArc: 'Extra' }),
  ];

  it('offers a view only when some work of the sequence has the data for it', () => {
    expect(viewsOf([w(1, 'chapter', 1)]).map((v) => v.key)).toEqual(['units']);
    expect(viewsOf(works).map((v) => v.key)).toEqual(['units', 'volumes', 'arcs']);
    expect(viewsOf([w(1, 'chapter', 1, { volumeNumber: 3 })]).map((v) => v.key)).toEqual(['units', 'volumes']);
    expect(viewsOf([w(1, 'chapter', 1, { storyArc: 'A' })]).map((v) => v.key)).toEqual(['units', 'arcs']);
    // the complementary works are not the sequence: their data does not make a view
    expect(viewsOf([w(1, 'chapter', 1), w(2, 'extra', 1, { volumeNumber: 1, storyArc: 'x' })]).map((v) => v.key)).toEqual(['units']);
  });

  it('puts the chapters in the volume that collected them, with the volume file first, in numeric order', () => {
    // given out of numeric order, and with the volume file after its chapter
    const groups = groupByVolume([works[2], works[3], works[4], works[0], works[1], works[5]]);
    expect(groups.map((g) => [g.heading, g.works.map((x) => x.id)])).toEqual([
      ['Volume 1', [1, 2]],
      ['Volume 2', [4, 3]],
      ['Sem volume', [5]],
    ]);
    expect(groups[1].works.map((x) => x.unit)).toEqual(['volume', 'chapter']);
    expect(groups[0].note).toBe('2 capítulos');
    expect(groups[1].note).toBe('1 capítulo');
    expect(groups[2].note).toBeNull();
  });

  it('numbers the volumes as numbers, not as words', () => {
    const groups = groupByVolume([w(1, 'chapter', 1, { volumeNumber: 10 }), w(2, 'chapter', 2, { volumeNumber: 2 }), w(3, 'chapter', 3, { volumeNumber: 2.5 })]);
    expect(groups.map((g) => g.heading)).toEqual(['Volume 2', 'Volume 2,5', 'Volume 10']);
  });

  it('puts the works in their arc, in the order of the first chapter of each, saying the numbers it spans', () => {
    const groups = groupByArc([
      w(1, 'chapter', 5, { storyArc: 'Segundo' }), w(2, 'chapter', 6, { storyArc: 'Segundo' }),
      w(3, 'chapter', 1, { storyArc: 'Primeiro' }), w(4, 'chapter', 9),
      w(5, 'chapter', 2, { storyArc: 'Primeiro' }), w(6, 'chapter', 3, { storyArc: 'Primeiro' }),
    ]);
    expect(groups.map((g) => [g.heading, g.note, g.works.map((x) => x.id)])).toEqual([
      ['Primeiro', 'Capítulos 1 a 3 · 3 obras', [3, 5, 6]],
      ['Segundo', 'Capítulos 5 a 6 · 2 obras', [1, 2]],
      ['Sem arco', null, [4]],
    ]);
  });

  it('says a single number once, and a mix as works', () => {
    expect(groupByArc([w(1, 'chapter', 4, { storyArc: 'A' })])[0].note).toBe('Capítulo 4 · 1 obra');
    expect(groupByArc([w(1, 'chapter', 4, { storyArc: 'A' }), w(2, 'volume', 6, { storyArc: 'A' })])[0].note).toBe('Obras 4 a 6 · 2 obras');
    expect(groupByArc([w(1, 'volume', 1, { storyArc: 'A' }), w(2, 'volume', 2, { storyArc: 'A' })])[0].note).toBe('Volumes 1 a 2 · 2 obras');
  });

  it('keeps the complementary works out of the arcs and the volumes, in a group that ends every view', () => {
    expect(groupByVolume(works).flatMap((g) => g.works).some((x) => x.unit === 'extra')).toBe(false);
    expect(groupByArc(works).flatMap((g) => g.works).some((x) => x.unit === 'extra')).toBe(false);
    expect(extraGroup(works)).toEqual([{ key: 'extra', heading: 'Complementares', works: [works[5]] }]);
    expect(extraGroup([w(1, 'chapter', 1)])).toEqual([]);
  });
});
