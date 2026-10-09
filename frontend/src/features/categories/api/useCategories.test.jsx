import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn() } }));
import { api } from '../../../lib/api';
import { useCategories } from './useCategories';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let container;
let root;
function Probe({ options }) { useCategories(options); return null; }
const settle = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
beforeEach(() => {
  vi.clearAllMocks();
  api.get.mockResolvedValue({ data: { data: [] } });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => { act(() => root.unmount()); container.remove(); });

describe('useCategories', () => {
  it('asks for the tree alone, with no covers', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><Probe /></QueryClientProvider>); });
    await settle();
    expect(api.get).toHaveBeenCalledWith('/categories', { params: undefined });
  });

  it('asks for the covers when they are wanted', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><Probe options={{ covers: true }} /></QueryClientProvider>); });
    await settle();
    expect(api.get).toHaveBeenCalledWith('/categories', { params: { covers: 1 } });
  });

  it('does not give the tree without covers to who wants them, nor the other way round', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => {
      root.render(<QueryClientProvider client={client}><Probe /><Probe options={{ covers: true }} /></QueryClientProvider>);
    });
    await settle();
    expect(api.get).toHaveBeenCalledTimes(2);
    expect(api.get.mock.calls.map(([, config]) => config.params)).toEqual(expect.arrayContaining([undefined, { covers: 1 }]));
  });
});
