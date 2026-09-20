import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));
// The PDF engine is not what is under test: a three-page document that reports itself loaded.
vi.mock('react-pdf', () => {
  const React = require('react');
  return {
    pdfjs: { GlobalWorkerOptions: {} },
    Document: ({ children, onLoadSuccess }) => {
      React.useEffect(() => { onLoadSuccess({ numPages: 3 }); }, []);
      return <div>{children}</div>;
    },
    Page: ({ pageNumber }) => <div>page {pageNumber}</div>,
  };
});
vi.mock('react-pdf/dist/Page/AnnotationLayer.css', () => ({}));
vi.mock('react-pdf/dist/Page/TextLayer.css', () => ({}));

import PdfViewer from './PdfViewer';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('PdfViewer progress', () => {
  it('saves a page index from 0, finishes at the last page, and never says "not finished" going back', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await act(async () => { root.render(<PdfViewer fileUrl="/f.pdf" onProgress={onProgress} initialProgress="2" />); });
    const lastSaved = async (label) => {
      await act(async () => { button(label).click(); });
      await act(async () => { await new Promise((r) => setTimeout(r, 1100)); }); // debounced by a second
      return onProgress.mock.calls.at(-1);
    };

    const [end, endExtras] = await lastSaved('→'); // page 3 of 3
    expect(end).toEqual({ type: 'pdf', page: 2 });
    expect(endExtras).toMatchObject({ percent: 100, completed: true });

    const [back, backExtras] = await lastSaved('←'); // back to page 2
    expect(back).toEqual({ type: 'pdf', page: 1 });
    expect(backExtras.completed).toBeUndefined();
  });
});
