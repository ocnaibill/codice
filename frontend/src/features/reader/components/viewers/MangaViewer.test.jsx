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
const modeButton = () => [...container.querySelectorAll('button')].find((b) => /^(LTR|RTL|WEBTOON|DOUBLE)$/.test(b.textContent.trim()));

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
    expect(modeButton().textContent.trim()).toBe('LTR');
    await act(async () => { modeButton().click(); });
    expect(modeButton().textContent.trim()).toBe('RTL');
    expect(localStorage.getItem('codice:comic-mode:ana')).toBe('rtl');

    // Another comic, opened later: it starts in RTL.
    act(() => root.unmount());
    root = createRoot(container);
    await open();
    expect(modeButton().textContent.trim()).toBe('RTL');
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

    const [back, backExtras] = await lastSaved('ArrowLeft');
    expect(back).toEqual({ type: 'image', index: 1 });
    expect(backExtras.completed).toBeUndefined(); // going back says nothing about finishing
  });
});

const noteText = () => container.textContent;
const remembered = () => localStorage.getItem('codice:comic-mode:ana');
const modeNow = () => modeButton().textContent.trim();

describe('MangaViewer: how the file says it is read (#19)', () => {
  it('opens a comic that declares right to left that way, whatever was remembered', async () => {
    for (const before of [null, 'ltr', 'webtoon', 'double']) {
      localStorage.clear();
      if (before) localStorage.setItem('codice:comic-mode:ana', before);
      await open({ declaredMode: 'rtl' });
      expect(modeNow()).toBe('RTL');
      act(() => root.unmount());
      root = createRoot(container);
    }
  });

  it('opens a comic that declares itself a strip as a strip, even after a manga read right to left', async () => {
    localStorage.setItem('codice:comic-mode:ana', 'rtl');
    await open({ declaredMode: 'webtoon' });
    expect(modeNow()).toBe('WEBTOON');
    expect(container.textContent).toContain('Scroll');
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
      expect(modeNow(), String(declaredMode)).toBe('RTL');
      act(() => root.unmount());
      root = createRoot(container);
    }
  });

  it('says the mode came from the file, until the person changes it', async () => {
    await open({ declaredMode: 'rtl' });
    expect(noteText()).toContain('pelo arquivo');
    await act(async () => { modeButton().click(); }); // RTL -> WEBTOON
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
    await act(async () => { modeButton().click(); }); // RTL -> WEBTOON
    expect(remembered()).toBe('webtoon');
    act(() => root.unmount());
    root = createRoot(container);
    await open();
    expect(modeNow()).toBe('WEBTOON');
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
