import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { cleanQuote, menuPlacement, selectionIn, MAX_QUOTE, MENU_GAP, TOUCH_GAP, EDGE } from './selection';

describe('cleanQuote', () => {
  it('turns lines and runs of spaces into one space and trims the ends', () => {
    expect(cleanQuote('  Uma\n frase   com\tquebras.  ')).toBe('Uma frase com quebras.');
    expect(cleanQuote('linha com espaço duro')).toBe('linha com espaço duro');
  });

  it('takes what is not a text as nothing', () => {
    expect(cleanQuote(null)).toBe('');
    expect(cleanQuote(undefined)).toBe('');
    expect(cleanQuote('   \n ')).toBe('');
    expect(cleanQuote(42)).toBe('42');
  });

  it('keeps the limit of the server in one place', () => {
    expect(MAX_QUOTE).toBe(2000);
  });
});

describe('menuPlacement', () => {
  const viewport = { width: 400, height: 800 };
  const size = { width: 200, height: 48 };
  const rect = (o) => ({ left: 100, top: 300, width: 100, bottom: 320, ...o });

  it('with a mouse, goes above the selection, centered on it', () => {
    expect(menuPlacement({ rect: rect(), viewport, size })).toEqual({ left: 50, top: 300 - 48 - MENU_GAP });
  });

  it('with a finger, goes below the selection, further from it (the system draws its own there)', () => {
    expect(menuPlacement({ rect: rect(), viewport, size, touch: true })).toEqual({ left: 50, top: 320 + TOUCH_GAP });
  });

  it('goes to the other side where there is no room on the preferred one', () => {
    expect(menuPlacement({ rect: rect({ top: 20, bottom: 40 }), viewport, size }).top).toBe(40 + MENU_GAP);
    expect(menuPlacement({ rect: rect({ top: 760, bottom: 780 }), viewport, size, touch: true }).top).toBe(760 - 48 - TOUCH_GAP);
  });

  it('does not leave the screen at the sides', () => {
    expect(menuPlacement({ rect: rect({ left: 0, width: 10 }), viewport, size }).left).toBe(8);
    expect(menuPlacement({ rect: rect({ left: 390, width: 10 }), viewport, size }).left).toBe(192);
    expect(EDGE).toBe(8);
  });

  it('does not leave the screen at the top or the bottom when there is no room on either side', () => {
    const tall = { left: 100, top: 10, width: 100, bottom: 790 };
    const p = menuPlacement({ rect: tall, viewport, size });
    expect(p.top).toBeGreaterThanOrEqual(EDGE);
    expect(p.top + 48).toBeLessThanOrEqual(800 - EDGE);
    const q = menuPlacement({ rect: tall, viewport, size, touch: true });
    expect(q.top).toBeGreaterThanOrEqual(EDGE);
    expect(q.top + 48).toBeLessThanOrEqual(800 - EDGE);
  });

  it('exactly fits above when there are EDGE pixels to spare, and not when there are fewer', () => {
    const top = EDGE + 48 + MENU_GAP;
    expect(menuPlacement({ rect: rect({ top, bottom: top + 20 }), viewport, size }).top).toBe(EDGE);
    expect(menuPlacement({ rect: rect({ top: top - 1, bottom: top + 19 }), viewport, size }).top).toBe(top + 19 + MENU_GAP);
  });
});

describe('selectionIn', () => {
  let host;
  // jsdom does not lay anything out: a range is given the box a browser would measure.
  beforeEach(() => { Range.prototype.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 16, bottom: 36, right: 110 }); });
  afterEach(() => { host?.remove(); window.getSelection().removeAllRanges(); delete Range.prototype.getBoundingClientRect; });
  const setup = (html) => {
    host = document.createElement('div');
    host.innerHTML = html;
    document.body.appendChild(host);
    return host;
  };
  const select = (node, from, to) => {
    const range = document.createRange();
    range.setStart(node, from);
    range.setEnd(node, to);
    const s = window.getSelection();
    s.removeAllRanges();
    s.addRange(range);
  };

  it('gives the text and the place of a selection inside the root', () => {
    const root = setup('<p id="p">Era uma vez   um texto</p>');
    select(root.querySelector('#p').firstChild, 0, 12);
    const found = selectionIn(root);
    expect(found.text).toBe('Era uma vez');
    expect(found.rect).toEqual({ left: 10, top: 20, width: 100, height: 16, bottom: 36, right: 110 });
  });

  it('says where in the root the selection starts, and how long the root is', () => {
    const root = setup('<p id="p">Era uma vez</p><p>e depois outra</p>');
    select(root.querySelector('#p').firstChild, 4, 7);
    const found = selectionIn(root);
    expect(found.before).toBe(4);
    expect(found.total).toBe('Era uma vez'.length + 'e depois outra'.length);
    const second = root.querySelectorAll('p')[1].firstChild;
    select(second, 2, 8);
    expect(selectionIn(root).before).toBe('Era uma vez'.length + 2);
  });

  it('says nothing of the place when there is no root to measure from', () => {
    setup('<p id="p">solto</p>');
    select(document.querySelector('#p').firstChild, 1, 4);
    expect(selectionIn(null)).toMatchObject({ before: 0, total: 0 });
  });

  it('gives nothing for a selection outside the root, an empty one, or none', () => {
    const root = setup('<p id="a">dentro</p><p id="b">fora</p>');
    const inside = root.querySelector('#a');
    select(root.querySelector('#b').firstChild, 0, 4);
    expect(selectionIn(inside)).toBeNull();
    select(inside.firstChild, 2, 2);
    expect(selectionIn(root)).toBeNull();
    window.getSelection().removeAllRanges();
    expect(selectionIn(root)).toBeNull();
  });

  it('gives nothing for a selection of only spaces', () => {
    const root = setup('<p id="p">a    b</p>');
    select(root.querySelector('#p').firstChild, 1, 5);
    expect(selectionIn(root)).toBeNull();
  });

  it('does not need a root', () => {
    setup('<p id="p">solto</p>');
    select(document.querySelector('#p').firstChild, 0, 5);
    expect(selectionIn(null).text).toBe('solto');
  });

  it('reads the selection of another document (the page of a book is an iframe)', () => {
    const doc = { getSelection: () => null };
    expect(selectionIn(null, doc)).toBeNull();
    expect(selectionIn(null, {})).toBeNull();
  });
});
