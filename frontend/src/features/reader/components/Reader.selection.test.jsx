import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
vi.mock('../copyText', () => ({ copyText: vi.fn() }));
vi.mock('../../../lib/download', () => ({ downloadFile: vi.fn() }));
// A viewer that tells of a selection when asked, and says what it was given about the highlights to underline.
const sent = { selection: null, marks: null, cleared: 0 };
const stub = (kind) => ({
  default: ({ onSelection, marks }) => {
    sent.marks = marks;
    return (
      <div data-testid="viewer" data-kind={kind} data-has-handler={String(typeof onSelection === 'function')}>
        <button onClick={() => onSelection(sent.selection)}>select</button>
        <button onClick={() => onSelection(null)}>deselect</button>
      </div>
    );
  },
});
vi.mock('./viewers/PdfViewer', () => stub('pdf'));
vi.mock('./viewers/EpubViewer', () => stub('epub'));
vi.mock('./viewers/TextViewer', () => stub('txt'));
vi.mock('./viewers/MarkdownViewer', () => stub('md'));
vi.mock('./viewers/MangaViewer', () => stub('cbz'));
vi.mock('./viewers/AudioViewer', () => stub('mp3'));

import { api } from '../../../lib/api';
import { copyText } from '../copyText';
import { useToasts } from '../../../components/ui/toast';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const work = (format) => ({
  id: 7, title: 'Duna', author: 'Frank Herbert', fileId: 10, fileUrl: '/f/10', format, finished: false,
  editions: [{ id: 1, language: 'pt', files: [
    { id: 10, format, availability: 'available', url: '/f/10' },
    { id: 11, format, availability: 'available', url: '/f/11' },
  ] }],
});
const RANGE = 'epubcfi(/6/4!/4/2,/1:0,/1:20)';
const notes = [
  { id: 1, kind: 'highlight', fileId: 10, quote: 'a', locator: { type: 'epub', cfi: RANGE } },
  { id: 2, kind: 'note', fileId: 10, quote: 'b', body: 'x', locator: { type: 'epub', cfi: 'epubcfi(/6/6!/4/2,/1:3,/1:9)' } },
  { id: 3, kind: 'bookmark', fileId: 10, locator: { type: 'epub', cfi: 'epubcfi(/6/8!/4/2,/1:0,/1:1)' } }, // a bookmark is not underlined
  { id: 4, kind: 'highlight', fileId: 11, locator: { type: 'epub', cfi: 'epubcfi(/6/2!/4/2,/1:0,/1:5)' } }, // another file
  { id: 5, kind: 'highlight', fileId: 10, locator: { type: 'epub', cfi: 'epubcfi(/6/4!/4/2)' } }, // a point, not a passage
  { id: 6, kind: 'highlight', fileId: 10, locator: { type: 'pdf', page: 3 } },
  { id: 7, kind: 'highlight', fileId: 10 },
].map((n) => ({ tags: [], body: '', quote: '', ...n }));

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const menu = () => container.querySelector('[role="toolbar"]');
const press = (label, scope = container) => act(async () => { [...scope.querySelectorAll('button')].find((b) => b.textContent === label).click(); });
const toasts = () => useToasts.getState().items;

async function open(format, { saved = null } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work(format) };
    if (url.startsWith('/progress/files/')) return { data: { revision: 1, position: '', locator: saved } };
    if (url === '/notes') return { data: { data: notes, total: notes.length } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: { id: 99 } });
  api.put.mockResolvedValue({ data: {} });
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek: null });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
}
const selectIt = async () => { await press('select'); };

beforeEach(() => {
  vi.clearAllMocks();
  useToasts.getState().clear();
  sent.selection = { text: 'um trecho escolhido', rect: { left: 100, top: 300, width: 100, height: 20, bottom: 320, right: 200 }, touch: false, clear: vi.fn() };
  sent.marks = null;
  copyText.mockResolvedValue(true);
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('Reader: the menu by a selection', () => {
  it.each(['pdf', 'epub', 'txt', 'md'])('is given to the %s viewer, which can tell of a selection', async (format) => {
    await open(format);
    expect(container.querySelector('[data-testid="viewer"]').dataset.hasHandler).toBe('true');
  });

  it.each(['cbz', 'mp3'])('is not given to the %s viewer, which has no text to select', async (format) => {
    await open(format);
    expect(container.querySelector('[data-testid="viewer"]').dataset.hasHandler).toBe('false');
  });

  it('shows the menu when a text is selected, and takes it away when the selection goes', async () => {
    await open('txt');
    expect(menu()).toBeNull();
    await selectIt();
    expect(menu()).not.toBeNull();
    await press('deselect');
    expect(menu()).toBeNull();
  });

  it('copies the passage, says so, clears the selection and closes the menu', async () => {
    await open('txt');
    await selectIt();
    await press('Copiar', menu());
    expect(copyText).toHaveBeenCalledWith('um trecho escolhido');
    expect(sent.selection.clear).toHaveBeenCalled();
    expect(menu()).toBeNull();
    expect(toasts().map((t) => [t.tone, t.title])).toEqual([['success', 'Trecho copiado']]);
  });

  it('says so when the browser does not let it copy', async () => {
    copyText.mockResolvedValue(false);
    await open('txt');
    await selectIt();
    await press('Copiar', menu());
    expect(toasts()).toHaveLength(1);
    expect(toasts()[0]).toMatchObject({ tone: 'error', title: 'Não foi possível copiar' });
    expect(toasts()[0].message).toContain('menu do sistema');
  });

  it('highlights the passage: a highlight with the quote, tied to the file and the place of the reader', async () => {
    await open('pdf', { saved: { type: 'pdf', page: 4 } });
    await selectIt();
    await press('Destacar', menu());
    await flush();
    expect(api.post).toHaveBeenCalledWith('/works/7/notes', { kind: 'highlight', quote: 'um trecho escolhido', fileId: 10, locator: { type: 'pdf', page: 4 } });
    expect(sent.selection.clear).toHaveBeenCalled();
    expect(menu()).toBeNull();
    expect(toasts().map((t) => [t.tone, t.title])).toEqual([['success', 'Destaque salvo']]);
  });

  it('highlights with the place the viewer gave of the passage (a range of an EPUB), and not with the reader\'s', async () => {
    sent.selection.locator = { type: 'epub', cfi: RANGE, excerpt: 'um trecho' };
    await open('epub', { saved: { type: 'epub', cfi: 'epubcfi(/6/2!/4)' } });
    await selectIt();
    await press('Destacar', menu());
    await flush();
    expect(api.post.mock.calls.at(-1)[1]).toEqual({ kind: 'highlight', quote: 'um trecho escolhido', fileId: 10, locator: { type: 'epub', cfi: RANGE, excerpt: 'um trecho' } });
  });

  it('highlights without a place when the reader has none yet', async () => {
    await open('txt');
    await selectIt();
    await press('Destacar', menu());
    await flush();
    expect(api.post.mock.calls.at(-1)[1]).toEqual({ kind: 'highlight', quote: 'um trecho escolhido' });
  });

  it('says why when it could not save the highlight', async () => {
    await open('txt');
    api.post.mockRejectedValue({ response: { status: 400, data: 'a quote is needed' } });
    await selectIt();
    await press('Destacar', menu());
    await flush();
    expect(toasts()).toHaveLength(1);
    expect(toasts()[0]).toMatchObject({ tone: 'error', title: 'Não foi possível salvar o destaque' });
  });

  it('writes a note on the passage: the notes panel opens with it in the quote field, and the menu is gone', async () => {
    await open('txt');
    await selectIt();
    await press('Nota', menu());
    await flush();
    const panel = container.querySelector('aside[aria-label="Anotações"]');
    expect(panel).not.toBeNull();
    expect(panel.querySelector('[aria-label="Trecho do livro"]').value).toBe('um trecho escolhido');
    expect(menu()).toBeNull();
    expect(sent.selection.clear).toHaveBeenCalled();
    expect(api.post.mock.calls.filter(([url]) => url.includes('/notes'))).toEqual([]); // nothing is saved until the person saves the note
  });

  it('ties the note to the place of the passage it was asked on, and the place of the passage is not kept after the note', async () => {
    sent.selection.locator = { type: 'epub', cfi: RANGE };
    await open('epub', { saved: { type: 'epub', cfi: 'epubcfi(/6/2!/4)' } });
    await selectIt();
    await press('Nota', menu());
    await flush();
    const panel = container.querySelector('aside[aria-label="Anotações"]');
    const body = panel.querySelector('[aria-label="Sua anotação"]');
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(body, 'minha ideia');
      body.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await press('Salvar nota', panel);
    await flush();
    expect(api.post).toHaveBeenCalledWith('/works/7/notes', expect.objectContaining({ kind: 'note', quote: 'um trecho escolhido', body: 'minha ideia', fileId: 10, locator: { type: 'epub', cfi: RANGE } }));
  });

  it('does not bring the passage back when the notes panel is opened again later', async () => {
    await open('txt');
    await selectIt();
    await press('Nota', menu());
    await flush();
    await press('✕', container.querySelector('aside[aria-label="Anotações"]'));
    await flush();
    expect(container.querySelector('aside[aria-label="Anotações"]')).toBeNull();
    await act(async () => { container.querySelector('button[aria-label="Notas, destaques e marcadores deste livro"]').click(); });
    await flush();
    expect(container.querySelector('aside [aria-label="Trecho do livro"]').value).toBe('');
  });

  it('does not show the menu over the notes panel', async () => {
    await open('txt');
    await selectIt();
    await press('Nota', menu());
    await flush();
    await selectIt();
    expect(menu()).toBeNull();
  });

  it('closes the menu with Escape, clearing the selection', async () => {
    await open('txt');
    await selectIt();
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(menu()).toBeNull();
    expect(sent.selection.clear).toHaveBeenCalled();
  });

  it('takes the menu away when another file is opened', async () => {
    await open('txt');
    await selectIt();
    expect(menu()).not.toBeNull();
    await act(async () => { useGlobalStore.setState({ activeFileId: 11 }); });
    await flush();
    expect(menu()).toBeNull();
  });

  it('copes with a selection that has nothing to clear', async () => {
    delete sent.selection.clear;
    await open('txt');
    await selectIt();
    await press('Copiar', menu());
    expect(menu()).toBeNull();
  });
});

describe('Reader: the passages underlined on an EPUB', () => {
  it('gives the viewer the passages of this file that were highlighted or written on, by the range of the book', async () => {
    await open('epub');
    expect(sent.marks).toEqual([RANGE, 'epubcfi(/6/6!/4/2,/1:3,/1:9)']);
  });
});
