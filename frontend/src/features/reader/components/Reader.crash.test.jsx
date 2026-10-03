import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// A viewer that breaks while it is drawn, when it is told to.
const state = { crash: true };
vi.mock('./viewers/TextViewer', () => ({
  default: () => {
    if (state.crash) throw new Error('o leitor quebrou');
    return <div data-testid="viewer">texto</div>;
  },
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let client;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const alert = () => container.querySelector('[role="alert"]');

async function open() {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: { id: 7, title: 'Notas', author: 'Autora', fileId: 10, fileUrl: '/f/10', format: 'txt', finished: false, editions: [{ id: 1, language: 'pt', files: [{ id: 10, format: 'txt', availability: 'available', url: '/f/10' }] }] } };
    if (url.startsWith('/progress/files/')) return { data: { revision: 1, position: '', locator: null } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: {} });
  api.post.mockResolvedValue({});
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek: null });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
}

beforeEach(() => {
  state.crash = true;
  vi.spyOn(console, 'error').mockImplementation(() => {});
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.restoreAllMocks();
});

describe('Reader: a viewer that breaks', () => {
  it('says so in Portuguese, in the place of the viewer, and the header of the reader is still there', async () => {
    await open();
    expect(alert().textContent).toContain('Algo deu errado');
    expect(alert().textContent).toContain('Tivemos um problema ao mostrar o leitor.');
    expect(container.textContent).toContain('Notas');
  });

  it('lets the person go back to the library', async () => {
    await open();
    const back = [...alert().querySelectorAll('button')].find((b) => b.textContent === 'Voltar');
    await act(async () => { back.click(); });
    expect(useGlobalStore.getState().activeBookId).toBeNull();
  });

  it('starts again when another place is asked for, so that a book that broke does not leave the next one broken', async () => {
    await open();
    expect(alert()).not.toBeNull();
    state.crash = false;
    await act(async () => { useGlobalStore.setState({ seek: { n: 1, locator: null } }); });
    await flush();
    expect(alert()).toBeNull();
    expect(container.querySelector('[data-testid="viewer"]')).not.toBeNull();
  });
});
