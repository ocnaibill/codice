import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { getComicMode, getDictionaryTarget, getHighlightColor, saveComicMode, saveDictionaryTarget, saveHighlightColor, setPreferenceOwner } from './preferences';

describe('comic mode', () => {
  beforeEach(() => {
    localStorage.clear();
    setPreferenceOwner('ana');
  });
  afterEach(() => vi.restoreAllMocks());

  it('starts left to right and is remembered from one comic to the next', () => {
    expect(getComicMode()).toBe('ltr');
    saveComicMode('rtl');
    expect(getComicMode()).toBe('rtl');
    saveComicMode('webtoon');
    expect(getComicMode()).toBe('webtoon');
  });

  it('belongs to the account: another person on the same browser starts from the default', () => {
    saveComicMode('rtl');
    setPreferenceOwner('bob');
    expect(getComicMode()).toBe('ltr');
    saveComicMode('webtoon');
    setPreferenceOwner('ana');
    expect(getComicMode()).toBe('rtl'); // and does not disturb ana's
    setPreferenceOwner('bob');
    expect(getComicMode()).toBe('webtoon');
  });

  it('remembers nothing when nobody is signed in', () => {
    setPreferenceOwner(null);
    saveComicMode('rtl');
    expect(getComicMode()).toBe('ltr');
    expect(localStorage.length).toBe(0);
    setPreferenceOwner('ana');
    expect(getComicMode()).toBe('ltr');
  });

  it('ignores a mode it does not know, stored or offered', () => {
    saveComicMode('sideways');
    expect(getComicMode()).toBe('ltr');
    localStorage.setItem('codice:comic-mode:ana', 'zigzag');
    expect(getComicMode()).toBe('ltr');
  });

  it('works when the browser will not store anything', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
    expect(getComicMode()).toBe('ltr');
    expect(() => saveComicMode('rtl')).not.toThrow();
  });
});

describe('language of the definitions', () => {
  beforeEach(() => {
    localStorage.clear();
    setPreferenceOwner('ana');
  });
  afterEach(() => vi.restoreAllMocks());

  it('is Portuguese until chosen, and is remembered', () => {
    expect(getDictionaryTarget()).toBe('pt');
    saveDictionaryTarget('ja');
    expect(getDictionaryTarget()).toBe('ja');
  });

  it('belongs to the account, and nothing is remembered with nobody signed in', () => {
    saveDictionaryTarget('fr');
    setPreferenceOwner('bob');
    expect(getDictionaryTarget()).toBe('pt');
    setPreferenceOwner(null);
    saveDictionaryTarget('de');
    localStorage.setItem('codice:dictionary-target:null', 'de');
    expect(getDictionaryTarget()).toBe('pt');
    setPreferenceOwner('ana');
    expect(getDictionaryTarget()).toBe('fr');
  });

  it('does not keep a language that is not one, and reads what is stored that is not one as the default', () => {
    saveDictionaryTarget('xx');
    expect(localStorage.getItem('codice:dictionary-target:ana')).toBeNull(); // not even written
    expect(getDictionaryTarget()).toBe('pt');
    localStorage.setItem('codice:dictionary-target:ana', 'klingon');
    expect(getDictionaryTarget()).toBe('pt');
  });

  it('works when the storage is blocked', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
    expect(getDictionaryTarget()).toBe('pt');
    expect(() => saveDictionaryTarget('fr')).not.toThrow();
  });
});

describe('color of the highlights', () => {
  beforeEach(() => {
    localStorage.clear();
    setPreferenceOwner('ana');
  });
  afterEach(() => vi.restoreAllMocks());

  it('is terracotta until chosen, and is remembered', () => {
    expect(getHighlightColor()).toBe('terracotta');
    saveHighlightColor('sage');
    expect(getHighlightColor()).toBe('sage');
  });

  it('belongs to the account, and nothing is remembered with nobody signed in', () => {
    saveHighlightColor('indigo');
    setPreferenceOwner('bob');
    expect(getHighlightColor()).toBe('terracotta');
    setPreferenceOwner(null);
    saveHighlightColor('sepia');
    localStorage.setItem('codice:highlight-color:null', 'sepia');
    expect(getHighlightColor()).toBe('terracotta');
    setPreferenceOwner('ana');
    expect(getHighlightColor()).toBe('indigo');
  });

  it('does not keep what is not a color, and reads what is stored that is not one as terracotta', () => {
    saveHighlightColor('red');
    expect(localStorage.getItem('codice:highlight-color:ana')).toBeNull();
    localStorage.setItem('codice:highlight-color:ana', 'pink');
    expect(getHighlightColor()).toBe('terracotta');
  });

  it('works when the storage is blocked', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
    expect(getHighlightColor()).toBe('terracotta');
    expect(() => saveHighlightColor('sage')).not.toThrow();
  });
});
