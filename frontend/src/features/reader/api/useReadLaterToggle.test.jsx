import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { put: vi.fn(), delete: vi.fn() } }));

import { api } from '../../../lib/api';
import { useReadLaterToggle } from './useReadLaterToggle';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let toggle;
let client;

function Probe({ workId }) {
  toggle = useReadLaterToggle(workId);
  return null;
}

beforeEach(async () => {
  vi.clearAllMocks();
  api.put.mockResolvedValue({});
  api.delete.mockResolvedValue({});
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  client = new QueryClient();
  vi.spyOn(client, 'invalidateQueries');
  await act(async () => { root.render(<QueryClientProvider client={client}><Probe workId={7} /></QueryClientProvider>); });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('useReadLaterToggle', () => {
  it('puts the work in the list when asked to, and takes it out when asked not to', async () => {
    await act(async () => { await toggle.mutateAsync(true); });
    expect(api.put).toHaveBeenCalledWith('/works/7/read-later');
    expect(api.delete).not.toHaveBeenCalled();
    await act(async () => { await toggle.mutateAsync(false); });
    expect(api.delete).toHaveBeenCalledWith('/works/7/read-later');
  });

  it('asks again for the work, and for the lists of the person (the one of "Ler depois" is born with the first)', async () => {
    await act(async () => { await toggle.mutateAsync(true); });
    const keys = client.invalidateQueries.mock.calls.map((c) => JSON.stringify(c[0].queryKey));
    expect(keys).toContain('["work",7]');
    expect(keys).toContain('["collections"]');
    expect(keys).toContain('["collection"]');
  });
});
