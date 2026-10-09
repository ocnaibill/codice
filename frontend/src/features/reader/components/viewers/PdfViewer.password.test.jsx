import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// The PDF library asks for the password through `onPassword(callback, reason)`; here it asks on mount, and the test answers.
const asks = vi.hoisted(() => ({ onPassword: null, props: null }));
vi.mock('react-pdf', () => {
  const React = require('react');
  return {
    pdfjs: { GlobalWorkerOptions: {} },
    Document: ({ children, onLoadSuccess, onPassword, file }) => {
      asks.onPassword = onPassword;
      asks.props = { file };
      React.useEffect(() => { /* waits for the password */ }, []);
      return <div data-testid="document">{children}</div>;
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
let onCancelPassword;
const dialog = () => container.querySelector('[role="dialog"][aria-label="Este PDF tem senha"]');
const field = () => dialog()?.querySelector('input[type="password"]');
const button = (text) => [...dialog().querySelectorAll('button')].find((b) => b.textContent === text);
const type = (value) => act(async () => {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field(), value);
  field().dispatchEvent(new Event('input', { bubbles: true }));
});
const render = () => act(async () => { root.render(<PdfViewer fileUrl="/files/trancado.pdf" onProgress={vi.fn()} onCancelPassword={onCancelPassword} />); });

beforeEach(() => {
  asks.onPassword = null;
  onCancelPassword = vi.fn();
  localStorage.clear();
  sessionStorage.clear();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('PdfViewer: a PDF with a password (#89)', () => {
  it('asks for the password with its own dialog when the reader of the PDF asks, and not before', async () => {
    await render();
    expect(dialog()).toBeNull();
    expect(typeof asks.onPassword).toBe('function');
    await act(async () => { asks.onPassword(vi.fn(), 1); });
    expect(dialog()).not.toBeNull();
    expect(dialog().textContent).not.toContain('Senha incorreta');
  });

  it('gives the password to the reader of the PDF, once, and closes the dialog', async () => {
    await render();
    const callback = vi.fn();
    await act(async () => { asks.onPassword(callback, 1); });
    await type('segredo');
    await act(async () => { button('Abrir').click(); });
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith('segredo');
    expect(dialog()).toBeNull();
  });

  it('gives the password as it was typed, with its spaces', async () => {
    await render();
    const callback = vi.fn();
    await act(async () => { asks.onPassword(callback, 1); });
    await type('  com espaços  ');
    await act(async () => { button('Abrir').click(); });
    expect(callback).toHaveBeenCalledWith('  com espaços  ');
  });

  it('asks again, and says the password was wrong, when the reader of the PDF refuses it, with the field empty', async () => {
    await render();
    await act(async () => { asks.onPassword(vi.fn(), 1); });
    await type('errada');
    await act(async () => { button('Abrir').click(); });
    await act(async () => { asks.onPassword(vi.fn(), 2); }); // pdf.js: INCORRECT_PASSWORD
    expect(dialog().querySelector('[role="alert"]').textContent).toBe('Senha incorreta. Tente de novo.');
    expect(field().value).toBe('');
  });

  it('does not say "wrong" for the first ask, whatever the other reasons are', async () => {
    await render();
    await act(async () => { asks.onPassword(vi.fn(), 1); });
    expect(dialog().querySelector('[role="alert"]')).toBeNull();
  });

  it('gives up when the person cancels: the book is closed and nothing is given to the reader of the PDF', async () => {
    await render();
    const callback = vi.fn();
    await act(async () => { asks.onPassword(callback, 1); });
    await act(async () => { button('Cancelar').click(); });
    expect(onCancelPassword).toHaveBeenCalledTimes(1);
    expect(callback).not.toHaveBeenCalled();
    expect(dialog()).toBeNull();
  });

  it('gives up with Escape too', async () => {
    await render();
    await act(async () => { asks.onPassword(vi.fn(), 1); });
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(onCancelPassword).toHaveBeenCalledTimes(1);
  });

  it('does not keep the password anywhere', async () => {
    await render();
    await act(async () => { asks.onPassword(vi.fn(), 1); });
    await type('segredo-que-nao-se-guarda');
    await act(async () => { button('Abrir').click(); });
    const kept = JSON.stringify({ ...localStorage }) + JSON.stringify({ ...sessionStorage }) + document.body.innerHTML;
    expect(kept).not.toContain('segredo-que-nao-se-guarda');
  });

  it('works when no one is listening for the cancel', async () => {
    await act(async () => { root.render(<PdfViewer fileUrl="/files/trancado.pdf" onProgress={vi.fn()} />); });
    await act(async () => { asks.onPassword(vi.fn(), 1); });
    await act(async () => { button('Cancelar').click(); });
    expect(dialog()).toBeNull();
  });
});
