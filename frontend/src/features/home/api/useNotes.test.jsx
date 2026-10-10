import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn() }, authenticatedUrl: (u) => u }));

import { api } from '../../../lib/api';
import { useNotes } from './useNotes';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let client;
let seen;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

function Probe({ options }) {
  seen = useNotes(options);
  return null;
}
async function show(options) {
  await act(async () => { root.render(<QueryClientProvider client={client}><Probe options={options} /></QueryClientProvider>); });
  await flush();
}

beforeEach(() => {
  vi.clearAllMocks();
  api.get.mockResolvedValue({ data: { data: [{ id: 1 }], total: 1 } });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('useNotes', () => {
  it('asks for the newest, as many as it is told', async () => {
    await show({ limit: 3 });
    expect(api.get).toHaveBeenCalledWith('/notes?limit=3');
    expect(seen.data.data).toEqual([{ id: 1 }]);
  });

  it('asks for a sample when it is told to, which is another few every time it comes to the page', async () => {
    await show({ limit: 2, sample: true });
    expect(api.get).toHaveBeenCalledWith('/notes?limit=2&sample=true');
    // arriving again at the page draws again, though the answer was fresh a moment ago
    act(() => root.unmount());
    root = createRoot(container);
    await show({ limit: 2, sample: true });
    expect(api.get).toHaveBeenCalledTimes(2);
  });

  it('keeps the sample while the window is only left and come back to', async () => {
    await show({ limit: 2, sample: true });
    const visibility = vi.spyOn(document, 'visibilityState', 'get');
    visibility.mockReturnValue('hidden');
    await act(async () => { document.dispatchEvent(new Event('visibilitychange', { bubbles: true })); });
    visibility.mockReturnValue('visible');
    await act(async () => { document.dispatchEvent(new Event('visibilitychange', { bubbles: true })); });
    await flush();
    expect(api.get).toHaveBeenCalledTimes(1);
    visibility.mockRestore();
  });

  it('asks again when the window is come back to, for the newest, which is no shuffle', async () => {
    await show({ limit: 2 });
    const visibility = vi.spyOn(document, 'visibilityState', 'get');
    visibility.mockReturnValue('hidden');
    await act(async () => { document.dispatchEvent(new Event('visibilitychange', { bubbles: true })); });
    visibility.mockReturnValue('visible');
    await act(async () => { document.dispatchEvent(new Event('visibilitychange', { bubbles: true })); });
    await flush();
    expect(api.get).toHaveBeenCalledTimes(2);
    visibility.mockRestore();
  });

  it('does not ask for a sample when it is not told to, and a sample and the newest are not the same answer', async () => {
    await show({ limit: 2 });
    expect(api.get).toHaveBeenCalledWith('/notes?limit=2');
    await show({ limit: 2, sample: true });
    expect(api.get).toHaveBeenCalledWith('/notes?limit=2&sample=true');
    expect(api.get).toHaveBeenCalledTimes(2);
  });
});
