import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

vi.mock('../../lib/api', () => ({ api: { get: vi.fn() } }));

import { api } from '../../lib/api';
import { useGlobalStore } from '../../store/useGlobalStore';
import { HighlightedSnippet, SearchPage } from './SearchPage';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let client;
const flush = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 300)); });
const button = (label) => [...container.querySelectorAll('button')].find((item) => item.textContent.trim() === label);

const work = { id: 7, title: 'Duna', author: 'Frank Herbert', editions: [
  { files: [{ id: 10, format: 'epub', textStatus: 'ready', textSegments: 35 },
    { id: 11, format: 'pdf', textStatus: 'empty', needsOcr: true },
    { id: 12, format: 'txt', textStatus: 'failed' }] },
] };
const locator = { type: 'epub', href: 'chapter.xhtml', progression: 0.5 };

beforeEach(() => {
  vi.clearAllMocks();
  useGlobalStore.setState({ activeBookId: null, activeFileId: null, sheetWorkId: null });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  api.get.mockImplementation(async (url) => {
    if (url.startsWith('/works?')) return { data: { data: [work], totalPages: 1 } };
    if (url === '/works/7') return { data: work };
    if (url === '/search') return { data: { data: [{ segmentId: 5, workId: 7, fileId: 10, workTitle: 'Duna', workAuthor: 'Frank Herbert', format: 'epub', language: 'pt', section: 'Capítulo 2', origin: 'native', snippet: 'O 💫 universo <script>!', matches: [[2, 3]], locator }], hasMore: false } };
    if (url === '/notes') return { data: { data: [{ id: 1, workId: 7, fileId: 10, sourceAvailable: true, fileAvailable: true, workTitle: 'Duna', workAuthor: 'Frank Herbert', quote: 'universo', body: '<script>não executar</script>', tags: [], locator }], total: 1 } };
    throw new Error(url);
  });
});

afterEach(() => {
  act(() => root.unmount());
  client.clear();
  container.remove();
});

it('renders catalog, passage and personal notes safely, and opens the exact file locator', async () => {
  await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query="universo" /></QueryClientProvider>));
  await flush();
  expect(container.querySelector('[aria-label="Obras"]').textContent).toContain('Duna');
  expect(container.querySelector('[aria-label="Passagens"] mark').textContent).toBe('💫');
  expect(container.querySelector('script')).toBeNull();
  expect(container.querySelector('[aria-label="Anotações"]').textContent).toContain('<script>não executar</script>');
  const passage = container.querySelector('[aria-label="Passagens"]');
  await act(async () => passage.querySelector('button').click());
  expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 10, seek: { locator } });
});

it('limits passages and notes to a selected work and reports files without searchable text', async () => {
  await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query="universo" /></QueryClientProvider>));
  await flush();
  await act(async () => button('Buscar só nesta obra').click());
  await flush();
  expect(api.get).toHaveBeenCalledWith('/search', { params: { q: 'universo', workId: 7, limit: 10, offset: 0 } });
  expect(api.get).toHaveBeenCalledWith('/notes', { params: { q: 'universo', workId: 7, limit: 10, offset: 0 } });
  expect(container.textContent).toContain('Sem texto pesquisável; OCR necessário');
  expect(container.textContent).toContain('Falha ao ler o texto');
});

it('uses character offsets from the backend even before an astral Unicode symbol', async () => {
  await act(async () => root.render(<HighlightedSnippet text="a💫b" matches={[[1, 2]]} />));
  expect(container.querySelector('mark').textContent).toBe('💫');
});

it('loads the next passage page using the backend offset', async () => {
  api.get.mockImplementation(async (url, options) => {
    if (url.startsWith('/works?')) return { data: { data: [], totalPages: 0 } };
    if (url === '/search') return { data: { data: [{ segmentId: options.params.offset + 1, workId: 7, fileId: 10, workTitle: 'Duna', workAuthor: 'Frank Herbert', snippet: 'universo', matches: [], locator }], hasMore: options.params.offset === 0 } };
    if (url === '/notes') return { data: { data: [], total: 0 } };
    throw new Error(url);
  });
  await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query="universo" /></QueryClientProvider>));
  await flush();
  const passage = container.querySelector('[aria-label="Passagens"]');
  await act(async () => [...passage.querySelectorAll('button')].find((item) => item.textContent === 'Próxima').click());
  await flush();
  expect(api.get).toHaveBeenCalledWith('/search', { params: { q: 'universo', workId: undefined, limit: 10, offset: 10 } });
  expect(passage.textContent).toContain('Página 2');
});
