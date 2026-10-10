import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { RefreshMetadata, outcomeText } from './RefreshMetadata';
import { isActive, pollInterval } from '../api/useMetadataRefresh';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let client;
let current; // what the server says about the last search
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = () => [...container.querySelectorAll('button')][0];
const status = () => container.querySelector('[role="status"], [role="alert"]');
const done = (result, over = {}) => ({ id: 5, state: 'succeeded', finishedAt: '2026-10-09T12:00:00Z', result, ...over });

async function show({ job = null } = {}) {
  current = job;
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/works/7/metadata-refresh') return { data: { job: current } };
    throw new Error(`unexpected GET ${url}`);
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  await act(async () => {
    root.render(<QueryClientProvider client={client}><RefreshMetadata workId={7} /></QueryClientProvider>);
  });
  await flush();
}
const refetch = async () => { await act(async () => { await client.invalidateQueries({ queryKey: ['metadata-refresh', 7] }); }); await flush(); };
const click = async () => { await act(async () => { button().click(); }); await flush(); };

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

describe('RefreshMetadata: search the providers again for a work (DEC-143)', () => {
  it('offers the button and says what it asks with and that nothing changes until someone accepts', async () => {
    await show();
    expect(button().textContent).toBe('Buscar metadados de novo');
    expect(button().disabled).toBe(false);
    expect(container.textContent).toContain('com o que a obra diz hoje (título, autor, série, ISBN).');
    expect(status()).toBeNull();
  });

  it('asks for the search and, while it waits, says so and does not let it be asked twice', async () => {
    await show();
    api.post.mockImplementation(async () => { current = { id: 5, state: 'pending' }; return { data: { queued: true, jobId: 5 } }; });
    await click();
    expect(api.post).toHaveBeenCalledWith('/admin/works/7/metadata-refresh');
    expect(button().textContent).toBe('Buscando…');
    expect(button().disabled).toBe(true);
    expect(status().textContent).toBe('Buscando nos provedores ligados… as sugestões aparecem abaixo.');
  });

  it('says what the search found, how many suggestions are new, and asks for the suggestions again when it ends', async () => {
    await show();
    api.post.mockImplementation(async () => { current = { id: 5, state: 'running' }; return { data: { queued: true } }; });
    await click();
    const askedBefore = api.get.mock.calls.length;
    current = done({ found: true, source: 'Google Books', title: 'A nuvem', new: 3 });
    await refetch();
    expect(status().textContent).toBe('Achou “A nuvem” (Google Books): 3 sugestões novas abaixo.');
    expect(button().disabled).toBe(false);
    expect(button().textContent).toBe('Buscar metadados de novo');
    // the candidates and the queue of the administration are marked to be asked for again
    const invalidated = client.getQueryCache().findAll({ queryKey: ['candidates', 7] });
    expect(invalidated.length === 0 || invalidated.every((q) => q.state.isInvalidated)).toBe(true);
    expect(api.get.mock.calls.length).toBeGreaterThan(askedBefore);
  });

  it('tells a search that is already under way when the page opens, and what it came to', async () => {
    await show({ job: { id: 5, state: 'running' } });
    expect(button().disabled).toBe(true);
    expect(status().textContent).toContain('Buscando nos provedores ligados');
    current = done({ found: true, source: 'OpenLibrary', title: 'Dune', new: 1 });
    await refetch();
    expect(status().textContent).toBe('Achou “Dune” (OpenLibrary): 1 sugestão nova abaixo.');
  });

  it('says nothing of an old search the page did not ask for and is not watching', async () => {
    await show({ job: done({ found: true, source: 'OpenLibrary', title: 'Dune', new: 4 }) });
    expect(status()).toBeNull();
    expect(button().disabled).toBe(false);
  });

  it('says so when no provider is on, and why nothing was asked', async () => {
    await show();
    api.post.mockResolvedValue({ data: { queued: false, providersOff: true } });
    await click();
    expect(status().textContent).toBe('Nenhum provedor está ligado. O owner liga em Administração → Provedores.');
    expect(button().disabled).toBe(false);
  });

  it('says a request that was refused', async () => {
    await show();
    api.post.mockRejectedValue({ response: { status: 404, data: 'Book not found' } });
    await click();
    expect(status().getAttribute('role')).toBe('alert');
    expect(status().className).toContain('text-danger');
    expect(status().textContent).toBe('Obra não encontrada.');
    expect(button().disabled).toBe(false);
  });

  it('says it could not ask for the search when the server gives no reason', async () => {
    await show();
    api.post.mockRejectedValue(new Error('network'));
    await click();
    expect(status().textContent).toBe('Não foi possível pedir a busca.');
  });

  it('says a search that failed, with why, as an alert', async () => {
    await show();
    api.post.mockImplementation(async () => { current = { id: 5, state: 'pending' }; return { data: { queued: true } }; });
    await click();
    current = { id: 5, state: 'failed', error: 'HTTP 429: too many requests' };
    await refetch();
    expect(status().getAttribute('role')).toBe('alert');
    expect(status().textContent).toBe('A busca falhou (HTTP 429: too many requests). Tente de novo.');
    expect(button().disabled).toBe(false);
  });

  it('can be asked again once it is over', async () => {
    await show();
    api.post.mockImplementation(async () => { current = { id: 6, state: 'pending' }; return { data: { queued: true } }; });
    await click();
    current = done({ found: false, new: 0 });
    await refetch();
    expect(status().textContent).toBe('Nenhum provedor ligado reconheceu esta obra.');
    await click();
    expect(api.post).toHaveBeenCalledTimes(2);
    expect(button().disabled).toBe(true);
  });
});

describe('what the search came to, in words', () => {
  it('has a word for each outcome', () => {
    expect(outcomeText(done({ found: false, new: 0 }))).toBe('Nenhum provedor ligado reconheceu esta obra.');
    expect(outcomeText(done({ found: true, source: 'Google Books', title: 'A nuvem', new: 0 }))).toBe(
      'Achou “A nuvem” (Google Books), mas não tem nada novo: o que traz já está na obra, já espera decisão ou foi recusado antes.');
    expect(outcomeText(done({ found: true, source: 'Google Books', title: 'A nuvem', new: 1 }))).toBe('Achou “A nuvem” (Google Books): 1 sugestão nova abaixo.');
    expect(outcomeText(done({ found: true, source: 'Google Books', title: 'A nuvem', new: 5 }))).toBe('Achou “A nuvem” (Google Books): 5 sugestões novas abaixo.');
    expect(outcomeText({ state: 'failed' })).toBe('A busca falhou. Tente de novo.');
    expect(outcomeText({ state: 'succeeded' })).toBe('Nenhum provedor ligado reconheceu esta obra.');
  });

  it('polls every two seconds while the search is waiting or running, and not otherwise', () => {
    expect(pollInterval({ state: 'pending' })).toBe(2000);
    expect(pollInterval({ state: 'running' })).toBe(2000);
    for (const job of [{ state: 'succeeded' }, { state: 'failed' }, { state: 'cancelled' }, null, undefined]) expect(pollInterval(job)).toBe(false);
    expect(isActive({ state: 'pending' })).toBe(true);
    expect(isActive(null)).toBe(false);
  });
});
