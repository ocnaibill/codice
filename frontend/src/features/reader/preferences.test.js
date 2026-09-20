import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { getComicMode, saveComicMode } from './preferences';

describe('comic mode', () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.restoreAllMocks());

  it('starts left to right and is remembered from one comic to the next', () => {
    expect(getComicMode()).toBe('ltr');
    saveComicMode('rtl');
    expect(getComicMode()).toBe('rtl');
    saveComicMode('webtoon');
    expect(getComicMode()).toBe('webtoon');
  });

  it('ignores a mode it does not know, stored or offered', () => {
    saveComicMode('sideways');
    expect(getComicMode()).toBe('ltr');
    localStorage.setItem('codice:comic-mode', 'zigzag');
    expect(getComicMode()).toBe('ltr');
  });

  it('works when the browser will not store anything', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
    expect(getComicMode()).toBe('ltr');
    expect(() => saveComicMode('rtl')).not.toThrow();
  });
});
