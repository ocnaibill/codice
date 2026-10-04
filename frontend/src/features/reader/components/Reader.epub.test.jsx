import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// What the Reader hands the EPUB viewer is what is checked here.
vi.mock('./viewers/EpubViewer', () => ({
  default: function EpubStub({ immersive, onImmersiveChange, title }) {
    return (
      <div data-testid="epub" data-immersive={String(immersive)} data-title={title}>
        <button onClick={() => onImmersiveChange(!immersive)}>toggle</button>
      </div>
    );
  },
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const detail = {
  id: 7, title: 'Duna', author: 'Autora', fileId: 10, fileUrl: '/f/10', format: 'epub', finished: false,
  editions: [{ id: 1, language: 'pt', files: [{ id: 10, format: 'epub', availability: 'available', url: '/f/10' }] }],
};
let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const seen = () => container.querySelector('[data-testid="epub"]').dataset.immersive;
const toggle = () => act(async () => { container.querySelector('[data-testid="epub"] button').click(); });

beforeEach(() => {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: detail };
    if (url.startsWith('/progress/files/')) return { data: { revision: 1, position: '', locator: null } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: {} });
  api.post.mockResolvedValue({});
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek: null });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('Reader: the EPUB viewer', () => {
  it('hands the viewer the title of the book, to name the frame of the page for a screen reader', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
    await flush();
    await flush();
    expect(container.querySelector('[data-testid="epub"]').dataset.title).toBe('Duna');
  });

  it('is told whether the reader is immersive, and can ask for it to change', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
    await flush();
    await flush();
    expect(seen()).toBe('false');
    await toggle();
    expect(seen()).toBe('true');
    expect(container.querySelector('[data-immersive]').getAttribute('data-immersive')).toBe('true');
    await toggle();
    expect(seen()).toBe('false');
  });
});
