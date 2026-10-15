import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// The viewer reports its place as soon as it opens (before the effects of the Reader run) and on each page turned.
vi.mock('./viewers/PdfViewer', () => ({
  default: function PdfStub({ initialProgress, onProgress }) {
    const [page, setPage] = React.useState(Number(initialProgress ?? 1) - 1);
    React.useEffect(() => {
      onProgress({ type: 'pdf', page }, { percent: page * 10 });
    }, [page]); // eslint-disable-line react-hooks/exhaustive-deps
    return <button onClick={() => setPage((p) => p + 1)}>turn the page</button>;
  },
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const detail = {
  id: 7, title: 'Duna', author: 'Frank Herbert', fileId: 10, fileUrl: '/f/10', format: 'pdf', finished: false,
  editions: [{ id: 1, language: 'pt', files: [{ id: 10, format: 'pdf', availability: 'available', started: true, completed: false, url: '/f/10' }] }],
};
const wiki = { project: { id: 3 }, books: [], cursor: { book: 0, chapter: 2, label: 'Duna · 2', source: 'progress' }, own: {}, index: [], hasSide: false };

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const click = async (el) => { await act(async () => { el.click(); }); await flush(); };
const saves = () => api.put.mock.calls.filter(([url]) => url === '/progress/files/10').map(([, body]) => body.locator);

async function open(seek) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: detail };
    if (url === '/progress/files/10') return { data: { revision: 1, position: '4', locator: { type: 'pdf', page: 3 } } };
    if (url === '/works/7/wiki') return { data: { wiki } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: { revision: 2 } });
  api.post.mockResolvedValue({});
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek, wikiWorkId: 7, wikiEntity: 'paul' });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
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
  useGlobalStore.getState().goHome();
});

const reference = { locator: { type: 'pdf', page: 1 }, context: { kind: 'wiki', quote: 'Fear is the mind-killer.' }, peek: true, n: 1 };

describe('Reader: a place opened only to look at it (a reference of the wiki, DEC-179)', () => {
  it('does not move the reading position, however many pages are turned, and says so', async () => {
    await open(reference);
    await click(button('turn the page'));
    expect(saves()).toEqual([]);
    expect(container.querySelector('[aria-label="Consulta de um trecho"]').textContent).toContain('Sua posição de leitura continua onde estava');
  });

  it('goes back to the wiki, which was under it', async () => {
    await open(reference);
    await click(button('Voltar à wiki'));
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: null, wikiWorkId: 7, wikiEntity: 'paul' });
  });

  it('moves the position here at once when the person goes on reading from it, and from then on as usual', async () => {
    await open(reference);
    await click(button('turn the page'));
    await click(button('Continuar lendo daqui'));
    expect(saves()).toEqual([{ type: 'pdf', page: 2 }]); // where the person is now: the reference, one page further
    expect(container.querySelector('[aria-label="Consulta de um trecho"]')).toBeNull();
    await click(button('turn the page'));
    expect(saves().at(-1)).toEqual({ type: 'pdf', page: 3 });
  });

  it('saves as always when the place was not opened only to look (a note)', async () => {
    await open({ ...reference, peek: false, context: { kind: 'note', quote: 'x' } });
    expect(saves()).toEqual([{ type: 'pdf', page: 1 }]);
    expect(container.querySelector('[aria-label="Consulta de um trecho"]')).toBeNull();
  });

  it('opens the wiki from the reader, remembering the book to come back to', async () => {
    await open(null);
    await click(container.querySelector('button[aria-label="Wiki desta obra, até onde você leu"]'));
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: null, wikiWorkId: 7, wikiEntity: null, wikiFrom: { workId: 7, fileId: 10 } });
  });
});
