import React, { act, useRef } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { offsetFromFraction, scrollFraction, scrollTopFromOffset, useScrollPosition } from './scrollPosition';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('scroll arithmetic', () => {
  it('turns the scroll into a fraction, and all-visible text into "seen the end"', () => {
    expect(scrollFraction(0, 2000, 500)).toBe(0);
    expect(scrollFraction(750, 2000, 500)).toBe(0.5);
    expect(scrollFraction(5000, 2000, 500)).toBe(1);
    expect(scrollFraction(0, 400, 500)).toBe(1);
  });

  it('goes to an offset and back', () => {
    expect(offsetFromFraction(0.5, 1000)).toBe(500);
    expect(offsetFromFraction(3, 1000)).toBe(1000);
    expect(scrollTopFromOffset(500, 1000, 2000, 500)).toBe(750);
    expect(scrollTopFromOffset(5000, 1000, 2000, 500)).toBe(1500);
    expect(scrollTopFromOffset(10, 0, 2000, 500)).toBe(0);
  });
});

function Text({ length, onProgress, initialProgress, ready = true }) {
  const ref = useRef(null);
  useScrollPosition({ ref, ready, length, onProgress, initialProgress, debounceMs: 20 });
  return (
    <div id="scroller" style={{ overflowY: 'auto' }}>
      <pre ref={ref}>text</pre>
    </div>
  );
}

let container;
let root;
const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });

// jsdom has no layout: the scroller is given the sizes a browser would measure.
async function mount(props) {
  // The sizes must exist before the hook measures, so the element is sized as it is created.
  const define = HTMLElement.prototype;
  const sh = Object.getOwnPropertyDescriptor(define, 'scrollHeight');
  const ch = Object.getOwnPropertyDescriptor(define, 'clientHeight');
  Object.defineProperty(define, 'scrollHeight', { get() { return this.id === 'scroller' ? props.scrollHeight : 0; }, configurable: true });
  Object.defineProperty(define, 'clientHeight', { get() { return this.id === 'scroller' ? props.clientHeight : 0; }, configurable: true });
  mount.restore = () => {
    if (sh) Object.defineProperty(define, 'scrollHeight', sh); else delete define.scrollHeight;
    if (ch) Object.defineProperty(define, 'clientHeight', ch); else delete define.clientHeight;
  };
  await act(async () => { root.render(<Text {...props} />); });
  return container.querySelector('#scroller');
}

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  mount.restore?.();
});

describe('useScrollPosition', () => {
  it('saves where the text was scrolled to, as a character offset and a percentage', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    const scroller = await mount({ length: 1000, onProgress, scrollHeight: 2000, clientHeight: 500 });
    expect(onProgress).not.toHaveBeenCalled(); // opening a long text at the top says nothing

    scroller.scrollTop = 750; // halfway
    await act(async () => { scroller.dispatchEvent(new Event('scroll')); });
    await wait(40);
    expect(onProgress).toHaveBeenCalledTimes(1);
    const [locator, extras] = onProgress.mock.calls[0];
    expect(locator).toEqual({ type: 'text', offset: 500 });
    expect(extras).toEqual({ percent: 50, completed: undefined });
  });

  it('debounces a burst of scrolling into one save', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    const scroller = await mount({ length: 1000, onProgress, scrollHeight: 2000, clientHeight: 500 });
    for (const top of [100, 300, 600]) {
      scroller.scrollTop = top;
      await act(async () => { scroller.dispatchEvent(new Event('scroll')); });
    }
    await wait(60);
    expect(onProgress).toHaveBeenCalledTimes(1);
    expect(onProgress.mock.calls[0][0].offset).toBe(400); // 600 of 1500
  });

  it('is finished when the end has been reached, and says nothing when going back', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    const scroller = await mount({ length: 1000, onProgress, scrollHeight: 2000, clientHeight: 500 });
    scroller.scrollTop = 1500;
    await act(async () => { scroller.dispatchEvent(new Event('scroll')); });
    await wait(40);
    expect(onProgress.mock.calls.at(-1)[1]).toEqual({ percent: 100, completed: true });
    scroller.scrollTop = 300;
    await act(async () => { scroller.dispatchEvent(new Event('scroll')); });
    await wait(40);
    expect(onProgress.mock.calls.at(-1)[1].completed).toBeUndefined();
  });

  it('returns to the saved place, and does not save just for having returned', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    const scroller = await mount({ length: 1000, onProgress, initialProgress: '500', scrollHeight: 2000, clientHeight: 500 });
    expect(scroller.scrollTop).toBe(750);
    // A browser scrolls when the text is put back; that is not the person reading on.
    await act(async () => { scroller.dispatchEvent(new Event('scroll')); });
    await wait(40);
    expect(onProgress).not.toHaveBeenCalled();

    // Reading on from there is saved.
    scroller.scrollTop = 1000;
    await act(async () => { scroller.dispatchEvent(new Event('scroll')); });
    await wait(40);
    expect(onProgress).toHaveBeenCalledTimes(1);
    expect(onProgress.mock.calls[0][0].offset).toBe(667);
  });

  it('counts a text that fits on the screen as read', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await mount({ length: 200, onProgress, scrollHeight: 400, clientHeight: 500 });
    expect(onProgress).toHaveBeenCalledWith({ type: 'text', offset: 200 }, { percent: 100, completed: true });
  });

  it('waits until the text is there', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await mount({ length: 200, onProgress, scrollHeight: 400, clientHeight: 500, ready: false });
    expect(onProgress).not.toHaveBeenCalled();
  });
});
