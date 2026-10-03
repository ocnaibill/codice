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
import { WorkSuggestions } from './WorkSuggestions';
import { suggestionText } from '../suggestionText';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const candidate = (over) => ({ id: 1, field: 'isbn', value: '9788576573135', source: 'Google Books', current: '', evidence: {}, keys: [], ...over });
const author = candidate({
  id: 2, field: 'author', value: 'Terry Pratchett', source: 'Open Library', current: 'T. Pratchett',
  keys: [{ name: 'Terry Pratchett', scheme: 'openlibrary', value: 'OL25712A' }],
});
const people = candidate({
  id: 3, field: 'contributors', source: 'Open Library', current: 'T. Pratchett (author); Paul Kidby (illustrator)',
  value: '[{"name":"Neil Gaiman","role":"author"},{"name":"Paul Kidby","role":"translator"}]',
  keys: [{ name: 'Neil Gaiman', scheme: 'openlibrary', value: 'OL53305A' }],
});
const synopsis = candidate({ id: 4, field: 'description', value: 'Uma sinopse.\nEm duas linhas.', source: 'Google Books', current: '' });
const tags = candidate({ id: 5, field: 'tags', value: '["Sci-Fi","Clássico"]', source: 'Open Library' });

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text, within = container) => [...within.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const items = () => [...container.querySelectorAll('li')];
const itemOf = (text) => items().find((li) => li.textContent.includes(text));

async function show(list, { post } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7/candidates') return { data: { data: list } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockImplementation(post || (async () => ({ data: {} })));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  client.invalidateQueries = vi.fn(client.invalidateQueries.bind(client));
  await act(async () => { root.render(<QueryClientProvider client={client}><WorkSuggestions workId={7} /></QueryClientProvider>); });
  await flush();
  return client;
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

describe('WorkSuggestions (#70)', () => {
  it('is not there when nothing waits for a decision', async () => {
    await show([]);
    expect(container.textContent).toBe('');
  });

  it('says what each provider suggests, what the work has today, and that nothing changes until it is accepted', async () => {
    await show([author, synopsis]);
    expect(container.textContent).toContain('Nada muda até você aceitar');
    const a = itemOf('Terry Pratchett');
    expect(a.textContent).toContain('Autor · Open Library');
    expect(a.textContent).toContain('Hoje: T. Pratchett');
    expect(itemOf('Uma sinopse.').textContent).toContain('Hoje: em branco');
    expect(api.post).not.toHaveBeenCalled();
  });

  it('lists the fields in a fixed order, the same field by the order they arrived', async () => {
    await show([tags, synopsis, candidate({ id: 9, field: 'title', value: 'Dune' }), candidate({ id: 6, field: 'author', value: 'Pratchett, T.' }), author]);
    const labels = items().map((li) => li.querySelector('p').textContent.split(' · ')[0]);
    expect(labels).toEqual(['Título', 'Autor', 'Autor', 'Sinopse', 'Etiquetas']);
    expect(items()[1].textContent).toContain('Terry Pratchett'); // id 2 before id 6
  });

  it('reads lists as text and the roles of the people in Portuguese, today and suggested', async () => {
    await show([people, tags]);
    expect(itemOf('Neil Gaiman').textContent).toContain('Neil Gaiman (autor); Paul Kidby (tradutor)');
    expect(itemOf('Neil Gaiman').textContent).toContain('Hoje: T. Pratchett (autor); Paul Kidby (ilustrador)');
    expect(container.textContent).toContain('Sci-Fi, Clássico');
    expect(container.textContent).not.toContain('{"name"');
    expect(container.textContent).not.toContain('["Sci-Fi"');
  });

  it('names the languages, the suggested one and the one the work has today', async () => {
    await show([candidate({ id: 7, field: 'language', value: 'fr', current: 'en', source: 'Open Library' })]);
    expect(items()[0].textContent).toContain('Francês');
    expect(items()[0].textContent).toContain('Hoje: Inglês');
  });

  it('says which key accepting would keep for whom, and says nothing when there is none', async () => {
    await show([author, people, synopsis]);
    expect(itemOf('T. Pratchett').textContent).toContain('Ao aceitar, guarda a chave de Terry Pratchett: Open Library OL25712A.');
    expect(itemOf('Neil Gaiman').textContent).toContain('Ao aceitar, guarda a chave de Neil Gaiman: Open Library OL53305A.');
    expect(itemOf('Uma sinopse').textContent).not.toContain('chave');
  });

  it('accepts and rejects through the API and asks again for everything that depends on it', async () => {
    const client = await show([author, synopsis]);
    await act(async () => { button('Aceitar', itemOf('Terry Pratchett')).click(); });
    await flush();
    expect(api.post).toHaveBeenCalledWith('/works/7/candidates/2/accept');
    const keys = client.invalidateQueries.mock.calls.map(([arg]) => arg.queryKey[0]);
    expect(keys).toEqual(expect.arrayContaining(['candidates', 'admin', 'works', 'work', 'stats', 'favorites']));

    await act(async () => { button('Recusar', itemOf('Uma sinopse')).click(); });
    await flush();
    expect(api.post).toHaveBeenCalledWith('/works/7/candidates/4/reject');
  });

  it('shows what the server said when a decision fails, and clears it on the next try', async () => {
    let fail = true;
    await show([author], { post: async () => { if (fail) throw { response: { data: 'Candidate not found' } }; return { data: {} }; } });
    await act(async () => { button('Aceitar').click(); });
    await flush();
    expect(container.querySelector('[role="alert"]').textContent).toBe('Sugestão não encontrada.');
    fail = false;
    await act(async () => { button('Aceitar').click(); });
    await flush();
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('says it could not decide when the server gives no reason', async () => {
    await show([author], { post: async () => { throw new Error('offline'); } });
    await act(async () => { button('Recusar').click(); });
    await flush();
    expect(container.querySelector('[role="alert"]').textContent).toBe('Não foi possível recusar a sugestão.');
    await act(async () => { button('Aceitar').click(); });
    await flush();
    expect(container.querySelector('[role="alert"]').textContent).toBe('Não foi possível aceitar a sugestão.');
  });

  it('disables both buttons of every suggestion while a decision is on its way', async () => {
    let finish;
    await show([author, synopsis], { post: () => new Promise((resolve) => { finish = resolve; }) });
    await act(async () => { button('Aceitar', itemOf('Terry Pratchett')).click(); });
    expect([...container.querySelectorAll('button')].every((b) => b.disabled)).toBe(true);
    await act(async () => { finish({ data: {} }); });
    await flush();
    expect([...container.querySelectorAll('button')].some((b) => b.disabled)).toBe(false);
  });
});

describe('suggestionText', () => {
  it('shows a value that is not the list its field promises as it came', () => {
    expect(suggestionText({ field: 'contributors', value: 'Neil Gaiman' })).toBe('Neil Gaiman');
    expect(suggestionText({ field: 'tags', value: '{"a":1}' })).toBe('{"a":1}');
    expect(suggestionText({ field: 'isbn', value: '9788576573135' })).toBe('9788576573135');
  });

  it('names a language the way the sheet does', () => {
    expect(suggestionText({ field: 'language', value: 'pt' })).toBe('Português');
  });
});

describe('without a heading of its own', () => {
  it('says so when there is nothing to decide, if it was told what to say', async () => {
    await show([]);
    expect(container.textContent).toBe('');
    act(() => root.unmount());
    root = createRoot(container);
    api.get.mockImplementation(async () => ({ data: { data: [] } }));
    await act(async () => { root.render(<QueryClientProvider client={new QueryClient()}><WorkSuggestions workId={7} emptyText="Nada por aqui." /></QueryClientProvider>); });
    await flush();
    expect(container.textContent).toBe('Nada por aqui.');
  });
});
