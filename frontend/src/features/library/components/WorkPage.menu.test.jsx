import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkPage } from './WorkPage';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const work = { id: 7, title: 'Duna', author: 'Frank Herbert', metadata: { description: 'Uma sinopse.' }, editions: [] };
const suggestion = (id) => ({ id, field: 'isbn', value: String(id), source: 'Google Books', current: '', keys: [] });

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const dots = () => container.querySelector('button[aria-haspopup="menu"]');
const item = (text) => [...container.querySelectorAll('[role="menuitem"]')].find((i) => i.textContent.includes(text));
const key = (k, target = window) => act(async () => { target.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true })); });

async function open({ role = 'admin', pending = 0 } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work };
    if (url === '/auth/me') return { data: { role } };
    if (url === '/works/7/candidates') return { data: { data: Array.from({ length: pending }, (_, i) => suggestion(i + 1)) } };
    throw new Error(`unexpected GET ${url}`);
  });
  useGlobalStore.setState({ sheetWorkId: 7, metadataWorkId: null, metadataTab: 'suggestions' });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><WorkPage /></QueryClientProvider>); });
  await flush();
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
  useGlobalStore.setState({ sheetWorkId: null, metadataWorkId: null });
});

describe('the sheet of a work: the actions for owner and admin (#70)', () => {
  it('is as a reader sees it: no dots, no request for suggestions', async () => {
    await open({ role: 'reader', pending: 2 });
    expect(dots()).toBeNull();
    expect(api.get).not.toHaveBeenCalledWith('/works/7/candidates');
    expect(container.textContent).not.toContain('Sugestões');
  });

  it('has the dots for the owner and the admin, and the sheet itself never lists the suggestions', async () => {
    await open({ role: 'owner', pending: 2 });
    expect(dots()).toBeTruthy();
    expect(container.textContent).not.toContain('Nada muda até você aceitar');
    expect(container.textContent).not.toContain('Google Books');
  });

  it('carries a mark and says how many suggestions wait, in the singular and in the plural', async () => {
    await open({ pending: 3 });
    expect(dots().getAttribute('aria-label')).toBe('Mais ações da obra (3 sugestões)');
    expect(container.querySelector('[data-testid="pending-mark"]')).toBeTruthy();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ pending: 1 });
    expect(dots().getAttribute('aria-label')).toBe('Mais ações da obra (1 sugestão)');
  });

  it('has no mark when nothing waits', async () => {
    await open({ pending: 0 });
    expect(dots().getAttribute('aria-label')).toBe('Mais ações da obra');
    expect(container.querySelector('[data-testid="pending-mark"]')).toBeNull();
    await act(async () => { dots().click(); });
    expect(item('Sugestões dos provedores').textContent).toBe('Sugestões dos provedores');
  });

  it('offers the suggestions with their count and the edit, and each opens the metadata at its part', async () => {
    await open({ pending: 3 });
    expect(container.querySelector('[role="menu"]')).toBeNull();
    await act(async () => { dots().click(); });
    expect(dots().getAttribute('aria-expanded')).toBe('true');
    expect(item('Sugestões dos provedores').textContent).toContain('3');

    await act(async () => { item('Sugestões dos provedores').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ metadataWorkId: 7, metadataTab: 'suggestions' });
    expect(container.querySelector('[role="menu"]')).toBeNull();

    await act(async () => { dots().click(); });
    await act(async () => { item('Editar metadados').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ metadataWorkId: 7, metadataTab: 'edit' });
  });

  it('closes the menu on a click outside and on Escape, and the page stays', async () => {
    await open();
    await act(async () => { dots().click(); });
    await act(async () => { document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true })); });
    expect(container.querySelector('[role="menu"]')).toBeNull();

    await act(async () => { dots().click(); });
    await key('Escape');
    expect(container.querySelector('[role="menu"]')).toBeNull();
    expect(useGlobalStore.getState().sheetWorkId).toBe(7);
    await key('Escape');
    expect(useGlobalStore.getState().sheetWorkId).toBe(7); // a page is left by going back, not by Escape
  });

  it('lets an open menu have the Escape, from wherever it comes: the focused element or the window', async () => {
    await open();
    await act(async () => { dots().click(); });
    await key('Escape', dots());
    expect(container.querySelector('[role="menu"]')).toBeNull();
    expect(useGlobalStore.getState().sheetWorkId).toBe(7);
    // Chrome runs every listener of the window even when one stops the propagation, so the menu must not depend on it.
    await act(async () => { dots().click(); });
    await act(async () => {
      const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true });
      event.stopPropagation = () => {};
      window.dispatchEvent(event);
    });
    expect(container.querySelector('[role="menu"]')).toBeNull();
    expect(useGlobalStore.getState().sheetWorkId).toBe(7);
  });
});
