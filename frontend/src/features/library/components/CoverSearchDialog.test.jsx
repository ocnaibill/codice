import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axe from 'axe-core';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => `${u}?rt=T`,
}));

import { api } from '../../../lib/api';
import { CoverSearchDialog } from './CoverSearchDialog';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...document.querySelectorAll('button')].find((b) => b.textContent.trim().startsWith(text));
const work = { id: 7, title: 'Duna' };
const found = [
  { file: '7_0_aaaaaaaa.jpg', provider: 'openlibrary', source: 'OpenLibrary', title: 'Duna', author: 'Frank Herbert' },
  { file: '7_1_bbbbbbbb.png', provider: 'google_books', source: 'Google Books', title: 'Dune', author: '' },
];

// What the server answers: the request, then the answer (states are given in the order the page reads them).
function server(...answers) {
  const queue = [...answers];
  api.post.mockImplementation(async (url, body) => ({ data: { state: 'pending', query: body.query, results: [] } }));
  api.get.mockImplementation(async () => {
    const next = queue.length > 1 ? queue.shift() : queue[0];
    return { data: next };
  });
}

async function show() {
  const handlers = { onChoose: vi.fn(), onClose: vi.fn() };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><CoverSearchDialog work={work} {...handlers} /></QueryClientProvider>); });
  await flush();
  await flush();
  return handlers;
}

beforeEach(() => {
  vi.clearAllMocks();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('CoverSearchDialog (DEC-174)', () => {
  it('asks for the search as soon as it opens, with what the work says, and shows what was found', async () => {
    server({ state: 'done', query: '', results: found });
    await show();
    expect(api.post).toHaveBeenCalledWith('/works/7/cover/search', { query: '' });
    const options = [...document.querySelectorAll('li button')];
    expect(options).toHaveLength(2);
    expect(options[0].textContent).toContain('Frank Herbert · OpenLibrary');
    expect(options[1].textContent).toContain('Google Books');
    expect(document.querySelector('li img').getAttribute('src')).toBe('/works/7/cover/candidates/7_0_aaaaaaaa.jpg?rt=T');
  });

  it('says it is searching while the worker has not answered, and reads again', async () => {
    server({ state: 'pending', query: '', results: [] }, { state: 'done', query: '', results: found });
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      await show();
      expect(document.body.textContent).toContain('Buscando nas fontes ligadas');
      expect(button('Buscar').disabled).toBe(true);
      await act(async () => { await vi.advanceTimersByTimeAsync(1600); });
      await flush();
      expect(document.querySelectorAll('li button')).toHaveLength(2);
      expect(document.body.textContent).not.toContain('Buscando nas fontes ligadas');
    } finally {
      vi.useRealTimers();
    }
  });

  it('says where to turn the providers on when none is', async () => {
    server({ state: 'off', query: '', results: [] });
    await show();
    expect(document.body.textContent).toContain('Nenhuma fonte externa está ligada');
    expect(document.body.textContent).toContain('Administração › Provedores');
    expect(document.querySelector('li')).toBeNull();
  });

  it('says so when nothing was found, and when the search failed', async () => {
    server({ state: 'done', query: '', results: [] });
    await show();
    expect(document.body.textContent).toContain('Nenhuma capa encontrada');
    act(() => root.unmount());
    root = createRoot(container);
    server({ state: 'failed', query: '', results: [] });
    await show();
    expect(document.body.textContent).toContain('Não foi possível buscar as capas');
  });

  it('searches again by what was typed', async () => {
    server({ state: 'done', query: '', results: found });
    await show();
    const input = document.querySelector('input');
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, '  Dune Frank Herbert ');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => { button('Buscar').click(); });
    await flush();
    expect(api.post).toHaveBeenLastCalledWith('/works/7/cover/search', { query: 'Dune Frank Herbert' });
  });

  it('hands over the picture chosen as a file, read from the server', async () => {
    server({ state: 'done', query: '', results: found });
    const bytes = new Blob(['jpeg'], { type: 'image/jpeg' });
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, blob: async () => bytes });
    vi.stubGlobal('fetch', fetchMock);
    const { onChoose } = await show();
    await act(async () => { document.querySelectorAll('li button')[0].click(); });
    await flush();
    expect(fetchMock).toHaveBeenCalledWith('/works/7/cover/candidates/7_0_aaaaaaaa.jpg?rt=T');
    const [file] = onChoose.mock.calls[0];
    expect(file).toBeInstanceOf(File);
    expect(file.name).toBe('7_0_aaaaaaaa.jpg');
    expect(file.type).toBe('image/jpeg');
  });

  it('says so, and chooses nothing, when the picture is gone from the server', async () => {
    server({ state: 'done', query: '', results: found });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 404, blob: async () => new Blob(['not found'], { type: 'text/plain' }) }));
    const { onChoose } = await show();
    await act(async () => { document.querySelectorAll('li button')[0].click(); });
    await flush();
    expect(onChoose).not.toHaveBeenCalled();
    expect(document.querySelector('[role="alert"]').textContent).toContain('não está mais disponível');
    expect(document.querySelectorAll('li button')[0].disabled).toBe(false);
  });

  it('shows that it is opening the picture, and takes no other choice meanwhile', async () => {
    server({ state: 'done', query: '', results: found });
    vi.stubGlobal('fetch', vi.fn().mockReturnValue(new Promise(() => {})));
    await show();
    await act(async () => { document.querySelectorAll('li button')[0].click(); });
    const options = [...document.querySelectorAll('li button')];
    expect(options[0].textContent).toContain('Abrindo…');
    expect(options.every((b) => b.disabled)).toBe(true);
  });

  it('closes with the button and with Escape', async () => {
    server({ state: 'done', query: '', results: found });
    const { onClose } = await show();
    await act(async () => { button('Fechar').click(); });
    expect(onClose).toHaveBeenCalledTimes(1);
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it('has no accessibility violations', async () => {
    server({ state: 'done', query: '', results: found });
    await show();
    const results = await axe.run(document.body, { rules: { 'color-contrast': { enabled: false } } });
    expect(results.violations.map((v) => v.id)).toEqual([]);
  });
});
