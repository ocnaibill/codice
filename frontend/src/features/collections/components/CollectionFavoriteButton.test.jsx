import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { post: vi.fn(), delete: vi.fn() }, authenticatedUrl: (u) => u }));

import { api } from '../../../lib/api';
import { CollectionFavoriteButton } from './CollectionFavoriteButton';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let client;
const render = async (collection) => {
  await act(async () => { root.render(<QueryClientProvider client={client}><CollectionFavoriteButton collection={collection} /></QueryClientProvider>); });
};

beforeEach(() => {
  vi.clearAllMocks();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('CollectionFavoriteButton', () => {
  it('waits for the answer before it can be pressed again, so a double click is one request', async () => {
    api.post.mockReturnValue(new Promise(() => {}));
    await render({ id: 4, name: 'Duna', isFavorite: false });
    const heart = container.querySelector('button');
    expect(heart.disabled).toBe(false);
    await act(async () => { heart.click(); });
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(container.querySelector('button').disabled).toBe(true);
    await act(async () => { heart.click(); });
    expect(api.post).toHaveBeenCalledTimes(1);
  });

  it('refreshes what shows collections and favorites once it is done', async () => {
    api.post.mockResolvedValue({});
    const spy = vi.spyOn(client, 'invalidateQueries');
    await render({ id: 4, name: 'Duna', isFavorite: false });
    await act(async () => { container.querySelector('button').click(); });
    await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
    const keys = spy.mock.calls.map(([arg]) => arg.queryKey[0]).sort();
    expect(keys).toEqual(expect.arrayContaining(['collection', 'collections', 'favorites']));
  });
});
