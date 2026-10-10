import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkPage } from './WorkPage';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const file = (id, format) => ({ id, format, availability: 'available', url: `/file/${id}`, percentComplete: 0, completed: false });
const work = (editions) => ({
  id: 7, title: 'Duna', author: 'Frank Herbert', coverUrl: '/covers/7.jpg', fileId: 10,
  metadata: { description: 'Uma sinopse.' }, fileCount: editions.reduce((n, e) => n + e.files.length, 0),
  editions,
});
const oneEdition = work([{ id: 1, language: 'pt', isPrimary: true, files: [file(10, 'epub')] }]);
const twoEditions = work([
  { id: 1, language: 'pt', isPrimary: true, files: [file(10, 'epub')] },
  { id: 2, language: 'en', isPrimary: false, files: [file(20, 'pdf')] },
]);
const dune = { id: 3, title: 'Dune', author: 'Frank Herbert', coverUrl: '/covers/3.jpg', format: 'pdf', fileCount: 1 };

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const dialog = (label) => container.querySelector(`[aria-label="${label}"]`);

async function open({ role = 'admin', detail = twoEditions, results = [dune] } = {}) {
  api.get.mockImplementation(async (url, options) => {
    if (url === '/works/7') return { data: detail };
    if (url === '/works/7/candidates') return { data: { data: [] } };
    if (url === '/auth/me') return { data: { role } };
    if (url === '/works') return { data: { data: options?.params?.search ? results : [] } };
    throw new Error(`unexpected GET ${url}`);
  });
  useGlobalStore.setState({ sheetWorkId: 7 });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><WorkPage /></QueryClientProvider>); });
  await flush();
  await flush();
}

async function type(input, value) {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await wait(320);
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
  useGlobalStore.setState({ sheetWorkId: null });
});

describe('WorkPage: putting the files of one book under one work (#37)', () => {
  it('offers to join only to the owner and the admin', async () => {
    await open({ role: 'admin' });
    expect(button('Juntar com outra obra…')).toBeTruthy();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ role: 'owner' });
    expect(button('Juntar com outra obra…')).toBeTruthy();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ role: 'reader' });
    expect(button('Juntar com outra obra…')).toBeUndefined();
    expect(button('Separar em obra própria')).toBeUndefined();
  });

  it('searches for the work, asks for confirmation saying what happens, and joins it', async () => {
    api.post.mockResolvedValue({ data: { workId: 3, editions: 1 } });
    await open({ detail: oneEdition });
    await act(async () => { button('Juntar com outra obra…').click(); });
    const input = dialog('Juntar com outra obra').querySelector('input');
    await type(input, 'dune');

    expect(api.get).toHaveBeenCalledWith('/works', { params: { search: 'dune', limit: 8 } });
    expect(dialog('Juntar com outra obra').textContent).toContain('Dune');

    await act(async () => { [...dialog('Juntar com outra obra').querySelectorAll('button')].find((b) => b.textContent.includes('Dune')).click(); });
    const confirm = dialog('Confirmar');
    expect(confirm.textContent).toContain('passa a ser');
    expect(confirm.textContent).toContain('Todos os arquivos continuam guardados');
    expect(api.post).not.toHaveBeenCalled(); // nothing happens until the person says so

    await act(async () => { button('Juntar').click(); });
    await flush();
    expect(api.post).toHaveBeenCalledWith('/admin/works/7/join', { into: 3 });
    // The work that was open is retired: the sheet goes to the one that has its files.
    expect(useGlobalStore.getState().sheetWorkId).toBe(3);
    expect(dialog('Juntar com outra obra')).toBeNull();
  });

  it('does not offer the work itself, and waits for two letters', async () => {
    await open({ detail: oneEdition, results: [{ ...dune, id: 7, title: 'Duna' }, dune] });
    await act(async () => { button('Juntar com outra obra…').click(); });
    const input = dialog('Juntar com outra obra').querySelector('input');
    await type(input, 'd');
    expect(api.get.mock.calls.some(([url]) => url === '/works')).toBe(false);
    await type(input, 'du');
    const text = dialog('Juntar com outra obra').textContent;
    expect(text).toContain('Dune');
    expect(dialog('Juntar com outra obra').querySelectorAll('ul li').length).toBe(1);
  });

  it('records that it is not the same work and goes back to the search', async () => {
    api.post.mockResolvedValue({});
    await open({ detail: oneEdition });
    await act(async () => { button('Juntar com outra obra…').click(); });
    await type(dialog('Juntar com outra obra').querySelector('input'), 'dune');
    await act(async () => { [...dialog('Juntar com outra obra').querySelectorAll('button')].find((b) => b.textContent.includes('Dune')).click(); });
    await act(async () => { button('Não é a mesma obra').click(); });
    await flush();
    expect(api.post).toHaveBeenCalledWith('/admin/works/7/not-same-as', { workId: 3 });
    expect(dialog('Juntar com outra obra').textContent).toContain('não é a mesma obra');
    expect(dialog('Confirmar')).toBeNull();
    expect(useGlobalStore.getState().sheetWorkId).toBe(7);
  });

  it('shows what the server said when it cannot join, and stays where it is', async () => {
    api.post.mockRejectedValue({ response: { data: 'a retired work cannot be joined: restore it first\n' } });
    await open({ detail: oneEdition });
    await act(async () => { button('Juntar com outra obra…').click(); });
    await type(dialog('Juntar com outra obra').querySelector('input'), 'dune');
    await act(async () => { [...dialog('Juntar com outra obra').querySelectorAll('button')].find((b) => b.textContent.includes('Dune')).click(); });
    await act(async () => { button('Juntar').click(); });
    await flush();
    expect(dialog('Juntar com outra obra').textContent).toContain('Uma obra retirada não pode ser juntada: restaure-a antes.');
    expect(useGlobalStore.getState().sheetWorkId).toBe(7);
  });

  it('closes the dialog with Escape without closing the sheet under it', async () => {
    await open({ detail: oneEdition });
    await act(async () => { button('Juntar com outra obra…').click(); });
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(dialog('Juntar com outra obra')).toBeNull();
    expect(container.querySelector('[aria-label="Ficha da obra"]')).not.toBeNull();
    expect(useGlobalStore.getState().sheetWorkId).toBe(7);
  });

  it('only offers to separate an edition when the work has more than one', async () => {
    await open({ detail: oneEdition });
    expect(button('Separar em obra própria')).toBeUndefined();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ detail: twoEditions });
    expect(container.querySelectorAll('button[title^="Tira esta edição"]').length).toBe(2);
  });

  it('asks before separating an edition, and says where it went', async () => {
    api.post.mockResolvedValue({ data: { workId: 9, restored: true } });
    await open({ detail: twoEditions });
    await act(async () => { button('Separar em obra própria').click(); });
    expect(dialog('Confirmar a separação').textContent).toContain('As notas e as conclusões dos arquivos dela vão junto');
    expect(api.post).not.toHaveBeenCalled();

    await act(async () => { button('Cancelar').click(); });
    expect(dialog('Confirmar a separação')).toBeNull();

    await act(async () => { button('Separar em obra própria').click(); });
    await act(async () => { button('Separar').click(); });
    await flush();
    expect(api.post).toHaveBeenCalledWith('/admin/editions/1/split');
    expect(container.querySelector('[role=status]').textContent).toContain('voltou para a obra de onde veio');

    await act(async () => { button('Abrir essa obra').click(); });
    expect(useGlobalStore.getState().sheetWorkId).toBe(9);
  });

  it('says a new work was made when the edition had nowhere to go back to', async () => {
    api.post.mockResolvedValue({ data: { workId: 12, restored: false } });
    await open({ detail: twoEditions });
    await act(async () => { button('Separar em obra própria').click(); });
    await act(async () => { button('Separar').click(); });
    await flush();
    expect(container.querySelector('[role=status]').textContent).toContain('uma obra nova');
  });

  it('shows why an edition could not be separated', async () => {
    api.post.mockRejectedValue({ response: { data: 'the only edition of a work cannot be separated from it' } });
    await open({ detail: twoEditions });
    await act(async () => { button('Separar em obra própria').click(); });
    await act(async () => { button('Separar').click(); });
    await flush();
    expect(container.querySelector('[role=status]').textContent).toContain('A única edição de uma obra não pode ser separada dela.');
    expect(dialog('Confirmar a separação')).toBeNull();
  });
});
