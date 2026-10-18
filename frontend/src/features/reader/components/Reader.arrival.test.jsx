import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// The viewers are not under test: the EPUB one says what it was given to underline.
vi.doMock('./viewers/EpubViewer', () => ({ default: ({ arrival }) => <div data-testid="epub" data-arrival={arrival?.cfi ?? ''}>EpubViewer</div> }));
for (const name of ['PdfViewer', 'MangaViewer', 'TextViewer', 'MarkdownViewer', 'AudioViewer']) {
  vi.doMock(`./viewers/${name}`, () => ({ default: () => <div>{name}</div> }));
}

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useToasts } from '../../../components/ui/toast';
import { setPreferenceOwner } from '../preferences';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

const CFI = 'epubcfi(/6/12!/4/2/336)';

async function open(progress, { seek = null, fromStart = false } = {}) {
  api.get.mockImplementation(async (path) => {
    if (path === '/works/7') {
      return { data: { id: 7, title: 'Livro', author: 'Autora', fileId: 10, fileUrl: '/f/10.epub', format: 'epub', finished: false,
        editions: [{ id: 1, language: 'pt', files: [{ id: 10, format: 'epub', availability: 'available', url: '/f/10.epub' }] }] } };
    }
    if (path === '/progress/files/10') return { data: { revision: 4, position: CFI, completed: false, locator: { type: 'epub', cfi: CFI }, ...progress } };
    throw new Error(`unexpected GET ${path}`);
  });
  api.post.mockResolvedValue({});
  api.put.mockResolvedValue({ data: {} });
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart, seek });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  // the viewers are loaded when they are needed: as many turns as it takes (a slow machine takes more than a few), and one more once it is there
  for (let i = 0; i < 60; i += 1) {
    await flush();
    if (container.querySelector('[data-testid="epub"]')) {
      await flush();
      break;
    }
  }
}

const said = () => useToasts.getState().items.map((t) => t.title);

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  useToasts.getState().clear();
  setPreferenceOwner('ana');
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  useGlobalStore.getState().closeBook();
});

describe('the reader says where another device left the book', () => {
  it('opens an EPUB where a Kindle stopped, says so, and gives the viewer the paragraph to underline', async () => {
    await open({ source: 'koreader', device: 'Kindle da sala' });
    expect(said()).toEqual(['Seu Kindle da sala parou bem aqui']);
    expect(container.querySelector('[data-testid="epub"]').dataset.arrival).toBe(CFI);
  });

  it('says it once for each save: the book opened again says nothing', async () => {
    await open({ source: 'koreader', device: 'Kindle' });
    expect(said()).toHaveLength(1);
    act(() => root.unmount());
    useToasts.getState().clear();
    root = createRoot(container);
    await open({ source: 'koreader', device: 'Kindle' });
    expect(said()).toEqual([]);
    useToasts.getState().clear();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ source: 'koreader', device: 'Kindle', revision: 5 });
    expect(said()).toHaveLength(1);
  });

  it('says nothing when the last save was the web, when the person chose to start over, or when a place was asked for', async () => {
    await open({ source: '', device: 'Web (Linux)' });
    expect(said()).toEqual([]);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ source: 'koreader', device: 'Kindle' }, { fromStart: true });
    expect(said()).toEqual([]);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ source: 'koreader', device: 'Kindle' }, { seek: { locator: { type: 'epub', cfi: 'epubcfi(/6/2!/4/2)' }, n: 1 } });
    expect(said()).toEqual([]);
  });
});
