import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }));
import { api } from '../../../lib/api';
import { MAX_DISPLAY_NAME, useSetDisplayName, useSetLibraryNameOrder, useSetNameOrder, useSetReadingShared } from './usePreferences';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let container;
let root;
let client;
let hooks;
function Probe() {
  hooks = { account: useSetNameOrder(), library: useSetLibraryNameOrder(), calledBy: useSetDisplayName(), shared: useSetReadingShared() };
  return null;
}
beforeEach(() => {
  vi.clearAllMocks();
  api.put.mockResolvedValue({ data: {} });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
});
afterEach(() => { act(() => root.unmount()); container.remove(); });

async function mount() {
  await act(async () => { root.render(<QueryClientProvider client={client}><Probe /></QueryClientProvider>); });
}
const invalidated = (spy) => spy.mock.calls.map(([filters]) => filters.queryKey[0]).sort();

describe('changing how names are shown', () => {
  it('asks again for everything that shows an author, for an account and for the library', async () => {
    await mount();
    const spy = vi.spyOn(client, 'invalidateQueries');
    await act(async () => { await hooks.account.mutateAsync('family_first'); });
    expect(invalidated(spy)).toEqual(['favorites', 'preferences', 'search', 'work', 'works']);
    spy.mockClear();
    await act(async () => { await hooks.library.mutateAsync('family_first'); });
    expect(invalidated(spy)).toEqual(['favorites', 'preferences', 'search', 'work', 'works']);
  });

  it('does not ask again when the change failed', async () => {
    await mount();
    api.put.mockRejectedValue(new Error('no'));
    const spy = vi.spyOn(client, 'invalidateQueries');
    await act(async () => { await hooks.account.mutateAsync('family_first').catch(() => {}); });
    expect(spy).not.toHaveBeenCalled();
  });
});

describe('changing how the person wants to be called (#179)', () => {
  it('sends the name, and asks the account and the preferences to be read again so that the greeting changes', async () => {
    await mount();
    const spy = vi.spyOn(client, 'invalidateQueries');
    await act(async () => { await hooks.calledBy.mutateAsync('Aninha'); });
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { displayName: 'Aninha' });
    expect(invalidated(spy)).toEqual(['me', 'preferences']);
  });

  it('holds the same limit as the server', () => {
    expect(MAX_DISPLAY_NAME).toBe(60);
  });
});

describe('keeping how the text looks the same on every device (#180)', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('turning it on takes the choice of this kind of device to the others, and asks the preferences to be read again', async () => {
    vi.stubGlobal('matchMedia', () => ({ matches: true }));
    await mount();
    const spy = vi.spyOn(client, 'invalidateQueries');
    await act(async () => { await hooks.shared.mutateAsync(true); });
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { reader: { shared: true, from: 'touch' } });
    expect(invalidated(spy)).toEqual(['preferences']);
  });

  it('turning it on from a computer says so', async () => {
    vi.stubGlobal('matchMedia', () => ({ matches: false }));
    await mount();
    await act(async () => { await hooks.shared.mutateAsync(true); });
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { reader: { shared: true, from: 'desktop' } });
  });

  it('turning it off lets each kind have its own, and names no source', async () => {
    await mount();
    await act(async () => { await hooks.shared.mutateAsync(false); });
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { reader: { shared: false } });
  });
});
