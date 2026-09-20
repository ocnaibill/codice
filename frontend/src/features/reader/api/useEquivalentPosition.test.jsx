import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
}));

import { api } from '../../../lib/api';
import { useAcceptEquivalentPosition, useEquivalentPosition } from './useEquivalentPosition';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let hook;

function Probe({ destFileId, sourceFileId }) {
  const query = useEquivalentPosition(destFileId, sourceFileId);
  const accept = useAcceptEquivalentPosition(destFileId);
  hook = { query, accept };
  return null;
}

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

async function mount(destFileId, sourceFileId) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => {
    root.render(
      <QueryClientProvider client={client}>
        <Probe destFileId={destFileId} sourceFileId={sourceFileId} />
      </QueryClientProvider>
    );
  });
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

describe('useEquivalentPosition', () => {
  it('asks the destination file for a match against the source file', async () => {
    api.get.mockResolvedValue({ data: { status: 'found', candidates: [] } });
    await mount(20, 10);
    expect(api.get).toHaveBeenCalledWith('/progress/files/20/equivalent', { params: { from: 10 } });
    expect(hook.query.data.status).toBe('found');
  });

  it('asks nothing without both files, or when they are the same file', async () => {
    await mount(20, null);
    expect(api.get).not.toHaveBeenCalled();
    await mount(20, 20);
    expect(api.get).not.toHaveBeenCalled();
  });
});

describe('useAcceptEquivalentPosition', () => {
  it('posts the chosen candidate to the destination file', async () => {
    api.post.mockResolvedValue({});
    await mount(20, 10);
    await act(async () => {
      await hook.accept.mutateAsync({ sourceFileId: 10, locator: { type: 'pdf', page: 3 }, method: 'text', confidence: 'high', precision: 'passage' });
    });
    expect(api.post).toHaveBeenCalledWith('/progress/files/20/equivalent/accept', {
      sourceFileId: 10, locator: { type: 'pdf', page: 3 }, method: 'text', confidence: 'high', precision: 'passage',
    });
  });
});
