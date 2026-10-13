import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import MangaViewer from './MangaViewer';
import { setPreferenceOwner } from '../../preferences';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;

// The pages of the comic are 200×300 on the screen, 10 and 20 pixels from the corner of the area that holds them.
const SIZES = { offsetLeft: 10, offsetTop: 20, offsetWidth: 200, offsetHeight: 300 };
let restore = [];
function sizeImages(sizes) {
  restore.forEach((fn) => fn());
  restore = Object.entries(sizes).map(([name, value]) => {
    const before = Object.getOwnPropertyDescriptor(HTMLElement.prototype, name);
    Object.defineProperty(HTMLElement.prototype, name, { configurable: true, get() { return this.tagName === 'IMG' ? value : 0; } });
    return () => (before ? Object.defineProperty(HTMLElement.prototype, name, before) : delete HTMLElement.prototype[name]);
  });
}

const settle = () => act(async () => { for (let i = 0; i < 10; i++) await Promise.resolve(); });
async function open(props = {}) {
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ['1.jpg', '2.jpg', '3.jpg', '4.jpg'] })));
  await act(async () => { root.render(<MangaViewer fileUrl="/files/7/x.cbz" workId={7} onProgress={vi.fn()} {...props} />); });
  await settle();
}
// The browser has drawn the image of the page.
const drawn = (alt) => act(async () => { container.querySelector(`img[alt="${alt}"]`).dispatchEvent(new Event('load')); });
const mark = () => container.querySelector('[data-region-mark]');

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  localStorage.clear();
  setPreferenceOwner('ana');
  sizeImages(SIZES);
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  restore.forEach((fn) => fn());
  restore = [];
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('MangaViewer: the balloon a search hit came from', () => {
  const highlight = { index: 1, x: 0.1, y: 0.2, w: 0.5, h: 0.25 };

  it('marks the balloon over the page, from the size the page has on the screen', async () => {
    await open({ initialProgress: '1', highlight });
    await drawn('Página 2');
    const el = mark();
    expect(el).not.toBeNull();
    expect(el.style.left).toBe('30px'); // 10 + 0.1 × 200
    expect(el.style.top).toBe('80px'); // 20 + 0.2 × 300
    expect(el.style.width).toBe('100px');
    expect(el.style.height).toBe('75px');
    // It is a mark, not something the taps stop at; and it is said to the ones who do not see it.
    expect(el.className).toContain('pointer-events-none');
    expect(el.getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelector('[role="status"]').textContent).toContain('página 2');
  });

  it('waits for the image to be drawn: it has no size before that, and the mark is placed when it has', async () => {
    sizeImages({ offsetLeft: 0, offsetTop: 0, offsetWidth: 0, offsetHeight: 0 });
    await open({ initialProgress: '1', highlight });
    expect(mark()).toBeNull();
    sizeImages(SIZES);
    await drawn('Página 2');
    expect(mark()).not.toBeNull();
    expect(mark().style.width).toBe('100px');
  });

  it('is placed again when the window changes size', async () => {
    await open({ initialProgress: '1', highlight });
    await drawn('Página 2');
    sizeImages({ ...SIZES, offsetWidth: 400, offsetHeight: 600 });
    await act(async () => { window.dispatchEvent(new Event('resize')); });
    expect(mark().style.width).toBe('200px');
    expect(mark().style.top).toBe('140px'); // 20 + 0.2 × 600
  });

  it('is not there when the comic opens without a hit', async () => {
    await open({ initialProgress: '1' });
    await drawn('Página 2');
    expect(mark()).toBeNull();
    expect(container.querySelector('[role="status"]')).toBeNull();
  });

  it('goes away with the page that is turned', async () => {
    await open({ initialProgress: '1', highlight });
    await drawn('Página 2');
    expect(mark()).not.toBeNull();
    await act(async () => { container.querySelector('button[aria-label="Próxima página, à direita"]').click(); });
    expect(mark()).toBeNull();
    await act(async () => { container.querySelector('button[aria-label="Página anterior, à esquerda"]').click(); });
    await drawn('Página 2');
    expect(mark()).toBeNull(); // it was a hit, and it was seen
  });

  it('fades by itself after a while', async () => {
    await open({ initialProgress: '1', highlight });
    await drawn('Página 2');
    await act(async () => { vi.advanceTimersByTime(7000); });
    expect(mark()).not.toBeNull();
    await act(async () => { vi.advanceTimersByTime(1500); });
    expect(mark()).toBeNull();
  });

  it('is not shown on a page that is not the one of the hit (the place was not found)', async () => {
    // The page asked for is not in the file: the viewer goes to the first one and says so, and has nothing to mark there.
    await open({ initialProgress: '9', highlight: { ...highlight, index: 9 }, onPlaceFailed: vi.fn() });
    await drawn('Página 1');
    expect(mark()).toBeNull();
  });

  it('marks the page that holds it in a strip, and brings it to the middle until the person scrolls', async () => {
    sizeImages({ ...SIZES, offsetHeight: 3000 });
    container.scrollTo = vi.fn();
    Object.defineProperty(container, 'clientHeight', { configurable: true, value: 600 });
    await open({ initialProgress: '1', declaredMode: 'webtoon', highlight: { index: 1, x: 0.1, y: 0.5, w: 0.5, h: 0.1 } });
    await drawn('Página 2');
    expect(mark()).not.toBeNull();
    // 20 + 0.5 × 3000 = 1520 is the top of the balloon, 300 high: its middle (1670) goes to the middle of the screen (300).
    expect(container.scrollTo).toHaveBeenLastCalledWith({ top: 1370 });

    // A person who is scrolling is not taken back when a page above finishes drawing.
    const calls = container.scrollTo.mock.calls.length;
    await act(async () => { window.dispatchEvent(new Event('wheel')); });
    await drawn('Página 1');
    expect(container.scrollTo.mock.calls.length).toBe(calls);
    expect(mark()).not.toBeNull();
  });
});
