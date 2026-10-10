import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

const getBook = vi.fn();
vi.mock('../../../../lib/api', () => ({ api: { get: (...a) => getBook(...a) }, authenticatedUrl: (u) => u }));

// epub.js is not what is under test: a book with a table of contents, and a page that records what it is asked.
const state = { toc: [], locationsLength: 0, locationAt: -1, percent: 0.37, rendition: null, events: {}, hooks: [], displayed: [], renderOptions: [], current: null, generate: () => Promise.resolve() };
vi.mock('epubjs', () => ({
  default: () => {
    const rendition = {
      display: vi.fn(async (target) => { state.displayed.push(target ?? null); }),
      on: (name, fn) => { state.events[name] = fn; },
      themes: { register: vi.fn(), select: vi.fn(), fontSize: vi.fn() },
      hooks: { content: { register: (fn) => state.hooks.push(fn) } },
      prev: vi.fn(),
      next: vi.fn(),
      resize: vi.fn(),
      annotations: { highlight: vi.fn(), remove: vi.fn() },
      currentLocation: () => state.current,
      destroy: vi.fn(),
    };
    state.rendition = rendition;
    return {
      loaded: { navigation: Promise.resolve({ toc: state.toc }) },
      opened: Promise.resolve(),
      spine: {
        spineItems: [{ href: 'Text/c1.xhtml', index: 0 }, { href: 'Text/c2.xhtml', index: 1 }],
        get: (target) => [{ href: 'Text/c1.xhtml', index: 0 }, { href: 'Text/c2.xhtml', index: 1 }].find((c) => c.href === target) ?? null,
      },
      locations: {
        length: () => state.locationsLength,
        generate: () => state.generate(),
        percentageFromCfi: () => state.percent,
        locationFromCfi: () => state.locationAt,
      },
      renderTo: (element, options) => { state.renderOptions.push(options); return rendition; },
      destroy: () => {},
    };
  },
}));

vi.mock('../../readingSync', () => ({ pushReadingSettings: vi.fn() }));
import EpubViewer from './EpubViewer';
import { pushReadingSettings } from '../../readingSync';
import { setPreferenceOwner } from '../../preferences';
import { themeName } from '../../epubThemes';
import { MOUSE_DELAY, TOUCH_DELAY } from '../../useSelectionWatcher';
import { QUIET_AFTER_TURN_MS, TOUCH_TAP_MAX_MS } from '../../epubGestures';
import { REDRAW_DELAY } from '../../epubMarks';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let onImmersiveChange;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
async function open(props = {}) {
  await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn().mockResolvedValue({})} onImmersiveChange={onImmersiveChange} {...props} />); });
  await flush();
  await flush();
}
const button = (label) => container.querySelector(`button[aria-label="${label}"]`);
const click = (el) => act(async () => { el.click(); });
const chrome = () => container.querySelector('[data-chrome]');
const keyup = (key, init = {}, target = document) => act(async () => { target.dispatchEvent(new KeyboardEvent('keyup', { key, bubbles: true, ...init })); });
const relocate = (cfi = 'epubcfi(/6/2!/4)') => act(async () => { state.events.relocated({ start: { cfi, href: 'c1.xhtml', percentage: 0.1, displayed: { page: 1, total: 4 } }, end: { cfi } }); });

beforeEach(() => {
  localStorage.clear();
  setPreferenceOwner('ana');
  state.toc = [];
  state.locationsLength = 0;
  state.locationAt = -1;
  state.percent = 0.37;
  state.events = {};
  state.hooks = [];
  state.displayed = [];
  state.renderOptions = [];
  state.current = null;
  state.generate = () => Promise.resolve();
  getBook.mockReset();
  getBook.mockResolvedValue({ data: new ArrayBuffer(8) });
  onImmersiveChange = vi.fn();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('the EPUB reader: the buttons', () => {
  it('turn the page', async () => {
    await open();
    await click(button('Próxima página'));
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    expect(state.rendition.prev).not.toHaveBeenCalled();
    await click(button('Página anterior'));
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
  });

  it('say how far through the book the person is, once the positions of the book are known', async () => {
    await open();
    expect(chrome().textContent).toContain('–');
    await relocate();
    expect(chrome().textContent).not.toContain('%'); // no positions yet
    act(() => root.unmount());
    root = createRoot(container);
    state.locationsLength = 40;
    await open();
    await relocate();
    expect(chrome().textContent).toContain('37%');
    expect(container.querySelector('[data-chrome-mark]').textContent).toBe('37%');
  });

  it('round the percentage to the nearest whole number', async () => {
    state.locationsLength = 40;
    state.percent = 0.376;
    await open();
    await relocate();
    expect(chrome().textContent).toContain('38%');
  });

  it('show it as soon as the positions of the book are computed, for the place the person is at', async () => {
    let done;
    state.generate = () => new Promise((resolve) => { done = resolve; });
    await open();
    await relocate();
    expect(chrome().textContent).toContain('–');
    state.locationsLength = 40;
    await act(async () => { done(); });
    await flush();
    expect(chrome().textContent).toContain('37%');
  });

  it('hide the controls', async () => {
    await open();
    expect(chrome().dataset.chrome).toBe('shown');
    expect(chrome().hasAttribute('inert')).toBe(false);
    await click(button('Esconder os controles'));
    expect(onImmersiveChange).toHaveBeenCalledWith(true);
  });

  it('are out of reach when hidden, and leave a mark of the place', async () => {
    state.locationsLength = 40;
    await open({ immersive: true });
    await relocate();
    expect(chrome().dataset.chrome).toBe('hidden');
    expect(chrome().hasAttribute('inert')).toBe(true);
    const mark = container.querySelector('[data-chrome-mark]');
    expect(mark.getAttribute('aria-hidden')).toBe('false');
    expect(mark.className).toContain('!opacity-60');
  });

  it('keep the mark out of the way of a screen reader while they are shown', async () => {
    await open();
    const mark = container.querySelector('[data-chrome-mark]');
    expect(mark.getAttribute('aria-hidden')).toBe('true');
    expect(mark.className).not.toContain('!opacity-60');
  });
});

describe('the EPUB reader: the look of the text', () => {
  const settingsKey = 'codice:epub-settings:ana';
  const panel = () => container.querySelector('[role="dialog"]');

  it('opens its panel from a button, and closes it from the same button', async () => {
    await open();
    expect(panel()).toBeNull();
    expect(button('Aparência do texto').getAttribute('aria-expanded')).toBe('false');
    await click(button('Aparência do texto'));
    expect(panel()).not.toBeNull();
    expect(button('Aparência do texto').getAttribute('aria-expanded')).toBe('true');
    await click(button('Aparência do texto'));
    expect(panel()).toBeNull();
  });

  it('takes the focus into its panel, and Escape closes it and gives the focus back to its button', async () => {
    await open();
    const opener = button('Aparência do texto');
    opener.focus();
    await click(opener);
    expect(panel().contains(document.activeElement)).toBe(true);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })); });
    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  it('starts with the page the person chose last, as it was saved', async () => {
    localStorage.setItem(settingsKey, JSON.stringify({ theme: 'sepia', font: 'serifada', size: 120, spacing: 'media' }));
    await open();
    const name = themeName({ theme: 'sepia', font: 'serifada', spacing: 'media' });
    expect(state.rendition.themes.select).toHaveBeenCalledWith(name);
    expect(state.rendition.themes.fontSize).toHaveBeenCalledWith('120%');
    expect(container.firstElementChild.style.backgroundColor).toBe('rgb(242, 232, 213)');
  });

  it('starts with the paper page when nothing was chosen', async () => {
    await open();
    expect(state.rendition.themes.select).toHaveBeenCalledWith(themeName({ theme: 'papel', font: 'livro', spacing: 'livro' }));
    expect(state.rendition.themes.fontSize).toHaveBeenCalledWith('100%');
    expect(container.firstElementChild.style.backgroundColor).toBe('rgb(250, 248, 244)');
  });

  it('changes the size by one step at a time, at once, and remembers it', async () => {
    await open();
    await click(button('Aparência do texto'));
    await click(button('Aumentar a letra'));
    expect(state.rendition.themes.fontSize).toHaveBeenLastCalledWith('110%');
    expect(panel().textContent).toContain('110%');
    await click(button('Diminuir a letra'));
    await click(button('Diminuir a letra'));
    expect(state.rendition.themes.fontSize).toHaveBeenLastCalledWith('90%');
    expect(JSON.parse(localStorage.getItem(settingsKey)).size).toBe(90);
  });

  it('stops at the least and the most', async () => {
    localStorage.setItem(settingsKey, JSON.stringify({ size: 80 }));
    await open();
    await click(button('Aparência do texto'));
    expect(button('Diminuir a letra').disabled).toBe(true);
    expect(button('Aumentar a letra').disabled).toBe(false);
    act(() => root.unmount());
    root = createRoot(container);
    localStorage.setItem(settingsKey, JSON.stringify({ size: 200 }));
    await open();
    await click(button('Aparência do texto'));
    expect(button('Aumentar a letra').disabled).toBe(true);
    expect(button('Diminuir a letra').disabled).toBe(false);
  });

  it('changes the color of the page: six to choose from, the one in use marked', async () => {
    await open();
    await click(button('Aparência do texto'));
    const swatches = [...panel().querySelectorAll('[role="radio"][aria-label]')];
    expect(swatches.map((s) => s.getAttribute('aria-label'))).toEqual(['Branco', 'Papel', 'Sépia', 'Cinza', 'Escuro', 'Preto']);
    expect(swatches.map((s) => s.getAttribute('aria-checked'))).toEqual(['false', 'true', 'false', 'false', 'false', 'false']);
    await click(button('Preto'));
    expect(state.rendition.themes.select).toHaveBeenLastCalledWith(themeName({ theme: 'preto', font: 'livro', spacing: 'livro' }));
    expect(container.firstElementChild.style.backgroundColor).toBe('rgb(0, 0, 0)');
    expect(button('Preto').getAttribute('aria-checked')).toBe('true');
    expect(button('Papel').getAttribute('aria-checked')).toBe('false');
    expect(JSON.parse(localStorage.getItem(settingsKey)).theme).toBe('preto');
  });

  it('changes the font and the spacing', async () => {
    await open();
    await click(button('Aparência do texto'));
    const group = (label) => panel().querySelector(`[role="group"][aria-label="${label}"]`);
    const radio = (groupLabel, text) => [...group(groupLabel).querySelectorAll('[role="radio"]')].find((b) => b.textContent === text);
    await click(radio('Fonte', 'Serifada'));
    expect(state.rendition.themes.select).toHaveBeenLastCalledWith(themeName({ theme: 'papel', font: 'serifada', spacing: 'livro' }));
    await click(radio('Entrelinha', 'Ampla'));
    expect(state.rendition.themes.select).toHaveBeenLastCalledWith(themeName({ theme: 'papel', font: 'serifada', spacing: 'ampla' }));
    expect(JSON.parse(localStorage.getItem(settingsKey))).toEqual({ theme: 'papel', font: 'serifada', size: 100, spacing: 'ampla', margins: 'livro', justify: false });
    expect(radio('Fonte', 'Serifada').getAttribute('aria-checked')).toBe('true');
    expect(radio('Fonte', 'Do livro').getAttribute('aria-checked')).toBe('false');
    expect(radio('Entrelinha', 'Ampla').getAttribute('aria-checked')).toBe('true');
    expect(radio('Entrelinha', 'Do livro').getAttribute('aria-checked')).toBe('false');
    await click(radio('Fonte', 'Sem serifa'));
    expect(state.rendition.themes.select).toHaveBeenLastCalledWith(themeName({ theme: 'papel', font: 'sem-serifa', spacing: 'ampla' }));
    await click(radio('Entrelinha', 'Média'));
    expect(state.rendition.themes.select).toHaveBeenLastCalledWith(themeName({ theme: 'papel', font: 'sem-serifa', spacing: 'media' }));
  });

  describe('the font for dyslexia, the margins and the justified lines', () => {
    const group = (label) => panel().querySelector(`[role="group"][aria-label="${label}"]`);
    const radio = (groupLabel, text) => [...group(groupLabel).querySelectorAll('[role="radio"]')].find((b) => b.textContent === text);
    const page = () => container.querySelector('[data-epub-page]').parentElement;
    const lastRules = () => state.rendition.themes.register.mock.calls.at(-1)[1];

    it('has the font for dyslexia among the fonts, and gives it to the book', async () => {
      await open();
      await click(button('Aparência do texto'));
      expect([...group('Fonte').querySelectorAll('[role="radio"]')].map((b) => b.textContent)).toEqual(['Do livro', 'Serifada', 'Sem serifa', 'Dislexia']);
      await click(radio('Fonte', 'Dislexia'));
      expect(state.rendition.themes.select).toHaveBeenLastCalledWith(themeName({ theme: 'papel', font: 'dislexia', spacing: 'livro' }));
      const rule = Object.entries(lastRules()).find(([selector]) => selector.includes(':not(pre)'))[1];
      expect(rule['font-family']).toContain('Codice OpenDyslexic');
    });

    it('gives the page the room chosen on each side, and none of its own until then', async () => {
      await open();
      expect(page().style.paddingInline).toBe('');
      await click(button('Aparência do texto'));
      expect([...group('Margens').querySelectorAll('[role="radio"]')].map((b) => b.textContent)).toEqual(['Do livro', 'Estreita', 'Média', 'Larga']);
      await click(radio('Margens', 'Larga'));
      expect(page().style.paddingInline).toBe('14%');
      await click(radio('Margens', 'Do livro'));
      expect(page().style.paddingInline).toBe('');
    });

    it('starts with the margins it was left with', async () => {
      localStorage.setItem(settingsKey, JSON.stringify({ theme: 'papel', font: 'livro', size: 100, spacing: 'livro', margins: 'media', justify: true }));
      await open();
      expect(page().style.paddingInline).toBe('6%');
      const rule = Object.entries(lastRules()).find(([selector]) => selector.includes('blockquote'))[1];
      expect(rule['text-align']).toBe('justify !important');
    });

    it('justifies the paragraphs of the book when the switch is on, and takes it back when it is off', async () => {
      await open();
      await click(button('Aparência do texto'));
      const justify = () => panel().querySelector('[role="switch"]');
      expect(justify().getAttribute('aria-checked')).toBe('false');
      expect(Object.values(lastRules()).some((r) => 'text-align' in r)).toBe(false);
      await click(justify());
      expect(justify().getAttribute('aria-checked')).toBe('true');
      expect(Object.values(lastRules()).some((r) => r['text-align'] === 'justify !important')).toBe(true);
      expect(state.rendition.themes.select).toHaveBeenLastCalledWith(themeName({ theme: 'papel', font: 'livro', spacing: 'livro', justify: true }));
      await click(justify());
      expect(Object.values(lastRules()).some((r) => 'text-align' in r)).toBe(false);
    });

    it('sends the whole choice to be kept on the server, each time it changes', async () => {
      pushReadingSettings.mockClear();
      await open();
      await click(button('Aparência do texto'));
      await click(radio('Margens', 'Média'));
      await click(panel().querySelector('[role="switch"]'));
      expect(pushReadingSettings).toHaveBeenCalledTimes(2);
      expect(pushReadingSettings).toHaveBeenLastCalledWith({ theme: 'papel', font: 'livro', size: 100, spacing: 'livro', margins: 'media', justify: true });
    });
  });

  it('keeps each choice when another one is made', async () => {
    await open();
    await click(button('Aparência do texto'));
    await click(button('Sépia'));
    await click(button('Aumentar a letra'));
    await click(button('Aumentar a letra'));
    expect(JSON.parse(localStorage.getItem(settingsKey))).toEqual({ theme: 'sepia', font: 'livro', size: 120, spacing: 'livro', margins: 'livro', justify: false });
  });

  it('closes by Escape and by a press outside, but not by a press inside it or on the bar it opens from', async () => {
    await open();
    await click(button('Aparência do texto'));
    await act(async () => { panel().dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); });
    expect(panel()).not.toBeNull();
    await act(async () => { button('Aparência do texto').dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); });
    expect(panel()).not.toBeNull();
    await act(async () => { button('Próxima página').dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); });
    expect(panel()).not.toBeNull();
    await act(async () => { document.body.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); });
    expect(panel()).toBeNull();
    await click(button('Aparência do texto'));
    await keyup('Escape');
    expect(panel()).toBeNull();
    expect(onImmersiveChange).not.toHaveBeenCalled(); // that Escape was for the panel
  });

  it('is taken away with the controls when they are hidden', async () => {
    await open();
    await click(button('Aparência do texto'));
    expect(panel()).not.toBeNull();
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} immersive onImmersiveChange={onImmersiveChange} />); });
    expect(panel()).toBeNull();
  });

  it('does not keep what is not valid in storage', async () => {
    localStorage.setItem(settingsKey, JSON.stringify({ theme: '"><script>', size: 999 }));
    await open();
    expect(state.rendition.themes.fontSize).toHaveBeenCalledWith('100%');
    expect(container.firstElementChild.style.backgroundColor).toBe('rgb(250, 248, 244)');
  });
});

describe('the EPUB reader: coming back to the place after the text is laid out again', () => {
  const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });

  it('goes back to where the page in view started when the size, the font or the spacing change', async () => {
    await open();
    await relocate('epubcfi(/6/2!/4/10)');
    state.displayed = [];
    await click(button('Aparência do texto'));
    await click(button('Aumentar a letra'));
    expect(state.displayed).toEqual([]); // not before the text has been laid out
    await wait(150);
    expect(state.displayed).toEqual(['epubcfi(/6/2!/4/10)']);
    state.displayed = [];
    const radio = (group, text) => [...container.querySelector(`[role="group"][aria-label="${group}"]`).querySelectorAll('[role="radio"]')].find((b) => b.textContent === text);
    await click(radio('Fonte', 'Serifada'));
    await wait(150);
    await click(radio('Entrelinha', 'Ampla'));
    await wait(150);
    expect(state.displayed).toEqual(['epubcfi(/6/2!/4/10)', 'epubcfi(/6/2!/4/10)']);
  });

  it('follows the page the person is on now, not the one they were on when the book opened', async () => {
    await open();
    await relocate('epubcfi(/6/2!/4/2)');
    await relocate('epubcfi(/6/4!/4/8)');
    state.displayed = [];
    await click(button('Aparência do texto'));
    await click(button('Diminuir a letra'));
    await wait(150);
    expect(state.displayed).toEqual(['epubcfi(/6/4!/4/8)']);
  });

  it('does not take for the place of the person what epub.js says while it lays the text out, and saves the page that shows the place', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    await relocate('epubcfi(/6/2!/4/10)');
    await wait(1100);
    onProgress.mockClear();
    state.displayed = [];
    await click(button('Aparência do texto'));
    await click(button('Aumentar a letra'));
    await relocate('epubcfi(/6/2!/4/2)'); // what epub.js says while the text is laid out: the page at the old position
    state.current = { start: { cfi: 'epubcfi(/6/2!/4/8)', href: 'c1.xhtml', displayed: { page: 2, total: 5 } }, end: { cfi: 'x' } };
    await wait(150);
    expect(state.displayed).toEqual(['epubcfi(/6/2!/4/10)']); // back to the place, not to what epub.js said
    await wait(1100);
    expect(onProgress).toHaveBeenCalledTimes(1);
    expect(onProgress.mock.calls[0][0]).toMatchObject({ type: 'epub', cfi: 'epubcfi(/6/2!/4/8)' });
    // the next change goes back to the same place, which the layout did not move
    state.displayed = [];
    await click(button('Aumentar a letra'));
    await wait(150);
    expect(state.displayed).toEqual(['epubcfi(/6/2!/4/10)']);
  });

  it('listens to epub.js again once the text is laid out', async () => {
    await open();
    await relocate('epubcfi(/6/2!/4/10)');
    await click(button('Aparência do texto'));
    await click(button('Aumentar a letra'));
    await wait(450); // the display, and the time the layout takes to settle
    state.displayed = [];
    await relocate('epubcfi(/6/4!/4/2)'); // the person turned a page
    await click(button('Aumentar a letra'));
    await wait(150);
    expect(state.displayed).toEqual(['epubcfi(/6/4!/4/2)']);
  });

  it('copes with a display that fails, and with a book that does not say where it is', async () => {
    await open();
    await relocate('epubcfi(/6/2!/4/10)');
    state.rendition.display.mockRejectedValueOnce(new Error('No Section Found'));
    await click(button('Aparência do texto'));
    await click(button('Aumentar a letra'));
    await wait(450);
    state.displayed = [];
    await relocate('epubcfi(/6/4!/4/2)'); // still listened to afterwards
    await click(button('Aumentar a letra'));
    await wait(150);
    expect(state.displayed).toEqual(['epubcfi(/6/4!/4/2)']);
  });

  it('does nothing for a change of the color of the page, which does not move the text', async () => {
    await open();
    await relocate();
    state.displayed = [];
    await click(button('Aparência do texto'));
    await click(button('Sépia'));
    await wait(150);
    expect(state.displayed).toEqual([]);
  });

  it('does nothing when the person has not got anywhere yet, and when a choice is made and then undone it is only asked once', async () => {
    await open();
    state.displayed = [];
    await click(button('Aparência do texto'));
    await click(button('Aumentar a letra'));
    await wait(150);
    expect(state.displayed).toEqual([]); // no place known
    await relocate('epubcfi(/6/2!/4/6)');
    await click(button('Aumentar a letra'));
    await click(button('Diminuir a letra'));
    await wait(150);
    expect(state.displayed).toEqual(['epubcfi(/6/2!/4/6)']);
  });

  it('does not come back once the book is closed', async () => {
    await open();
    await relocate();
    state.displayed = [];
    await click(button('Aparência do texto'));
    await click(button('Aumentar a letra'));
    act(() => root.unmount());
    root = createRoot(container);
    await wait(150);
    expect(state.displayed).toEqual([]);
  });
});

describe('the EPUB reader: the table of contents', () => {
  const toc = [
    { id: 'p1', href: 'c1.xhtml', label: ' Parte I ', subitems: [{ id: 'p1a', href: 'c2.xhtml#a', label: 'Capítulo 1' }] },
    { id: 'p2', href: 'nowhere.xhtml', label: 'Parte II' },
  ];

  it('has no button for a book without one', async () => {
    await open();
    expect(button('Sumário do livro')).toBeNull();
  });

  it('lists the entries, the deeper ones set in, and goes to the one chosen, closing the list', async () => {
    state.toc = toc;
    await open();
    await click(button('Sumário do livro'));
    const nav = container.querySelector('nav[aria-label="Sumário do livro"]');
    const items = [...nav.querySelectorAll('button')];
    expect(items.map((i) => i.textContent)).toEqual(['Parte I', 'Capítulo 1', 'Parte II']);
    expect(items[0].style.paddingLeft).toBe('0.75rem');
    expect(items[1].style.paddingLeft).toBe('1.75rem');
    await click(items[1]);
    expect(state.displayed.at(-1)).toBe('Text/c2.xhtml');
    expect(container.querySelector('nav[aria-label="Sumário do livro"]')).toBeNull();
  });

  it('takes the focus into the contents, and Escape closes them and gives the focus back to their button', async () => {
    state.toc = toc;
    await open();
    const opener = button('Sumário do livro');
    opener.focus();
    await click(opener);
    const nav = container.querySelector('nav[aria-label="Sumário do livro"]');
    expect(nav.contains(document.activeElement)).toBe(true);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })); });
    expect(container.querySelector('nav[aria-label="Sumário do livro"]')).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  it('goes by the address given when the book has no such chapter by that name', async () => {
    state.toc = toc;
    await open();
    await click(button('Sumário do livro'));
    await click([...container.querySelectorAll('nav button')].find((b) => b.textContent === 'Parte II'));
    expect(state.displayed.at(-1)).toBe('nowhere.xhtml');
  });

  it('opens one panel at a time, and the button of each closes its own', async () => {
    state.toc = toc;
    await open();
    await click(button('Sumário do livro'));
    expect(button('Sumário do livro').getAttribute('aria-expanded')).toBe('true');
    await click(button('Aparência do texto'));
    expect(container.querySelector('nav[aria-label="Sumário do livro"]')).toBeNull();
    expect(container.querySelector('[role="dialog"]')).not.toBeNull();
    await click(button('Sumário do livro'));
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    await click(button('Sumário do livro'));
    expect(container.querySelector('nav[aria-label="Sumário do livro"]')).toBeNull();
  });
});

describe('the EPUB reader: the keyboard', () => {
  it('turns the page with the arrows and Page Up and Page Down', async () => {
    await open();
    await keyup('ArrowRight');
    await keyup('PageDown');
    expect(state.rendition.next).toHaveBeenCalledTimes(2);
    await keyup('ArrowLeft');
    await keyup('PageUp');
    expect(state.rendition.prev).toHaveBeenCalledTimes(2);
  });

  it('is not taken from someone who is typing, and not used with a modifier', async () => {
    await open();
    const field = document.createElement('input');
    document.body.appendChild(field);
    await keyup('ArrowRight', {}, field);
    await keyup('ArrowRight', { ctrlKey: true });
    await keyup('ArrowRight', { metaKey: true });
    await keyup('ArrowRight', { altKey: true });
    expect(state.rendition.next).not.toHaveBeenCalled();
    field.remove();
  });

  it('shows the controls again with Escape, when they were hidden, and does nothing when they are shown', async () => {
    await open({ immersive: true });
    await keyup('Escape');
    expect(onImmersiveChange).toHaveBeenCalledWith(false);
    onImmersiveChange.mockClear();
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} immersive={false} onImmersiveChange={onImmersiveChange} />); });
    await keyup('Escape');
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('names the frame of the book for a screen reader, with the title when it is known', async () => {
    await open({ title: 'Duna' });
    const first = document.createElement('iframe');
    container.appendChild(first);
    state.hooks[0]({ document: first.contentDocument, addStylesheetCss: vi.fn() });
    expect(first.getAttribute('title')).toBe('Texto de “Duna”');
    const second = document.createElement('iframe');
    second.setAttribute('title', 'já tem nome');
    container.appendChild(second);
    state.hooks[0]({ document: second.contentDocument, addStylesheetCss: vi.fn() });
    expect(second.getAttribute('title')).toBe('já tem nome'); // a name that is already there is not taken
  });

  it('names the frame of the book even when its title is not known', async () => {
    await open();
    const frame = document.createElement('iframe');
    container.appendChild(frame);
    state.hooks[0]({ document: frame.contentDocument, addStylesheetCss: vi.fn() });
    expect(frame.getAttribute('title')).toBe('Texto do livro');
  });

  it('is heard from inside the page of the book too, which is an iframe of its own, with the same rules', async () => {
    await open();
    expect(state.hooks.length).toBe(1);
    const iframe = document.createElement('iframe');
    container.appendChild(iframe);
    const contents = { document: iframe.contentDocument, addStylesheetCss: vi.fn() };
    state.hooks[0](contents);
    const doc = iframe.contentDocument;
    await act(async () => { doc.dispatchEvent(new KeyboardEvent('keyup', { key: 'ArrowRight', bubbles: true })); });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await act(async () => { doc.dispatchEvent(new KeyboardEvent('keyup', { key: 'ArrowLeft', bubbles: true, ctrlKey: true })); });
    expect(state.rendition.prev).not.toHaveBeenCalled();
    expect(contents.addStylesheetCss).toHaveBeenCalledTimes(2); // the fonts, and the touch rule
    const [css, key] = contents.addStylesheetCss.mock.calls[0];
    expect(key).toBe('codice-fonts');
    expect(css).toContain('@font-face');
    expect(css).toContain('Codice Serif');
    expect(contents.addStylesheetCss.mock.calls[1][1]).toBe('codice-touch');
  });
});

describe('the EPUB reader: opening the book', () => {
  it('shows a placeholder while it is being opened, and not when it is open', async () => {
    let release;
    getBook.mockReturnValue(new Promise((r) => { release = r; }));
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} />); });
    expect(container.querySelector('[role="status"]').getAttribute('aria-label')).toBe('Carregando o livro');
    await act(async () => { release({ data: new ArrayBuffer(8) }); });
    await flush();
    await flush();
    expect(container.querySelector('[role="status"]')).toBeNull();
  });

  it('says in words that it could not, with the reason, and tries again when asked', async () => {
    getBook.mockRejectedValueOnce(new Error('Request failed with status code 404'));
    vi.spyOn(console, 'error').mockImplementation(() => {});
    await open();
    const alert = container.querySelector('[role="alert"]');
    expect(alert.textContent).toContain('Não foi possível abrir este EPUB.');
    expect(alert.textContent).toContain('Request failed with status code 404');
    expect(container.querySelector('[role="status"]')).toBeNull();
    await click([...alert.querySelectorAll('button')].find((b) => b.textContent === 'Tentar de novo'));
    await flush();
    await flush();
    expect(getBook).toHaveBeenCalledTimes(2);
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(state.displayed).toEqual([null]);
  });
});

describe('the EPUB reader: the room it is given', () => {
  it('lays the book out again, at the same place, when the room changes, once things settle', async () => {
    let notify;
    const watchers = [];
    vi.stubGlobal('ResizeObserver', class { constructor(fn) { watchers.push(fn); } observe() {} disconnect() {} });
    notify = () => watchers.forEach((fn) => fn());
    await open();
    notify(); notify(); notify();
    expect(state.rendition.resize).not.toHaveBeenCalled();
    await act(async () => { await new Promise((r) => setTimeout(r, 250)); });
    expect(state.rendition.resize).toHaveBeenCalledTimes(1);
  });

  it('works where there is no ResizeObserver', async () => {
    vi.stubGlobal('ResizeObserver', undefined);
    await open();
    expect(container.querySelector('[data-epub-page]')).not.toBeNull();
  });
});

describe('the EPUB reader: what is on screen', () => {
  let height;
  const pageFrame = () => container.querySelector('[data-epub-page]').parentElement;
  beforeEach(() => {
    height = 640;
    Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, get() { return this.parentElement === container ? height : 0; } });
  });
  afterEach(() => { delete HTMLElement.prototype.clientHeight; });

  it('lays the text out for the room there is, less the room for the controls and a margin at the top', async () => {
    await open();
    expect(pageFrame().style.height).toBe(`${640 - 72 - 12}px`);
    expect(pageFrame().className).toContain('bottom-[4.5rem]');
  });

  it('leaves the text where it is, and the same size, when the controls are hidden and the room grows', async () => {
    let notify;
    const watchers = [];
    vi.stubGlobal('ResizeObserver', class { constructor(fn) { watchers.push(fn); } observe() {} disconnect() {} });
    notify = () => watchers.forEach((fn) => fn());
    await open();
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} immersive onImmersiveChange={onImmersiveChange} />); });
    height = 701; // the header folded away
    notify();
    await act(async () => { await new Promise((r) => setTimeout(r, 350)); });
    expect(pageFrame().style.height).toBe(`${640 - 72 - 12}px`);
  });

  it('does not take a room that is still changing (the header folding or coming back), only the one that settles', async () => {
    const watchers = [];
    vi.stubGlobal('ResizeObserver', class { constructor(fn) { watchers.push(fn); } observe() {} disconnect() {} });
    await open();
    height = 500; // on the way
    watchers.forEach((fn) => fn());
    await act(async () => { await new Promise((r) => setTimeout(r, 100)); });
    expect(pageFrame().style.height).toBe(`${640 - 72 - 12}px`);
    height = 580;
    watchers.forEach((fn) => fn());
    await act(async () => { await new Promise((r) => setTimeout(r, 350)); });
    expect(pageFrame().style.height).toBe(`${580 - 72 - 12}px`);
  });

  it('keeps the touch of the page for the vertical scroll of the browser and the pinch, and not for a swipe across', async () => {
    await open();
    expect(container.firstElementChild.style.touchAction).toBe('pan-y pinch-zoom');
  });

  it('measures again when the room changes with the controls shown (a phone turned), once things settle', async () => {
    let notify;
    const watchers = [];
    vi.stubGlobal('ResizeObserver', class { constructor(fn) { watchers.push(fn); } observe() {} disconnect() {} });
    notify = () => watchers.forEach((fn) => fn());
    await open();
    height = 300;
    notify(); notify();
    expect(pageFrame().style.height).toBe(`${640 - 72 - 12}px`); // not at once
    await act(async () => { await new Promise((r) => setTimeout(r, 350)); });
    expect(pageFrame().style.height).toBe(`${300 - 72 - 12}px`);
  });

  it('measures again on a window resize where there is no ResizeObserver', async () => {
    vi.stubGlobal('ResizeObserver', undefined);
    await open();
    height = 500;
    window.dispatchEvent(new Event('resize'));
    await act(async () => { await new Promise((r) => setTimeout(r, 350)); });
    expect(pageFrame().style.height).toBe(`${500 - 72 - 12}px`);
  });

  it('does not set a height before it has measured one, and does not take a room of nothing', async () => {
    height = 0;
    await open();
    expect(pageFrame().style.height).toBe('calc(100% - 5.25rem)');
  });
});

describe('the EPUB reader: what is selected on the page of the book', () => {
  const RANGE = 'epubcfi(/6/4!/4/2,/1:0,/1:20)';
  let frame;
  let doc;
  let contents;
  const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });
  async function bookPage(onSelection) {
    await open({ onSelection });
    frame = document.createElement('iframe');
    container.appendChild(frame);
    doc = frame.contentDocument;
    doc.body.innerHTML = '<p id="p">Era uma vez um texto que se lê devagar.</p>';
    contents = { document: doc, addStylesheetCss: vi.fn(), cfiFromRange: vi.fn(() => RANGE) };
    state.hooks[0](contents);
    // jsdom lays nothing out: the passage is given the box a browser would measure, and the iframe a place on the screen.
    doc.defaultView.Range.prototype.getBoundingClientRect = () => ({ left: 30, top: 40, width: 100, height: 16 });
    vi.spyOn(frame, 'getBoundingClientRect').mockReturnValue({ left: -600, top: 50, width: 1200, height: 600 });
  }
  const selectWords = async (from, to, pointerType = 'mouse') => {
    const node = doc.querySelector('#p').firstChild;
    const range = doc.createRange();
    range.setStart(node, from);
    range.setEnd(node, to);
    const sel = doc.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
    const down = new frame.contentWindow.MouseEvent('pointerdown', { bubbles: true });
    Object.defineProperty(down, 'pointerType', { value: pointerType });
    await act(async () => { doc.dispatchEvent(down); });
    await act(async () => { doc.dispatchEvent(new frame.contentWindow.Event('selectionchange')); });
  };

  it('is told to whoever offers what to do with it: the passage, the range of the book it is, and where it is on the screen', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await selectWords(0, 11);
    expect(onSelection).not.toHaveBeenCalled();
    await wait(MOUSE_DELAY + 80);
    expect(onSelection).toHaveBeenCalledTimes(1);
    const found = onSelection.mock.calls[0][0];
    expect(found.text).toBe('Era uma vez');
    expect(found.locator).toEqual({ type: 'epub', cfi: RANGE, excerpt: 'Era uma vez' });
    expect(found.touch).toBe(false);
    // the iframe is at -600, 50 on the screen: the passage at 30, 40 inside it is at -570, 90
    expect(found.rect).toEqual({ left: -570, top: 90, width: 100, height: 16, right: -470, bottom: 106 });
    expect(contents.cfiFromRange).toHaveBeenCalled();
    found.clear();
    expect(doc.getSelection().isCollapsed).toBe(true);
  });

  it('waits longer for a finger, and says it was one', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await selectWords(0, 11, 'touch');
    await wait(MOUSE_DELAY + 80);
    expect(onSelection).not.toHaveBeenCalled();
    await wait(TOUCH_DELAY);
    expect(onSelection.mock.calls[0][0].touch).toBe(true);
  });

  it('cuts the excerpt of a long passage', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    doc.body.innerHTML = `<p id="p">${'palavra '.repeat(40)}</p>`;
    await selectWords(0, 300);
    await wait(MOUSE_DELAY + 80);
    expect(onSelection.mock.calls[0][0].locator.excerpt.length).toBe(120);
  });

  it('says nothing again while the selection changes, and not when nothing is selected', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await selectWords(0, 11);
    await wait(MOUSE_DELAY + 80);
    await selectWords(0, 4);
    expect(onSelection).toHaveBeenLastCalledWith(null);
    doc.getSelection().removeAllRanges();
    await act(async () => { doc.dispatchEvent(new frame.contentWindow.Event('selectionchange')); });
    await wait(MOUSE_DELAY + 80);
    expect(onSelection.mock.calls.filter(([v]) => v !== null)).toHaveLength(1);
  });

  it('does not tell about a selection of only spaces', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    doc.body.innerHTML = '<p id="p">a        b</p>';
    await selectWords(1, 8);
    await wait(MOUSE_DELAY + 80);
    expect(onSelection).not.toHaveBeenCalled();
  });

  it('takes the selection away when the page turns, which leaves it behind', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await selectWords(0, 11);
    await wait(MOUSE_DELAY + 80);
    await relocate('epubcfi(/6/4!/4/6)');
    expect(onSelection).toHaveBeenLastCalledWith(null);
  });

  it('listens to nothing when nobody asked', async () => {
    await open();
    frame = document.createElement('iframe');
    container.appendChild(frame);
    doc = frame.contentDocument;
    doc.body.innerHTML = '<p id="p">Era uma vez um texto.</p>';
    state.hooks[0]({ document: doc, addStylesheetCss: vi.fn(), cfiFromRange: vi.fn() });
    const node = doc.querySelector('#p').firstChild;
    const range = doc.createRange();
    range.setStart(node, 0);
    range.setEnd(node, 5);
    doc.getSelection().addRange(range);
    await act(async () => { doc.dispatchEvent(new frame.contentWindow.Event('selectionchange')); });
    await wait(MOUSE_DELAY + 80);
    expect(container.querySelector('[data-epub-page]')).not.toBeNull();
  });
});

describe('the EPUB reader: a finger and the word under it (#180)', () => {
  const RANGE = 'epubcfi(/6/4!/4/2,/1:0,/1:20)';
  let frame;
  let doc;
  const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });
  const later = (ms) => vi.setSystemTime(Date.now() + ms);

  async function bookPage(onSelection) {
    await open({ onSelection });
    frame = document.createElement('iframe');
    container.appendChild(frame);
    doc = frame.contentDocument;
    doc.body.innerHTML = '<p id="p">Era uma vez um texto que se lê devagar.</p>';
    state.hooks[0]({ document: doc, addStylesheetCss: vi.fn(), cfiFromRange: vi.fn(() => RANGE) });
    doc.defaultView.Range.prototype.getBoundingClientRect = () => ({ left: 30, top: 40, width: 100, height: 16 });
    vi.spyOn(frame, 'getBoundingClientRect').mockReturnValue({ left: -600, top: 50, width: 1200, height: 600 });
  }
  const fire = (name, init = {}, type = 'touch') => {
    const event = new frame.contentWindow.MouseEvent(name, { bubbles: true, cancelable: true, clientX: 900, clientY: 300, ...init });
    Object.defineProperty(event, 'pointerType', { value: type });
    return act(async () => { doc.dispatchEvent(event); });
  };
  // What a phone does with a word: selects it, and says so.
  const selectWord = async () => {
    const node = doc.querySelector('#p').firstChild;
    const range = doc.createRange();
    range.setStart(node, 0);
    range.setEnd(node, 3);
    const sel = doc.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
    await act(async () => { doc.dispatchEvent(new frame.contentWindow.Event('selectionchange')); });
  };
  const selected = () => !doc.getSelection().isCollapsed;
  const told = (onSelection) => onSelection.mock.calls.filter(([v]) => v !== null);

  beforeEach(() => vi.useFakeTimers({ toFake: ['Date'] }));
  afterEach(() => vi.useRealTimers());

  it('a short tap turns the page, and the word the phone selected under it goes, with no menu for it', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown');
    await selectWord();
    later(120);
    await fire('pointerup');
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    expect(selected()).toBe(false);
    await wait(TOUCH_DELAY + 80);
    expect(told(onSelection)).toHaveLength(0);
  });

  it('a tap at the left side turns back, in the same way', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown', { clientX: 100 });
    await selectWord();
    later(100);
    await fire('pointerup', { clientX: 100 });
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
    expect(selected()).toBe(false);
  });

  it('a selection that the phone makes after the tap, for the page that has turned, is let go too', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown');
    later(120);
    await fire('pointerup');
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    later(150); // the word comes a moment after the finger went up
    await selectWord();
    expect(selected()).toBe(false);
    await wait(TOUCH_DELAY + 80);
    expect(told(onSelection)).toHaveLength(0);
  });

  it('but a selection that comes after the quiet time is the person\'s, and is offered', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown');
    later(120);
    await fire('pointerup');
    later(QUIET_AFTER_TURN_MS + 10);
    await fire('pointerdown', { clientX: 300 }); // the finger is back, to select
    await selectWord();
    await wait(TOUCH_DELAY + 80);
    expect(told(onSelection)).toHaveLength(1);
    expect(selected()).toBe(true);
  });

  it('a finger that stays is selecting: it turns nothing, and the selection is offered', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown');
    await selectWord();
    later(TOUCH_TAP_MAX_MS + 100); // a press of 400 ms
    await fire('pointerup');
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(selected()).toBe(true);
    await wait(TOUCH_DELAY + 80);
    expect(told(onSelection)).toHaveLength(1);
  });

  it('a tap on a page that has a selection only lets it go: it does not turn the page', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await selectWord();
    await wait(TOUCH_DELAY + 80);
    expect(told(onSelection)).toHaveLength(1);
    await fire('pointerdown');
    later(100);
    await fire('pointerup');
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(selected()).toBe(false);
  });

  it('a finger that drags to select does not turn the page by the distance it went', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown', { clientX: 900 });
    await selectWord();
    later(200);
    await fire('pointerup', { clientX: 600 }); // far and across: it would be a swipe, if nothing were selected
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(state.rendition.prev).not.toHaveBeenCalled();
    expect(selected()).toBe(true);
  });

  it('a swipe with nothing selected turns the page, and quiets the late word as well', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown', { clientX: 900 });
    later(150);
    await fire('pointerup', { clientX: 600 });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await selectWord();
    expect(selected()).toBe(false);
  });

  it('a mouse does not get the quiet time: a double click that selects a word is offered at once', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown', {}, 'mouse');
    later(100);
    await fire('pointerup', {}, 'mouse'); // a click at the side: it turns, and there is no selection to let go of
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await fire('pointerdown', {}, 'mouse');
    await selectWord();
    await wait(MOUSE_DELAY + 80);
    expect(told(onSelection)).toHaveLength(1);
  });

  it('a mouse that selects a word by double click does not turn the page', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown', {}, 'mouse');
    await selectWord();
    later(100);
    await fire('pointerup', {}, 'mouse');
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(selected()).toBe(true);
  });

  it('a tap in the middle shows the controls and lets go of the word under it the same way', async () => {
    const onSelection = vi.fn();
    await bookPage(onSelection);
    await fire('pointerdown', { clientX: 600 }); // the middle of the page (the page is as wide as the iframe in this test)
    await selectWord();
    later(100);
    await fire('pointerup', { clientX: 600 });
    expect(selected()).toBe(false);
  });
});

describe('the EPUB reader: the highlights underlined on the page', () => {
  const A = 'epubcfi(/6/4!/4/2,/1:0,/1:20)';
  const B = 'epubcfi(/6/6!/4/2,/1:3,/1:9)';
  const mark = (cfi, color) => ({ cfi, color });

  it('underlines each passage it is given, in the color of the app, once the book is open', async () => {
    await open({ marks: [mark(A), mark(B)] });
    const calls = state.rendition.annotations.highlight.mock.calls;
    expect(calls.map((c) => c[0])).toEqual([A, B]);
    expect(calls[0][3]).toBe('codice-highlight');
    expect(calls[0][4]).toEqual({ fill: '#944516', 'fill-opacity': '0.28', 'mix-blend-mode': 'multiply' });
  });

  it('paints each passage with the color it was given, and terracotta when it has none or one that is not one', async () => {
    await open({ marks: [mark(A, 'indigo'), mark(B, 'sage'), mark('epubcfi(/6/8!/4/2,/1:0,/1:4)', 'sepia'), mark('epubcfi(/6/10!/4/2,/1:0,/1:4)'), mark('epubcfi(/6/12!/4/2,/1:0,/1:4)', 'pink')] });
    const fills = state.rendition.annotations.highlight.mock.calls.map((c) => c[4].fill);
    expect(fills).toEqual(['#3c4d9c', '#4f7a55', '#a8801c', '#944516', '#944516']);
    for (const c of state.rendition.annotations.highlight.mock.calls) expect(c[4]['fill-opacity']).toBe('0.28');
  });

  it('paints a passage again, in the new color, when its color is changed, and leaves the others', async () => {
    await open({ marks: [mark(A, 'terracotta'), mark(B, 'sage')] });
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(A, 'indigo'), mark(B, 'sage')]} />); });
    expect(state.rendition.annotations.remove.mock.calls).toEqual([[A, 'highlight']]);
    const calls = state.rendition.annotations.highlight.mock.calls;
    expect(calls.map((c) => [c[0], c[4].fill])).toEqual([[A, '#944516'], [B, '#4f7a55'], [A, '#3c4d9c']]);
  });

  it('does not paint a passage again when it comes back in the same color', async () => {
    await open({ marks: [mark(A, 'sage')] });
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(A, 'sage')]} />); });
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(A, 'sage'), mark(B, 'sage')]} />); });
    expect(state.rendition.annotations.remove).not.toHaveBeenCalled();
    expect(state.rendition.annotations.highlight).toHaveBeenCalledTimes(2);
  });

  it('underlines the passages once the book has been opened, and not before', async () => {
    let release;
    getBook.mockReturnValue(new Promise((r) => { release = r; }));
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(A)]} />); });
    await act(async () => { release({ data: new ArrayBuffer(8) }); });
    await flush();
    await flush();
    expect(state.rendition.annotations.highlight).toHaveBeenCalledTimes(1);
  });

  it('underlines a new passage, and does not do it again for the ones that are already', async () => {
    await open({ marks: [mark(A)] });
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(A), mark(B)]} />); });
    expect(state.rendition.annotations.highlight.mock.calls.map((c) => c[0])).toEqual([A, B]);
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(A), mark(B)]} />); });
    expect(state.rendition.annotations.highlight).toHaveBeenCalledTimes(2);
  });

  it('takes away the underline of a passage that is not given any more', async () => {
    await open({ marks: [mark(A), mark(B)] });
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(B)]} />); });
    expect(state.rendition.annotations.remove).toHaveBeenCalledTimes(1);
    expect(state.rendition.annotations.remove).toHaveBeenCalledWith(A, 'highlight');
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} />); });
    expect(state.rendition.annotations.remove).toHaveBeenCalledWith(B, 'highlight');
  });

  it('underlines again a passage that was taken away and then given back', async () => {
    await open({ marks: [mark(A)] });
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[]} />); });
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(A)]} />); });
    expect(state.rendition.annotations.highlight.mock.calls.map((c) => c[0])).toEqual([A, A]);
  });

  it('underlines the passages again on the page of another book, which is a new page', async () => {
    await open({ marks: [mark(A)] });
    const first = state.rendition;
    await act(async () => { root.render(<EpubViewer fileUrl="/g.epub" onProgress={vi.fn()} marks={[mark(A)]} />); });
    await flush();
    await flush();
    expect(state.rendition).not.toBe(first);
    expect(state.rendition.annotations.highlight.mock.calls.map((c) => c[0])).toEqual([A]);
  });

  it('is not stopped by a passage the book does not have any more', async () => {
    await open({ marks: [mark(A)] });
    state.rendition.annotations.highlight.mockImplementationOnce(() => { throw new Error('No Section Found'); });
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(A), mark(B)]} />); });
    expect(state.rendition.annotations.highlight.mock.calls.map((c) => c[0])).toContain(B);
  });

  it('does it again for a book that is opened again (after a failure, asked to try again)', async () => {
    getBook.mockRejectedValueOnce(new Error('falhou'));
    vi.spyOn(console, 'error').mockImplementation(() => {});
    await open({ marks: [mark(A)] });
    expect(container.querySelector('[role="alert"]')).not.toBeNull();
    await click([...container.querySelectorAll('[role="alert"] button')].find((b) => b.textContent === 'Tentar de novo'));
    await flush();
    await flush();
    expect(state.rendition.annotations.highlight.mock.calls.map((c) => c[0])).toEqual([A]);
  });

  it('copes with a book whose page has no annotations', async () => {
    await open({ marks: [mark(A)] });
    state.rendition.annotations = undefined;
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[mark(A), mark(B)]} />); });
    expect(container.querySelector('[data-epub-page]')).not.toBeNull();
  });
});

describe('the EPUB reader: the marks are drawn again where the text may have moved (#180)', () => {
  const A = 'epubcfi(/6/4!/4/2,/1:0,/1:20)';
  const B = 'epubcfi(/6/6!/4/2,/1:3,/1:9)';
  const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });
  const painted = () => state.rendition.annotations.highlight.mock.calls.map((c) => c[0]);
  const removed = () => state.rendition.annotations.remove.mock.calls.map((c) => c[0]);

  async function withMarks(marks = [{ cfi: A, color: 'indigo' }, { cfi: B }]) {
    await open({ marks });
    state.rendition.annotations.highlight.mockClear();
    state.rendition.annotations.remove.mockClear();
  }
  const expectRedrawn = () => {
    expect(removed()).toEqual([A, B]);
    expect(painted()).toEqual([A, B]);
  };

  it('when the window or the phone is resized', async () => {
    await withMarks();
    await act(async () => { state.events.resized(); });
    expect(painted()).toEqual([]); // not before the text has settled
    await wait(REDRAW_DELAY - 60);
    expect(painted()).toEqual([]);
    await wait(120);
    expectRedrawn();
  });

  it('when a page comes: the marks of the new page are drawn over the text as it is now', async () => {
    await withMarks();
    await relocate('epubcfi(/6/4!/4/8)');
    await wait(REDRAW_DELAY + 80);
    expectRedrawn();
  });

  it('when the text is laid out again for another letter size', async () => {
    await withMarks();
    await relocate(); // the place the person is at, which the layout goes back to
    await wait(REDRAW_DELAY + 80);
    state.rendition.annotations.highlight.mockClear();
    state.rendition.annotations.remove.mockClear();
    await click(button('Aparência do texto'));
    await click(container.querySelector('[role="dialog"] button[aria-label="Aumentar a letra"]'));
    await wait(80 + 250 + REDRAW_DELAY + 120);
    expect(painted()).toEqual(expect.arrayContaining([A, B]));
    expect(removed()).toEqual(expect.arrayContaining([A, B]));
  });

  it('when a font that arrives after the page is drawn moves the text, and when the fonts of the page are all there', async () => {
    await withMarks();
    const doc = document.implementation.createHTMLDocument('p');
    let loadingdone;
    doc.fonts = { ready: Promise.resolve(), addEventListener: (type, fn) => { if (type === 'loadingdone') loadingdone = fn; } };
    await act(async () => { state.hooks[0]({ document: doc, addStylesheetCss: vi.fn(), cfiFromRange: vi.fn() }); });
    await wait(REDRAW_DELAY + 80); // the fonts that were there already
    expectRedrawn();
    state.rendition.annotations.highlight.mockClear();
    state.rendition.annotations.remove.mockClear();
    await act(async () => { loadingdone(); }); // a font came a moment later
    await wait(REDRAW_DELAY + 80);
    expectRedrawn();
  });

  it('once for a few reasons that come together', async () => {
    await withMarks();
    await act(async () => { state.events.resized(); });
    await relocate();
    await act(async () => { state.events.resized(); });
    await wait(REDRAW_DELAY + 120);
    expect(painted()).toEqual([A, B]);
    expect(removed()).toEqual([A, B]);
  });

  it('draws each mark in its own color, as it did the first time', async () => {
    await withMarks();
    await act(async () => { state.events.resized(); });
    await wait(REDRAW_DELAY + 80);
    const fills = state.rendition.annotations.highlight.mock.calls.map((c) => c[4].fill);
    expect(fills).toEqual(['#3c4d9c', '#944516']);
  });

  it('draws only the marks that are on the page, not the one that was taken away', async () => {
    await withMarks();
    await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} marks={[{ cfi: A, color: 'indigo' }]} />); });
    state.rendition.annotations.highlight.mockClear();
    state.rendition.annotations.remove.mockClear();
    await act(async () => { state.events.resized(); });
    await wait(REDRAW_DELAY + 80);
    expect(painted()).toEqual([A]);
  });

  it('is not stopped by a passage the book does not have any more, and does nothing without any marks', async () => {
    await withMarks();
    state.rendition.annotations.highlight.mockImplementationOnce(() => { throw new Error('No Section Found'); });
    await act(async () => { state.events.resized(); });
    await wait(REDRAW_DELAY + 80);
    expect(painted()).toEqual([A, B]);
    state.rendition.annotations.highlight.mockClear();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ marks: [] });
    state.rendition.annotations.highlight.mockClear();
    await act(async () => { state.events.resized(); });
    await wait(REDRAW_DELAY + 80);
    expect(painted()).toEqual([]);
  });

  it('does not draw after the reader is gone', async () => {
    await withMarks();
    const rendition = state.rendition;
    await act(async () => { state.events.resized(); });
    act(() => root.unmount());
    root = createRoot(container);
    await wait(REDRAW_DELAY + 80);
    expect(rendition.annotations.highlight).not.toHaveBeenCalled();
  });
});

describe('the EPUB reader: a book that carries scripts', () => {
  it('is opened with no script allowed: the page of the book is on the origin of the app, and a script there could read the session', async () => {
    await open();
    expect(state.renderOptions).toHaveLength(1);
    expect(state.renderOptions[0].allowScriptedContent).toBe(false);
    expect(state.renderOptions[0].allowPopups).toBeUndefined(); // nor a window of its own: epub.js leaves it off
  });

  it('is opened the same way each time, after a failure and a new attempt', async () => {
    getBook.mockRejectedValueOnce(new Error('falhou'));
    vi.spyOn(console, 'error').mockImplementation(() => {});
    await open();
    await click([...container.querySelectorAll('[role="alert"] button')].find((b) => b.textContent === 'Tentar de novo'));
    await flush();
    await flush();
    expect(state.renderOptions.length).toBeGreaterThan(0);
    for (const options of state.renderOptions) expect(options.allowScriptedContent).toBe(false);
  });
});

describe('the EPUB reader: where the person is, in words (DEC-148)', () => {
  const saved = async (onProgress) => {
    await act(async () => { await new Promise((r) => setTimeout(r, 1100)); }); // the save is debounced by a second
    return onProgress.mock.calls.at(-1);
  };
  const relocateTo = (index, cfi = 'epubcfi(/6/4!/4)') =>
    act(async () => { state.events.relocated({ start: { cfi, href: 'Text/c2.xhtml', index, percentage: 0.5, displayed: { page: 1, total: 4 } }, end: { cfi } }); });

  it('saves the chapter that holds the place and which position of how many', async () => {
    state.toc = [{ label: 'Um', href: 'Text/c1.xhtml' }, { label: ' Dois ', href: 'Text/c2.xhtml#inicio' }];
    state.locationsLength = 40;
    state.locationAt = 11;
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    await relocateTo(1);
    const [locator, extras] = await saved(onProgress);
    expect(locator).toMatchObject({ type: 'epub', cfi: 'epubcfi(/6/4!/4)' });
    expect(extras).toMatchObject({ chapter: 'Dois', unitIndex: 12, unitTotal: 40 });
  });

  it('names the chapter, not a section of it, when the file holds both: the person may not have reached the anchor', async () => {
    state.toc = [{ label: 'Capítulo 2', href: 'Text/c2.xhtml' }, { label: 'Uma seção do 2', href: 'Text/c2.xhtml#sec' }];
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    await relocateTo(1);
    expect((await saved(onProgress))[1].chapter).toBe('Capítulo 2');
  });

  it('says no position while the positions of the book are not known, and the chapter all the same', async () => {
    state.toc = [{ label: 'Um', href: 'Text/c1.xhtml' }];
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    await relocateTo(1);
    const [, extras] = await saved(onProgress);
    expect(extras.chapter).toBe('Um');
    expect(extras).not.toHaveProperty('unitIndex');
    expect(extras).not.toHaveProperty('unitTotal');
  });

  it('says nothing of a chapter for a book with no table of contents', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    await relocateTo(0);
    const [, extras] = await saved(onProgress);
    expect(extras).not.toHaveProperty('chapter');
  });
});
