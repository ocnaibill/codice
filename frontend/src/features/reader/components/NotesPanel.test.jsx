import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
vi.mock('../../../lib/download', () => ({ downloadFile: vi.fn() }));

import { api } from '../../../lib/api';
import { downloadFile } from '../../../lib/download';
import { NotesPanel } from './NotesPanel';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const pdfPlace = { type: 'pdf', page: 11 };

const notes = [
  {
    id: 1, kind: 'note', workId: 7, fileId: 4, fileAvailable: true, quote: 'Fear is the mind-killer.',
    body: 'Lembra **Hobbes**. <script>window.hacked = 1</script>', tags: ['medo', 'dune'],
    locator: pdfPlace, createdAt: '2026-09-19T10:00:00Z',
  },
  // The file is in the catalog but missing from the disk: the address is kept, and cannot be opened.
  { id: 2, kind: 'highlight', workId: 7, fileId: 4, fileAvailable: false, quote: 'Um trecho', body: '', tags: [], locator: { type: 'pdf', page: 2 }, createdAt: '2026-09-18T10:00:00Z' },
  { id: 3, kind: 'bookmark', workId: 7, fileId: 4, fileAvailable: true, quote: '', body: '', tags: [], locator: { type: 'pdf', page: 0 }, createdAt: '2026-09-17T10:00:00Z' },
];

let container;
let root;
let props;

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim().includes(text));
const field = (label) => container.querySelector(`[aria-label="${label}"]`);
const item = (id) => container.querySelector(`li[aria-label$=" ${id}"]`);

// React tracks the value of a controlled field itself: type through the native setter.
async function type(el, value) {
  const proto = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  await act(async () => {
    Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, value);
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

async function render(extra = {}) {
  props = { workId: 7, fileId: 4, getLocator: () => pdfPlace, onOpenAt: vi.fn(), onClose: vi.fn(), ...extra };
  api.get.mockResolvedValue({ data: { data: notes, total: notes.length } });
  api.post.mockResolvedValue({ data: { id: 9 } });
  api.patch.mockResolvedValue({});
  api.delete.mockResolvedValue({});
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => {
    root.render(<QueryClientProvider client={client}><NotesPanel {...props} /></QueryClientProvider>);
  });
  await flush();
}

beforeEach(() => {
  vi.clearAllMocks();
  delete window.hacked;
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('NotesPanel', () => {
  it("asks only for this work's notes and lists them with their place and tags", async () => {
    await render();
    expect(api.get).toHaveBeenCalledWith('/notes', { params: { workId: 7, limit: 200 } });
    const first = item(1).textContent;
    expect(first).toContain('Nota');
    expect(first).toContain('PDF, página 12');
    expect(first).toContain('Fear is the mind-killer.');
    expect(first).toContain('#medo');
    expect(item(2).textContent).toContain('Destaque');
    expect(item(3).textContent).toContain('Marcador');
  });

  it('is drawn above the toolbars of the viewers, which sit at z-50', async () => {
    await render();
    const layer = /\bz-\[(\d+)\]/.exec(container.querySelector('aside').className);
    expect(layer && Number(layer[1])).toBeGreaterThan(50);
  });

  it('shows the Markdown of a note as text, never as live HTML', async () => {
    await render();
    expect(item(1).querySelector('strong').textContent).toBe('Hobbes');
    expect(container.querySelector('script')).toBeNull();
    expect(window.hacked).toBeUndefined();
  });

  it('saves a note tied to the place where the person is, with parsed tags', async () => {
    await render();
    await type(field('Sua anotação'), 'minha leitura');
    await type(field('Tags'), ' filosofia , poder ,, ');
    await act(async () => { button('Salvar nota').click(); });
    expect(api.post).toHaveBeenCalledWith('/works/7/notes', {
      kind: 'note', quote: '', body: 'minha leitura', tags: ['filosofia', 'poder'], fileId: 4, locator: pdfPlace,
    });
  });

  it('is a highlight when there is only a quotation, and cannot be saved empty', async () => {
    await render();
    expect(button('Salvar nota').disabled).toBe(true);
    await type(field('Trecho do livro'), 'um trecho');
    expect(button('Salvar nota').disabled).toBe(false);
    await act(async () => { button('Salvar nota').click(); });
    expect(api.post.mock.calls[0][1]).toMatchObject({ kind: 'highlight', quote: 'um trecho', body: '' });
  });

  it('bookmarks the current place with nothing else', async () => {
    await render();
    await act(async () => { button('Marcar aqui').click(); });
    expect(api.post).toHaveBeenCalledWith('/works/7/notes', { kind: 'bookmark', fileId: 4, locator: pdfPlace });
  });

  it('cannot bookmark before there is a place, and still lets a note be written', async () => {
    await render({ getLocator: () => null });
    expect(button('Marcar aqui').disabled).toBe(true);
    expect(container.textContent).toContain('Sem ponto do arquivo ainda');
    await type(field('Sua anotação'), 'sem ponto');
    await act(async () => { button('Salvar nota').click(); });
    expect(api.post.mock.calls[0][1]).toEqual({ kind: 'note', quote: '', body: 'sem ponto', tags: [] });
  });

  it('edits a note through PATCH, starting from what it has', async () => {
    await render();
    await act(async () => { [...item(1).querySelectorAll('button')].find((b) => b.textContent === 'Editar').click(); });
    expect(item(1).querySelector('[aria-label="Tags"]').value).toBe('medo, dune');
    await type(item(1).querySelector('[aria-label="Sua anotação"]'), 'reescrita');
    await act(async () => { [...item(1).querySelectorAll('button')].find((b) => b.textContent === 'Salvar').click(); });
    expect(api.patch).toHaveBeenCalledWith('/notes/1', {
      quote: 'Fear is the mind-killer.', body: 'reescrita', tags: ['medo', 'dune'],
    });
  });

  it('asks twice before deleting', async () => {
    await render();
    const del = () => [...item(2).querySelectorAll('button')].find((b) => /Excluir|Confirmar/.test(b.textContent));
    await act(async () => { del().click(); });
    expect(api.delete).not.toHaveBeenCalled();
    expect(del().textContent).toBe('Confirmar exclusão');
    await act(async () => { del().click(); });
    expect(api.delete).toHaveBeenCalledWith('/notes/2');
  });

  it('opens the book at a note only when its file can still be opened', async () => {
    await render();
    const open = (id) => [...item(id).querySelectorAll('button')].find((b) => b.textContent === 'Abrir neste ponto');
    expect(open(2)).toBeUndefined(); // the file is gone
    expect(item(2).textContent).toContain('arquivo indisponível');
    await act(async () => { open(1).click(); });
    expect(props.onOpenAt).toHaveBeenCalledWith(expect.objectContaining({ id: 1, fileId: 4, locator: pdfPlace }));
  });

  it('tells the person why a note was refused, and what to do when the server only failed', async () => {
    await render();
    await type(field('Sua anotação'), 'x');
    api.post.mockRejectedValueOnce({ response: { status: 400, data: 'a tag has at most 40 characters\n' } });
    await act(async () => { button('Salvar nota').click(); });
    await flush();
    expect(container.textContent).toContain('a tag has at most 40 characters');

    api.post.mockRejectedValueOnce({ response: { status: 500, data: 'pq: connection refused' } });
    await act(async () => { button('Salvar nota').click(); });
    await flush();
    expect(container.textContent).toContain('Não foi possível salvar.');
    expect(container.textContent).not.toContain('pq: connection');
  });

  it('exports the notes of this book', async () => {
    await render();
    await act(async () => { button('Markdown').click(); });
    expect(downloadFile).toHaveBeenCalledWith('/notes/export?format=md&workId=7', 'codice-anotacoes.md');
    await act(async () => { button('JSON').click(); });
    expect(downloadFile).toHaveBeenLastCalledWith('/notes/export?format=json&workId=7', 'codice-anotacoes.json');
  });

  it('says so when there is nothing yet, and offers no export', async () => {
    await render();
    api.get.mockResolvedValue({ data: { data: [], total: 0 } });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => {
      root.render(<QueryClientProvider client={client}><NotesPanel {...props} workId={8} /></QueryClientProvider>);
    });
    await flush();
    expect(container.textContent).toContain('Nenhuma anotação neste livro ainda');
    expect(button('Markdown')).toBeUndefined();
  });
});
