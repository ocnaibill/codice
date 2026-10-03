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
    Page: ({ pageNumber }) => <div data-testid="page"><span id="words">Era uma vez, na página {pageNumber}, um texto.</span></div>,
  };
});
vi.mock('react-pdf/dist/Page/AnnotationLayer.css', () => ({}));
vi.mock('react-pdf/dist/Page/TextLayer.css', () => ({}));

import PdfViewer from './PdfViewer';
import { MOUSE_DELAY } from '../../useSelectionWatcher';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });
async function open(props = {}) {
  await act(async () => { root.render(<PdfViewer fileUrl="/f.pdf" initialProgress="5" onProgress={vi.fn().mockResolvedValue({})} {...props} />); });
  await flush();
}
const selectWords = async (from, to) => {
  const node = container.querySelector('#words').firstChild;
  const range = document.createRange();
  range.setStart(node, from);
  range.setEnd(node, to);
  const sel = window.getSelection();
  sel.removeAllRanges();
  sel.addRange(range);
  await act(async () => { document.dispatchEvent(new Event('selectionchange')); });
  await wait(MOUSE_DELAY + 80);
};

beforeEach(() => {
  Range.prototype.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 16, bottom: 36, right: 110 });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  window.getSelection().removeAllRanges();
  delete Range.prototype.getBoundingClientRect;
  vi.restoreAllMocks();
});

describe('the PDF reader: what is selected on the page', () => {
  it('is told to whoever offers what to do with it, with the passage and where it is', async () => {
    const onSelection = vi.fn();
    await open({ onSelection });
    await selectWords(0, 11);
    expect(onSelection).toHaveBeenCalledTimes(1);
    expect(onSelection.mock.calls[0][0].text).toBe('Era uma vez');
    expect(onSelection.mock.calls[0][0].rect.top).toBe(20);
  });

  it('is tied to the page that is shown (the pages count from 0 in the place)', async () => {
    const onSelection = vi.fn();
    await open({ onSelection, initialProgress: '5' });
    await selectWords(0, 11);
    expect(onSelection.mock.calls[0][0].locator).toEqual({ type: 'pdf', page: 4 });
    expect(Object.keys(onSelection.mock.calls[0][0]).sort()).toEqual(['clear', 'locator', 'rect', 'text', 'touch']);
  });

  it('follows the page when it turns', async () => {
    const onSelection = vi.fn();
    await open({ onSelection, initialProgress: '5' });
    await act(async () => { container.querySelector('button[aria-label="Próxima página"]').click(); });
    await selectWords(0, 11);
    expect(onSelection.mock.calls.at(-1)[0].locator).toEqual({ type: 'pdf', page: 5 });
  });

  it('is told that it is gone when the selection is', async () => {
    const onSelection = vi.fn();
    await open({ onSelection });
    await selectWords(0, 11);
    window.getSelection().removeAllRanges();
    await act(async () => { document.dispatchEvent(new Event('selectionchange')); });
    await wait(MOUSE_DELAY + 80);
    expect(onSelection).toHaveBeenLastCalledWith(null);
  });

  it('works when nobody is listening', async () => {
    await open();
    await selectWords(0, 11);
    expect(container.querySelector('[data-testid="page"]')).not.toBeNull();
  });
});
