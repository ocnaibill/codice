import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// The viewer is not what is under test: a button that says "I reached the end".
vi.mock('./viewers/PdfViewer', () => ({
  default: ({ onProgress }) => (
    <button onClick={() => onProgress({ type: 'pdf', page: 9 }, { percent: 100, completed: true })}>reach the end</button>
  ),
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const detail = (over = {}) => ({
  id: 7, title: 'One Piece Cap. 2', author: 'Oda', fileId: 10, fileUrl: '/f/10', format: 'pdf', finished: false,
  editions: [{ id: 1, language: 'pt', files: [{ id: 10, format: 'pdf', availability: 'available', started: true, completed: false, url: '/f/10' }] }],
  ...over,
});
const next = { id: 8, title: 'One Piece Cap. 3', unit: 'chapter', position: 3, started: false };
const inSeries = { collection: { id: 4, name: 'One Piece' }, next };

let container;
let root;
let series;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim().includes(text));
const nextButton = () => container.querySelector('header [aria-label^="Ler o próximo da série"]');
const prompt = () => container.querySelector('[aria-label="Próximo da série"]');

async function open({ work = detail(), completed = false } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work };
    if (url === '/works/7/series') return { data: series };
    if (url === '/progress/files/10') return { data: { revision: 1, position: '4', completed } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockImplementation(async (url) => (url === '/progress/files/10' ? { data: { revision: 2, completed: true } } : { data: {} }));
  api.post.mockResolvedValue({});
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek: null });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
}
const reachTheEnd = async () => {
  await act(async () => { button('reach the end').click(); });
  await flush();
  await flush();
};

beforeEach(() => {
  vi.clearAllMocks();
  series = inSeries;
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  useGlobalStore.getState().closeBook();
});

describe('Reader: the next of a series (#187)', () => {
  it('asks for the series of the work being read', async () => {
    await open();
    expect(api.get).toHaveBeenCalledWith('/works/7/series');
  });

  it('puts the next in the header, says what it is, and opens it', async () => {
    await open();
    expect(nextButton().getAttribute('aria-label')).toBe('Ler o próximo da série: Cap. 3');
    expect(nextButton().title).toBe('Próximo da série: Cap. 3 (One Piece Cap. 3)');
    expect(nextButton().textContent).toContain('Cap. 3');
    await act(async () => { nextButton().click(); });
    expect(useGlobalStore.getState().activeBookId).toBe(8);
    expect(useGlobalStore.getState().activeFileId).toBeNull();
  });

  it('names a next that has no number by its title, with no repeated title in the hint', async () => {
    series = { ...inSeries, next: { id: 8, title: 'Especial', unit: '', position: null, started: false } };
    await open();
    expect(nextButton().getAttribute('aria-label')).toBe('Ler o próximo da série: Especial');
    expect(nextButton().title).toBe('Próximo da série: Especial');
  });

  it('has nothing in the header for the last of its group, or for a work in no series', async () => {
    series = { collection: { id: 4, name: 'One Piece' }, next: null };
    await open();
    expect(nextButton()).toBeNull();
    act(() => root.unmount());
    root = createRoot(container);
    series = { collection: null, next: null };
    await open();
    expect(nextButton()).toBeNull();
  });

  it('offers the next when this one is finished, and "Agora não" only closes the offer', async () => {
    await open();
    expect(prompt()).toBeNull();
    await reachTheEnd();
    expect(prompt().textContent).toContain('Você terminou One Piece Cap. 2.');
    expect(prompt().textContent).toContain('O próximo da série é Cap. 3.');
    await act(async () => { button('Agora não').click(); });
    expect(prompt()).toBeNull();
    expect(useGlobalStore.getState().activeBookId).toBe(7);
    expect(nextButton()).not.toBeNull(); // still one tap away
  });

  it('opens the next from the offer', async () => {
    await open();
    await reachTheEnd();
    await act(async () => { button('Ler Cap. 3 agora').click(); });
    expect(useGlobalStore.getState().activeBookId).toBe(8);
  });

  it('does not carry the offer over to the next work when that one is opened', async () => {
    await open();
    await reachTheEnd();
    expect(prompt()).not.toBeNull();
    api.get.mockImplementation(async (url) => {
      if (url === '/works/8') return { data: detail({ id: 8, title: 'One Piece Cap. 3', fileId: 12, editions: [{ id: 3, language: 'pt', files: [{ id: 12, format: 'pdf', availability: 'available', started: false, completed: false, url: '/f/12' }] }] }) };
      if (url === '/works/8/series') return { data: { collection: { id: 4, name: 'One Piece' }, next: { id: 9, title: 'One Piece Cap. 4', unit: 'chapter', position: 4, started: false } } };
      if (url === '/progress/files/12') return { data: { revision: 1, position: '', completed: false } };
      throw new Error(`unexpected GET ${url}`);
    });
    await act(async () => { nextButton().click(); });
    await flush();
    await flush();
    expect(useGlobalStore.getState().activeBookId).toBe(8);
    expect(container.querySelector('h1').textContent).toBe('One Piece Cap. 3');
    expect(prompt()).toBeNull();
    expect(nextButton().getAttribute('aria-label')).toBe('Ler o próximo da série: Cap. 4');
  });

  it('offers nothing when there is no next, and does not offer again for a file that was already finished', async () => {
    series = { collection: { id: 4, name: 'One Piece' }, next: null };
    await open();
    await reachTheEnd();
    expect(prompt()).toBeNull();
    act(() => root.unmount());
    root = createRoot(container);
    series = inSeries;
    await open({ completed: true });
    await reachTheEnd();
    expect(prompt()).toBeNull();
  });

  it('waits for the answer about the whole work when another version is in progress, and then offers the next', async () => {
    const work = detail({
      editions: [
        { id: 1, language: 'pt', files: [{ id: 10, format: 'pdf', availability: 'available', started: true, completed: true, url: '/f/10' }] },
        { id: 2, language: 'en', files: [{ id: 11, format: 'epub', availability: 'available', started: true, completed: false, percentComplete: 30, url: '/f/11' }] },
      ],
    });
    await open({ work });
    await reachTheEnd();
    expect(container.querySelector('[role=dialog]')).not.toBeNull();
    expect(prompt()).toBeNull();
    await act(async () => { button('Não, continuar a outra versão depois').click(); });
    expect(prompt()).not.toBeNull();
  });
});
