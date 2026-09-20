import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import MangaViewer from './MangaViewer';

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
    expect(localStorage.getItem('codice:comic-mode')).toBe('rtl');

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
