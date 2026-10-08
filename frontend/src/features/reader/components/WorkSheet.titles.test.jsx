import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkSheet } from './WorkSheet';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const work = (alternativeTitles, contributors, extraMeta = {}) => ({
  id: 7, title: 'Duna', author: 'Frank Herbert', coverUrl: '/covers/7.jpg', fileId: 10,
  metadata: { description: 'Uma sinopse.', alternativeTitles, contributors, ...extraMeta },
  editions: [{ id: 1, language: 'pt', isPrimary: true, files: [{ id: 10, format: 'epub', availability: 'available', url: '/file/10', percentComplete: 0, completed: false }] }],
});

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

async function open(titles, contributors, extraMeta) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work(titles, contributors, extraMeta) };
    if (url === '/works/7/candidates') return { data: { data: [] } };
    if (url === '/auth/me') return { data: { role: 'reader' } };
    throw new Error(`unexpected GET ${url}`);
  });
  useGlobalStore.setState({ sheetWorkId: 7 });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><WorkSheet /></QueryClientProvider>); });
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
  useGlobalStore.setState({ sheetWorkId: null });
});

describe('WorkSheet: the people on the work, each opening a page (#186)', () => {
  const credits = [
    { personId: 1, name: 'Frank Herbert', displayName: 'Herbert, Frank', role: 'author', position: 0 },
    { personId: 2, name: 'Brian Herbert', displayName: 'Brian Herbert', role: 'author', position: 1 },
    { personId: 3, name: 'Maria Tradutora', displayName: 'Maria Tradutora', role: 'translator', position: 0 },
  ];
  const named = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent === text);

  it('shows each author by the name the account is shown, and opens the page of the one that is pressed', async () => {
    await open([], credits);
    expect(named('Herbert, Frank')).toBeTruthy();
    expect(named('Brian Herbert')).toBeTruthy();
    expect(named('Frank Herbert')).toBeUndefined();
    await act(async () => { named('Brian Herbert').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ personSheetId: 2, sheetWorkId: null });
  });

  it('puts a comma between the authors, and opens the page of a translator too', async () => {
    await open([], credits);
    const line = [...container.querySelectorAll('p')].find((p) => p.textContent.startsWith('Herbert, Frank'));
    expect(line.textContent).toBe('Herbert, Frank, Brian Herbert');
    await act(async () => { named('Maria Tradutora').click(); });
    expect(useGlobalStore.getState().personSheetId).toBe(3);
  });

  it('says the author as the card says it when the work has no credits', async () => {
    await open([], []);
    expect([...container.querySelectorAll('p')].some((p) => p.textContent === 'Frank Herbert')).toBe(true);
    expect(named('Frank Herbert')).toBeUndefined();
  });

  it('falls back to the stored name when there is no name to show', async () => {
    await open([], [{ personId: 1, name: 'Frank Herbert', role: 'author', position: 0 }]);
    expect(named('Frank Herbert')).toBeTruthy();
  });
});

describe('WorkSheet: the other names of the work (#185)', () => {
  it('says what the work is also known as, with the language of each, for everybody', async () => {
    await open([{ id: 3, title: 'Arrakis', language: '', source: 'manual' }, { id: 0, title: 'Dune', language: 'en', source: 'edition' }]);
    const line = [...container.querySelectorAll('p')].find((p) => p.textContent.startsWith('Também conhecida como'));
    expect(line.textContent).toBe('Também conhecida como Arrakis · Dune (Inglês)');
  });

  it('says nothing when the work goes by one name only', async () => {
    await open([]);
    expect(container.textContent).not.toContain('Também conhecida como');
  });

  it('says nothing when the server says nothing of it', async () => {
    await open(undefined);
    expect(container.textContent).not.toContain('Também conhecida como');
  });

  it('says who else is credited besides the authors, by role', async () => {
    await open([], [
      { personId: 1, name: 'Frank Herbert', role: 'author', position: 0 },
      { personId: 2, name: 'Tradutora', role: 'translator', position: 0 },
      { personId: 3, name: 'Narrador', role: 'narrator', position: 0 },
    ]);
    const line = [...container.querySelectorAll('p')].find((p) => p.textContent.startsWith('Tradução:'));
    expect(line.textContent).toBe('Tradução: Tradutora · Narração: Narrador');
  });

  it('says nothing more when only authors are credited', async () => {
    await open([], [{ personId: 1, name: 'Frank Herbert', role: 'author', position: 0 }]);
    expect([...container.querySelectorAll('p')].some((p) => p.textContent.includes('Tradução:') || p.textContent.includes('Narração:'))).toBe(false);
  });
});

describe('WorkSheet: what a comic or manga work is of its series (#187)', () => {
  const seriesLine = () => [...container.querySelectorAll('p')].find((p) => p.textContent.startsWith('One Piece'));

  it('says the unit and the number in the series, and whether it is a manga', async () => {
    await open([], [], { series: 'One Piece', seriesIndex: 27.5, unit: 'chapter', comicKind: 'manga' });
    expect(seriesLine().textContent).toBe('One Piece · Cap. 27,5');
    expect([...container.querySelectorAll('span')].some((s) => s.textContent === 'Mangá')).toBe(true);
  });

  it('says a volume, and a one-shot with no number', async () => {
    await open([], [], { series: 'One Piece', seriesIndex: 3, unit: 'volume', comicKind: 'comic' });
    expect(seriesLine().textContent).toBe('One Piece · Vol. 3');
    expect([...container.querySelectorAll('span')].some((s) => s.textContent === 'Quadrinho')).toBe(true);
    act(() => root.unmount());
    root = createRoot(container);
    await open([], [], { series: 'One Piece', seriesIndex: 0, unit: 'oneshot' });
    expect(seriesLine().textContent).toBe('One Piece · Único');
  });

  it('says a dash for a number that is not there, and not zero', async () => {
    await open([], [], { series: 'One Piece', seriesIndex: 0, unit: 'volume' });
    expect(seriesLine().textContent).toBe('One Piece · Vol. —');
  });

  it('keeps saying "Livro" for a series whose works have no unit', async () => {
    await open([], [], { series: 'One Piece', seriesIndex: 2 });
    expect(seriesLine().textContent).toBe('One Piece · Livro 2');
    expect([...container.querySelectorAll('span')].some((s) => s.textContent === 'Mangá' || s.textContent === 'Quadrinho')).toBe(false);
  });
});
