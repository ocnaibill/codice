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
import { CollectionPage } from './CollectionPage';

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
let notesReply = { data: [] };
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const labelled = (label) => container.querySelector(`[aria-label="${label}"]`);
const click = (el) => act(async () => { el.click(); });

async function open({ role = 'admin', data = detail(trio) } = {}) {
  reply = data;
  api.get.mockImplementation(async (url, options) => {
    if (url === '/collections/5') return { data: reply };
    if (url === '/notes') return { data: notesReply };
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
  await act(async () => { root.render(<QueryClientProvider client={client}><CollectionPage /></QueryClientProvider>); });
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
  notesReply = { data: [] };
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  useGlobalStore.setState({ collectionSheetId: null, sheetWorkId: null });
});

describe('CollectionPage: what everybody sees', () => {
  it('shows the works in order, with their numbers and which were read, and says how much was read', async () => {
    await open({ role: 'reader' });
    const rows = [...container.querySelectorAll('ol li')].map((li) => li.textContent);
    expect(rows).toHaveLength(3);
    expect(rows[0]).toContain('Pedra Filosofal');
    expect(rows[0]).toContain('Lida');
    expect(rows[1]).toContain('Câmara Secreta');
    expect(rows[2]).toContain('Contos');
    expect(container.textContent).toContain('3 obras · Leu 1 de 3');
    expect(container.querySelector('[role="region"]').getAttribute('aria-label')).toBe('Coleção');
    expect(labelled('Obras da coleção').tagName).toBe('OL');
    expect(container.querySelector('h1').textContent).toBe('Harry Potter');
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

  it('goes back with its button, and is a page: Escape does not leave it', async () => {
    await open({ role: 'reader' });
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(useGlobalStore.getState().collectionSheetId).toBe(5);
    await click(button('← Voltar'));
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
  });

  it('goes to the library from the trail', async () => {
    await open({ role: 'reader' });
    await click(button('Biblioteca'));
    expect(useGlobalStore.getState()).toMatchObject({ collectionSheetId: null, libraryView: 'all' });
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
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionPage /></QueryClientProvider>); });
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
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionPage /></QueryClientProvider>); });
    await flush();
    expect(container.textContent).toContain('Não foi possível abrir esta coleção.');
  });
});

describe('CollectionPage: what owner and admin do', () => {
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
    expect(container.querySelector('h1').textContent).toBe('Outra');
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

describe('CollectionPage: a list of the person', () => {
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
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionPage /></QueryClientProvider>); });
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
    expect(container.querySelector('[role="region"]').getAttribute('aria-label')).toBe('Lista');
    expect(container.querySelector('nav[aria-label="Onde você está"]').textContent).toContain('Lista');
    for (const text of ['Renomear', 'Acrescentar obra', 'Aposentar']) expect(button(text)).toBeTruthy();
    expect(labelled('Obras da lista').tagName).toBe('OL');
  });

  it('is not renamed or put away when it is one the Códice keeps ("Ler depois"), but its works are managed like any list', async () => {
    await openList({ role: 'reader', data: list([entry(10, 1, 'Duna')], { name: 'Ler depois', system: 'read_later' }) });
    expect(button('Renomear')).toBeUndefined();
    expect(button('Aposentar')).toBeUndefined();
    expect(button('Acrescentar obra')).toBeTruthy();
    expect(container.textContent).toContain('É uma lista do Códice');
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

describe('CollectionPage: favoriting', () => {
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

describe('CollectionPage: the works in groups by unit (#187)', () => {
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
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionPage /></QueryClientProvider>); });
    await flush();
    await flush();
    expect(container.querySelector('section h3')).toBeNull();
    expect([...container.querySelectorAll('ol li > div > span:first-child')].map((n) => n.textContent)).toEqual(['1', '2']);
  });
});

describe('CollectionPage: classifying the works of a collection (#187)', () => {
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
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionPage /></QueryClientProvider>); });
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

describe('CollectionPage: where to go on in a series (#187)', () => {
  const step = (over = {}) => ({ id: 2, title: 'Câmara Secreta', unit: 'chapter', position: 2, started: false, begun: true, ...over });
  const goOnButton = () => [...container.querySelectorAll('button')].find((b) => /^(Continuar|Próximo|Começar):/.test(b.textContent.trim()));

  it('offers to begin, to go on with what was begun, or to read the next, and opens the reader on it', async () => {
    for (const [over, text] of [
      [{ begun: false }, 'Começar: Cap. 2'],
      [{ begun: true }, 'Próximo: Cap. 2'],
      [{ begun: true, started: true }, 'Continuar: Cap. 2'],
    ]) {
      await open({ data: { ...detail(trio), continue: step(over) } });
      expect(goOnButton().textContent.trim()).toBe(text);
      act(() => root.unmount());
      root = createRoot(container);
    }
    await open({ data: { ...detail(trio), continue: step() } });
    useGlobalStore.setState({ activeBookId: null });
    await click(goOnButton());
    expect(useGlobalStore.getState().activeBookId).toBe(2);
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
  });

  it('names a work with no number by its title', async () => {
    await open({ data: { ...detail(trio), continue: step({ unit: '', position: null, title: 'Contos' }) } });
    expect(goOnButton().textContent.trim()).toBe('Próximo: Contos');
  });

  it('offers nothing when there is nothing left to read, for a retired collection, or when the server says nothing', async () => {
    await open({ data: { ...detail(trio), continue: null } });
    expect(goOnButton()).toBeUndefined();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ data: detail(trio) });
    expect(goOnButton()).toBeUndefined();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ data: { ...detail(trio, { retired: true }), continue: step() } });
    expect(goOnButton()).toBeUndefined();
  });

  it('is there for a reader too, who manages nothing', async () => {
    await open({ role: 'reader', data: { ...detail(trio), continue: step() } });
    expect(goOnButton()).toBeDefined();
    expect(button('Renomear')).toBeUndefined();
  });
});

describe('CollectionPage: the page of a series (DEC-162)', () => {
  const row = (entryId, id, title, extra = {}) => ({
    entryId, id, title, author: 'J. K. Rowling', coverUrl: `/c/${id}.jpg`, position: id, completed: false, available: true, unit: 'volume', comicKind: '',
    percent: 0, started: false, formats: ['epub'], rating: 0, ...extra,
  });
  const works = [
    row(10, 1, 'Pedra Filosofal', { completed: true, percent: 100, originalYear: 1997, synopsis: 'Um menino descobre que é bruxo.', completedAt: '2023-09-12T10:00:00Z', rating: 5, formats: ['epub', 'm4b'] }),
    row(20, 2, 'Câmara Secreta', { started: true, percent: 64, chapter: 'Capítulo 14', unitIndex: 204, unitTotal: 318, readFormat: 'epub', originalYear: 1998 }),
    row(30, 4, 'Cálice de Fogo', {}),
  ];
  const full = (extra = {}) => ({
    collection: { id: 5, kind: 'official', name: 'Harry Potter', workCount: 3, completedCount: 1, coverUrl: '/c/1.jpg' },
    works,
    continue: { id: 2, title: 'Câmara Secreta', unit: 'volume', position: 2, started: true, begun: true },
    summary: {
      works: 3, finished: 1, inProgress: 1, percent: 54.7, readingSeconds: 3 * 3600 + 20 * 60, yearFrom: 1997, yearTo: 2000,
      missing: [{ unit: 'volume', number: 3 }],
      authors: [{ id: 9, name: 'J. K. Rowling', works: 3 }], translators: [{ id: 11, name: 'Lia Wyler', works: 3 }],
      tags: [{ name: 'magia', works: 3 }, { name: 'escola', works: 2 }],
    },
    ...extra,
  });
  const show = async (data = full(), role = 'reader') => {
    await open({ role, data });
  };

  it('says who wrote and translated it, the years, and the tags, with a way to the page of each person', async () => {
    await show();
    const hero = container.querySelector('section[aria-label="Sobre a coleção"]');
    expect(hero.textContent).toContain('1997–2000 · 3 obras');
    expect(hero.textContent).toContain('J. K. Rowling');
    expect(hero.textContent).toContain('Trad.: Lia Wyler');
    expect([...hero.querySelectorAll('ul[aria-label="Etiquetas"] li')].map((li) => li.textContent)).toEqual(['#magia', '#escola']);
    await click([...hero.querySelectorAll('button')].find((b) => b.textContent === 'Lia Wyler'));
    expect(useGlobalStore.getState().personSheetId).toBe(11);
  });

  it('says how far the person is in the whole, the hours and how many are in progress', async () => {
    await show();
    const box = container.querySelector('[aria-label="Seu progresso"]');
    expect(box.textContent).toContain('55%');
    expect(box.textContent).toContain('1 de 3 volumes lidos');
    expect(box.textContent).toContain('1 em andamento');
    expect(box.textContent).toMatch(/3.*lidas/); // the hours, apart from the works
  });

  it('continues the series with how far it is and where it stopped', async () => {
    await show();
    const go = [...container.querySelectorAll('button')].find((b) => b.textContent.includes('(64%)'));
    expect(go).toBeTruthy();
    expect(container.textContent).toContain('Parou em Capítulo 14');
    await click(go);
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 2 });
  });

  it('says the numbers of the series that are not in the library', async () => {
    await show();
    expect(container.querySelector('[role="status"]').textContent).toBe('Falta no acervo: Vol. 3.');
    act(() => root.unmount());
    root = createRoot(container);
    await open({ role: 'reader', data: full({ summary: { ...full().summary, missing: [{ unit: 'volume', number: 3 }, { unit: 'volume', number: 5 }] } }) });
    expect(container.querySelector('[role="status"]').textContent).toBe('Faltam no acervo: Vol. 3, Vol. 5.');
  });

  it('says nothing of what is missing in a collection that was put away', async () => {
    await show(full({ collection: { ...full().collection, retired: true } }), 'admin');
    expect(container.querySelector('[role="status"]')).toBeNull();
  });

  it('says nothing of what is missing when nothing is', async () => {
    await show(full({ summary: { ...full().summary, missing: [] } }));
    expect(container.querySelector('[role="status"]')).toBeNull();
  });

  it('says of each work what it is and what the person did: the year, the formats, the stars, when it was finished and how far they are', async () => {
    await show();
    const rows = [...container.querySelectorAll('ol li')].map((li) => li.textContent);
    expect(rows[0]).toContain('Publicado em 1997');
    expect(rows[0]).toContain('EPUB · M4B');
    expect(rows[0]).toContain('★★★★★');
    expect(rows[0]).toContain('Lida em setembro de 2023');
    expect(rows[0]).toContain('Um menino descobre que é bruxo.');
    expect(rows[1]).toContain('Em leitura 64%');
    expect(rows[1]).toContain('Capítulo 14');
    expect(rows[1]).toContain('Pos. 204 de 318');
    expect(rows[1]).toContain('Retomar');
    expect(rows[2]).not.toContain('Retomar');
  });

  it('says which one is next, and no more of the rest', async () => {
    await show(full({ continue: { id: 4, title: 'Cálice de Fogo', unit: 'volume', position: 4, started: false, begun: true } }));
    const rows = [...container.querySelectorAll('ol li')].map((li) => li.textContent);
    expect(rows[2]).toContain('Próxima da fila');
    expect(rows[0]).not.toContain('Próxima da fila');
  });

  it('opens the reader at the work from "Retomar"', async () => {
    await show();
    await click(button('Retomar'));
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 2 });
  });

  it('can be seen as a compact list, with none of the details of the cards', async () => {
    await show();
    await click(button('Lista compacta'));
    const rows = [...container.querySelectorAll('ol li')].map((li) => li.textContent);
    expect(rows[0]).not.toContain('Publicado em');
    expect(rows[0]).not.toContain('Um menino');
    expect(rows[1]).not.toContain('Retomar');
    expect(rows[1]).toContain('Em leitura 64%'); // what it is stays
    await click(button('Cartões ricos'));
    expect([...container.querySelectorAll('ol li')][0].textContent).toContain('Publicado em 1997');
  });

  it('says no summary for a collection with no work to say it of', async () => {
    await show({ collection: full().collection, works: [], continue: null, summary: { ...full().summary, works: 0, authors: [], translators: [], tags: [], missing: [] } });
    expect(container.querySelector('[aria-label="Seu progresso"]')).toBeNull();
  });
});

describe('CollectionPage: the notes and the description (DEC-163)', () => {
  const row = (entryId, id, title, extra = {}) => ({
    entryId, id, title, author: 'J. K. Rowling', coverUrl: `/c/${id}.jpg`, position: id, completed: false, available: true, unit: 'volume', comicKind: '',
    percent: 0, started: false, formats: ['epub'], rating: 0, notes: 0, ...extra,
  });
  const works = [row(10, 1, 'Pedra Filosofal', { notes: 3 }), row(20, 2, 'Câmara Secreta', { notes: 1 }), row(30, 3, 'Cálice de Fogo')];
  const full = (extra = {}, notes = 4) => ({
    collection: { id: 5, kind: 'official', name: 'Harry Potter', workCount: 3, completedCount: 0, coverUrl: '/c/1.jpg', ...extra },
    works,
    continue: null,
    summary: { works: 3, finished: 0, inProgress: 0, percent: 0, readingSeconds: 0, notes, missing: [], authors: [], translators: [], tags: [] },
  });
  const note = (id, workId, workTitle, quote) => ({ id, kind: 'highlight', workId, fileId: workId * 100, workTitle, quote, body: '', locator: { type: 'epub', cfi: `c${id}` }, chapter: 'Capítulo 2' });
  const typeInto = async (el, value) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set.call(el, value);
      el.dispatchEvent(new Event('input', { bubbles: true }));
    });
  };

  it('says how many notes the person has on the collection and on each work, in the singular too', async () => {
    notesReply = { data: [note(1, 1, 'Pedra Filosofal', 'Um trecho.')] };
    await open({ role: 'reader', data: full() });
    const box = labelled('Seu progresso');
    expect(box.textContent).toContain('4 anotações suas');
    const rows = [...container.querySelectorAll('ol li')].map((li) => li.textContent);
    expect(rows[0]).toContain('3 anotações');
    expect(rows[1]).toContain('1 anotação');
    expect(rows[1]).not.toContain('1 anotações');
    expect(rows[2]).not.toContain('anotaç');
    act(() => root.unmount());
    root = createRoot(container);
    await open({ role: 'reader', data: full({}, 1) });
    expect(labelled('Seu progresso').textContent).toContain('1 anotação sua');
  });

  it('asks for the notes of the collection, and lists them with the work each one is of, opening the reader at its place', async () => {
    notesReply = { data: [note(1, 1, 'Pedra Filosofal', 'Um trecho do primeiro.'), note(2, 2, 'Câmara Secreta', 'Um trecho do segundo.')] };
    await open({ role: 'reader', data: full() });
    expect(api.get).toHaveBeenCalledWith('/notes', { params: { collectionId: 5, limit: 200, chapters: true } });
    const section = labelled('Suas anotações nesta coleção');
    expect(section.textContent).toContain('Um trecho do primeiro.');
    expect(section.textContent).toContain('Câmara Secreta · Capítulo 2');
    await click(section.querySelector('button[title="Abrir no livro"]'));
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 1 });
  });

  it('takes the person to the notes from "Ver notas"', async () => {
    notesReply = { data: [note(1, 1, 'Pedra Filosofal', 'Um trecho.')] };
    const scroll = vi.fn();
    const original = Element.prototype.scrollIntoView;
    Element.prototype.scrollIntoView = scroll;
    try {
      await open({ role: 'reader', data: full() });
      scroll.mockClear();
      await click(button('Ver notas'));
      expect(scroll).toHaveBeenCalledTimes(1);
      expect(scroll.mock.contexts[0].contains(labelled('Suas anotações nesta coleção'))).toBe(true);
    } finally {
      Element.prototype.scrollIntoView = original;
    }
  });

  it('asks for no notes, and has no section or link, when the person has none there', async () => {
    await open({ role: 'reader', data: full({}, 0) });
    expect(api.get).not.toHaveBeenCalledWith('/notes', expect.anything());
    expect(button('Ver notas')).toBeUndefined();
    expect(labelled('Suas anotações nesta coleção')).toBeNull();
  });

  it('calls the section of a list by its name', async () => {
    notesReply = { data: [note(1, 1, 'Pedra Filosofal', 'Um trecho.')] };
    await open({ role: 'reader', data: full({ kind: 'personal' }) });
    expect(labelled('Suas anotações nesta lista')).toBeTruthy();
  });

  it('shows the description under the name, with its line breaks', async () => {
    await open({ role: 'reader', data: full({ description: 'Sete livros.\nUma saga.' }, 0) });
    const hero = labelled('Sobre a coleção');
    const text = [...hero.querySelectorAll('p')].find((p) => p.textContent.startsWith('Sete livros.'));
    expect(text.textContent).toBe('Sete livros.\nUma saga.');
    expect(text.className).toContain('whitespace-pre-line');
    expect(button('Escrever descrição')).toBeUndefined(); // a reader writes nothing
    expect(button('Editar descrição')).toBeUndefined();
  });

  it('writes the description for the first time: only the description is sent, and the form closes', async () => {
    await open({ role: 'admin', data: full({}, 0) });
    await click(button('Escrever descrição'));
    const area = container.querySelector('textarea');
    expect(area.value).toBe('');
    await typeInto(area, 'Uma saga de bruxos.');
    expect(container.textContent).toContain('19 de 2000');
    await click(button('Salvar'));
    expect(api.patch).toHaveBeenCalledWith('/collections/5', { description: 'Uma saga de bruxos.' });
    await flush();
    expect(container.querySelector('textarea')).toBeNull();
  });

  it('edits the description it has, and clears it when saved empty', async () => {
    await open({ role: 'admin', data: full({ description: 'Antiga.' }, 0) });
    await click(button('Editar descrição'));
    const area = container.querySelector('textarea');
    expect(area.value).toBe('Antiga.');
    await typeInto(area, '');
    await click(button('Salvar'));
    expect(api.patch).toHaveBeenCalledWith('/collections/5', { description: '' });
  });

  it('says what the server refused and keeps the form', async () => {
    await open({ role: 'admin', data: full({}, 0) });
    await click(button('Escrever descrição'));
    api.patch.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 409, data: { error: 'A coleção está aposentada: restaure antes de mudar.', collectionId: 5 } } }));
    await click(button('Salvar'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('A coleção está aposentada: restaure antes de mudar.');
    expect(container.querySelector('textarea')).toBeTruthy();
  });

  it('writes the description of a list through its own route, and not of the list the Códice keeps', async () => {
    await open({ role: 'reader', data: full({ kind: 'personal' }, 0) });
    await click(button('Escrever descrição'));
    await typeInto(container.querySelector('textarea'), 'Para o fim de semana.');
    await click(button('Salvar'));
    expect(api.patch).toHaveBeenCalledWith('/my/collections/5', { description: 'Para o fim de semana.' });

    act(() => root.unmount());
    root = createRoot(container);
    await open({ role: 'reader', data: full({ kind: 'personal', system: 'read_later' }, 0) });
    expect(button('Escrever descrição')).toBeUndefined();
    expect(button('Editar descrição')).toBeUndefined();
  });

  it('offers no description to write on a collection that was put away', async () => {
    await open({ role: 'admin', data: full({ retired: true }, 0) });
    expect(button('Escrever descrição')).toBeUndefined();
  });
});

describe('CollectionPage: the complementary works (DEC-164)', () => {
  const row = (entryId, id, title, unit, position, extra = {}) => ({
    entryId, id, title, author: 'J. K. Rowling', coverUrl: `/c/${id}.jpg`, position, completed: false, available: true, unit, comicKind: '',
    percent: 0, started: false, formats: ['epub'], rating: 0, notes: 0, ...extra,
  });
  const works = [
    row(10, 1, 'Pedra Filosofal', 'volume', 1),
    row(20, 2, 'Câmara Secreta', 'volume', 2),
    row(30, 7, 'Animais Fantásticos', 'extra', 1),
    row(40, 8, 'Quadribol Através dos Séculos', 'extra', 2),
  ];
  const data = {
    collection: { id: 5, kind: 'official', name: 'Harry Potter', workCount: 4, completedCount: 0, coverUrl: '/c/1.jpg' },
    works,
    continue: null,
    summary: { works: 2, finished: 0, inProgress: 0, percent: 0, readingSeconds: 0, notes: 0, missing: [], authors: [], translators: [], tags: [] },
  };

  it('puts them in a section of their own, after the sequence, and says they are not in the progress', async () => {
    await open({ role: 'reader', data });
    const sections = [...container.querySelectorAll('section[aria-label]')].map((s) => s.getAttribute('aria-label')).filter((l) => l !== 'Sobre a coleção');
    expect(sections).toEqual(['Volumes', 'Complementares']);
    const extra = labelled('Complementares');
    expect(extra.querySelector('h3').textContent).toContain('Complementares (2)');
    expect(extra.textContent).toContain('Fora da sequência: não entram no progresso');
    expect(labelled('Volumes').textContent).not.toContain('Fora da sequência');
    // No number in the sequence: they are called by the word, and the progress counts the volumes only.
    const rows = [...extra.querySelectorAll('ol li')].map((li) => li.textContent);
    expect(rows[0]).toContain('Compl.');
    expect(rows[0]).toContain('Animais Fantásticos');
    expect(labelled('Seu progresso').textContent).toContain('0 de 2 volumes lidos');
  });

  it('is offered in the classification, next to the other units', async () => {
    await open({ role: 'admin', data });
    await click(button('Classificar obras'));
    const options = [...labelled('Classificar as obras').querySelectorAll('option')].map((o) => o.textContent);
    expect(options).toContain('Complementar');
  });

  it('is moved inside its own group, with the unit said', async () => {
    await open({ role: 'admin', data });
    await click(container.querySelector('button[aria-label="Descer “Animais Fantásticos”"]'));
    expect(api.put).toHaveBeenCalledWith('/collections/5/order', { workIds: [8, 7], unit: 'extra' });
  });
});

describe('CollectionPage: a series of chapters (DEC-165)', () => {
  const chapter = (n, extra = {}) => ({
    entryId: n * 10, id: n, title: `Capítulo ${n}`, author: 'Kentarō Miura', coverUrl: `/c/${n}.jpg`, position: n, completed: false, available: true,
    unit: 'chapter', comicKind: 'manga', percent: 0, started: false, formats: ['cbz'], rating: 0, notes: 0, bookmarks: 0, ...extra,
  });
  const chapters = [
    chapter(1, { title: 'O Espadachim Negro', completed: true, percent: 100, completedAt: '2024-10-12T10:00:00Z' }),
    chapter(2, { title: 'O Eclipse', completed: true, percent: 100, bookmarks: 4 }),
    chapter(3, { title: 'O Nascimento', started: true, percent: 40, notes: 2, readFormat: 'cbz', unitIndex: 3, unitTotal: 8 }),
    chapter(4, { title: 'O Despertar' }),
    chapter(5, { title: 'A Marca' }),
    chapter(6, { title: 'Os Apóstolos' }),
  ];
  const data = (works = chapters, summary = {}) => ({
    collection: { id: 5, kind: 'official', name: 'Berserk', workCount: works.length, completedCount: 2, coverUrl: '/c/1.jpg' },
    works,
    continue: { id: 3, title: 'O Nascimento', unit: 'chapter', position: 3, started: true, begun: true },
    summary: {
      works: works.length, finished: 2, inProgress: 1, percent: 40, readingSeconds: 7200, notes: 2, bookmarks: 4, remainingSeconds: 34 * 3600,
      missing: [], authors: [], translators: [], tags: [], ...summary,
    },
  });
  const titles = () => [...container.querySelectorAll('ol li')].map((li) => li.querySelector('.font-display')?.textContent);
  const typeInSearch = async (value) => {
    const input = container.querySelector('input[type="search"]');
    await act(async () => {
      Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set.call(input, value);
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
  };

  it('says how many of the chapters were read, in the word of the unit, and the volumes in theirs', async () => {
    await open({ role: 'reader', data: data() });
    expect(labelled('Seu progresso').textContent).toContain('2 de 6 capítulos lidos');
    act(() => root.unmount());
    root = createRoot(container);
    await open({ role: 'reader', data: data([chapter(1, { unit: 'volume', completed: true })], { works: 1, finished: 1 }) });
    expect(labelled('Seu progresso').textContent).toContain('1 de 1 volume lido');
  });

  it('keeps "lidas" when the works are of more than one unit', async () => {
    const mixed = [chapter(1, { completed: true }), chapter(2, { unit: 'volume' })];
    await open({ role: 'reader', data: data(mixed, { works: 2, finished: 1 }) });
    expect(labelled('Seu progresso').textContent).toContain('1 de 2 lidas');
  });

  it('says how long is left, the marked places and the notes, with the way to the notes', async () => {
    await open({ role: 'reader', data: data() });
    const box = labelled('Seu progresso').textContent;
    expect(box).toContain('~34 h restantes');
    expect(box).toContain('4 marcadores · 2 anotações suas');
    act(() => root.unmount());
    root = createRoot(container);
    await open({ role: 'reader', data: data(chapters, { remainingSeconds: 0, bookmarks: 1, notes: 0 }) });
    const text = labelled('Seu progresso').textContent;
    expect(text).not.toContain('restantes');
    expect(text).toContain('1 marcador');
    expect(text).not.toContain('1 marcadores');
    expect(button('Ver notas')).toBeUndefined();
  });

  it('filters by name or by number, and says when nothing is left, with a way to clear it', async () => {
    await open({ role: 'reader', data: data() });
    await typeInSearch('eclipse');
    expect(titles()).toEqual(['O Eclipse']);
    await typeInSearch('cap. 5');
    expect(titles()).toEqual(['A Marca']);
    await typeInSearch('zzz');
    expect(container.querySelector('ol')).toBeNull();
    expect(container.textContent).toContain('Nenhuma obra com esse filtro.');
    await click(button('Limpar o filtro'));
    expect(titles()).toHaveLength(6);
    expect(container.querySelector('input[type="search"]').value).toBe('');
  });

  it('filters by what was read, with how many there are of each, and by the ones with something saved', async () => {
    await open({ role: 'reader', data: data() });
    const chip = (text) => [...container.querySelectorAll('[aria-label="Situação"] button')].find((b) => b.textContent.startsWith(text));
    expect(chip('Todas').textContent).toBe('Todas (6)');
    expect(chip('Não lidas').textContent).toBe('Não lidas (4)');
    expect(chip('Lidas').textContent).toBe('Lidas (2)');
    expect(chip('Salvas').textContent).toBe('Salvas (2)'); // one with marks and one with notes
    await click(chip('Lidas'));
    expect(titles()).toEqual(['O Espadachim Negro', 'O Eclipse']);
    await click(chip('Não lidas'));
    expect(titles()).toEqual(['O Nascimento', 'O Despertar', 'A Marca', 'Os Apóstolos']);
    await click(chip('Salvas'));
    expect(titles()).toEqual(['O Eclipse', 'O Nascimento']);
    expect(chip('Salvas').getAttribute('aria-pressed')).toBe('true');
  });

  it('shows the order the other way without changing the collection, and gives no moving while it is a view', async () => {
    await open({ role: 'admin', data: data() });
    expect(container.querySelector('button[aria-label="Subir “O Eclipse”"]')).toBeTruthy();
    await click(button('Ordem crescente'));
    expect(titles()).toEqual(['Os Apóstolos', 'A Marca', 'O Despertar', 'O Nascimento', 'O Eclipse', 'O Espadachim Negro']);
    expect(button('Ordem decrescente').getAttribute('aria-pressed')).toBe('true');
    expect(container.querySelector('button[aria-label="Subir “O Eclipse”"]')).toBeNull();
    await click(button('Ordem decrescente'));
    expect(container.querySelector('button[aria-label="Subir “O Eclipse”"]')).toBeTruthy();
    await typeInSearch('o');
    expect(container.querySelector('button[aria-label="Subir “O Eclipse”"]')).toBeNull();
    expect(container.querySelector('button[aria-label="Tirar “O Eclipse” da coleção"]')).toBeTruthy(); // taking out is not a matter of order
  });

  it('has no filters for a series of a few works', async () => {
    await open({ role: 'reader', data: data(chapters.slice(0, 5), { works: 5 }) });
    expect(container.querySelector('[aria-label="Filtrar as obras"]')).toBeNull();
    expect(container.querySelector('button[aria-label="Subir “O Eclipse”"]')).toBeNull(); // a reader moves nothing
  });

  it('offers to read again a work that was finished, and not one that was not', async () => {
    await open({ role: 'reader', data: data() });
    const rows = [...container.querySelectorAll('ol li')];
    expect([...rows[0].querySelectorAll('button')].some((b) => b.textContent === 'Reler')).toBe(true);
    expect([...rows[3].querySelectorAll('button')].some((b) => b.textContent === 'Reler')).toBe(false);
    await click([...rows[0].querySelectorAll('button')].find((b) => b.textContent === 'Reler'));
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 1 });
  });

  it('forgets the filters when another collection is opened', async () => {
    await open({ role: 'reader', data: data() });
    await typeInSearch('eclipse');
    await act(async () => { useGlobalStore.setState({ collectionSheetId: 6 }); });
    await flush();
    await act(async () => { useGlobalStore.setState({ collectionSheetId: 5 }); });
    await flush();
    expect(container.querySelector('input[type="search"]')?.value ?? '').toBe('');
  });
});
