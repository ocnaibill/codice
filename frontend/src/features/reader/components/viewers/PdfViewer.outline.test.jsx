import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));
// The engine reports the document it was given (a stub the test sets), and shows the page it was asked for.
let pdf;
vi.mock('react-pdf', () => {
  const React = require('react');
  return {
    pdfjs: { GlobalWorkerOptions: {} },
    Document: ({ children, onLoadSuccess }) => {
      React.useEffect(() => { onLoadSuccess(pdf); }, []);
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
const at = (num) => [{ num, gen: 0 }, { name: 'XYZ' }];
const outline = [
  { title: 'Parte I', dest: at(3), items: [{ title: 'Capítulo 1', dest: at(4) }] },
  { title: 'Parte II', dest: at(9) },
];
const withOutline = (items = outline, numPages = 12) => ({ numPages, getOutline: async () => items, getDestination: async () => null, getPageIndex: async (ref) => ref.num - 1 });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim().includes(text));
const page = () => container.querySelector('[data-testid="page"]').textContent;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
async function open(props = {}) {
  await act(async () => { root.render(<PdfViewer fileUrl="/f.pdf" onProgress={vi.fn().mockResolvedValue({})} {...props} />); });
  await flush();
  await flush();
}
const jumpField = () => container.querySelector('[aria-label="Ir para a página"]');
async function typeIn(el, value) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, value);
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
const enter = () => act(async () => { jumpField().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })); });

beforeEach(() => {
  pdf = withOutline();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('PdfViewer: the outline of the PDF (#14)', () => {
  it('offers the outline only when the PDF has one', async () => {
    pdf = withOutline(null);
    await open();
    expect(button('Sumário')).toBeUndefined();
    act(() => root.unmount());
    root = createRoot(container);
    pdf = withOutline();
    await open();
    expect(button('Sumário')).toBeTruthy();
    expect(button('Sumário').getAttribute('aria-expanded')).toBe('false');
    expect(container.querySelector('nav[aria-label="Sumário do PDF"]')).toBeNull();
  });

  it('lists the entries with their pages, indented by depth, and opens a page from one and closes the list', async () => {
    await open();
    await act(async () => { button('Sumário').click(); });
    expect(button('Sumário').getAttribute('aria-expanded')).toBe('true');
    const nav = container.querySelector('nav[aria-label="Sumário do PDF"]');
    const rows = [...nav.querySelectorAll('button')];
    expect(rows.map((r) => r.textContent)).toEqual(['Parte I3', 'Capítulo 14', 'Parte II9']);
    expect(rows[1].style.paddingLeft).not.toBe(rows[0].style.paddingLeft);
    await act(async () => { rows[2].click(); });
    expect(page()).toBe('page 9');
    expect(container.querySelector('nav[aria-label="Sumário do PDF"]')).toBeNull();
    expect(container.textContent).toContain('9 / 12');
  });

  it('saves the page it went to as the progress, like any other page change', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    await act(async () => { button('Sumário').click(); });
    await act(async () => { [...container.querySelectorAll('nav button')][1].click(); });
    await act(async () => { await new Promise((r) => setTimeout(r, 1100)); });
    expect(onProgress.mock.calls.at(-1)[0]).toEqual({ type: 'pdf', page: 3 });
  });

  it('does not offer an entry that goes nowhere', async () => {
    pdf = withOutline([{ title: 'Link', url: 'https://x.org' }, { title: 'Bom', dest: at(2) }, { title: 'Longe', dest: at(99) }]);
    await open();
    await act(async () => { button('Sumário').click(); });
    expect([...container.querySelectorAll('nav button')].map((b) => b.textContent)).toEqual(['Bom2']);
  });
});

describe('PdfViewer: going to a page by number', () => {
  it('goes to the page typed on Enter, and clears the field', async () => {
    await open();
    await typeIn(jumpField(), '7');
    await enter();
    expect(page()).toBe('page 7');
    expect(jumpField().value).toBe('');
  });

  it('ignores a page that does not exist, a number that is not one, and does nothing without Enter', async () => {
    await open();
    await typeIn(jumpField(), '13');
    await enter();
    expect(page()).toBe('page 1');
    await typeIn(jumpField(), '0');
    await enter();
    await typeIn(jumpField(), 'abc');
    await enter();
    await typeIn(jumpField(), '5');
    await act(async () => { jumpField().dispatchEvent(new KeyboardEvent('keydown', { key: '6', bubbles: true })); });
    expect(page()).toBe('page 1'); // another key is not Enter
    await enter();
    expect(page()).toBe('page 5');
  });
});

describe('PdfViewer: a page that is not there (#14)', () => {
  it('opens the first page and says why, when the place asked for is past the end', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: '40', onPlaceFailed });
    expect(onPlaceFailed).toHaveBeenCalledTimes(1);
    expect(onPlaceFailed).toHaveBeenCalledWith({ reason: 'A página 40 não existe: o arquivo tem 12 páginas.' });
    expect(page()).toBe('page 1');
  });

  it('says so too when what was saved is not a page at all', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: 'abc', onPlaceFailed });
    expect(onPlaceFailed).toHaveBeenCalledWith({ reason: 'A página pedida não é válida.' });
    expect(page()).toBe('page 1');
  });

  it('says nothing when the page is there, or nothing was asked for', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: '12', onPlaceFailed });
    expect(onPlaceFailed).not.toHaveBeenCalled();
    expect(page()).toBe('page 12');
    act(() => root.unmount());
    root = createRoot(container);
    await open({ onPlaceFailed });
    expect(onPlaceFailed).not.toHaveBeenCalled();
    expect(page()).toBe('page 1');
  });

  it('works without anyone listening', async () => {
    await open({ initialProgress: '40' });
    expect(page()).toBe('page 1');
  });
});
