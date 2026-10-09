import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn() } }));
import { api } from '../../../lib/api';
import { useWorks } from './useWorks';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let container;
let root;
function Probe(props) { useWorks(props); return null; }
async function mount(props) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Probe {...props} /></QueryClientProvider>); });
  await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
}
beforeEach(() => { vi.clearAllMocks(); api.get.mockResolvedValue({ data: { data: [] } }); container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container); });
afterEach(() => { act(() => root.unmount()); container.remove(); });

describe('useWorks: sorting', () => {
  it('asks for the sort the person picked', async () => {
    await mount({ sort: 'author' });
    expect(api.get.mock.calls[0][0]).toContain('sort=author');
    await mount({ sort: 'title' });
    expect(api.get.mock.calls.at(-1)[0]).toContain('sort=title');
  });

  it('sends nothing for the default, newest first', async () => {
    await mount({});
    expect(api.get.mock.calls[0][0]).not.toContain('sort=');
    await mount({ sort: 'added' });
    expect(api.get.mock.calls.at(-1)[0]).not.toContain('sort=');
  });
});

describe('useWorks: the category', () => {
  it('asks for the works of a category, and of no category when none is given', async () => {
    await mount({ category: 7 });
    expect(api.get.mock.calls[0][0]).toContain('category=7');
    await mount({});
    expect(api.get.mock.calls.at(-1)[0]).not.toContain('category');
  });
});
