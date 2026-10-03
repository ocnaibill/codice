import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// What the Reader hands the text viewers is what is checked here.
vi.mock('./viewers/TextViewer', () => ({
  default: ({ immersive, onImmersiveChange }) => (
    <div data-testid="viewer" data-kind="txt" data-immersive={String(immersive)}><button onClick={() => onImmersiveChange(!immersive)}>toggle</button></div>
  ),
}));
vi.mock('./viewers/MarkdownViewer', () => ({
  default: ({ immersive, onImmersiveChange }) => (
    <div data-testid="viewer" data-kind="md" data-immersive={String(immersive)}><button onClick={() => onImmersiveChange(!immersive)}>toggle</button></div>
  ),
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const work = (format) => ({
  id: 7, title: 'Notas', author: 'Autora', fileId: 10, fileUrl: '/f/10', format, finished: false,
  editions: [{ id: 1, language: 'pt', files: [{ id: 10, format, availability: 'available', url: '/f/10' }] }],
});
let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const viewer = () => container.querySelector('[data-testid="viewer"]');
const toggle = () => act(async () => { viewer().querySelector('button').click(); });

async function open(format) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work(format) };
    if (url.startsWith('/progress/files/')) return { data: { revision: 1, position: '', locator: null } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: {} });
  api.post.mockResolvedValue({});
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek: null });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
}

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe.each([['txt'], ['md']])('Reader: the %s viewer', (format) => {
  it('is the right one, is told whether the reader is immersive, and can ask for it to change', async () => {
    await open(format);
    expect(viewer().dataset.kind).toBe(format);
    expect(viewer().dataset.immersive).toBe('false');
    await toggle();
    expect(viewer().dataset.immersive).toBe('true');
    expect(container.querySelector('[data-immersive]').getAttribute('data-immersive')).toBe('true');
    await toggle();
    expect(viewer().dataset.immersive).toBe('false');
  });
});
