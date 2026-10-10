import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axe from 'axe-core';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { PersonSheet } from './PersonSheet';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const person = (extra = {}) => ({
  id: 9, name: 'Frank Herbert', displayName: 'Herbert, Frank', aliases: ['F. Herbert', 'Herbert, Frank'],
  roles: [{ role: 'author', works: 14 }, { role: 'translator', works: 1 }],
  collections: [{ id: 5, name: 'Crônicas de Duna', works: 6 }, { id: 8, name: 'Antologias', works: 1 }],
  ...extra,
});
const work = (id, title) => ({ id, title, author: 'Frank Herbert', coverUrl: '/c.jpg', tags: [], format: 'epub', mediaStatus: 'READY', fileCount: 1 });

let container;
let root;
let asked;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const click = async (el) => { await act(async () => { el.click(); }); await flush(); };
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const chip = (label) => [...container.querySelectorAll('[aria-label="Função na obra"] button')].find((b) => b.textContent.startsWith(label));

async function open({ data = person(), works = { data: [work(1, 'Duna'), work(2, 'Messias')], total: 2, totalPages: 1 }, id = 9 } = {}) {
  asked = [];
  api.get.mockImplementation(async (url) => {
    if (url === '/auth/me') return { data: { id: 'u', role: 'reader' } };
    if (url === `/people/${id}`) {
      if (data instanceof Error) throw data;
      return { data };
    }
    if (url.startsWith('/works?')) {
      asked.push(new URLSearchParams(url.split('?')[1]));
      return { data: typeof works === 'function' ? works(asked.at(-1)) : works };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  useGlobalStore.setState({ personSheetId: id, sheetWorkId: null, collectionSheetId: null });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><PersonSheet /></QueryClientProvider>); });
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
  useGlobalStore.setState({ personSheetId: null, sheetWorkId: null, collectionSheetId: null });
});

describe('PersonSheet: the page of a person (#186)', () => {
  it('shows the profile of the author above the works, with where it came from, when one was read', async () => {
    await open({ data: person({ profile: { wikidataId: 'Q7934', description: 'escritor americano', born: '1920', bio: 'Frank Herbert foi um escritor.',
      bioSource: { language: 'pt', title: 'Frank Herbert', url: 'https://pt.wikipedia.org/wiki/Frank_Herbert', license: 'CC BY-SA 4.0' } } }) });
    const profile = container.querySelector('[aria-label="Perfil"]');
    expect(profile.textContent).toContain('escritor americano');
    expect(profile.textContent).toContain('Fonte: Wikipédia (pt), CC BY-SA 4.0');
    expect(container.textContent.indexOf('escritor americano')).toBeLessThan(container.textContent.indexOf('Também aparece como'));
  });

  it('has no profile when none was read', async () => {
    await open({ data: person({ profile: null }) });
    expect(container.querySelector('[aria-label="Perfil"]')).toBeNull();
  });

  it('says who they are, the names they are also written with, and lists their works as author first', async () => {
    await open();
    expect(container.querySelector('[role="dialog"]').getAttribute('aria-label')).toBe('Pessoa');
    expect(container.querySelector('h2').textContent).toBe('Herbert, Frank');
    expect(container.textContent).toContain('Também aparece como F. Herbert · Herbert, Frank');
    expect(asked[0].get('person')).toBe('9');
    expect(asked[0].get('role')).toBe('author');
    expect(asked[0].get('sort')).toBe('title');
    expect(asked[0].get('limit')).toBe('12');
    expect(container.querySelectorAll('.library-book')).toHaveLength(2);
    expect(container.textContent).toContain('[ 2 obras ]');
  });

  it('offers the roles the person has, with how many works in each, and the first one is on', async () => {
    await open();
    expect([...container.querySelectorAll('[aria-label="Função na obra"] button')].map((b) => b.textContent)).toEqual(['Autor14', 'Tradutor1']);
    expect(chip('Autor').getAttribute('aria-pressed')).toBe('true');
    expect(chip('Tradutor').getAttribute('aria-pressed')).toBe('false');
  });

  it('shows the works of another role when it is picked, from the first page', async () => {
    await open({ works: (q) => ({ data: [work(q.get('role') === 'translator' ? 7 : 1, 'X')], total: 30, totalPages: 3 }) });
    await click(button('Próxima'));
    expect(asked.at(-1).get('page')).toBe('2');
    await click(chip('Tradutor'));
    expect(asked.at(-1).get('role')).toBe('translator');
    expect(asked.at(-1).get('page')).toBe('1');
    expect(chip('Tradutor').getAttribute('aria-pressed')).toBe('true');
    expect(container.querySelector('h2 + span, .library-section-heading h2').textContent).toBe('Tradução');
  });

  it('has no roles to choose between when there is one', async () => {
    await open({ data: person({ roles: [{ role: 'author', works: 3 }] }) });
    expect(container.querySelector('[aria-label="Função na obra"]')).toBeNull();
  });

  it('pages through the works', async () => {
    await open({ works: { data: [work(1, 'A')], total: 30, totalPages: 3 } });
    expect(container.textContent).toContain('1 de 3');
    expect(button('Anterior').disabled).toBe(true);
    await click(button('Próxima'));
    expect(asked.at(-1).get('page')).toBe('2');
    expect(container.textContent).toContain('2 de 3');
  });

  it('has no pages when the works fit in one', async () => {
    await open();
    expect(container.querySelector('nav[aria-label="Páginas das obras da pessoa"]')).toBeNull();
  });

  it('lists the collections that have works of theirs, and opens one in place of this page', async () => {
    await open();
    const names = [...container.querySelectorAll('section[aria-label="Coleções"] button')].map((b) => b.textContent);
    expect(names).toEqual(['Crônicas de Duna6', 'Antologias1']);
    await click(container.querySelector('[aria-label="Abrir a coleção Crônicas de Duna"]'));
    expect(useGlobalStore.getState()).toMatchObject({ collectionSheetId: 5, personSheetId: null });
  });

  it('shows no line of collections or of names when there are none', async () => {
    await open({ data: person({ collections: [], aliases: [] }) });
    expect(container.querySelector('section[aria-label="Coleções"]')).toBeNull();
    expect(container.textContent).not.toContain('Também aparece como');
  });

  it('says the library has no work of a person who has none, and asks for none', async () => {
    await open({ data: person({ roles: [], collections: [] }) });
    expect(container.textContent).toContain('O acervo não tem obra dessa pessoa.');
    expect(asked).toHaveLength(0);
  });

  it('opens the sheet of a work over the page, without closing it', async () => {
    await open();
    await click(container.querySelector('.library-book-cover'));
    expect(useGlobalStore.getState()).toMatchObject({ sheetWorkId: 1, personSheetId: 9 });
  });

  it('closes with the button and with Escape, and renders nothing when none is open', async () => {
    await open();
    await click(container.querySelector('[aria-label="Fechar"]'));
    expect(useGlobalStore.getState().personSheetId).toBeNull();
    await open();
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(useGlobalStore.getState().personSheetId).toBeNull();
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });

  it('asks for nothing while no page is open', async () => {
    api.get.mockImplementation(async () => { throw new Error('should not ask'); });
    useGlobalStore.setState({ personSheetId: null });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><PersonSheet /></QueryClientProvider>); });
    await flush();
    expect(api.get).not.toHaveBeenCalled();
  });

  it('says why a person could not be opened', async () => {
    await open({ data: Object.assign(new Error('x'), { response: { status: 404, data: 'Person not found' } }) });
    expect(container.textContent).toContain('Não foi possível abrir esta página.');
  });

  it('says why the works could not be loaded', async () => {
    await open({ works: () => { throw Object.assign(new Error('x'), { response: { status: 500, data: '' } }); } });
    expect(container.textContent).toContain('Não foi possível carregar as obras.');
  });

  it('starts at the first role and page when another person is opened', async () => {
    await open({ works: { data: [work(1, 'A')], total: 30, totalPages: 3 } });
    await click(chip('Tradutor'));
    await click(button('Próxima'));
    api.get.mockImplementation(async (url) => (url === '/people/10' ? { data: person({ id: 10, displayName: 'Outra' }) } : { data: { data: [], total: 0, totalPages: 1 } }));
    await act(async () => { useGlobalStore.setState({ personSheetId: 10 }); });
    await flush();
    await flush();
    expect(container.querySelector('h2').textContent).toBe('Outra');
    expect(chip('Autor').getAttribute('aria-pressed')).toBe('true');
  });

  it('has no accessibility violation (axe)', async () => {
    await open();
    const result = await axe.run(document.body, {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
      rules: { 'color-contrast': { enabled: false }, region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
    });
    expect(result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.html.slice(0, 90)).join(' | ')}`)).toEqual([]);
  });
});
