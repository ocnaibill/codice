import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { CollectionsGrid } from './CollectionsGrid';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const col = (id, name, extra = {}) => ({ id, kind: 'official', name, workCount: 3, completedCount: 1, coverUrl: `/c/${id}.jpg`, ...extra });
const page = (data, extra = {}) => ({ data, total: data.length, page: 1, limit: 24, totalPages: 1, ...extra });

let container;
let root;
let asked;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const click = (el) => act(async () => { el.click(); });

async function open({ role = 'reader', list = page([col(1, 'Duna'), col(2, 'Fundação')]), retired = page([]), kind } = {}) {
  asked = [];
  api.get.mockImplementation(async (url, options) => {
    if (url === '/auth/me') return { data: { role } };
    if (url === '/collections') {
      asked.push(options?.params);
      return { data: options?.params?.retired ? retired : list };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  api.delete.mockResolvedValue({});
  api.post.mockImplementation(async (url, body) => (url === '/collections' || url === '/my/collections' ? { data: { id: 77, name: body.name } } : {}));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><CollectionsGrid kind={kind} /></QueryClientProvider>); });
  await flush();
  await flush();
}

async function type(input, value) {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.setState({ collectionSheetId: null });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('CollectionsGrid', () => {
  it('shows a card per collection, with how many works and how many were read, and counts them', async () => {
    await open();
    const cards = [...container.querySelectorAll('article')];
    expect(cards).toHaveLength(2);
    expect(cards[0].textContent).toContain('Duna');
    expect(cards[0].textContent).toContain('3 obras · Leu 1 de 3');
    expect(cards[0].textContent).toContain('Coleção');
    expect(container.textContent).toContain('[ 2 coleções ]');
    expect(asked[0]).toEqual({ page: 1, limit: 24 });
  });

  it('opens the page of a collection from its card', async () => {
    await open();
    await click(container.querySelector('[aria-label="Abrir a coleção Fundação"]'));
    expect(useGlobalStore.getState().collectionSheetId).toBe(2);
  });

  it('says there are none, in the words of who reads and of who can make them', async () => {
    await open({ list: page([]) });
    expect(container.textContent).toContain('Elas aparecem quando as obras de uma série entram no acervo.');
    expect(button('Nova coleção')).toBeUndefined();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ role: 'admin', list: page([]) });
    expect(container.textContent).toContain('Elas nascem do nome da série no metadado das obras, ou você cria uma');
  });

  it('says it could not load, and tries again', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/auth/me') return { data: { role: 'reader' } };
      throw new Error('boom');
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><CollectionsGrid /></QueryClientProvider>); });
    await flush();
    expect(container.querySelector('[role="alert"]').textContent).toContain('Não foi possível carregar as coleções.');
    expect(button('Tentar novamente')).toBeTruthy();
  });

  it('has no pages when there is only one', async () => {
    await open({ list: page([col(1, 'A')]) });
    expect(container.querySelector('nav[aria-label="Páginas das coleções"]')).toBeNull();
  });

  it('pages through them', async () => {
    await open({ list: page([col(1, 'A')], { total: 30, totalPages: 2 }) });
    expect(container.textContent).toContain('1 de 2');
    expect(button('Anterior').disabled).toBe(true);
    await click(button('Próxima'));
    expect(asked.at(-1)).toEqual({ page: 2, limit: 24 });
  });

  it('gives a reader no way to make or restore anything', async () => {
    await open();
    for (const text of ['Nova coleção', 'Aposentadas', 'Restaurar']) expect(button(text)).toBeUndefined();
  });

  it('lets owner and admin make one, and opens it to fill it', async () => {
    await open({ role: 'admin' });
    await click(button('Nova coleção'));
    expect(button('Criar').disabled).toBe(true);
    await type(container.querySelector('input'), 'Marvel');
    await click(button('Criar'));
    expect(api.post).toHaveBeenCalledWith('/collections', { name: 'Marvel' });
    expect(useGlobalStore.getState().collectionSheetId).toBe(77);
    expect(container.querySelector('form')).toBeNull();
  });

  it('does not make one with a name of spaces, not even by sending the form', async () => {
    await open({ role: 'admin' });
    await click(button('Nova coleção'));
    await type(container.querySelector('input'), '   ');
    await act(async () => { container.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    expect(api.post).not.toHaveBeenCalled();
  });

  it('offers to restore only the retired ones', async () => {
    await open({ role: 'admin' });
    expect(container.querySelectorAll('[aria-label^="Restaurar a coleção"]')).toHaveLength(0);
  });

  it('goes back to the first page when it switches to the retired ones', async () => {
    await open({ role: 'admin', list: page([col(1, 'A')], { total: 30, totalPages: 2 }), retired: page([]) });
    await click(button('Próxima'));
    expect(asked.at(-1).page).toBe(2);
    await click(button('Aposentadas'));
    expect(asked.at(-1)).toEqual({ page: 1, limit: 24, retired: 'true' });
  });

  it('shows why a collection was not made', async () => {
    await open({ role: 'owner' });
    await click(button('Nova coleção'));
    await type(container.querySelector('input'), 'Duna');
    api.post.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 409, data: { error: 'Já existe uma coleção com esse nome.', collectionId: 1 } } }));
    await click(button('Criar'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('Já existe uma coleção com esse nome.');
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
    await click(button('Cancelar'));
    expect(container.querySelector('form')).toBeNull();
  });

  it('lists the retired ones for the staff, with a way to restore each', async () => {
    const gone = col(4, 'Antiga', { retired: true, workCount: 0, completedCount: 0 });
    await open({ role: 'admin', retired: page([gone]) });
    expect(button('Nova coleção')).toBeTruthy();
    await click(button('Aposentadas'));
    await flush();
    expect(asked.at(-1)).toEqual({ page: 1, limit: 24, retired: 'true' });
    expect(container.querySelector('h2').textContent).toBe('Coleções aposentadas');
    expect(container.textContent).toContain('Coleção aposentada');
    expect(button('Nova coleção')).toBeUndefined();
    await click(container.querySelector('[aria-label="Restaurar a coleção Antiga"]'));
    expect(api.post).toHaveBeenCalledWith('/collections/4/restore');
  });

  it('says there is no retired one', async () => {
    await open({ role: 'admin' });
    await click(button('Aposentadas'));
    await flush();
    expect(container.textContent).toContain('Nenhuma coleção aposentada.');
  });
});

describe('CollectionsGrid: the lists of the person', () => {
  const mine = (id, name, extra = {}) => ({ id, kind: 'personal', name, workCount: 2, completedCount: 0, coverUrl: '/c.jpg', ...extra });

  it('asks for the lists of the caller, and says so', async () => {
    await open({ kind: 'personal', list: page([mine(3, 'Para ler')]) });
    expect(asked[0]).toEqual({ page: 1, limit: 24, kind: 'personal' });
    expect(container.querySelector('h2').textContent).toBe('Minhas listas');
    expect(container.textContent).toContain('[ 1 lista ]');
    const card = container.querySelector('article');
    expect(card.textContent).toContain('Lista');
    expect(card.textContent).toContain('2 obras');
    expect(container.querySelector('[aria-label="Abrir a lista Para ler"]')).not.toBeNull();
    expect(container.querySelector('nav[aria-label]')).toBeNull();
  });

  it('counts them in the plural', async () => {
    await open({ kind: 'personal', list: page([mine(3, 'A'), mine(4, 'B')]) });
    expect(container.textContent).toContain('[ 2 listas ]');
  });

  it('gives a reader the way to make one, to look at the ones put away, and to restore them', async () => {
    await open({ role: 'reader', kind: 'personal', list: page([mine(3, 'Para ler')]), retired: page([mine(9, 'Antiga', { retired: true })]) });
    expect(button('Nova lista')).toBeTruthy();
    expect(button('Aposentadas')).toBeTruthy();
    await click(button('Aposentadas'));
    await flush();
    expect(asked.at(-1)).toEqual({ page: 1, limit: 24, kind: 'personal', retired: 'true' });
    expect(container.querySelector('h2').textContent).toBe('Listas aposentadas');
    expect(container.textContent).toContain('Lista aposentada');
    await click(container.querySelector('[aria-label="Restaurar a lista Antiga"]'));
    expect(api.post).toHaveBeenCalledWith('/my/collections/9/restore');
  });

  it('makes a list through the route of the person, and opens it to fill it', async () => {
    await open({ role: 'reader', kind: 'personal', list: page([]) });
    expect(container.textContent).toContain('Você ainda não tem listas. Crie uma e acrescente as obras que quer reunir.');
    await click(button('Nova lista'));
    expect(container.textContent).toContain('Nome da lista');
    await type(container.querySelector('input'), 'Verão');
    await click(button('Criar'));
    expect(api.post).toHaveBeenCalledWith('/my/collections', { name: 'Verão' });
    expect(useGlobalStore.getState().collectionSheetId).toBe(77);
  });

  it('says there is no list put away, in its own words', async () => {
    await open({ kind: 'personal', list: page([]) });
    await click(button('Aposentadas'));
    await flush();
    expect(container.textContent).toContain('Nenhuma lista aposentada.');
  });

  it('leaves the official ones to the staff: the ones of the library still ask for their own route', async () => {
    await open({ role: 'reader' });
    expect(asked[0]).toEqual({ page: 1, limit: 24 });
    expect(button('Nova coleção')).toBeUndefined();
    expect(button('Nova lista')).toBeUndefined();
  });
});

describe('CollectionsGrid: favoriting a collection', () => {
  it('has a heart on each card, filled when the collection is a favorite, and toggles it', async () => {
    await open({ list: page([col(1, 'Duna', { isFavorite: true }), col(2, 'Fundação', { isFavorite: false })]) });
    const hearts = [...container.querySelectorAll('.library-favorite')];
    expect(hearts.map((h) => h.getAttribute('aria-pressed'))).toEqual(['true', 'false']);
    expect(hearts[0].getAttribute('aria-label')).toBe('Remover dos favoritos: Duna');
    expect(hearts[1].getAttribute('aria-label')).toBe('Adicionar aos favoritos: Fundação');
    await click(hearts[1]);
    expect(api.post).toHaveBeenCalledWith('/collections/2/favorite');
    await click(hearts[0]);
    expect(api.delete).toHaveBeenCalledWith('/collections/1/favorite');
  });

  it('has the heart for a list too, and for a reader', async () => {
    await open({ role: 'reader', kind: 'personal', list: page([col(3, 'Para ler', { kind: 'personal', isFavorite: false })]) });
    await click(container.querySelector('[aria-label="Adicionar aos favoritos: Para ler"]'));
    expect(api.post).toHaveBeenCalledWith('/collections/3/favorite');
  });

  it('has no heart on a retired one, which is to be restored, not favorited', async () => {
    await open({ role: 'admin', retired: page([col(4, 'Antiga', { retired: true })]) });
    await click(button('Aposentadas'));
    await flush();
    expect(container.querySelector('.library-favorite')).toBeNull();
    expect(container.querySelector('[aria-label="Restaurar a coleção Antiga"]')).not.toBeNull();
  });
});
