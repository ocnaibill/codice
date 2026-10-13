import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// What the Reader hands the comic viewer is what is checked here.
let mounts = 0;
vi.mock('./viewers/MangaViewer', () => ({
  default: function MangaStub({ declaredMode, comicKind, workId, immersive, onImmersiveChange, highlight, initialProgress }) {
    React.useEffect(() => { mounts += 1; }, []);
    return (
      <div data-testid="comic" data-declared={declaredMode ?? ''} data-kind={comicKind ?? ''} data-work={workId} data-immersive={String(immersive)} data-highlight={highlight ? JSON.stringify(highlight) : ''} data-start={initialProgress ?? ''}>
        <button onClick={() => onImmersiveChange(!immersive)}>toggle</button>
      </div>
    );
  },
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const file = (id, declaredMode) => ({ id, format: 'cbz', availability: 'available', started: false, completed: false, url: `/f/${id}`, ...(declaredMode ? { declaredMode } : {}) });
const detail = {
  id: 7, title: 'Volume Dois', author: 'Autora', fileId: 10, fileUrl: '/f/10', format: 'cbz', finished: false,
  editions: [{ id: 1, language: 'pt', files: [file(10, 'rtl')] }, { id: 2, language: 'pt', files: [file(11)] }, { id: 3, language: 'pt', files: [file(12, 'webtoon')] }],
};
let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const declared = () => container.querySelector('[data-testid="comic"]').dataset.declared;

async function render() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
}
const openFile = async (id) => { await act(async () => { useGlobalStore.setState({ activeBookId: 7, activeFileId: id, fromStart: false, seek: null }); }); await flush(); await flush(); };

beforeEach(() => {
  mounts = 0;
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

describe('Reader: the comic viewer', () => {
  it('is told whether the reader is immersive, and can ask for it to change', async () => {
    await render();
    const seen = () => container.querySelector('[data-testid="comic"]').dataset.immersive;
    const toggle = () => act(async () => { container.querySelector('[data-testid="comic"] button').click(); });
    expect(seen()).toBe('false');
    await toggle();
    expect(seen()).toBe('true');
    expect(container.querySelector('[data-immersive]').getAttribute('data-immersive')).toBe('true');
    await toggle();
    expect(seen()).toBe('false');
  });

  it('is told the kind of the work, when it has one', async () => {
    const kind = () => container.querySelector('[data-testid="comic"]').dataset.kind;
    await render();
    expect(kind()).toBe('');
    detail.metadata = { comicKind: 'manga' };
    try {
      await openFile(11);
      expect(kind()).toBe('manga');
    } finally {
      delete detail.metadata;
    }
  });

  it('is told the balloon a search hit came from, with the page it opens at, and nothing when it was not asked for', async () => {
    const mark = () => container.querySelector('[data-testid="comic"]').dataset;
    await render();
    expect(mark().highlight).toBe('');
    const region = { x: 0.1, y: 0.2, w: 0.3, h: 0.4 };
    await act(async () => { useGlobalStore.setState({ seek: { n: 1, locator: { type: 'image', index: 5, region }, context: { kind: 'search', quote: 'olá' } } }); });
    await flush();
    expect(mark().start).toBe('5');
    expect(JSON.parse(mark().highlight)).toEqual({ index: 5, ...region });
    // A hit that has no balloon (a text read from the file's own metadata, say) opens the page and marks nothing.
    await act(async () => { useGlobalStore.setState({ seek: { n: 2, locator: { type: 'image', index: 6 }, context: null } }); });
    await flush();
    expect(mark().start).toBe('6');
    expect(mark().highlight).toBe('');
  });

  it('is told how the file being read says it is read', async () => {
    await render();
    expect(declared()).toBe('rtl');
  });

  it('is told nothing for a file that says nothing, and each file speaks for itself', async () => {
    await render();
    await openFile(11);
    expect(declared()).toBe('');
    await openFile(12);
    expect(declared()).toBe('webtoon');
  });

  it('starts the viewer again for another file, so the mode it opened with is that file\'s own', async () => {
    // The position of a file is asked for again every time it is opened, and the Reader waits for it: the viewer is
    // never carried over from one file to the next.
    await render();
    const before = mounts;
    await openFile(12);
    expect(mounts).toBe(before + 1);
    expect(declared()).toBe('webtoon');
    await openFile(10);
    expect(mounts).toBe(before + 2);
    expect(declared()).toBe('rtl');
  });
});
