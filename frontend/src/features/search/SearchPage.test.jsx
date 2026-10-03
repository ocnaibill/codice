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
  expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 10, seek: { locator, context: { kind: 'search', quote: 'O 💫 universo <script>!' } } });
});

it('says, when the reader cannot open the place, that it was a search hit or a note', async () => {
  await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query="universo" /></QueryClientProvider>));
  await flush();
  const notes = container.querySelector('[aria-label="Anotações"]');
  await act(async () => [...notes.querySelectorAll('button')].find((b) => b.textContent === 'Abrir neste ponto').click());
  expect(useGlobalStore.getState().seek).toMatchObject({ locator, context: { kind: 'note', quote: 'universo' } });
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

const searchWith = (coverage) => api.get.mockImplementation(async (url) => {
  if (url.startsWith('/works?')) return { data: { data: [work], totalPages: 1 } };
  if (url === '/search') return { data: { data: [], hasMore: false, mode: 'stem', ...(coverage === undefined ? {} : { coverage }) } };
  if (url === '/notes') return { data: { data: [], total: 0 } };
  throw new Error(url);
});
const passagesBox = () => container.querySelector('[aria-label="Passagens"]');

it('says what the search cannot see yet, under the passages, in a notice', async () => {
  searchWith({ reading: 2, failed: 1, noText: 0 });
  await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query="nada" /></QueryClientProvider>));
  await flush();
  const note = passagesBox().querySelector('[role="status"]');
  expect(note.textContent).toContain('A busca ainda não vê tudo: o texto de 2 arquivos ainda está sendo lido e o texto de 1 arquivo não pôde ser lido.');
  expect(passagesBox().textContent).toContain('Nenhuma passagem encontrada.');
  expect(container.querySelector('[aria-label="Obras"] [role="status"]')).toBeNull();
  expect(container.querySelector('[aria-label="Anotações"] [role="status"]')).toBeNull();
});

it('says nothing of it when the search sees everything, or the server does not say', async () => {
  searchWith({ reading: 0, failed: 0, noText: 0 });
  await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query="nada" /></QueryClientProvider>));
  await flush();
  expect(passagesBox().querySelector('[role="status"]')).toBeNull();
  act(() => root.unmount());
  root = createRoot(container);
  client.clear();
  searchWith(null);
  await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query="outra" /></QueryClientProvider>));
  await flush();
  expect(passagesBox().querySelector('[role="status"]')).toBeNull();
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

const passagesWith = (extra) => api.get.mockImplementation(async (url) => {
  if (url.startsWith('/works?')) return { data: { data: [], totalPages: 0 } };
  if (url === '/notes') return { data: { data: [], total: 0 } };
  if (url === '/search') return { data: { data: [{ segmentId: 5, workId: 7, fileId: 10, workTitle: 'Duna', workAuthor: 'Frank Herbert', format: 'epub', language: 'pt', section: '', origin: 'native', snippet: 'A corrida', matches: [[2, 9]], locator }], hasMore: false, ...extra } };
  throw new Error(url);
});
const passagesText = () => container.querySelector('[aria-label="Passagens"]').textContent;
const render = async (query) => { await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query={query} /></QueryClientProvider>)); await flush(); };

it('tells a search by stem what it did and how to ask for the exact word', async () => {
  passagesWith({ mode: 'stem' });
  await render('correr');
  expect(passagesText()).toContain('Inclui outras formas das palavras');
  expect(passagesText()).toContain('use aspas');
  expect(passagesText()).not.toContain('Busca exata');
});

it('says when the search was exact', async () => {
  passagesWith({ mode: 'exact' });
  await render('"correr"');
  expect(passagesText()).toContain('Busca exata: só as palavras como estão escritas.');
  expect(passagesText()).not.toContain('Inclui outras formas');
});

it('says nothing about the mode when there is no passage, or when the server does not tell it', async () => {
  passagesWith({ mode: 'stem', data: [] });
  await render('correr');
  expect(passagesText()).not.toContain('Inclui outras formas');
  passagesWith({});
  client.clear();
  await render('correr outra');
  expect(passagesText()).not.toContain('Inclui outras formas');
  expect(passagesText()).not.toContain('Busca exata');
});

it('says where the text of a passage comes from: the file, OCR, or what a comic or an audio file says of itself', async () => {
  const hit = (id, extra) => ({ segmentId: id, workId: 7, fileId: 10, workTitle: 'Duna', workAuthor: 'Frank Herbert', format: 'epub', language: 'pt', section: '', origin: 'native', snippet: 'texto', matches: [], locator, ...extra });
  api.get.mockImplementation(async (url) => {
    if (url.startsWith('/works?')) return { data: { data: [], totalPages: 0 } };
    if (url === '/notes') return { data: { data: [], total: 0 } };
    if (url === '/search') return { data: { data: [
      hit(1, {}),
      hit(2, { origin: 'ocr', locator: { type: 'pdf', page: 3 } }),
      hit(3, { format: 'm4b', locator: { type: 'audio', track: 0, ms: 65250 } }),
      hit(4, { format: 'cbz', locator: { type: 'image', index: 0, item: 'ComicInfo.xml' } }),
      hit(5, { format: 'cbz', locator: { type: 'image', index: 3 } }),
    ], hasMore: false } };
    throw new Error(url);
  });
  await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query="texto" /></QueryClientProvider>));
  await flush();
  const labels = [...container.querySelectorAll('[aria-label="Passagens"] li')].map((li) => li.querySelectorAll('p')[1].textContent.split(' · ').pop());
  expect(labels).toEqual(['Texto do arquivo', 'Texto reconhecido por OCR', 'Metadados do arquivo', 'Metadados do arquivo', 'Texto do arquivo']);
});

it('says what became of the scan that has no text yet, as far as reading it has got', async () => {
  const files = (ocr) => [{ id: 11, format: 'pdf', textStatus: 'empty', needsOcr: true, ...(ocr ? { ocr } : {}) }];
  const states = [
    [undefined, 'PDF: Sem texto pesquisável; OCR necessário'],
    [{ pages: 5, read: 0, failed: 0 }, 'PDF: Sem texto pesquisável; OCR necessário'],
    [{ pages: 5, read: 0, failed: 0, state: 'queued' }, 'PDF: Páginas sem texto na fila para leitura'],
    [{ pages: 5, read: 2, failed: 0, state: 'reading' }, 'PDF: Lendo as páginas sem texto (2 de 5)'],
    [{ pages: 5, read: 2, failed: 3 }, 'PDF: Texto reconhecido por OCR em 2 de 5 páginas; 3 falharam'],
  ];
  for (const [ocr, expected] of states) {
    api.get.mockImplementation(async (url) => {
      if (url.startsWith('/works?')) return { data: { data: [{ id: 7, title: 'Duna', author: 'F' }], totalPages: 1 } };
      if (url === '/works/7') return { data: { ...work, editions: [{ files: files(ocr) }] } };
      if (url === '/search') return { data: { data: [], hasMore: false } };
      if (url === '/notes') return { data: { data: [], total: 0 } };
      throw new Error(url);
    });
    client.clear();
    await act(async () => root.render(<QueryClientProvider client={client}><SearchPage query="duna" /></QueryClientProvider>));
    await flush();
    await act(async () => button('Buscar só nesta obra')?.click());
    await flush();
    expect(container.textContent, JSON.stringify(ocr)).toContain(expected);
  }
});
