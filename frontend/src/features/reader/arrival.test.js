import { describe, it, expect, beforeEach, vi } from 'vitest';
import EpubCFI from 'epubjs/src/epubcfi';
import { ARRIVAL_CLASS, ago, arrivalOf, lastTold, markPendingTold, markTold, paintArrival, pendingOf, pendingTold } from './arrival';

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

describe('ago', () => {
  const now = Date.parse('2026-10-16T12:00:00Z');
  const before = (ms) => new Date(now - ms).toISOString();
  const MIN = 60000;
  const H = 60 * MIN;
  const D = 24 * H;

  it('says how long ago in words, and nothing for the last few minutes', () => {
    expect(ago(before(3 * MIN), now)).toBe('');
    expect(ago(before(25 * MIN), now)).toBe('há 25 minutos');
    expect(ago(before(1 * H + 5 * MIN), now)).toBe('há 1 hora');
    expect(ago(before(5 * H), now)).toBe('há 5 horas');
    expect(ago(before(30 * H), now)).toBe('ontem');
    expect(ago(before(5 * D), now)).toBe('há 5 dias');
    expect(ago(before(21 * D), now)).toBe('há 3 semanas');
    expect(ago(before(35 * D), now)).toBe('há 5 semanas');
    expect(ago(before(70 * D), now)).toBe('há 2 meses');
    expect(ago(before(400 * D), now)).toBe('há 13 meses');
  });

  it('says nothing for what is not a time', () => {
    expect(ago(undefined, now)).toBe('');
    expect(ago('ontem à noite', now)).toBe('');
  });

  it('goes into what a device left, so that the person knows it may have read more without a network', () => {
    const progress = fromDevice({ updatedAt: before(5 * D) });
    expect(arrivalOf(progress, 0, now).message).toBe('Seu Kindle parou bem aqui, há 5 dias');
    expect(arrivalOf(fromDevice({ updatedAt: before(2 * MIN) }), 0, now).message).toBe('Seu Kindle parou bem aqui');
  });
});

describe('pendingOf', () => {
  const now = Date.parse('2026-10-16T12:00:00Z');
  const pending = (extra = {}) => ({ pending: { device: 'Kindle da sala', percent: 42.4, locator: epub, at: '2026-10-11T12:00:00Z', ...extra } });

  it('says the device stopped at another place, with how far and how long ago, and gives the place to go to', () => {
    expect(pendingOf(pending(), '', now)).toEqual({
      device: 'Kindle da sala', at: '2026-10-11T12:00:00Z', locator: epub, message: 'Seu Kindle da sala parou em outro ponto (42%), há 5 dias',
    });
  });

  it('is silent when there is none, when this one was told of, and when it has no place', () => {
    expect(pendingOf({}, '', now)).toBeNull();
    expect(pendingOf(null, '', now)).toBeNull();
    expect(pendingOf(pending(), '2026-10-11T12:00:00Z', now)).toBeNull();
    expect(pendingOf(pending({ locator: null }), '', now)).toBeNull();
    expect(pendingOf(pending({ at: '2026-10-12T00:00:00Z' }), '2026-10-11T12:00:00Z', now)).not.toBeNull(); // a newer one
  });

  it('remembers what was told of for each file', () => {
    localStorage.clear();
    expect(pendingTold(7)).toBe('');
    markPendingTold(7, '2026-10-11T12:00:00Z');
    expect(pendingTold(7)).toBe('2026-10-11T12:00:00Z');
    expect(pendingTold(8)).toBe('');
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

describe('paintArrival with the CFI the server writes', () => {
  // The server turns what KOReader says into a CFI of an element (body > div > the nth child): epub.js must read it as the reader does.
  const page = '<html xmlns="http://www.w3.org/1999/xhtml"><head><title>t</title></head><body><div><h1>Titulo</h1><p>um texto</p><p>dois texto</p><p>tres <b>negrito</b> fim</p></div></body></html>';
  const contents = () => {
    const doc = new DOMParser().parseFromString(page, 'application/xhtml+xml');
    return { doc, range: (cfi) => new EpubCFI(cfi).toRange(doc), addStylesheetRules: vi.fn() };
  };

  it('underlines the second paragraph for the path /4/2/6, and the paragraph around a text of it', () => {
    const c = contents();
    paintArrival(c, 'epubcfi(/6/4!/4/2/6)', { delay: 1e9 });
    expect(c.doc.querySelector('p:nth-of-type(2)').classList.contains(ARRIVAL_CLASS)).toBe(true);
    expect(c.doc.querySelectorAll(`.${ARRIVAL_CLASS}`)).toHaveLength(1);
    const d = contents();
    paintArrival(d, 'epubcfi(/6/4!/4/2/8/1:3)', { delay: 1e9 });
    expect(d.doc.querySelector('p:nth-of-type(3)').classList.contains(ARRIVAL_CLASS)).toBe(true);
  });

  it('underlines nothing for a path that leads to the body', () => {
    const c = contents();
    paintArrival(c, 'epubcfi(/6/4!/4)', { delay: 1e9 });
    expect(c.doc.querySelectorAll(`.${ARRIVAL_CLASS}`)).toHaveLength(0);
  });
});
