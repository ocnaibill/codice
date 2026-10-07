import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { CollectionSheet } from './CollectionSheet';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const work = (id, title, position, completed = false) => ({ id, title, author: 'J. K. Rowling', coverUrl: `/c/${id}.jpg`, position, completed });
const detail = (works, extra = {}) => ({
  collection: { id: 5, kind: 'official', name: 'Harry Potter', workCount: works.length, completedCount: works.filter((w) => w.completed).length, coverUrl: '/c/1.jpg', ...extra },
  works,
});
const trio = [work(1, 'Pedra Filosofal', 1, true), work(2, 'Câmara Secreta', 2), work(3, 'Contos', null)];
const same = { id: 10, title: 'Outro Potter', author: 'x', coverUrl: '/c/10.jpg', series: ' harry POTTER ' };
const loose = { id: 11, title: 'Livro Solto', author: 'x', coverUrl: '/c/11.jpg', series: '' };
const found = { id: 9, title: 'Animais Fantásticos', author: 'J. K. Rowling', coverUrl: '/c/9.jpg', series: 'Wizarding World' };

let container;
let root;
let reply;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const labelled = (label) => container.querySelector(`[aria-label="${label}"]`);
const click = (el) => act(async () => { el.click(); });

async function open({ role = 'admin', data = detail(trio) } = {}) {
  reply = data;
  api.get.mockImplementation(async (url, options) => {
    if (url === '/collections/5') return { data: reply };
    if (url === '/auth/me') return { data: { role } };
    if (url === '/collections/6') return { data: { collection: { id: 6, kind: 'official', name: 'Outra', workCount: 0, completedCount: 0, coverUrl: '/c/x.jpg' }, works: [] } };
    if (url === '/works') {
      const term = options?.params?.search;
      if (term === 'zzz') return { data: { data: [] } };
      if (term === 'unica') return { data: { data: [found] } };
      return { data: { data: term ? [found, same, loose, { id: 1, title: 'Pedra Filosofal', author: 'x' }] : [] } };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({});
  api.post.mockResolvedValue({});
  api.patch.mockResolvedValue({ data: {} });
  api.delete.mockResolvedValue({});
  useGlobalStore.setState({ collectionSheetId: 5, sheetWorkId: null });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><CollectionSheet /></QueryClientProvider>); });
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
  useGlobalStore.setState({ collectionSheetId: null, sheetWorkId: null });
});

describe('CollectionSheet: what everybody sees', () => {
  it('shows the works in order, with their numbers and which were read, and says how much was read', async () => {
    await open({ role: 'reader' });
    const rows = [...container.querySelectorAll('ol li')].map((li) => li.textContent);
    expect(rows).toHaveLength(3);
    expect(rows[0]).toContain('Pedra Filosofal');
    expect(rows[0]).toContain('Lida');
    expect(rows[1]).toContain('Câmara Secreta');
    expect(rows[2]).toContain('Contos');
    expect(container.textContent).toContain('3 obras · Leu 1 de 3');
    expect(container.querySelector('[role="dialog"]').getAttribute('aria-label')).toBe('Coleção');
    expect(labelled('Obras da coleção').tagName).toBe('OL');
    expect(container.querySelector('h2').textContent).toBe('Harry Potter');
    // the work with no number says so, and the others say theirs
    const numbers = [...container.querySelectorAll('ol li > div > span:first-child')].map((n) => n.textContent);
    expect(numbers).toEqual(['1', '2', '—']);
  });

  it('opens the sheet of a work without closing the collection', async () => {
    await open({ role: 'reader' });
    await click(labelled('Abrir a obra Câmara Secreta'));
    expect(useGlobalStore.getState().sheetWorkId).toBe(2);
    expect(useGlobalStore.getState().collectionSheetId).toBe(5);
  });

  it('closes with the button and with Escape', async () => {
    await open({ role: 'reader' });
    await click(labelled('Fechar'));
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
    await open({ role: 'reader' });
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
  });

  it('offers a reader nothing to manage', async () => {
    await open({ role: 'reader' });
    for (const text of ['Renomear', 'Acrescentar obra', 'Aposentar']) expect(button(text)).toBeUndefined();
    expect(labelled('Subir “Câmara Secreta”')).toBeNull();
    expect(labelled('Tirar “Câmara Secreta” da coleção')).toBeNull();
  });

  it('says when a collection has no work, and renders nothing when none is open', async () => {
    await open({ role: 'reader', data: detail([]) });
    expect(container.textContent).toContain('Esta coleção ainda não tem obras.');
    act(() => useGlobalStore.setState({ collectionSheetId: null }));
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });

  it('asks for nothing while no collection is open', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/auth/me') return { data: { role: 'admin' } };
      throw new Error(`unexpected GET ${url}`);
    });
    useGlobalStore.setState({ collectionSheetId: null });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionSheet /></QueryClientProvider>); });
    await flush();
    expect(api.get.mock.calls.filter(([url]) => String(url).startsWith('/collections'))).toEqual([]);
  });

  it('offers a reader no restore, even on a collection that came retired', async () => {
    await open({ role: 'reader', data: detail([], { retired: true }) });
    expect(button('Restaurar a coleção')).toBeUndefined();
  });

  it('says why it could not be opened, and tries again', async () => {
    api.get.mockRejectedValue(Object.assign(new Error('x'), { response: { status: 404, data: 'Collection not found' } }));
    useGlobalStore.setState({ collectionSheetId: 5 });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionSheet /></QueryClientProvider>); });
    await flush();
    expect(container.textContent).toContain('Não foi possível abrir esta coleção.');
  });
});

describe('CollectionSheet: what owner and admin do', () => {
  it('moves a work up and down by sending the whole new order', async () => {
    await open();
    expect(labelled('Subir “Pedra Filosofal”').disabled).toBe(true);
    expect(labelled('Descer “Contos”').disabled).toBe(true);
    await click(labelled('Descer “Pedra Filosofal”'));
    expect(api.put).toHaveBeenCalledWith('/collections/5/order', { workIds: [2, 1, 3] });
    await click(labelled('Subir “Contos”'));
    expect(api.put).toHaveBeenLastCalledWith('/collections/5/order', { workIds: [1, 3, 2] });
  });

  it('asks before taking a work out, and says what that does', async () => {
    await open();
    await click(labelled('Tirar “Câmara Secreta” da coleção'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(container.textContent).toContain('A série da obra é limpa e travada');
    await click(button('Cancelar'));
    expect(container.querySelector('[role="alertdialog"]')).toBeNull();
    await click(labelled('Tirar “Câmara Secreta” da coleção'));
    await click(button('Tirar da coleção'));
    expect(api.delete).toHaveBeenCalledWith('/collections/5/works/2');
    expect(container.querySelector('[role="alertdialog"]')).toBeNull(); // done: the question goes away
  });

  it('refreshes the page after a change, so the new order and the missing work show', async () => {
    await open();
    const reads = () => api.get.mock.calls.filter(([url]) => url === '/collections/5').length;
    const before = reads();
    await click(labelled('Descer “Pedra Filosofal”'));
    await flush();
    expect(reads()).toBeGreaterThan(before);
  });

  it('does not save a name that is only spaces, not even by sending the form', async () => {
    await open();
    await click(button('Renomear'));
    await type(container.querySelector('input'), '   ');
    await act(async () => { container.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    expect(api.patch).not.toHaveBeenCalled();
  });

  it('forgets the form that was open when another collection is opened', async () => {
    await open();
    await click(button('Renomear'));
    expect(container.querySelector('form')).not.toBeNull();
    await act(async () => { useGlobalStore.setState({ collectionSheetId: 6 }); });
    await flush();
    expect(container.querySelector('h2').textContent).toBe('Outra');
    expect(container.querySelector('form')).toBeNull();
  });

  it('renames, and shows what the server refused', async () => {
    await open();
    await click(button('Renomear'));
    const input = container.querySelector('input');
    expect(input.value).toBe('Harry Potter');
    await type(input, '  Bruxo ');
    api.patch.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 409, data: { error: 'Já existe outra coleção com esse nome.', collectionId: 8 } } }));
    await click(button('Salvar'));
    expect(api.patch).toHaveBeenCalledWith('/collections/5', { name: '  Bruxo ' });
    expect(container.querySelector('[role="alert"]').textContent).toBe('Já existe outra coleção com esse nome.');
    await click(button('Salvar'));
    await flush();
    expect(container.querySelector('form')).toBeNull(); // done: the form closes
  });

  it('does not save an empty name', async () => {
    await open();
    await click(button('Renomear'));
    await type(container.querySelector('input'), '   ');
    expect(button('Salvar').disabled).toBe(true);
  });

  it('looks for works to add, leaves out the ones that are in, and warns of the one that leaves another series', async () => {
    await open();
    await click(button('Acrescentar obra'));
    await type(container.querySelector('input'), 'fantá');
    expect(container.textContent).toContain('Animais Fantásticos');
    expect(container.textContent).toContain('sairá de “Wizarding World”');
    expect(container.textContent.match(/sairá de/g)).toHaveLength(1); // the one already in this series, and the one with none, do not leave anything
    expect(container.textContent).toContain('Outro Potter');
    expect(container.textContent).toContain('Livro Solto');
    expect(container.querySelector('[aria-label="Acrescentar “Pedra Filosofal”"]')).toBeNull(); // already in
    await click(labelled('Acrescentar “Animais Fantásticos”'));
    expect(api.put).toHaveBeenCalledWith('/collections/5/works/9', {});
    expect(container.textContent).toContain('“Animais Fantásticos” foi para o fim da coleção.');
    // a new search forgets what was said of the last one
    await type(container.querySelector('input'), 'fantás');
    expect(container.textContent).not.toContain('foi para o fim da coleção.');
  });

  it('does not say that nothing is left to add right after the last one was put in', async () => {
    await open();
    await click(button('Acrescentar obra'));
    await type(container.querySelector('input'), 'unica');
    expect(container.textContent).toContain('Animais Fantásticos');
    reply = detail([...trio, work(9, 'Animais Fantásticos', 4)]); // what the server answers once it is in
    await click(labelled('Acrescentar “Animais Fantásticos”'));
    await flush();
    expect(container.textContent).toContain('foi para o fim da coleção.');
    expect(container.textContent).not.toContain('Nenhuma obra para acrescentar com esse nome.');
  });

  it('does not say that nothing was found while it has not looked yet', async () => {
    await open();
    await click(button('Acrescentar obra'));
    const input = container.querySelector('input');
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
      setter.call(input, 'zzz');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect(container.textContent).not.toContain('Nenhuma obra para acrescentar com esse nome.'); // the search waits a moment before it asks
    await wait(320);
    await flush();
    expect(container.textContent).toContain('Nenhuma obra para acrescentar com esse nome.');
  });

  it('says when the search finds nothing to add', async () => {
    await open();
    await click(button('Acrescentar obra'));
    await type(container.querySelector('input'), 'zzz');
    expect(container.textContent).toContain('Nenhuma obra para acrescentar com esse nome.');
    await type(container.querySelector('input'), 'z');
    expect(container.textContent).not.toContain('Nenhuma obra para acrescentar com esse nome.'); // too short to search
  });

  it('retires only after asking, and closes', async () => {
    await open();
    await click(button('Aposentar'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(container.textContent).toContain('As obras não são apagadas nem mudam de série');
    await click(container.querySelector('[role="alertdialog"] button'));
    expect(api.delete).toHaveBeenCalledWith('/collections/5');
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
  });

  it('shows a retired collection as such, with no management but the restore', async () => {
    await open({ data: detail([], { retired: true }) });
    expect(container.textContent).toContain('Coleção aposentada.');
    expect(container.textContent).toContain('As obras voltam quando a coleção for restaurada.');
    for (const text of ['Renomear', 'Acrescentar obra', 'Aposentar']) expect(button(text)).toBeUndefined();
    await click(button('Restaurar a coleção'));
    expect(api.post).toHaveBeenCalledWith('/collections/5/restore');
  });

  it('shows what the server said when a change is refused', async () => {
    await open();
    api.put.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 400, data: 'A lista deve ter cada obra da coleção uma vez, e só elas.' } }));
    await click(labelled('Descer “Pedra Filosofal”'));
    expect(container.querySelector('[role="alert"]').textContent).toContain('A lista deve ter cada obra da coleção uma vez');
  });
});
