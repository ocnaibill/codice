import React from 'react';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, expect, it, vi } from 'vitest';
import { api } from '../../../lib/api';
import { useWorks } from './useWorks';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn() } }));
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

it('updates a pending work without WebSocket events and stops polling when ready', async () => {
  vi.useFakeTimers();
  const response = (mediaStatus) => ({ data: { data: [{ id: 1, mediaStatus }], total: 1 } });
  api.get.mockResolvedValueOnce(response('QUEUED')).mockResolvedValue(response('READY'));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const container = document.createElement('div');
  const root = createRoot(container);
  function Probe() {
    const { data } = useWorks();
    return <span>{data?.data[0]?.mediaStatus}</span>;
  }
  try {
    await act(async () => {
      root.render(<QueryClientProvider client={client}><Probe /></QueryClientProvider>);
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(10); });
    expect(container.textContent).toBe('QUEUED');
    await act(async () => { await vi.advanceTimersByTimeAsync(3010); });
    expect(container.textContent).toBe('READY');
    expect(api.get).toHaveBeenCalledTimes(2);
    await act(async () => { await vi.advanceTimersByTimeAsync(9000); });
    expect(api.get).toHaveBeenCalledTimes(2);
  } finally {
    await act(async () => root.unmount());
    client.clear();
  }
});
