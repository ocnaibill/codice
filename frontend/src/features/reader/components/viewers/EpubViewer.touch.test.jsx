import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ api: { get: vi.fn(async () => ({ data: new ArrayBuffer(8) })) }, authenticatedUrl: (u) => u }));

const state = { rendition: null, hooks: [] };
vi.mock('epubjs', () => ({
  default: () => {
    const rendition = {
      display: vi.fn(async () => {}),
      on: () => {},
      themes: { register: vi.fn(), select: vi.fn(), fontSize: vi.fn() },
      hooks: { content: { register: (fn) => state.hooks.push(fn) } },
      prev: vi.fn(),
      next: vi.fn(),
      resize: vi.fn(),
      destroy: vi.fn(),
    };
    state.rendition = rendition;
    return {
      loaded: { navigation: Promise.resolve({ toc: [] }) },
      opened: Promise.resolve(),
      spine: { spineItems: [], get: () => null },
      locations: { length: () => 0, generate: () => Promise.resolve(), percentageFromCfi: () => null },
      renderTo: () => rendition,
      destroy: () => {},
    };
  },
}));

import EpubViewer from './EpubViewer';
import { setPreferenceOwner } from '../../preferences';
import { TOUCH_CSS } from '../../epubGestures';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let onImmersiveChange;
let frame; // the iframe the book is in, where the test puts it on the screen
let framePosition;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
async function open(props = {}) {
  await act(async () => {
    root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn().mockResolvedValue({})} onImmersiveChange={onImmersiveChange} {...props} />);
  });
  await flush();
  await flush();
}
const panel = () => container.querySelector('[role="dialog"]');

// A finger (or a mouse) on the margin of the page, outside the book: down at `from`, up at `to` `ms` later.
async function touch({ from = [200, 300], to = from, ms = 80, type = 'touch', button = 0, target = null } = {}) {
  const el = target ?? container.querySelector('[data-epub-page]');
  const fire = (name, [x, y]) => {
    const event = new MouseEvent(name, { bubbles: true, cancelable: true, clientX: x, clientY: y, button });
    Object.defineProperty(event, 'pointerType', { value: type });
    el.dispatchEvent(event);
  };
  await act(async () => { fire('pointerdown', from); });
  vi.setSystemTime(Date.now() + ms);
  await act(async () => { fire('pointerup', to); });
}

// The same inside the page of the book (an iframe of its own), whose points are in its own corner.
function bookDoc() {
  frame = document.createElement('iframe');
  container.appendChild(frame);
  state.hooks[0]({ document: frame.contentDocument, addStylesheetCss: vi.fn() });
  return frame.contentDocument;
}
async function touchInBook(doc, { from = [200, 300], to = from, ms = 80, type = 'touch', button = 0, target = null, selected = '' } = {}) {
  frame.contentWindow.getSelection = () => ({ toString: () => selected });
  const el = target ?? doc.body;
  const fire = (name, [x, y]) => {
    const event = new frame.contentWindow.MouseEvent(name, { bubbles: true, cancelable: true, clientX: x, clientY: y, button });
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
  state.hooks = [];
  onImmersiveChange = vi.fn();
  framePosition = { left: 0, top: 0 };
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  // The page of the book is 400 wide, at 0; the iframe is where framePosition says (it moves as the pages turn).
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function rect() {
    if (this === frame) return { ...framePosition, width: 1200, height: 600, right: framePosition.left + 1200, bottom: 600 };
    return { left: 0, top: 0, width: 400, height: 600, right: 400, bottom: 600 };
  });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  frame = null;
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('the EPUB reader under a finger: a tap on the page around the book', () => {
  it('turns back at the left and forward at the right', async () => {
    await open();
    await touch({ from: [40, 300] });
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
    await touch({ from: [360, 300] });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
  });

  it('in the middle shows or hides the controls, and turns nothing', async () => {
    await open();
    await touch({ from: [200, 300] });
    expect(onImmersiveChange).toHaveBeenLastCalledWith(true);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ immersive: true });
    await touch({ from: [200, 300] });
    expect(onImmersiveChange).toHaveBeenLastCalledWith(false);
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(state.rendition.prev).not.toHaveBeenCalled();
  });

  it('the sides are 30% from each edge of the page', async () => {
    await open();
    await touch({ from: [119, 300] });
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
    await touch({ from: [121, 300] });
    await touch({ from: [279, 300] });
    expect(onImmersiveChange).toHaveBeenCalledTimes(2);
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
    expect(state.rendition.next).not.toHaveBeenCalled();
    await touch({ from: [281, 300] });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
  });

  it('is not a tap if the finger moved or stayed', async () => {
    await open();
    await touch({ from: [360, 300], to: [360, 325] });
    await touch({ from: [360, 300], ms: 900 });
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('does nothing on a button or a link, or while text is selected', async () => {
    await open();
    await touch({ from: [360, 300], target: container.querySelector('button[aria-label="Próxima página"]') });
    expect(onImmersiveChange).not.toHaveBeenCalled();
    expect(state.rendition.next).not.toHaveBeenCalled();
    vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => 'texto' });
    await touch({ from: [360, 300] });
    expect(state.rendition.next).not.toHaveBeenCalled();
  });

  it('a mouse turns by click, but only with the main button', async () => {
    await open();
    await touch({ from: [360, 300], type: 'mouse' });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await touch({ from: [360, 300], type: 'mouse', button: 2 });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
  });

  it('a touch that is cancelled (the start of a pinch) is not a tap, and one tap is one turn', async () => {
    await open();
    const el = container.querySelector('[data-epub-page]');
    await act(async () => {
      const down = new MouseEvent('pointerdown', { bubbles: true, clientX: 360, clientY: 300 });
      Object.defineProperty(down, 'pointerType', { value: 'touch' });
      el.dispatchEvent(down);
      el.dispatchEvent(new MouseEvent('pointercancel', { bubbles: true }));
    });
    const up = new MouseEvent('pointerup', { bubbles: true, clientX: 360, clientY: 300 });
    Object.defineProperty(up, 'pointerType', { value: 'touch' });
    await act(async () => { el.dispatchEvent(up); });
    expect(state.rendition.next).not.toHaveBeenCalled();
    await touch({ from: [360, 300] });
    await act(async () => { el.dispatchEvent(up); });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
  });

  it('is placed by where the page is on the screen', async () => {
    await open();
    Element.prototype.getBoundingClientRect.mockImplementation(() => ({ left: 100, top: 0, width: 400, height: 600, right: 500, bottom: 600 }));
    await touch({ from: [130, 300] }); // 30 px into the page: its left side
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
    await touch({ from: [490, 300] });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
  });
});

describe('the EPUB reader under a finger: how long a tap is, and the word a phone selects (#180)', () => {
  it('a finger taps for 300 ms at the most (a press that stays is the person selecting), a mouse clicks for 500', async () => {
    await open();
    await touch({ from: [360, 300], ms: 300 });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await touch({ from: [360, 300], ms: 450 });
    expect(state.rendition.next).toHaveBeenCalledTimes(1); // too long for a finger
    await touch({ from: [360, 300], ms: 450, type: 'mouse' });
    expect(state.rendition.next).toHaveBeenCalledTimes(2); // not for a mouse
  });

  it('around the book: a tap that the phone turned into a selected word still turns the page, and the word goes', async () => {
    await open();
    const clear = vi.fn();
    const current = { text: '' };
    vi.spyOn(window, 'getSelection').mockImplementation(() => ({ toString: () => current.text, removeAllRanges: clear }));
    const el = container.querySelector('[data-epub-page]');
    const fire = (name, [x, y]) => {
      const event = new MouseEvent(name, { bubbles: true, cancelable: true, clientX: x, clientY: y });
      Object.defineProperty(event, 'pointerType', { value: 'touch' });
      el.dispatchEvent(event);
    };
    await act(async () => { fire('pointerdown', [360, 300]); });
    current.text = 'palavra'; // the phone selects the word under the finger
    vi.setSystemTime(Date.now() + 120);
    await act(async () => { fire('pointerup', [360, 300]); });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    expect(clear).toHaveBeenCalledTimes(1);
  });

  it('around the book: a tap on a page that already had a selection only lets it go', async () => {
    await open();
    const clear = vi.fn();
    vi.spyOn(window, 'getSelection').mockImplementation(() => ({ toString: () => 'palavra', removeAllRanges: clear }));
    await touch({ from: [360, 300], ms: 100 });
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(clear).toHaveBeenCalledTimes(1);
  });
});

describe('the EPUB reader under a finger: across the page of the book', () => {
  it('the page of the book keeps up and down for the browser and takes the horizontal drag, so that the swipe that turns the page is not cancelled', async () => {
    await open();
    const addStylesheetCss = vi.fn();
    frame = document.createElement('iframe');
    container.appendChild(frame);
    state.hooks[0]({ document: frame.contentDocument, addStylesheetCss });
    expect(addStylesheetCss).toHaveBeenCalledWith(TOUCH_CSS, 'codice-touch');
    expect(TOUCH_CSS).toBe('html, body { touch-action: pan-y pinch-zoom; }');
  });
});

describe('the EPUB reader under a finger: a swipe', () => {
  it('to the left brings the next page, to the right the one before', async () => {
    await open();
    await touch({ from: [300, 300], to: [100, 310], ms: 150 });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await touch({ from: [100, 300], to: [300, 310], ms: 150 });
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
  });

  it('is not one if it is short or more along than across', async () => {
    await open();
    await touch({ from: [300, 300], to: [250, 300], ms: 150 });
    await touch({ from: [300, 100], to: [200, 400], ms: 150 });
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(state.rendition.prev).not.toHaveBeenCalled();
  });

  it('a mouse that drags does not turn the page', async () => {
    await open();
    await touch({ from: [300, 300], to: [100, 300], ms: 150, type: 'mouse' });
    expect(state.rendition.next).not.toHaveBeenCalled();
  });
});

describe('the EPUB reader under a finger: with a panel open', () => {
  it('a tap on the page only closes the panel, and a swipe does not turn it', async () => {
    await open();
    await act(async () => { container.querySelector('button[aria-label="Aparência do texto"]').click(); });
    expect(panel()).not.toBeNull();
    await touch({ from: [360, 300] });
    expect(panel()).toBeNull();
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(onImmersiveChange).not.toHaveBeenCalled();
    await act(async () => { container.querySelector('button[aria-label="Aparência do texto"]').click(); });
    await touch({ from: [300, 300], to: [100, 300], ms: 150 });
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(panel()).not.toBeNull(); // that was not a tap
  });
});

describe('the EPUB reader under a finger: a tap inside the book (the iframe)', () => {
  it('turns by the side of the page it is on, whatever page of the book the iframe is showing', async () => {
    await open();
    const doc = bookDoc();
    await touchInBook(doc, { from: [40, 300] });
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
    await touchInBook(doc, { from: [360, 300] });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await touchInBook(doc, { from: [200, 300] });
    expect(onImmersiveChange).toHaveBeenCalledWith(true);
  });

  it('brings its points to the screen: the iframe is moved left as the pages turn', async () => {
    await open();
    const doc = bookDoc();
    framePosition = { left: -800, top: 0 }; // the third page of the section is the one in view
    await touchInBook(doc, { from: [840, 300] }); // 40 px into the page in view: its left side
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
    await touchInBook(doc, { from: [1160, 300] });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await touchInBook(doc, { from: [1000, 300] });
    expect(onImmersiveChange).toHaveBeenCalledTimes(1);
  });

  it('keeps the top of the iframe in the count of how far a finger moved', async () => {
    await open();
    const doc = bookDoc();
    framePosition = { left: 0, top: 100 };
    await touchInBook(doc, { from: [360, 300] });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
  });

  it('a swipe turns, in the same way', async () => {
    await open();
    const doc = bookDoc();
    await touchInBook(doc, { from: [300, 300], to: [100, 310], ms: 150 });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await touchInBook(doc, { from: [100, 300], to: [300, 310], ms: 150 });
    expect(state.rendition.prev).toHaveBeenCalledTimes(1);
  });

  it('does nothing on a link, or while text is selected', async () => {
    await open();
    const doc = bookDoc();
    const link = doc.createElement('a');
    link.href = '#x';
    link.textContent = 'ver';
    doc.body.appendChild(link);
    await touchInBook(doc, { from: [360, 300], target: link });
    await touchInBook(doc, { from: [360, 300], selected: 'um trecho' });
    expect(state.rendition.next).not.toHaveBeenCalled();
    expect(onImmersiveChange).not.toHaveBeenCalled();
    const inner = doc.createElement('em');
    link.appendChild(inner);
    await touchInBook(doc, { from: [360, 300], target: inner }); // a link, inside it
    expect(state.rendition.next).not.toHaveBeenCalled();
  });

  it('a mouse clicks to turn, but not by the secondary button or by dragging', async () => {
    await open();
    const doc = bookDoc();
    await touchInBook(doc, { from: [360, 300], type: 'mouse' });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
    await touchInBook(doc, { from: [360, 300], type: 'mouse', button: 2 });
    await touchInBook(doc, { from: [300, 300], to: [100, 300], ms: 150, type: 'mouse' });
    expect(state.rendition.next).toHaveBeenCalledTimes(1);
  });

  it('a tap with the panel open only closes it', async () => {
    await open();
    const doc = bookDoc();
    await act(async () => { container.querySelector('button[aria-label="Aparência do texto"]').click(); });
    await touchInBook(doc, { from: [360, 300] });
    expect(panel()).toBeNull();
    expect(state.rendition.next).not.toHaveBeenCalled();
  });

  it('a cancelled touch is nothing', async () => {
    await open();
    const doc = bookDoc();
    await act(async () => {
      const down = new frame.contentWindow.MouseEvent('pointerdown', { bubbles: true, clientX: 360, clientY: 300 });
      Object.defineProperty(down, 'pointerType', { value: 'touch' });
      doc.body.dispatchEvent(down);
      doc.body.dispatchEvent(new frame.contentWindow.MouseEvent('pointercancel', { bubbles: true }));
    });
    const up = new frame.contentWindow.MouseEvent('pointerup', { bubbles: true, clientX: 360, clientY: 300 });
    Object.defineProperty(up, 'pointerType', { value: 'touch' });
    frame.contentWindow.getSelection = () => ({ toString: () => '' });
    await act(async () => { doc.body.dispatchEvent(up); });
    expect(state.rendition.next).not.toHaveBeenCalled();
  });
});

