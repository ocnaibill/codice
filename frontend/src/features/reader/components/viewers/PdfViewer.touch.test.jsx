import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));
vi.mock('react-pdf', () => {
  const React = require('react');
  return {
    pdfjs: { GlobalWorkerOptions: {} },
    Document: ({ children, onLoadSuccess }) => {
      React.useEffect(() => { onLoadSuccess({ numPages: 30, getOutline: async () => null }); }, []);
      return <div>{children}</div>;
    },
    Page: ({ pageNumber }) => <div data-testid="page">page {pageNumber}</div>,
  };
});
vi.mock('react-pdf/dist/Page/AnnotationLayer.css', () => ({}));
vi.mock('react-pdf/dist/Page/TextLayer.css', () => ({}));

import PdfViewer from './PdfViewer';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let onImmersiveChange;
const area = () => container.querySelector('[data-pdf-page-area]');
const page = () => container.querySelector('[data-testid="page"]').textContent;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
async function open(props = {}, progress = '10') {
  await act(async () => {
    root.render(<PdfViewer fileUrl="/f.pdf" initialProgress={progress} onProgress={vi.fn().mockResolvedValue({})} onImmersiveChange={onImmersiveChange} {...props} />);
  });
  await flush();
}
// A finger: down at (x1, y1), up at (x2, y2) `ms` later, on `target` (the page by default). The area is 400 wide.
async function touch({ from = [200, 300], to = from, ms = 80, type = 'touch', target = null } = {}) {
  const el = target ?? container.querySelector('[data-testid="page"]');
  const fire = (name, [x, y]) => {
    const event = new MouseEvent(name, { bubbles: true, cancelable: true, clientX: x, clientY: y });
    Object.defineProperty(event, 'pointerType', { value: type });
    el.dispatchEvent(event);
  };
  await act(async () => { fire('pointerdown', from); });
  vi.setSystemTime(Date.now() + ms);
  await act(async () => { fire('pointerup', to); });
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
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
});

describe('the PDF reader under a finger: a tap', () => {
  it('turns back at the left of the page and forward at the right', async () => {
    await open();
    await touch({ from: [40, 300] });
    expect(page()).toBe('page 9');
    await touch({ from: [360, 300] });
    await touch({ from: [360, 300] });
    expect(page()).toBe('page 11');
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('has the sides at three tenths of the page: a tap at 0.25 turns, at 0.35 it shows the controls', async () => {
    await open();
    await touch({ from: [100, 300] });
    expect(page()).toBe('page 9');
    await touch({ from: [140, 300] });
    expect(page()).toBe('page 9');
    expect(onImmersiveChange).toHaveBeenCalledTimes(1);
    await touch({ from: [270, 300] });
    expect(page()).toBe('page 9');
    await touch({ from: [290, 300] });
    expect(page()).toBe('page 10');
  });

  it('measures the tap from the left edge of the page, not of the screen', async () => {
    await open();
    Element.prototype.getBoundingClientRect.mockReturnValue({ left: 100, top: 0, width: 400, height: 600, right: 500, bottom: 600 });
    await touch({ from: [140, 300] }); // 40 px into a page that starts at 100: the left side
    expect(page()).toBe('page 9');
    await touch({ from: [460, 300] }); // 360 px in: the right side
    expect(page()).toBe('page 10');
  });

  it('does not divide by a page that has no width yet', async () => {
    await open();
    Element.prototype.getBoundingClientRect.mockReturnValue({ left: 0, top: 0, width: 0, height: 0, right: 0, bottom: 0 });
    await touch({ from: [0, 300] });
    expect(page()).toBe('page 9');
  });

  it('shows or hides the controls in the middle, according to how they are', async () => {
    await open({ immersive: false });
    await touch({ from: [200, 300] });
    expect(onImmersiveChange).toHaveBeenLastCalledWith(true);
    expect(page()).toBe('page 10');
    await open({ immersive: true });
    await touch({ from: [200, 300] });
    expect(onImmersiveChange).toHaveBeenLastCalledWith(false);
  });

  it('is also taken from a pen, but not from a mouse', async () => {
    await open();
    await touch({ from: [40, 300], type: 'pen' });
    expect(page()).toBe('page 9');
    await touch({ from: [360, 300], type: 'mouse' });
    await touch({ from: [40, 300], type: 'mouse' });
    await touch({ from: [200, 300], type: 'mouse' });
    expect(page()).toBe('page 9');
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('is not a tap if it went on too long or the finger moved', async () => {
    await open();
    await touch({ from: [40, 300], ms: 600 });
    await touch({ from: [40, 300], to: [52, 300] });
    await touch({ from: [40, 300], to: [40, 312] });
    expect(page()).toBe('page 10');
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('is not a tap on someone who is selecting text', async () => {
    await open();
    vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => 'um trecho' });
    await touch({ from: [40, 300] });
    await touch({ from: [200, 300] });
    expect(page()).toBe('page 10');
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('turns the page for an empty selection, and does not break where the browser has no selection', async () => {
    await open();
    vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => '' });
    await touch({ from: [360, 300] });
    expect(page()).toBe('page 11');
    vi.spyOn(window, 'getSelection').mockReturnValue(null);
    await touch({ from: [360, 300] });
    expect(page()).toBe('page 12');
  });

  it('leaves a link, a button or a field to do what it does', async () => {
    await open();
    for (const tag of ['a', 'button', 'input']) {
      const el = document.createElement(tag);
      area().appendChild(el);
      await touch({ from: [40, 300], target: el });
      await touch({ from: [200, 300], target: el });
    }
    expect(page()).toBe('page 10');
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('does not go past the first or the last page', async () => {
    await open({}, '1');
    await touch({ from: [40, 300] });
    expect(page()).toBe('page 1');
    act(() => root.unmount());
    root = createRoot(container);
    await open({}, '30');
    await touch({ from: [360, 300] });
    expect(page()).toBe('page 30');
  });

  it('is nothing for a finger that came up with none that went down, or that was cancelled', async () => {
    await open();
    await act(async () => {
      const up = new MouseEvent('pointerup', { bubbles: true, clientX: 40, clientY: 300 });
      Object.defineProperty(up, 'pointerType', { value: 'touch' });
      container.querySelector('[data-testid="page"]').dispatchEvent(up);
    });
    expect(page()).toBe('page 10');
    const el = container.querySelector('[data-testid="page"]');
    await act(async () => {
      const down = new MouseEvent('pointerdown', { bubbles: true, clientX: 40, clientY: 300 });
      Object.defineProperty(down, 'pointerType', { value: 'touch' });
      el.dispatchEvent(down);
      el.dispatchEvent(new Event('pointercancel', { bubbles: true }));
      const up = new MouseEvent('pointerup', { bubbles: true, clientX: 40, clientY: 300 });
      Object.defineProperty(up, 'pointerType', { value: 'touch' });
      el.dispatchEvent(up);
    });
    expect(page()).toBe('page 10');
  });
});

describe('the PDF reader under a finger: a swipe', () => {
  it('brings the next page to the left and the one before to the right', async () => {
    await open();
    await touch({ from: [300, 300], to: [180, 310] });
    expect(page()).toBe('page 11');
    await touch({ from: [100, 300], to: [250, 290] });
    await touch({ from: [100, 300], to: [250, 290] });
    expect(page()).toBe('page 9');
  });

  it('is not a swipe when it is short, or goes more along than across', async () => {
    await open();
    await touch({ from: [300, 300], to: [260, 300] });
    await touch({ from: [300, 100], to: [200, 500] });
    expect(page()).toBe('page 10');
  });

  it('does not turn the page, and does not toggle, when it went on to be a drag', async () => {
    await open();
    await touch({ from: [300, 300], to: [180, 300] });
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });
});

describe('the PDF reader under a finger: when the page is zoomed', () => {
  const zoomIn = async () => {
    await act(async () => { [...container.querySelectorAll('button')].find((b) => b.getAttribute('aria-label') === 'Aumentar o zoom').click(); });
  };

  it('does not turn the page by a tap at the sides, nor by a swipe: the finger is moving the page', async () => {
    await open();
    await zoomIn();
    await touch({ from: [40, 300] });
    await touch({ from: [360, 300] });
    await touch({ from: [300, 300], to: [120, 300] });
    expect(page()).toBe('page 10');
  });

  it('still shows or hides the controls with a tap, anywhere', async () => {
    await open({ immersive: false });
    await zoomIn();
    await touch({ from: [40, 300] });
    expect(onImmersiveChange).toHaveBeenLastCalledWith(true);
  });

  it('lets the finger move the page both ways, and only up and down when it is fit to the width', async () => {
    await open();
    expect(area().style.touchAction).toBe('pan-y');
    await zoomIn();
    expect(area().style.touchAction).toBe('pan-x pan-y');
  });
});
