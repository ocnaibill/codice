import { describe, it, expect } from 'vitest';
import { cardTitle, otherTitles, titleInUse } from './titles';

const work = (over = {}) => ({ title: 'A Nuvem 2', metadata: { alternativeTitles: [] }, ...over });

describe('titleInUse: the name the work goes by while an edition is read (#185)', () => {
  it('is the title written for the edition, and the main title when nobody wrote one', () => {
    expect(titleInUse(work(), { title: 'The Cloud 2', titleSet: true })).toBe('The Cloud 2');
    expect(titleInUse(work(), { title: 'cloud.epub', titleSet: false })).toBe('A Nuvem 2');
    expect(titleInUse(work(), { title: 'cloud.epub' })).toBe('A Nuvem 2');
  });

  it('is the main title when there is no edition, or the written title says nothing', () => {
    expect(titleInUse(work(), null)).toBe('A Nuvem 2');
    expect(titleInUse(work(), undefined)).toBe('A Nuvem 2');
    expect(titleInUse(work(), { title: '   ', titleSet: true })).toBe('A Nuvem 2');
    expect(titleInUse(work(), { title: undefined, titleSet: true })).toBe('A Nuvem 2');
  });

  it('takes the spaces off the ends of a written title, and is empty for no work', () => {
    expect(titleInUse(work(), { title: '  The Cloud 2 ', titleSet: true })).toBe('The Cloud 2');
    expect(titleInUse(undefined, null)).toBe('');
  });
});

describe('cardTitle: what a card says (#185)', () => {
  it('is the title written for the edition being read, or the main title', () => {
    expect(cardTitle({ title: 'A Nuvem 2', continue: { title: 'The Cloud 2' } })).toBe('The Cloud 2');
    expect(cardTitle({ title: 'A Nuvem 2', continue: { title: '' } })).toBe('A Nuvem 2');
    expect(cardTitle({ title: 'A Nuvem 2', continue: { title: '   ' } })).toBe('A Nuvem 2');
    expect(cardTitle({ title: 'A Nuvem 2', continue: {} })).toBe('A Nuvem 2');
    expect(cardTitle({ title: 'A Nuvem 2', continue: null })).toBe('A Nuvem 2');
    expect(cardTitle({ title: 'A Nuvem 2' })).toBe('A Nuvem 2');
  });
});

describe('otherTitles: the names under the one it goes by (#185)', () => {
  const kept = (title, language = '') => ({ id: 1, title, language, source: 'manual' });

  it('are the names kept for the work, in order, with their language', () => {
    expect(otherTitles(work({ metadata: { alternativeTitles: [kept('Arrakis'), kept('Dune', 'en')] } }), 'A Nuvem 2')).toEqual([
      { title: 'Arrakis', language: '' },
      { title: 'Dune', language: 'en' },
    ]);
  });

  it('have the main title first when the work goes by another name now, and never the name it goes by', () => {
    const w = work({ metadata: { alternativeTitles: [kept('Arrakis'), { id: 0, title: 'The Cloud 2', language: 'en', source: 'edition', editionId: 2 }] } });
    expect(otherTitles(w, 'The Cloud 2')).toEqual([{ title: 'A Nuvem 2', language: '' }, { title: 'Arrakis', language: '' }]);
  });

  it('say each name once, with no regard for capitals, accents, punctuation or a note in parentheses', () => {
    const w = work({ metadata: { alternativeTitles: [kept('A NUVEM 2'), kept('à nuvem, 2'), kept('Arrakis (edição de bolso)'), kept('arrakis'), kept('Outra')] } });
    expect(otherTitles(w, 'A Nuvem 2')).toEqual([{ title: 'Arrakis (edição de bolso)', language: '' }, { title: 'Outra', language: '' }]);
    expect(otherTitles(work(), 'a nuvem 2 (pt)')).toEqual([]);
  });

  it('leave out what has no letters, and are none when the server says none', () => {
    expect(otherTitles(work({ metadata: { alternativeTitles: [kept('!!!'), kept('   ')] } }), 'A Nuvem 2')).toEqual([]);
    expect(otherTitles(work({ metadata: undefined }), 'A Nuvem 2')).toEqual([]);
    expect(otherTitles(undefined, 'x')).toEqual([]);
  });

  it('keep the names of other scripts apart', () => {
    const w = work({ title: 'ドラゴンボール', metadata: { alternativeTitles: [kept('Dragon Ball', 'en'), kept('龍珠', 'zh')] } });
    expect(otherTitles(w, 'ドラゴンボール').map((t) => t.title)).toEqual(['Dragon Ball', '龍珠']);
  });
});
