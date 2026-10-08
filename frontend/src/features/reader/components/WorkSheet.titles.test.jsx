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

const work = (alternativeTitles) => ({
  id: 7, title: 'Duna', author: 'Frank Herbert', coverUrl: '/covers/7.jpg', fileId: 10,
  metadata: { description: 'Uma sinopse.', alternativeTitles },
  editions: [{ id: 1, language: 'pt', isPrimary: true, files: [{ id: 10, format: 'epub', availability: 'available', url: '/file/10', percentComplete: 0, completed: false }] }],
});

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

async function open(titles) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work(titles) };
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
});
