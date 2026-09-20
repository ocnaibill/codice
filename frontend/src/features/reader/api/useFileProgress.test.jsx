import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}));

import { api } from '../../../lib/api';
import { useFileProgress } from './useFileProgress';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let hook;

function Probe({ fileId }) {
  hook = useFileProgress(fileId);
  return null;
}

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

async function mount(fileId) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => {
    root.render(
      <QueryClientProvider client={client}>
        <Probe fileId={fileId} />
      </QueryClientProvider>
    );
  });
  await flush();
}

const conflict = (state) => Object.assign(new Error('conflict'), { response: { status: 409, data: state } });

beforeEach(() => {
  vi.clearAllMocks();
  api.post.mockResolvedValue({});
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('useFileProgress', () => {
  it("loads the file's own position and saves on top of the revision it saw", async () => {
    api.get.mockResolvedValue({ data: { fileId: 7, position: '12', revision: 4 } });
    api.put.mockResolvedValue({ data: { revision: 5 } });
    await mount(7);

    expect(api.get).toHaveBeenCalledWith('/progress/files/7');
    expect(hook.data.position).toBe('12');

    await act(async () => { await hook.save({ type: 'pdf', page: 12 }, { percent: 40 }); });
    const [url, body] = api.put.mock.calls[0];
    expect(url).toBe('/progress/files/7');
    expect(body).toMatchObject({ locator: { type: 'pdf', page: 12 }, percent: 40, baseRevision: 4 });
    expect(body.device).toMatch(/^Web/);
    expect(Number.isNaN(Date.parse(body.clientTime))).toBe(false);

    // The next save builds on the revision the server just gave.
    await act(async () => { await hook.save({ type: 'pdf', page: 13 }); });
    expect(api.put.mock.calls[1][1].baseRevision).toBe(5);
  });

  it('repeats a write once on top of the current revision when another device got there first', async () => {
    api.get.mockResolvedValue({ data: { position: '', revision: 1 } });
    api.put.mockRejectedValueOnce(conflict({ revision: 9, position: 'other' })).mockResolvedValueOnce({ data: { revision: 10 } });
    await mount(7);

    await act(async () => { await hook.save({ type: 'pdf', page: 3 }); });
    expect(api.put).toHaveBeenCalledTimes(2);
    expect(api.put.mock.calls[0][1].baseRevision).toBe(1);
    expect(api.put.mock.calls[1][1].baseRevision).toBe(9);
    await act(async () => { await hook.save({ type: 'pdf', page: 4 }); });
    expect(api.put.mock.calls[2][1].baseRevision).toBe(10);
  });

  it('does not retry other failures, and lets the caller see them', async () => {
    api.get.mockResolvedValue({ data: { revision: 1 } });
    api.put.mockRejectedValue(Object.assign(new Error('boom'), { response: { status: 500 } }));
    await mount(7);

    let error;
    await act(async () => { await hook.save({ type: 'pdf', page: 3 }).catch((e) => { error = e; }); });
    expect(error.message).toBe('boom');
    expect(api.put).toHaveBeenCalledTimes(1);
  });

  it('records that the file was opened, once, and nothing without a file', async () => {
    api.get.mockResolvedValue({ data: { revision: 0 } });
    await mount(7);
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(api.post).toHaveBeenCalledWith('/progress/files/7/opened');
  });

  it('asks for nothing and saves nothing without a file', async () => {
    await mount(null);
    expect(api.get).not.toHaveBeenCalled();
    expect(api.post).not.toHaveBeenCalled();
    let result;
    await act(async () => { result = await hook.save({ type: 'pdf', page: 1 }); });
    expect(result).toBeNull();
    expect(api.put).not.toHaveBeenCalled();
  });
});
