import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// The viewer is what the Reader talks to: it shows what it was given and can say "that place is not there".
const seen = [];
let mounts = 0;
vi.mock('./viewers/PdfViewer', () => ({
  default: function PdfStub({ initialProgress, locator, onPlaceFailed }) {
    seen.push({ initialProgress, locator });
    React.useEffect(() => { mounts += 1; }, []);
    return (
      <div>
        <span data-testid="at">{initialProgress ?? 'start'}</span>
        <button onClick={() => onPlaceFailed({ reason: 'A página 12 não existe: o arquivo tem 10 páginas.' })}>fail the place</button>
      </div>
    );
  },
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const detail = {
  id: 7, title: 'Duna', author: 'Frank Herbert', fileId: 10, fileUrl: '/f/10', format: 'pdf', finished: false,
  editions: [{ id: 1, language: 'pt', files: [{ id: 10, format: 'pdf', availability: 'available', started: true, completed: false, url: '/f/10' }] },
    { id: 2, language: 'pt', files: [{ id: 11, format: 'pdf', availability: 'available', started: false, completed: false, url: '/f/11' }] }],
};
let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const at = () => container.querySelector('[data-testid="at"]').textContent;
const notice = () => container.querySelector('[role="alert"]');

async function open({ saved = { position: '4', locator: { type: 'pdf', page: 3 } }, seek = null } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: detail };
    if (url === '/progress/files/10' || url === '/progress/files/11') return { data: { revision: 1, ...saved } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: {} });
  api.post.mockResolvedValue({});
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
}
const fail = async () => { await act(async () => { button('fail the place').click(); }); await flush(); };
const askFor = { locator: { type: 'pdf', page: 11 }, context: { kind: 'note', quote: 'Fear is the mind-killer.' }, n: 1 };

beforeEach(() => {
  vi.clearAllMocks();
  seen.length = 0;
  mounts = 0;
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  useGlobalStore.getState().closeBook();
});

describe('Reader: a place that cannot be opened (#14, RF-014)', () => {
  it('gives the viewer the place that was asked for, and the saved one otherwise', async () => {
    await open({ seek: askFor });
    expect(at()).toBe('12');
    expect(seen.at(-1).locator).toEqual({ type: 'pdf', page: 11 });
    act(() => root.unmount());
    root = createRoot(container);
    await open();
    expect(at()).toBe('4');
    expect(seen.at(-1).locator).toEqual({ type: 'pdf', page: 3 });
    expect(notice()).toBeNull();
  });

  it('says where the note pointed, why, and the passage, and goes on to the saved position', async () => {
    await open({ seek: askFor });
    await fail();
    expect(notice().textContent).toContain('Não foi possível abrir o ponto da anotação');
    expect(notice().textContent).toContain('Ele apontava para PDF, página 12.');
    expect(notice().textContent).toContain('A página 12 não existe: o arquivo tem 10 páginas.');
    expect(notice().textContent).toContain('Fear is the mind-killer.');
    expect(useGlobalStore.getState().seek).toBeNull();
    expect(at()).toBe('4'); // the saved position, where the person was
  });

  it('lets the person stay where they are, which takes the notice away and moves nothing', async () => {
    await open({ seek: askFor });
    await fail();
    await act(async () => { button('Ficar onde estou').click(); });
    expect(notice()).toBeNull();
    expect(at()).toBe('4');
  });

  it('opens from the start on request, and does not ask again', async () => {
    await open({ seek: askFor });
    await fail();
    await act(async () => { button('Abrir do começo').click(); });
    await flush();
    expect(notice()).toBeNull();
    expect(useGlobalStore.getState().fromStart).toBe(true);
    expect(at()).toBe('start');
    expect(mounts).toBe(3); // the viewer is opened again (asked, saved, start), not just given other props
  });

  it('says it was the search hit or the equivalent position that could not be opened', async () => {
    await open({ seek: { ...askFor, context: { kind: 'search', quote: 'um trecho' } } });
    await fail();
    expect(notice().textContent).toContain('Não foi possível abrir o trecho encontrado');
    act(() => root.unmount());
    root = createRoot(container);
    await open({ seek: { ...askFor, context: { kind: 'equivalent' } } });
    await fail();
    expect(notice().textContent).toContain('Não foi possível abrir a posição equivalente');
    expect(notice().querySelector('blockquote')).toBeNull();
  });

  it('is a note when nothing says what asked for the place', async () => {
    await open({ seek: { locator: { type: 'pdf', page: 11 }, context: null, n: 1 } });
    await fail();
    expect(notice().textContent).toContain('Não foi possível abrir o ponto da anotação');
  });

  it('says the saved position is gone when it is the saved position that fails, and that the start is shown', async () => {
    await open();
    await fail();
    expect(notice().textContent).toContain('Sua posição salva não existe mais neste arquivo');
    expect(notice().textContent).toContain('Ele apontava para PDF, página 4.');
    expect(notice().textContent).toContain('Abrimos do começo.');
    await act(async () => { button('Entendi').click(); });
    expect(notice()).toBeNull();
  });

  it('keeps what was asked for, and adds that the saved position failed too, when both do', async () => {
    await open({ seek: askFor });
    await fail(); // the place asked for
    await fail(); // then the saved position the reader went on to
    const text = notice().textContent;
    expect(text).toContain('Não foi possível abrir o ponto da anotação');
    expect(text).toContain('Ele apontava para PDF, página 12.');
    expect(text).toContain('A posição que você tinha salva também não pôde ser aberta');
    expect(text).toContain('Abrimos do começo.');
  });

  it('does not carry the notice to another file of the work', async () => {
    await open({ seek: askFor });
    await fail();
    expect(notice()).not.toBeNull();
    await act(async () => { useGlobalStore.getState().openBook(7, 11); });
    await flush();
    expect(notice()).toBeNull();
  });

  it('tells what a note was when the note panel opens it, so that the notice can say it', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/works/7') return { data: detail };
      if (url === '/progress/files/10') return { data: { revision: 1, position: '4', locator: { type: 'pdf', page: 3 } } };
      if (url === '/notes') return { data: { data: [{ id: 1, kind: 'highlight', workId: 7, fileId: 10, fileAvailable: true, quote: 'Fear is the mind-killer.', body: '', tags: [], locator: { type: 'pdf', page: 11 }, createdAt: '2026-09-19T10:00:00Z' }], total: 1 } };
      throw new Error(`unexpected GET ${url}`);
    });
    useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek: null });
    await act(async () => { root.render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><Reader /></QueryClientProvider>); });
    await flush();
    await flush();
    await act(async () => { container.querySelector('[aria-label="Notas, destaques e marcadores deste livro"]').click(); });
    await flush();
    await act(async () => { button('Abrir neste ponto').click(); });
    expect(useGlobalStore.getState().seek).toMatchObject({ locator: { type: 'pdf', page: 11 }, context: { kind: 'note', quote: 'Fear is the mind-killer.' } });
  });

  it('says nothing for a place that opens', async () => {
    await open({ seek: askFor });
    expect(notice()).toBeNull();
  });
});
