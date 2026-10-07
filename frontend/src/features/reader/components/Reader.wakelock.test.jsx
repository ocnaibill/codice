import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// The viewers are not under test.
for (const name of ['PdfViewer', 'EpubViewer', 'MangaViewer', 'TextViewer', 'MarkdownViewer', 'AudioViewer']) {
  vi.doMock(`./viewers/${name}`, () => ({ default: () => <div>{name}</div> }));
}

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { saveKeepScreenOn, setPreferenceOwner } from '../preferences';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let request;
let release;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

async function open(format, { availability = 'available', url = `/f/10.${format}` } = {}) {
  api.get.mockImplementation(async (path) => {
    if (path === '/works/7') {
      return { data: { id: 7, title: 'Livro', author: 'Autora', fileId: 10, fileUrl: url, format, finished: false,
        editions: [{ id: 1, language: 'pt', files: [{ id: 10, format, availability, url }] }] } };
    }
    if (path === '/progress/files/10') return { data: { revision: 1, position: '', completed: false } };
    throw new Error(`unexpected GET ${path}`);
  });
  api.post.mockResolvedValue({});
  api.put.mockResolvedValue({ data: {} });
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek: null });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  setPreferenceOwner('ana');
  release = vi.fn(async () => {});
  request = vi.fn(async () => ({ release, addEventListener: vi.fn() }));
  Object.defineProperty(navigator, 'wakeLock', { value: { request }, configurable: true });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  delete navigator.wakeLock;
  useGlobalStore.getState().closeBook();
});

describe('the reader keeps the screen on while a book is read on it (#180)', () => {
  for (const format of ['pdf', 'epub', 'cbz', 'cbr', 'txt', 'md']) {
    it(`for ${format}`, async () => {
      await open(format);
      expect(request).toHaveBeenCalledWith('screen');
    });
  }

  it('not for audio, which plays with the screen off', async () => {
    for (const format of ['mp3', 'm4b', 'flac']) {
      await open(format);
      expect(request).not.toHaveBeenCalled();
    }
  });

  it('not when the person turned it off on this device', async () => {
    saveKeepScreenOn(false);
    await open('epub');
    expect(request).not.toHaveBeenCalled();
  });

  it('not for a book that cannot be opened', async () => {
    await open('epub', { availability: 'missing' });
    expect(request).not.toHaveBeenCalled();
  });

  it('lets the screen go when the book is closed', async () => {
    await open('epub');
    await flush();
    act(() => root.unmount());
    expect(release).toHaveBeenCalledTimes(1);
    root = createRoot(container);
  });
});
