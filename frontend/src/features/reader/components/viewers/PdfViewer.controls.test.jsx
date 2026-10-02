import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));
// The engine: a document of 30 pages (or one that is loading, or one that failed), showing the width it was given.
const engine = vi.hoisted(() => ({ state: 'loaded', pages: 30 }));
vi.mock('react-pdf', () => {
  const React = require('react');
  return {
    pdfjs: { GlobalWorkerOptions: {} },
    Document: ({ children, onLoadSuccess, loading, error }) => {
      React.useEffect(() => { if (engine.state === 'loaded') onLoadSuccess({ numPages: engine.pages, getOutline: async () => null }); }, []);
      if (engine.state === 'loading') return <div>{loading}</div>;
      if (engine.state === 'error') return <div>{error}</div>;
      return <div>{children}</div>;
    },
    Page: ({ pageNumber, width }) => <div data-testid="page" data-width={width}>page {pageNumber}</div>,
  };
});
vi.mock('react-pdf/dist/Page/AnnotationLayer.css', () => ({}));
vi.mock('react-pdf/dist/Page/TextLayer.css', () => ({}));

import PdfViewer from './PdfViewer';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let scroller;
let root;
const label = (name) => [...container.querySelectorAll('button')].find((b) => b.getAttribute('aria-label') === name);
const page = () => container.querySelector('[data-testid="page"]')?.textContent;
const width = () => Number(container.querySelector('[data-testid="page"]').getAttribute('data-width'));
const box = () => container.querySelector('[aria-label="Ir para a página"]');
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const chrome = () => container.querySelector('[data-chrome]');
async function open(props = {}) {
  await act(async () => { root.render(<PdfViewer fileUrl="/f.pdf" onProgress={vi.fn().mockResolvedValue({})} {...props} />); });
  await flush();
}
const press = (key, options = {}, target = window) => act(async () => { target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...options })); });
const click = (el) => act(async () => { el.click(); });
const typeIn = async (value) => {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(box(), value);
    box().dispatchEvent(new Event('input', { bubbles: true }));
  });
};

beforeEach(() => {
  engine.state = 'loaded';
  engine.pages = 30;
  scroller = document.createElement('div');
  document.body.appendChild(scroller);
  container = document.createElement('div');
  // The viewer sits straight in the scrolling area of the reader, which is what scrolls to the top.
  container.scrollTo = vi.fn();
  scroller.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  scroller.remove();
  vi.restoreAllMocks();
});

describe('the controls of the PDF reader', () => {
  it('are in Portuguese, with a name each, and out of the dark palette of before', async () => {
    await open();
    for (const name of ['Página anterior', 'Próxima página', 'Diminuir o zoom', 'Aumentar o zoom', 'Esconder os controles']) {
      expect(label(name), name).toBeDefined();
    }
    expect(container.innerHTML).not.toMatch(/zinc|Rendering PDF|Previous|Next page/);
    expect(chrome().className).toContain('bg-white');
  });

  it('turn the page, and stop at the first and at the last', async () => {
    await open();
    expect(label('Página anterior').disabled).toBe(true);
    await click(label('Próxima página'));
    expect(page()).toBe('page 2');
    expect(label('Página anterior').disabled).toBe(false);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ initialProgress: '30' });
    expect(page()).toBe('page 30');
    expect(label('Próxima página').disabled).toBe(true);
  });

  it('cannot go forward before the document says how long it is', async () => {
    engine.state = 'loading';
    await open();
    expect(label('Próxima página').disabled).toBe(true);
    expect(box().value).toBe('1');
    expect(container.textContent).toContain('/ –');
  });

  it('start a new page at its top', async () => {
    await open();
    await click(label('Próxima página'));
    expect(container.scrollTo).toHaveBeenCalledWith({ top: 0 });
  });

  it('say how many pages there are', async () => {
    await open();
    expect(container.textContent).toContain('/ 30');
  });
});

describe('the zoom', () => {
  it('starts at the width of the screen and steps up and down', async () => {
    await open();
    const fit = width();
    expect(fit).toBe(900);
    await click(label('Aumentar o zoom'));
    expect(width()).toBe(Math.round(fit * 1.25));
    await click(label('Aumentar o zoom'));
    expect(width()).toBe(Math.round(fit * 1.5));
    await click(label('Diminuir o zoom'));
    expect(width()).toBe(Math.round(fit * 1.25));
  });

  it('goes round the steps with the one button of a phone, and back to the width of the screen', async () => {
    await open();
    const cycle = () => label('Zoom 100%, mudar') ?? [...container.querySelectorAll('button')].find((b) => /^Zoom \d+%, mudar$/.test(b.getAttribute('aria-label')));
    expect(cycle().textContent).toBe('100%');
    const seen = [];
    for (let i = 0; i < 6; i += 1) {
      await click(cycle());
      seen.push(cycle().textContent);
    }
    expect(seen).toEqual(['125%', '150%', '200%', '250%', '300%', '100%']);
    expect(width()).toBe(900);
    await click(cycle());
    expect(width()).toBe(Math.round(900 * 1.25));
  });

  it('keeps the two buttons and the number for a larger screen, and only the one button for a phone', async () => {
    await open();
    const classes = (name) => label(name).className;
    expect(classes('Diminuir o zoom')).toContain('max-sm:hidden');
    expect(classes('Aumentar o zoom')).toContain('max-sm:hidden');
    const cycle = [...container.querySelectorAll('button')].find((b) => /^Zoom \d+%, mudar$/.test(b.getAttribute('aria-label')));
    expect(cycle.className).toContain('sm:hidden');
    expect(cycle.className).not.toContain('max-sm');
  });

  it('keeps the page and the number of pages on one line, and the dividers for a larger screen', async () => {
    await open();
    expect(box().parentElement.className).toContain('whitespace-nowrap');
    expect(box().parentElement.className).toContain('shrink-0');
    for (const divider of container.querySelectorAll('[data-chrome] > span[aria-hidden="true"]')) {
      expect(divider.className).toContain('max-sm:hidden');
    }
  });

  it('stops at the ends of its steps', async () => {
    await open();
    expect(label('Diminuir o zoom').disabled).toBe(true);
    for (let i = 0; i < 10; i += 1) await click(label('Aumentar o zoom'));
    expect(label('Aumentar o zoom').disabled).toBe(true);
    expect(width()).toBe(900 * 3);
  });

  it('says where it is and goes back to the width of the screen with one touch', async () => {
    await open();
    await click(label('Aumentar o zoom'));
    const reset = [...container.querySelectorAll('button')].find((b) => b.textContent === '125%');
    expect(reset.getAttribute('aria-label')).toContain('125%');
    expect(reset.disabled).toBe(false);
    await click(reset);
    expect(width()).toBe(900);
    expect([...container.querySelectorAll('button')].find((b) => b.textContent === '100%').disabled).toBe(true);
  });

  it('follows the width the screen gives, when the engine of the browser tells it', async () => {
    let notify;
    globalThis.ResizeObserver = class { constructor(cb) { notify = cb; } observe() {} disconnect() {} };
    try {
      Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get() { return 360; } });
      await open();
      expect(width()).toBe(328);
      Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get() { return 700; } });
      await act(async () => { notify(); });
      expect(width()).toBe(668);
    } finally {
      delete globalThis.ResizeObserver;
      delete HTMLElement.prototype.clientWidth;
    }
  });

  it('follows the window when the browser has no such engine', async () => {
    const before = window.innerWidth;
    try {
      await open();
      expect(width()).toBe(900);
      window.innerWidth = 500;
      await act(async () => { window.dispatchEvent(new Event('resize')); });
      expect(width()).toBe(468);
    } finally {
      window.innerWidth = before;
    }
  });
});

describe('the box of the page', () => {
  it('shows the page it is on, takes digits only, and goes there on Enter', async () => {
    await open();
    expect(box().value).toBe('1');
    await typeIn('1a2b');
    expect(box().value).toBe('12');
    await act(async () => { box().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })); });
    expect(page()).toBe('page 12');
    expect(box().value).toBe('12');
  });

  it('goes there when it is left, and gives up on Escape', async () => {
    await open();
    await typeIn('9');
    await act(async () => { box().dispatchEvent(new FocusEvent('focusout', { bubbles: true })); });
    expect(page()).toBe('page 9');
    await typeIn('20');
    await act(async () => { box().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })); });
    expect(page()).toBe('page 9');
    expect(box().value).toBe('9');
  });

  it('takes at most six digits, and leaves the page alone for a number that is not a page', async () => {
    await open();
    await typeIn('1234567890');
    expect(box().value).toBe('123456');
    await act(async () => { box().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })); });
    expect(page()).toBe('page 1');
    await typeIn('0');
    await act(async () => { box().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })); });
    expect(page()).toBe('page 1');
  });

  it('is two characters wide at least, three with its margin, for one digit', async () => {
    await open({ initialProgress: '5' });
    expect(box().style.width).toBe('3ch');
  });

  it('is wide enough for what is in it', async () => {
    await open({ initialProgress: '12' });
    expect(box().style.width).toBe('3ch');
    await typeIn('12345');
    expect(box().style.width).toBe('6ch');
  });
});

describe('the keyboard', () => {
  it('turns the page with the arrows, Page Up and Page Down, and goes to the first and the last with Home and End', async () => {
    await open({ initialProgress: '10' });
    await press('ArrowRight');
    expect(page()).toBe('page 11');
    await press('ArrowLeft');
    await press('ArrowLeft');
    expect(page()).toBe('page 9');
    await press('PageDown');
    expect(page()).toBe('page 10');
    await press('PageUp');
    expect(page()).toBe('page 9');
    await press('End');
    expect(page()).toBe('page 30');
    await press('Home');
    expect(page()).toBe('page 1');
  });

  it('does not save again a page it is already on', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    container.scrollTo.mockClear();
    await press('Home');
    await typeIn('1');
    await act(async () => { box().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })); });
    await act(async () => { await new Promise((r) => setTimeout(r, 1100)); }); // the save waits a second
    expect(onProgress).not.toHaveBeenCalled();
    expect(container.scrollTo).not.toHaveBeenCalled();
  });

  it('stops zooming at the ends of the steps when the keyboard asks for more', async () => {
    await open();
    for (let i = 0; i < 7; i += 1) await press('+'); // one more than the steps there are
    expect(width()).toBe(900 * 3);
    await press('+');
    expect(width()).toBe(900 * 3);
    for (let i = 0; i < 7; i += 1) await press('-');
    expect(width()).toBe(900);
    await press('-');
    expect(width()).toBe(900);
  });

  it('does not take End before the document is known', async () => {
    engine.state = 'loading';
    await open();
    const event = new KeyboardEvent('keydown', { key: 'End', bubbles: true, cancelable: true });
    await act(async () => { window.dispatchEvent(event); });
    expect(event.defaultPrevented).toBe(false);
  });

  it('does not go past the ends', async () => {
    await open();
    await press('ArrowLeft');
    await press('PageUp');
    expect(page()).toBe('page 1');
    await press('End');
    await press('ArrowRight');
    await press('PageDown');
    expect(page()).toBe('page 30');
  });

  it('zooms with + and -, and goes back with 0', async () => {
    await open();
    await press('+');
    expect(width()).toBe(Math.round(900 * 1.25));
    await press('=');
    expect(width()).toBe(Math.round(900 * 1.5));
    await press('-');
    expect(width()).toBe(Math.round(900 * 1.25));
    await press('0');
    expect(width()).toBe(900);
  });

  it('leaves the arrows to the page when it is zoomed, and keeps Page Up and Page Down for turning', async () => {
    await open({ initialProgress: '10' });
    await press('+');
    await press('ArrowRight');
    await press('ArrowLeft');
    expect(page()).toBe('page 10');
    await press('PageDown');
    expect(page()).toBe('page 11');
    await press('PageUp');
    expect(page()).toBe('page 10');
  });

  it('is not taken from someone who is typing, and not used for a shortcut of the browser', async () => {
    await open({ initialProgress: '10' });
    await press('ArrowRight', {}, box());
    expect(page()).toBe('page 10');
    await press('ArrowRight', { ctrlKey: true });
    await press('ArrowRight', { metaKey: true });
    await press('ArrowRight', { altKey: true });
    expect(page()).toBe('page 10');
    const note = document.createElement('textarea');
    document.body.appendChild(note);
    await press('ArrowRight', {}, note);
    expect(page()).toBe('page 10');
    note.remove();
  });

  it('closes the contents first, then brings the controls back, with Escape', async () => {
    const onImmersiveChange = vi.fn();
    await open({ immersive: true, onImmersiveChange });
    await press('Escape');
    expect(onImmersiveChange).toHaveBeenCalledWith(false);
    onImmersiveChange.mockClear();
    await open({ immersive: false, onImmersiveChange });
    await press('Escape');
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('does not turn a page that is not there before the document is known', async () => {
    engine.state = 'loading';
    await open();
    await press('End');
    await press('ArrowRight');
    expect(page()).toBeUndefined();
    expect(box().value).toBe('1');
  });

  it('does nothing for a key it does not use', async () => {
    await open({ initialProgress: '10' });
    await press('a');
    await press('Enter');
    expect(page()).toBe('page 10');
  });
});

describe('the page when it is loading or cannot be read', () => {
  it('shows the shape of a page, in Portuguese, while it loads', async () => {
    engine.state = 'loading';
    await open();
    const shape = container.querySelector('[aria-label="Carregando o PDF"]');
    expect(shape).not.toBeNull();
    expect(shape.className).toContain('animate-shimmer');
  });

  it('says in Portuguese, as an alert, that the file could not be read', async () => {
    engine.state = 'error';
    await open();
    expect(container.querySelector('[role="alert"]').textContent).toBe('Não foi possível ler este PDF.');
  });
});

describe('the controls hidden: only the page on the screen', () => {
  it('shows the controls, and not the mark of the page, while it is not immersive', async () => {
    await open({ immersive: false });
    expect(chrome().getAttribute('data-chrome')).toBe('shown');
    expect(chrome().hasAttribute('inert')).toBe(false);
    expect(chrome().className).toContain('opacity-100');
    expect(container.querySelector('[data-chrome-mark]').getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelector('[data-chrome-mark]').className).toContain('opacity-0');
  });

  it('takes the controls away from touch and keyboard when it is immersive, and leaves a mark of the page', async () => {
    await open({ immersive: true, initialProgress: '7' });
    expect(chrome().getAttribute('data-chrome')).toBe('hidden');
    expect(chrome().hasAttribute('inert')).toBe(true);
    expect(chrome().className).toContain('opacity-0');
    expect(chrome().className).toContain('pointer-events-none');
    const mark = container.querySelector('[data-chrome-mark]');
    expect(mark.getAttribute('aria-hidden')).toBe('false');
    expect(mark.textContent).toBe('7 / 30');
    expect(mark.className).toContain('opacity-100');
  });

  it('is asked for by the button that hides the controls', async () => {
    const onImmersiveChange = vi.fn();
    await open({ onImmersiveChange });
    await click(label('Esconder os controles'));
    expect(onImmersiveChange).toHaveBeenCalledWith(true);
  });

  it('works when nobody is listening for it', async () => {
    await open();
    await click(label('Esconder os controles'));
    expect(page()).toBe('page 1');
  });
});
