import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { getComicMode, saveComicMode, setPreferenceOwner } from './preferences';

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
