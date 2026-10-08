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

const work = (id, title, position, completed = false) => ({ entryId: id * 10, id, title, author: 'J. K. Rowling', coverUrl: `/c/${id}.jpg`, position, completed, available: true });
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

describe('CollectionSheet: a list of the person', () => {
  const entry = (entryId, id, title, extra = {}) => ({ entryId, id, title, author: 'x', coverUrl: '/c/1.jpg', position: entryId, completed: false, available: true, ...extra });
  const gone = entry(30, 0, 'Obra que saiu', { available: false, coverUrl: '/covers/placeholder.svg' });
  const list = (works, extra = {}) => ({
    collection: { id: 7, kind: 'personal', name: 'Para ler', workCount: works.filter((w) => w.available).length, completedCount: 0, coverUrl: '/c/1.jpg', ...extra },
    works,
  });

  async function openList({ role = 'reader', data = list([entry(10, 1, 'Duna'), entry(20, 2, 'Fundação'), gone]) } = {}) {
    reply = data;
    api.get.mockImplementation(async (url, options) => {
      if (url === '/collections/7') return { data: reply };
      if (url === '/auth/me') return { data: { role } };
      if (url === '/works') return { data: { data: options?.params?.search ? [found, same] : [] } };
      throw new Error(`unexpected GET ${url}`);
    });
    api.put.mockResolvedValue({});
    api.post.mockResolvedValue({});
    api.patch.mockResolvedValue({ data: {} });
    api.delete.mockResolvedValue({});
    useGlobalStore.setState({ collectionSheetId: 7, sheetWorkId: null });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionSheet /></QueryClientProvider>); });
    await flush();
    await flush();
  }

  it('tells apart the places of two works that are gone, which have no work to tell them by', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    await openList({ data: list([entry(10, 1, 'Duna'), gone, { ...gone, entryId: 31, title: 'Outra que saiu' }]) });
    expect([...container.querySelectorAll('ol li')]).toHaveLength(3);
    expect(error).not.toHaveBeenCalled(); // React complains of two items with the same key
    error.mockRestore();
  });

  it('is managed by the person who reads it: no staff needed', async () => {
    await openList({ role: 'reader' });
    expect(container.querySelector('[role="dialog"]').getAttribute('aria-label')).toBe('Lista');
    expect(container.textContent).toContain('Biblioteca / Lista');
    for (const text of ['Renomear', 'Acrescentar obra', 'Aposentar']) expect(button(text)).toBeTruthy();
    expect(labelled('Obras da lista').tagName).toBe('OL');
  });

  it('shows a work that left the library with what the list kept of it, and no way to open it', async () => {
    await openList();
    const row = [...container.querySelectorAll('ol li')][2];
    expect(row.textContent).toContain('Obra que saiu');
    expect(row.textContent).toContain('Fora do acervo');
    expect(row.querySelector('button[aria-label^="Abrir a obra"]')).toBeNull();
    expect(row.querySelector('button').disabled).toBe(true);
    // the others open as always
    await click(labelled('Abrir a obra Duna'));
    expect(useGlobalStore.getState().sheetWorkId).toBe(1);
  });

  it('moves a place by sending the places in the new order, through the route of the person', async () => {
    await openList();
    await click(labelled('Descer “Duna”'));
    expect(api.put).toHaveBeenCalledWith('/my/collections/7/order', { entryIds: [20, 10, 30] });
    await click(labelled('Subir “Obra que saiu”'));
    expect(api.put).toHaveBeenLastCalledWith('/my/collections/7/order', { entryIds: [10, 30, 20] });
  });

  it('takes a place out by its own number, even of a work that is gone, and says the work stays', async () => {
    await openList();
    await click(labelled('Tirar “Obra que saiu” da lista'));
    expect(container.textContent).toContain('A obra continua no acervo: só sai da lista.');
    expect(container.textContent).not.toContain('A série da obra é limpa');
    await click(button('Tirar da lista'));
    expect(api.delete).toHaveBeenCalledWith('/my/collections/7/entries/30');
    await click(labelled('Tirar “Duna” da lista'));
    await click(button('Tirar da lista'));
    expect(api.delete).toHaveBeenLastCalledWith('/my/collections/7/entries/10');
  });

  it('renames through its own route, and says only the name changes', async () => {
    await openList();
    await click(button('Renomear'));
    expect(container.textContent).toContain('Só o nome da lista muda: as obras continuam como estão.');
    await type(container.querySelector('input'), 'Verão');
    await click(button('Salvar'));
    expect(api.patch).toHaveBeenCalledWith('/my/collections/7', { name: 'Verão' });
  });

  it('puts works in through its own route, warns of nothing, and leaves out the ones that are in', async () => {
    await openList();
    await click(button('Acrescentar obra'));
    await type(container.querySelector('input'), 'fantá');
    expect(container.textContent).toContain('Animais Fantásticos');
    expect(container.textContent).not.toContain('sairá de'); // a list takes the work out of nowhere
    await click(labelled('Acrescentar “Animais Fantásticos”'));
    expect(api.put).toHaveBeenCalledWith('/my/collections/7/works/9', {});
    expect(container.textContent).toContain('“Animais Fantásticos” foi para o fim da lista.');
  });

  it('puts a list away only after asking, and says the works stay in the library', async () => {
    await openList();
    await click(button('Aposentar'));
    expect(container.textContent).toContain('As obras continuam no acervo: a lista só sai do menu');
    expect(api.delete).not.toHaveBeenCalled();
    await click(container.querySelector('[role="alertdialog"] button'));
    expect(api.delete).toHaveBeenCalledWith('/my/collections/7');
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
  });

  it('restores a list that was put away', async () => {
    await openList({ data: list([], { retired: true }) });
    expect(container.textContent).toContain('Lista aposentada.');
    expect(container.textContent).toContain('As obras voltam quando a lista for restaurada.');
    for (const text of ['Renomear', 'Acrescentar obra', 'Aposentar']) expect(button(text)).toBeUndefined();
    await click(button('Restaurar a lista'));
    expect(api.post).toHaveBeenCalledWith('/my/collections/7/restore');
  });

  it('says what an empty list is', async () => {
    await openList({ data: list([]) });
    expect(container.textContent).toContain('Esta lista ainda não tem obras.');
  });

  it('counts only the works that are there', async () => {
    await openList();
    expect(container.textContent).toContain('2 obras');
  });
});

describe('CollectionSheet: favoriting', () => {
  it('has a heart in the header, filled when it is a favorite, for a reader too', async () => {
    await open({ role: 'reader', data: detail(trio, { isFavorite: true }) });
    const heart = labelled('Remover dos favoritos: Harry Potter');
    expect(heart.getAttribute('aria-pressed')).toBe('true');
    await click(heart);
    expect(api.delete).toHaveBeenCalledWith('/collections/5/favorite');
  });

  it('favorites with the same heart, and refreshes the page', async () => {
    await open({ role: 'reader', data: detail(trio, { isFavorite: false }) });
    const reads = () => api.get.mock.calls.filter(([url]) => url === '/collections/5').length;
    const before = reads();
    await click(labelled('Adicionar aos favoritos: Harry Potter'));
    expect(api.post).toHaveBeenCalledWith('/collections/5/favorite');
    await flush();
    expect(reads()).toBeGreaterThan(before);
  });

  it('has no heart on a retired collection', async () => {
    await open({ role: 'admin', data: detail([], { retired: true }) });
    expect(container.querySelector('.library-favorite')).toBeNull();
  });
});

describe('CollectionSheet: the works in groups by unit (#187)', () => {
  const w = (id, title, position, unit, extra = {}) => ({ ...work(id, title, position), unit, comicKind: '', ...extra });
  const mixed = [
    w(1, 'Vol 1', 1, 'volume'), w(2, 'Vol 2', 2, 'volume'),
    w(3, 'Cap 1', 1, 'chapter'), w(4, 'Cap 27,5', 27.5, 'chapter'),
    w(5, 'Livro de um só', null, 'oneshot'), w(6, 'Sem unidade', 9, ''),
  ];

  it('shows the volumes, the chapters, the one-shots and the rest each in a group of its own, with how many', async () => {
    await open({ role: 'reader', data: detail(mixed) });
    const headings = [...container.querySelectorAll('section h3')].map((h) => h.textContent);
    expect(headings).toEqual(['Volumes (2)', 'Capítulos (2)', 'Únicos (1)', 'Sem unidade (1)']);
    expect(labelled('Volumes da coleção').querySelectorAll('li')).toHaveLength(2);
    expect(labelled('Capítulos da coleção').querySelectorAll('li')).toHaveLength(2);
  });

  it('calls each work by its unit and number', async () => {
    await open({ role: 'reader', data: detail(mixed) });
    const labels = [...container.querySelectorAll('ol li > div > span:first-child')].map((n) => n.textContent);
    expect(labels).toEqual(['Vol. 1', 'Vol. 2', 'Cap. 1', 'Cap. 27,5', 'Único', '9']);
  });

  it('numbers a group on its own when a work is moved: the order sent is of that group, with its unit', async () => {
    await open({ data: detail(mixed) });
    await click(labelled('Descer “Cap 1”'));
    expect(api.put).toHaveBeenCalledWith('/collections/5/order', { workIds: [4, 3], unit: 'chapter' });
    await click(labelled('Subir “Vol 2”'));
    expect(api.put).toHaveBeenLastCalledWith('/collections/5/order', { workIds: [2, 1], unit: 'volume' });
  });

  it('sends the unit of the works with none as an empty one', async () => {
    const group = [w(1, 'A', 1, 'volume'), w(2, 'B', 1, ''), w(3, 'C', 2, '')];
    await open({ data: detail(group) });
    await click(labelled('Descer “B”'));
    expect(api.put).toHaveBeenCalledWith('/collections/5/order', { workIds: [3, 2], unit: '' });
  });

  it('keeps the order of the whole collection when no work has a unit, as before', async () => {
    await open({ data: detail([w(1, 'A', 1, ''), w(2, 'B', 2, '')]) });
    expect(container.querySelector('section h3')).toBeNull();
    await click(labelled('Descer “A”'));
    expect(api.put).toHaveBeenCalledWith('/collections/5/order', { workIds: [2, 1] });
  });

  it('shows the first fifty of a group and the rest when asked, fifty at a time', async () => {
    const many = Array.from({ length: 120 }, (_, i) => w(100 + i, `Cap ${i + 1}`, i + 1, 'chapter'));
    await open({ role: 'reader', data: detail(many) });
    expect(container.querySelectorAll('ol li')).toHaveLength(50);
    expect(container.querySelector('section h3').textContent).toBe('Capítulos (120)');
    await click(button('Mostrar mais 50 de 70'));
    expect(container.querySelectorAll('ol li')).toHaveLength(100);
    await click(button('Mostrar mais 20 de 20'));
    expect(container.querySelectorAll('ol li')).toHaveLength(120);
    expect(button('Mostrar mais', container)).toBeUndefined();
  });

  it('shows every work when there are fifty or fewer, with no button', async () => {
    await open({ role: 'reader', data: detail(Array.from({ length: 50 }, (_, i) => w(100 + i, `Cap ${i + 1}`, i + 1, 'chapter'))) });
    expect(container.querySelectorAll('ol li')).toHaveLength(50);
    expect([...container.querySelectorAll('button')].some((b) => b.textContent.startsWith('Mostrar mais'))).toBe(false);
  });

  it('does not group a list of the person: it is places in a row', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/collections/7') return { data: { collection: { id: 7, kind: 'personal', name: 'Minha', workCount: 2, completedCount: 0, coverUrl: '/c' }, works: [w(1, 'A', 1, 'volume', { entryId: 10 }), w(2, 'B', 2, 'chapter', { entryId: 20 })] } };
      if (url === '/auth/me') return { data: { role: 'reader' } };
      throw new Error(`unexpected GET ${url}`);
    });
    useGlobalStore.setState({ collectionSheetId: 7 });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionSheet /></QueryClientProvider>); });
    await flush();
    await flush();
    expect(container.querySelector('section h3')).toBeNull();
    expect([...container.querySelectorAll('ol li > div > span:first-child')].map((n) => n.textContent)).toEqual(['1', '2']);
  });
});

describe('CollectionSheet: classifying the works of a collection (#187)', () => {
  const select = (label) => [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith(label)).querySelector('select');
  const choose = (el, value) => act(async () => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, value);
    el.dispatchEvent(new Event('change', { bubbles: true }));
  });
  const form = () => container.querySelector('form[aria-label="Classificar as obras"]');
  const apply = () => [...form().querySelectorAll('button')].find((b) => b.textContent === 'Aplicar');

  it('is offered to the staff only, and not for a list of the person', async () => {
    await open({ role: 'reader' });
    expect(button('Classificar obras')).toBeUndefined();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ role: 'admin' });
    expect(button('Classificar obras')).toBeTruthy();
  });

  it('is not offered on a list of the person, which has no units', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/collections/7') return { data: { collection: { id: 7, kind: 'personal', name: 'Minha', workCount: 0, completedCount: 0, coverUrl: '/c' }, works: [] } };
      if (url === '/auth/me') return { data: { role: 'admin' } };
      throw new Error(`unexpected GET ${url}`);
    });
    useGlobalStore.setState({ collectionSheetId: 7 });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionSheet /></QueryClientProvider>); });
    await flush();
    await flush();
    expect(button('Renomear')).toBeTruthy();
    expect(button('Classificar obras')).toBeUndefined();
  });

  it('sends nothing when neither the kind nor the unit was chosen, not even by sending the form', async () => {
    await open();
    await click(button('Classificar obras'));
    await act(async () => { form().dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    expect(api.put).not.toHaveBeenCalled();
  });

  it('sends the kind and the unit that were chosen, for the works that have none by default', async () => {
    await open();
    api.put.mockResolvedValue({ data: { changed: { unit: 3, comic_kind: 3 } } });
    await click(button('Classificar obras'));
    expect(apply().disabled).toBe(true);
    await choose(select('Quadrinho ou mangá'), 'manga');
    await choose(select('Unidade'), 'chapter');
    expect(form().querySelector('input[type="checkbox"]').checked).toBe(true);
    await click(apply());
    expect(api.put).toHaveBeenCalledWith('/collections/5/classification', { unit: 'chapter', comicKind: 'manga', onlyUnset: true });
    expect(form().textContent).toContain('Mudei a unidade de 3 e o tipo de 3 obras.');
  });

  it('sends only what was chosen, and all the works when the box is cleared', async () => {
    await open();
    api.put.mockResolvedValue({ data: { changed: { comic_kind: 1 } } });
    await click(button('Classificar obras'));
    await choose(select('Quadrinho ou mangá'), 'comic');
    await click(form().querySelector('input[type="checkbox"]'));
    await click(apply());
    expect(api.put).toHaveBeenCalledWith('/collections/5/classification', { comicKind: 'comic', onlyUnset: false });
    expect(form().textContent).toContain('Mudei o tipo de 1 obra.');
  });

  it('takes the value away when "none" is chosen', async () => {
    await open();
    api.put.mockResolvedValue({ data: { changed: { unit: 2 } } });
    await click(button('Classificar obras'));
    await choose(select('Unidade'), 'none');
    await click(apply());
    expect(api.put).toHaveBeenCalledWith('/collections/5/classification', { unit: '', onlyUnset: true });
  });

  it('says what the server refused', async () => {
    await open();
    api.put.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 409, data: { error: 'A coleção está aposentada: restaure antes de mudar.' } } }));
    await click(button('Classificar obras'));
    await choose(select('Unidade'), 'volume');
    await click(apply());
    expect(form().textContent).toContain('A coleção está aposentada: restaure antes de mudar.');
  });

  it('closes, and refreshes the page after it changed the works', async () => {
    await open();
    api.put.mockResolvedValue({ data: { changed: { unit: 1 } } });
    const reads = () => api.get.mock.calls.filter(([url]) => url === '/collections/5').length;
    await click(button('Classificar obras'));
    await choose(select('Unidade'), 'volume');
    const before = reads();
    await click(apply());
    await flush();
    expect(reads()).toBeGreaterThan(before);
    await click([...form().querySelectorAll('button')].find((b) => b.textContent === 'Fechar'));
    expect(form()).toBeNull();
  });
});
