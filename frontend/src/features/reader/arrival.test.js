import { describe, it, expect, beforeEach, vi } from 'vitest';
import { ARRIVAL_CLASS, arrivalOf, lastTold, markTold, paintArrival } from './arrival';

const epub = { type: 'epub', cfi: 'epubcfi(/6/12!/4/2/336)', href: 'Text/c2.xhtml' };
const fromDevice = (extra = {}) => ({ source: 'koreader', device: 'Kindle', revision: 4, locator: epub, ...extra });

describe('arrivalOf', () => {
  it('says a device stopped right here, for an EPUB with the place of a paragraph', () => {
    expect(arrivalOf(fromDevice())).toEqual({ device: 'Kindle', message: 'Seu Kindle parou bem aqui', cfi: epub.cfi, revision: 4 });
  });

  it('says the page of a PDF or a comic, which have no paragraph to underline', () => {
    expect(arrivalOf(fromDevice({ locator: { type: 'pdf', page: 11 } }))).toMatchObject({ message: 'Seu Kindle parou na página 12', cfi: null });
    expect(arrivalOf(fromDevice({ locator: { type: 'image', index: 6 } }))).toMatchObject({ message: 'Seu Kindle parou na página 7' });
  });

  it('is silent when the last save was not a device, or was already told, or says no place', () => {
    expect(arrivalOf(fromDevice({ source: '' }))).toBeNull();
    expect(arrivalOf(fromDevice(), 4)).toBeNull();
    expect(arrivalOf(fromDevice(), 5)).toBeNull();
    expect(arrivalOf(fromDevice({ locator: null }))).toBeNull();
    expect(arrivalOf(null)).toBeNull();
    expect(arrivalOf(fromDevice(), 3)).not.toBeNull(); // a newer save than the one told of
  });

  it('says "aparelho" when the device has no name', () => {
    expect(arrivalOf(fromDevice({ device: '  ' })).message).toBe('Seu aparelho parou bem aqui');
  });
});

describe('what was told of is kept for each file', () => {
  beforeEach(() => localStorage.clear());
  it('remembers the revision, and forgets nothing for another file', () => {
    expect(lastTold(7)).toBe(0);
    markTold(7, 4);
    expect(lastTold(7)).toBe(4);
    expect(lastTold(8)).toBe(0);
  });

  it('does not fail when the browser keeps nothing', () => {
    const real = Storage.prototype.getItem;
    Storage.prototype.getItem = () => { throw new Error('blocked'); };
    try {
      expect(lastTold(7)).toBe(0);
    } finally {
      Storage.prototype.getItem = real;
    }
  });
});

describe('paintArrival', () => {
  const paragraph = () => document.createElement('p');
  const contentsOf = (node) => ({ range: vi.fn(() => ({ startContainer: node })), addStylesheetRules: vi.fn() });
  const clock = () => {
    const queue = [];
    return {
      timers: { set: (fn) => queue.push(fn) - 1, clear: (i) => { queue[i] = null; } },
      run: () => { const now = queue.splice(0); for (const fn of now) fn?.(); },
    };
  };

  it('underlines the paragraph, fades it, and takes it away', () => {
    const p = paragraph();
    const { timers, run } = clock();
    const contents = contentsOf(p);
    paintArrival(contents, 'epubcfi(/6/12!/4/2/336)', { timers });
    expect(contents.range).toHaveBeenCalledWith('epubcfi(/6/12!/4/2/336)');
    expect(contents.addStylesheetRules).toHaveBeenCalled();
    expect(p.classList.contains(ARRIVAL_CLASS)).toBe(true);
    run(); // the delay ends: it fades
    expect(p.classList.contains('codice-arrival-out')).toBe(true);
    run(); // and goes
    expect(p.classList.contains(ARRIVAL_CLASS)).toBe(false);
  });

  it('takes the paragraph that holds a text, and can be undone at once', () => {
    const p = paragraph();
    const text = document.createTextNode('Citra tem razão');
    p.appendChild(text);
    const { timers } = clock();
    const undo = paintArrival(contentsOf(text), 'x', { timers });
    expect(p.classList.contains(ARRIVAL_CLASS)).toBe(true);
    undo();
    expect(p.classList.contains(ARRIVAL_CLASS)).toBe(false);
  });

  it('underlines nothing for a place that leads to the whole page, and never throws', () => {
    const body = document.createElement('body');
    expect(() => paintArrival(contentsOf(body), 'x')).not.toThrow();
    expect(body.classList.contains(ARRIVAL_CLASS)).toBe(false);
    expect(() => paintArrival({ range: () => { throw new Error('no such place'); } }, 'x')).not.toThrow();
    expect(() => paintArrival(undefined, 'x')).not.toThrow();
    expect(() => paintArrival({ range: () => null }, 'x')).not.toThrow();
  });
});
