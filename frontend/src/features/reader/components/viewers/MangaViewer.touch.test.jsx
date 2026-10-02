import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import MangaViewer from './MangaViewer';
import { setPreferenceOwner } from '../../preferences';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let onImmersiveChange;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const shown = () => [...container.querySelectorAll('img')].map((i) => i.getAttribute('alt'));

async function open({ mode = 'ltr', progress = '4', ...props } = {}) {
  localStorage.setItem('codice:comic-mode:ana', mode);
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => Array.from({ length: 10 }, (_, i) => `${i}.jpg`) })));
  await act(async () => {
    root.render(<MangaViewer fileUrl="/files/7/x.cbz" workId={7} initialProgress={progress} onProgress={vi.fn().mockResolvedValue({})} onImmersiveChange={onImmersiveChange} {...props} />);
  });
  await flush();
}
// A finger (or a mouse): down at `from`, up at `to` `ms` later, on `target` (the page by default). The area is 400 wide.
async function touch({ from = [200, 300], to = from, ms = 80, type = 'touch', button = 0, target = null } = {}) {
  const el = target ?? container.querySelector('img') ?? container.querySelector('[data-comic-page-area]');
  const fire = (name, [x, y]) => {
    const event = new MouseEvent(name, { bubbles: true, cancelable: true, clientX: x, clientY: y, button });
    Object.defineProperty(event, 'pointerType', { value: type });
    el.dispatchEvent(event);
  };
  await act(async () => { fire('pointerdown', from); });
  vi.setSystemTime(Date.now() + ms);
  await act(async () => { fire('pointerup', to); });
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  localStorage.clear();
  setPreferenceOwner('ana');
  onImmersiveChange = vi.fn();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockReturnValue({ left: 0, top: 0, width: 400, height: 600, right: 400, bottom: 600 });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('the comic under a finger: a tap', () => {
  it('left to right: turns back at the left of the page and forward at the right', async () => {
    await open();
    await touch({ from: [40, 300] });
    expect(shown()).toEqual(['Página 4']);
    await touch({ from: [360, 300] });
    await touch({ from: [360, 300] });
    expect(shown()).toEqual(['Página 6']);
  });

  it('right to left: turns forward at the left of the page and back at the right', async () => {
    await open({ mode: 'rtl' });
    await touch({ from: [40, 300] });
    expect(shown()).toEqual(['Página 6']);
    await touch({ from: [360, 300] });
    await touch({ from: [360, 300] });
    expect(shown()).toEqual(['Página 4']);
  });

  it('in the middle shows or hides the controls, and does not turn the page', async () => {
    await open();
    await touch({ from: [200, 300] });
    expect(onImmersiveChange).toHaveBeenLastCalledWith(true);
    expect(shown()).toEqual(['Página 5']);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ immersive: true });
    await touch({ from: [200, 300] });
    expect(onImmersiveChange).toHaveBeenLastCalledWith(false);
  });

  it('the sides are the outer thirds, more or less: 30% from each edge', async () => {
    await open();
    await touch({ from: [119, 300] });
    expect(shown()).toEqual(['Página 4']);
    await touch({ from: [121, 300] });
    expect(shown()).toEqual(['Página 4']);
    expect(onImmersiveChange).toHaveBeenCalledTimes(1);
    await touch({ from: [279, 300] });
    expect(onImmersiveChange).toHaveBeenCalledTimes(2);
    await touch({ from: [281, 300] });
    expect(shown()).toEqual(['Página 5']);
  });

  it('on the empty part around the page it turns too', async () => {
    await open();
    await touch({ from: [360, 300], target: container.querySelector('[data-comic-page-area]') });
    expect(shown()).toEqual(['Página 6']);
  });

  it('two pages together: turns by two', async () => {
    await open({ mode: 'double', progress: '2' });
    await touch({ from: [360, 300], target: container.querySelector('[data-comic-page-area]') });
    expect(shown()).toEqual(['Página 5', 'Página 6']);
  });

  it('is placed by where the page is on the screen, not by the screen', async () => {
    await open();
    Element.prototype.getBoundingClientRect.mockReturnValue({ left: 100, top: 0, width: 400, height: 600, right: 500, bottom: 600 });
    await touch({ from: [130, 300] }); // 30 px into the page: the left side
    expect(shown()).toEqual(['Página 4']);
    await touch({ from: [490, 300] });
    await touch({ from: [490, 300] });
    expect(shown()).toEqual(['Página 6']);
  });

  it('is not a tap if the finger moved or stayed', async () => {
    await open();
    await touch({ from: [360, 300], to: [360, 325] });
    await touch({ from: [360, 300], ms: 900 });
    expect(shown()).toEqual(['Página 5']);
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('does nothing when someone is selecting text, or touches a button', async () => {
    await open();
    vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => 'texto' });
    await touch({ from: [360, 300] });
    window.getSelection.mockReturnValue({ toString: () => '' });
    const link = document.createElement('a');
    container.querySelector('[data-comic-page-area]').appendChild(link);
    await touch({ from: [360, 300], target: link });
    expect(shown()).toEqual(['Página 5']);
  });

  it('a mouse click does the same, but only the main button', async () => {
    await open();
    await touch({ from: [360, 300], type: 'mouse' });
    expect(shown()).toEqual(['Página 6']);
    await touch({ from: [360, 300], type: 'mouse', button: 2 });
    expect(shown()).toEqual(['Página 6']);
    await touch({ from: [200, 300], type: 'mouse' });
    expect(onImmersiveChange).toHaveBeenLastCalledWith(true);
  });

  it('in a strip, a tap anywhere shows or hides the controls and turns nothing', async () => {
    await open({ mode: 'webtoon' });
    const strip = container.querySelector('[data-comic-page-area]');
    await touch({ from: [40, 300], target: strip });
    await touch({ from: [360, 300], target: strip });
    await touch({ from: [200, 300], target: strip });
    expect(onImmersiveChange).toHaveBeenCalledTimes(3);
    expect(onImmersiveChange).toHaveBeenCalledWith(true);
  });

  it('a touch that is cancelled (the start of a pinch) is not a tap', async () => {
    await open();
    const el = container.querySelector('img');
    await act(async () => {
      const down = new MouseEvent('pointerdown', { bubbles: true, clientX: 360, clientY: 300 });
      Object.defineProperty(down, 'pointerType', { value: 'touch' });
      el.dispatchEvent(down);
      el.dispatchEvent(new MouseEvent('pointercancel', { bubbles: true }));
    });
    const up = new MouseEvent('pointerup', { bubbles: true, clientX: 360, clientY: 300 });
    Object.defineProperty(up, 'pointerType', { value: 'touch' });
    await act(async () => { el.dispatchEvent(up); }); // an up with no down is nothing
    expect(shown()).toEqual(['Página 5']);
    await touch({ from: [360, 300] }); // a new one is fine
    expect(shown()).toEqual(['Página 6']);
    await act(async () => { el.dispatchEvent(up); }); // and one tap is one turn
    expect(shown()).toEqual(['Página 6']);
  });
});

describe('the comic under a finger: a swipe', () => {
  it('to the left brings what is at the right: forward when read left to right, back when read right to left', async () => {
    await open();
    await touch({ from: [300, 300], to: [100, 310], ms: 150 });
    expect(shown()).toEqual(['Página 6']);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ mode: 'rtl' });
    await touch({ from: [300, 300], to: [100, 310], ms: 150 });
    expect(shown()).toEqual(['Página 4']);
  });

  it('to the right brings what is at the left: back when read left to right, forward when read right to left', async () => {
    await open();
    await touch({ from: [100, 300], to: [300, 310], ms: 150 });
    expect(shown()).toEqual(['Página 4']);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ mode: 'rtl' });
    await touch({ from: [100, 300], to: [300, 310], ms: 150 });
    expect(shown()).toEqual(['Página 6']);
  });

  it('is not one if it is short or more along than across', async () => {
    await open();
    await touch({ from: [300, 300], to: [250, 300], ms: 150 });
    await touch({ from: [300, 100], to: [200, 400], ms: 150 });
    expect(shown()).toEqual(['Página 5']);
  });

  it('a mouse that drags does not turn the page', async () => {
    await open();
    await touch({ from: [300, 300], to: [100, 300], ms: 150, type: 'mouse' });
    expect(shown()).toEqual(['Página 5']);
  });

  it('a strip is only scrolled: nothing is turned and no place is saved', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ mode: 'webtoon', onProgress });
    await touch({ from: [300, 300], to: [100, 300], ms: 150, target: container.querySelector('[data-comic-page-area]') });
    vi.useRealTimers();
    await act(async () => { await new Promise((r) => setTimeout(r, 1100)); }); // the save is debounced by a second
    expect(onImmersiveChange).not.toHaveBeenCalled();
    expect(onProgress).not.toHaveBeenCalled();
  });
});
