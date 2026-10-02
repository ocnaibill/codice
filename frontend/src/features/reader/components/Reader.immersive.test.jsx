import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// The viewer says whether the reader is immersive, and asks for it to change.
vi.mock('./viewers/PdfViewer', () => ({
  default: ({ immersive, onImmersiveChange }) => (
    <div>
      <span data-testid="seen">{String(immersive)}</span>
      <button onClick={() => onImmersiveChange(!immersive)}>toggle</button>
    </div>
  ),
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
const work = {
  id: 7, title: 'Duna', author: 'Frank Herbert', fileId: 10, fileUrl: '/f/10', format: 'pdf', finished: false,
  editions: [{ id: 1, language: 'pt', files: [
    { id: 10, format: 'pdf', availability: 'available', url: '/f/10' },
    { id: 11, format: 'pdf', availability: 'available', url: '/f/11' },
  ] }],
};
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const header = () => container.querySelector('header');
// The header sits in a frame that folds to nothing (the frame clips it, so that its padding does not stay as a strip).
const wrapper = () => header().closest('[data-immersive]');
const frame = () => header().parentElement;
const seen = () => container.querySelector('[data-testid="seen"]').textContent;
const toggle = () => act(async () => { [...container.querySelectorAll('button')].find((b) => b.textContent === 'toggle').click(); });
async function open() {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work };
    if (url.startsWith('/progress/files/')) return { data: { revision: 1, position: '4' } };
    throw new Error(`unexpected GET ${url}`);
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
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('the Reader, with only the page on the screen', () => {
  it('starts with its header, and the viewer knows it is not immersive', async () => {
    await open();
    expect(seen()).toBe('false');
    expect(wrapper().getAttribute('data-immersive')).toBe('false');
    expect(wrapper().hasAttribute('inert')).toBe(false);
    expect(wrapper().className).toContain('grid-rows-[1fr]');
  });

  it('folds the header away when the viewer asks, out of reach of touch and keyboard, and brings it back', async () => {
    await open();
    await toggle();
    expect(seen()).toBe('true');
    expect(wrapper().getAttribute('data-immersive')).toBe('true');
    expect(wrapper().hasAttribute('inert')).toBe(true);
    expect(wrapper().className).toContain('grid-rows-[0fr]');
    expect(wrapper().className).toContain('transition-[grid-template-rows]');
    await toggle();
    expect(seen()).toBe('false');
    expect(wrapper().hasAttribute('inert')).toBe(false);
    expect(wrapper().className).toContain('grid-rows-[1fr]');
  });

  it('clips the header inside the frame that folds, so that nothing of it stays', async () => {
    await open();
    expect(frame().className).toContain('overflow-hidden');
    expect(frame().className).toContain('min-h-0');
    expect(frame().parentElement).toBe(wrapper());
  });

  it('starts again with the header when another file is opened', async () => {
    await open();
    await toggle();
    expect(seen()).toBe('true');
    await act(async () => { useGlobalStore.setState({ activeFileId: 11 }); });
    await flush();
    expect(seen()).toBe('false');
    expect(wrapper().getAttribute('data-immersive')).toBe('false');
  });
});
