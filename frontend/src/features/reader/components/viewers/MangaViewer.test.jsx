import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import MangaViewer from './MangaViewer';
import { setPreferenceOwner } from '../../preferences';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

async function open(props = {}) {
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ['1.jpg', '2.jpg', '3.jpg'] })));
  await act(async () => { root.render(<MangaViewer fileUrl="/files/7/x.cbz" workId={7} onProgress={vi.fn()} {...props} />); });
  await flush();
}
const LABELS = { ltr: 'Esquerda para a direita', rtl: 'Direita para a esquerda', webtoon: 'Tira para rolar', double: 'Página dupla' };
const modeButton = () => container.querySelector('button[aria-haspopup="menu"]');
// The mode now in use, read from the button that opens the list of modes.
const modeNow = () => Object.keys(LABELS).find((id) => modeButton().getAttribute('aria-label') === `Modo de leitura: ${LABELS[id]}`);
// What the person does: opens the list and picks one.
const choose = async (id) => {
  await act(async () => { modeButton().click(); });
  const item = [...container.querySelectorAll('[role="menuitemradio"]')].find((b) => b.textContent.includes(LABELS[id]));
  await act(async () => { item.click(); });
};

beforeEach(() => {
  localStorage.clear();
  setPreferenceOwner('ana');
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('MangaViewer reading mode', () => {
  it('opens in the mode of the last comic read, and remembers the one chosen for the next', async () => {
    await open();
    expect(modeNow()).toBe('ltr');
    await choose('rtl');
    expect(modeNow()).toBe('rtl');
    expect(localStorage.getItem('codice:comic-mode:ana')).toBe('rtl');

    // Another comic, opened later: it starts in RTL.
    act(() => root.unmount());
    root = createRoot(container);
    await open();
    expect(modeNow()).toBe('rtl');
  });

  it('reports finishing at the last page and never reports "not finished" when going back', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress, initialProgress: '1' });
    const lastSaved = async (key) => {
      await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key })); });
      await act(async () => { await new Promise((r) => setTimeout(r, 1100)); }); // the save is debounced by a second
      return onProgress.mock.calls.at(-1);
    };

    const [atEnd, endExtras] = await lastSaved('ArrowRight'); // page index 2 of 3: the last one
    expect(atEnd).toEqual({ type: 'image', index: 2 });
    expect(endExtras.completed).toBe(true);
    expect(endExtras).toMatchObject({ unitIndex: 3, unitTotal: 3 }); // the page, from 1, of how many

    const [back, backExtras] = await lastSaved('ArrowLeft');
    expect(back).toEqual({ type: 'image', index: 1 });
    expect(backExtras.completed).toBeUndefined(); // going back says nothing about finishing
    expect(backExtras).toMatchObject({ unitIndex: 2, unitTotal: 3 });
    expect(backExtras).not.toHaveProperty('chapter'); // a comic's images have no chapter to name
  });
});

const noteText = () => container.textContent;
const remembered = () => localStorage.getItem('codice:comic-mode:ana');

describe('MangaViewer: how the file says it is read (#19)', () => {
  it('opens a comic that declares right to left that way, whatever was remembered', async () => {
    for (const before of [null, 'ltr', 'webtoon', 'double']) {
      localStorage.clear();
      if (before) localStorage.setItem('codice:comic-mode:ana', before);
      await open({ declaredMode: 'rtl' });
      expect(modeNow()).toBe('rtl');
      act(() => root.unmount());
      root = createRoot(container);
    }
  });

  it('opens a comic that declares itself a strip as a strip, even after a manga read right to left', async () => {
    localStorage.setItem('codice:comic-mode:ana', 'rtl');
    await open({ declaredMode: 'webtoon' });
    expect(modeNow()).toBe('webtoon');
    expect(container.textContent).toContain('3 páginas');
  });

  it('does not touch what is remembered: that was not a choice of the person', async () => {
    await open({ declaredMode: 'rtl' });
    expect(remembered()).toBeNull();
    act(() => root.unmount());
    root = createRoot(container);
    localStorage.setItem('codice:comic-mode:ana', 'double');
    await open({ declaredMode: 'webtoon' });
    expect(remembered()).toBe('double');
  });

  it('opens in the remembered mode when the file says nothing, or says something it does not understand', async () => {
    for (const declaredMode of [undefined, null, '', 'ltr', 'double', 'sideways']) {
      localStorage.setItem('codice:comic-mode:ana', 'rtl');
      await open({ declaredMode });
      expect(modeNow(), String(declaredMode)).toBe('rtl');
      act(() => root.unmount());
      root = createRoot(container);
    }
  });

  it('says the mode came from the file, until the person changes it', async () => {
    await open({ declaredMode: 'rtl' });
    expect(noteText()).toContain('pelo arquivo');
    await choose('webtoon');
    expect(noteText()).not.toContain('pelo arquivo');
  });

  it('says nothing about the file when it declared nothing', async () => {
    await open();
    expect(noteText()).not.toContain('pelo arquivo');
  });

  it('says it in the strip view too', async () => {
    await open({ declaredMode: 'webtoon' });
    expect(noteText()).toContain('pelo arquivo');
  });

  it('what the person chooses on it is what is remembered, and the next comic that declares nothing follows it', async () => {
    await open({ declaredMode: 'rtl' });
    await choose('webtoon');
    expect(remembered()).toBe('webtoon');
    act(() => root.unmount());
    root = createRoot(container);
    await open();
    expect(modeNow()).toBe('webtoon');
  });
});

describe('MangaViewer: an image that is not there (#14)', () => {
  it('says so, and shows the first image, when the place asked for is past the last', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: '9', onPlaceFailed });
    expect(onPlaceFailed).toHaveBeenCalledTimes(1);
    expect(onPlaceFailed).toHaveBeenCalledWith({ reason: 'A imagem 10 não existe: o arquivo tem 3 imagens.' });
    expect(container.textContent).toContain('1 / 3');
  });

  it('opens the image that was asked for when it is there, and says nothing', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: '2', onPlaceFailed });
    expect(onPlaceFailed).not.toHaveBeenCalled();
    expect(container.textContent).toContain('3 / 3');
  });

  it('says nothing when nothing was asked for, and works with nobody listening', async () => {
    const onPlaceFailed = vi.fn();
    await open({ onPlaceFailed });
    expect(onPlaceFailed).not.toHaveBeenCalled();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ initialProgress: '9' });
    expect(container.textContent).toContain('1 / 3');
  });
});

describe('MangaViewer: a work marked as a manga (#187)', () => {
  it('opens right to left when the file says nothing, whatever was remembered', async () => {
    for (const before of [null, 'ltr', 'webtoon', 'double']) {
      localStorage.clear();
      if (before) localStorage.setItem('codice:comic-mode:ana', before);
      await open({ comicKind: 'manga' });
      expect(modeNow(), String(before)).toBe('rtl');
      act(() => root.unmount());
      root = createRoot(container);
    }
  });

  it('lets what the file declares win, even a strip', async () => {
    await open({ comicKind: 'manga', declaredMode: 'webtoon' });
    expect(modeNow()).toBe('webtoon');
    expect(noteText()).toContain('pelo arquivo');
    expect(noteText()).not.toContain('pelo tipo da obra');
  });

  it('says the mode came from the kind, until the person changes it, and remembers only that choice', async () => {
    await open({ comicKind: 'manga' });
    expect(remembered()).toBeNull();
    expect(noteText()).toContain('pelo tipo da obra');
    expect(noteText()).not.toContain('pelo arquivo');
    const chip = [...container.querySelectorAll('span')].find((s) => s.textContent.includes('pelo tipo da obra'));
    expect(chip.title).toBe('A obra está marcada como mangá, que se lê da direita para a esquerda.');
    await choose('ltr');
    expect(noteText()).not.toContain('pelo tipo da obra');
    expect(remembered()).toBe('ltr');
  });

  it('follows the remembered mode for a comic, for a work with no kind and for a kind it does not know', async () => {
    for (const comicKind of ['comic', '', undefined, null, 'webcomic']) {
      localStorage.setItem('codice:comic-mode:ana', 'double');
      await open({ comicKind });
      expect(modeNow(), String(comicKind)).toBe('double');
      expect(noteText()).not.toContain('pelo tipo da obra');
      act(() => root.unmount());
      root = createRoot(container);
    }
  });
});

describe('MangaViewer: a series that says how it is read (DEC-166)', () => {
  it('opens in the direction of the series, whatever was remembered and whatever the type of the work says', async () => {
    for (const [direction, comicKind] of [['ltr', 'manga'], ['rtl', 'comic'], ['webtoon', ''], ['rtl', undefined]]) {
      localStorage.setItem('codice:comic-mode:ana', 'double');
      await open({ seriesDirection: direction, comicKind });
      expect(modeNow(), `${direction} ${comicKind}`).toBe(direction);
      act(() => root.unmount());
      root = createRoot(container);
    }
  });

  it('lets what the file declares win over the series', async () => {
    await open({ seriesDirection: 'ltr', declaredMode: 'webtoon' });
    expect(modeNow()).toBe('webtoon');
    expect(noteText()).toContain('pelo arquivo');
    expect(noteText()).not.toContain('pela série');
  });

  it('says it came from the series, until the person changes it, and remembers only that choice', async () => {
    await open({ seriesDirection: 'rtl' });
    expect(remembered()).toBeNull();
    expect(noteText()).toContain('pela série');
    const chip = [...container.querySelectorAll('span')].find((s) => s.textContent.includes('pela série'));
    expect(chip.title).toBe('A série está marcada para ler da direita para a esquerda.');
    await choose('double');
    expect(noteText()).not.toContain('pela série');
    expect(remembered()).toBe('double');
  });

  it('says what each direction of the series means', async () => {
    for (const [direction, said] of [['ltr', 'esquerda para a direita'], ['webtoon', 'tira para rolar']]) {
      await open({ seriesDirection: direction });
      const chip = [...container.querySelectorAll('span')].find((s) => s.textContent.includes('pela série'));
      expect(chip.title.toLowerCase()).toContain(said);
      act(() => root.unmount());
      root = createRoot(container);
    }
  });

  it('ignores a direction it does not know, and the page layout "double", which is not a direction', async () => {
    for (const seriesDirection of ['double', 'sideways', '', null, undefined]) {
      localStorage.setItem('codice:comic-mode:ana', 'ltr');
      await open({ seriesDirection });
      expect(modeNow(), String(seriesDirection)).toBe('ltr');
      expect(noteText()).not.toContain('pela série');
      act(() => root.unmount());
      root = createRoot(container);
    }
  });
});
