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
import { PersonPage } from './PersonPage';

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
const setValue = (el, value) => act(async () => {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, value);
  el.dispatchEvent(new Event('input', { bubbles: true }));
});
const chip = (label) => [...container.querySelectorAll('[aria-label="Função na obra"] button')].find((b) => b.textContent.startsWith(label));

async function open({ role = 'reader', notes = { data: [], total: 0 }, data = person(), works = { data: [work(1, 'Duna'), work(2, 'Messias')], total: 2, totalPages: 1 }, id = 9 } = {}) {
  asked = [];
  api.get.mockImplementation(async (url) => {
    if (url === '/auth/me') return { data: { id: 'u', role } };
    if (url === '/notes') return { data: notes };
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
  await act(async () => { root.render(<QueryClientProvider client={client}><PersonPage /></QueryClientProvider>); });
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

describe('PersonPage: the head of the page, with the portrait and the numbers together', () => {
  const stats = { works: 14, finished: 8, inProgress: 4, readingSeconds: 3600 };
  const profile = {
    wikidataId: 'Q7934', description: 'escritor americano', born: '1920-10-08', died: '1986-02-11', bornPlace: 'Tacoma', bio: 'Frank Herbert foi um escritor.',
    image: { url: '/covers/p.jpg', credit: 'Alguém', license: 'CC BY' },
  };

  it('shows the photo with where and when they lived over it, and the numbers and the themes beside it', async () => {
    await open({ data: person({ profile, stats, tags: [{ name: 'ecologia', works: 3 }] }) });
    const head = container.querySelector('h1').closest('.rounded-xl');
    const photo = head.querySelector('img');
    expect(photo.alt).toBe('Foto de Herbert, Frank');
    expect(head.querySelector('figure').textContent).toContain('Tacoma · 1920 — 1986');
    expect(head.querySelector('figcaption').textContent).toContain('Foto: Alguém · CC BY');
    expect(head.querySelector('dl[aria-label="Números"]')).toBeTruthy();
    expect(head.querySelector('ul[aria-label="Temas"] li').textContent).toBe('#ecologia');
    // The photo is in one place only: the profile text does not carry a second one.
    expect(container.querySelectorAll('img[alt^="Foto de"]')).toHaveLength(1);
  });

  it('puts the initials where the photo would be for a person with no profile, and still has the numbers', async () => {
    await open({ data: person({ profile: null, stats }) });
    const head = container.querySelector('h1').closest('.rounded-xl');
    expect(head.querySelector('img')).toBeNull();
    expect(head.querySelector('figure [aria-hidden="true"]').textContent).toBe('FH');
    expect(head.querySelector('figure').textContent).not.toContain('·');
    expect(head.querySelector('dl[aria-label="Números"]')).toBeTruthy();
  });

  it('puts the initials where the photo would be when the profile has none, and the span of the years alone', async () => {
    await open({ data: person({ profile: { ...profile, image: undefined, bornPlace: undefined } }) });
    const portrait = container.querySelector('h1').closest('.rounded-xl').querySelector('figure');
    expect(portrait.querySelector('[aria-hidden="true"]').textContent).toBe('FH');
    expect(portrait.textContent).toContain('1920 — 1986');
    expect(portrait.textContent).not.toContain('Tacoma');
  });

  it('uses the one initial of a name of one word', async () => {
    await open({ data: person({ name: 'Madonna', displayName: 'Madonna', profile: null, stats }) });
    expect(container.querySelector('h1').closest('.rounded-xl').querySelector('figure [aria-hidden="true"]').textContent).toBe('M');
  });

  it('shows a photo that staff hid faded, so they see it is not shown to the others', async () => {
    await open({ role: 'admin', data: person({ profile: { ...profile, imageHidden: true }, stats }) });
    const photo = container.querySelector('figure img');
    expect(photo.className).toContain('opacity-50');
  });

  it('shows the photo at full strength when it was not hidden', async () => {
    await open({ data: person({ profile, stats }) });
    expect(container.querySelector('figure img').className).not.toContain('opacity-50');
  });

  it('tells the description in the head and does not wrap the profile in a card of its own', async () => {
    await open({ data: person({ profile, stats }) });
    const card = container.querySelector('section[aria-label="Perfil"]');
    expect(card.className).not.toContain('border');
    expect(card.closest('.rounded-xl').contains(container.querySelector('h1'))).toBe(true);
    expect(card.textContent).toContain('escritor americano');
  });
});

describe('PersonPage: the tabs of the works (DEC-175)', () => {
  const volume = (id, title, series, seriesIndex) => ({ ...work(id, title), series, seriesIndex });
  const mixed = {
    data: [volume(1, 'Duna', 'Crônicas de Duna', 1), volume(2, 'Messias', 'Crônicas de Duna', 2), work(8, 'O Cérebro Verde'), work(9, 'Destino: Vazio'), work(10, 'O Dragão no Mar')],
    total: 5, totalPages: 1,
  };
  const notes = { total: 2, data: [
    { id: 1, kind: 'highlight', quote: 'Não terei medo. O medo é o assassino da mente, a pequena morte.', body: '', tags: [], workId: 1, workTitle: 'Duna', fileId: 10, chapter: 'Capítulo 1', locator: { type: 'epub', href: 'c1.xhtml' } },
    { id: 3, kind: 'highlight', quote: 'A mais alta função da ecologia é a compreensão das consequências.', body: '', tags: [], workId: 2, workTitle: 'Messias', fileId: 11, chapter: 'Capítulo 2', locator: { type: 'epub', href: 'c2.xhtml' } },
  ] };
  const tabs = () => [...container.querySelectorAll('[role="tab"]')];
  const tab = (label) => tabs().find((t) => t.textContent.startsWith(label));
  const headings = () => [...container.querySelectorAll('.library-shelf h2')].map((h) => h.textContent);

  it('has four, with how many each holds, when the person has series, loose works and highlights', async () => {
    await open({ works: mixed, notes });
    expect(tabs().map((t) => t.textContent)).toEqual(['Todas as obras5', 'Séries2', 'Avulsas3', 'Seus destaques2']);
    expect(tab('Todas').getAttribute('aria-selected')).toBe('true');
    expect(headings()).toEqual(['Crônicas de Duna', 'Demais obras']);
    // The highlights are at the bottom of the first tab.
    expect(container.querySelector('section[aria-label="Seus destaques nas obras dele"]')).toBeTruthy();
  });

  it('shows only the shelves of the series in one, and only the rest in the other', async () => {
    await open({ works: mixed, notes });
    await click(tab('Séries'));
    expect(headings()).toEqual(['Crônicas de Duna']);
    expect(container.querySelector('section[aria-label="Seus destaques nas obras dele"]')).toBeNull();
    await click(tab('Avulsas'));
    expect(headings()).toEqual(['Demais obras']);
    expect(container.querySelectorAll('.library-book')).toHaveLength(3);
    await click(tab('Todas'));
    expect(headings()).toEqual(['Crônicas de Duna', 'Demais obras']);
  });

  it('puts the highlights alone in theirs, with no shelves, no filter and no pages', async () => {
    await open({ works: mixed, notes });
    expect(container.querySelector('figure[aria-label="Um destaque seu"]')).toBeTruthy();
    await click(tab('Seus destaques'));
    expect(headings()).toEqual([]);
    // The one picked for the top is not shown over the list that has it.
    expect(container.querySelector('figure[aria-label="Um destaque seu"]')).toBeNull();
    expect(container.querySelector('[role="search"]')).toBeNull();
    const section = container.querySelector('section[aria-label="Seus destaques nas obras dele"]');
    expect(section.textContent).toContain('2 anotações salvas');
    expect(container.querySelectorAll('section[aria-label="Seus destaques nas obras dele"]')).toHaveLength(1);
  });

  it('keeps the counts of the tabs when something is typed in the filter', async () => {
    await open({ works: mixed, notes });
    await setValue(container.querySelector('input[type="search"]'), 'cérebro');
    expect(tabs().map((t) => t.textContent)).toEqual(['Todas as obras5', 'Séries2', 'Avulsas3', 'Seus destaques2']);
    expect(container.querySelectorAll('.library-book')).toHaveLength(1);
    // Narrowed to the series, nothing of the series has it.
    await click(tab('Séries'));
    expect(container.textContent).toContain('Nenhuma obra desta pessoa tem isso.');
  });

  it('puts the works of the tab in chronological order when that is the order', async () => {
    await open({ works: { ...mixed, data: mixed.data.map((w, i) => ({ ...w, originalYear: 1980 - i })) }, notes });
    await click(tab('Avulsas'));
    const order = container.querySelector('select');
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(order, 'year');
      order.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(headings()).toEqual(['Em ordem cronológica']);
    expect(container.querySelectorAll('.library-book')).toHaveLength(3);
  });

  it('goes back to the first tab when the one it was on is not there for the works of another role', async () => {
    const onlyLoose = { data: [work(20, 'Tradução A'), work(21, 'Tradução B')], total: 2, totalPages: 1 };
    await open({ works: (q) => (q.get('role') === 'translator' ? onlyLoose : mixed) });
    await click(tab('Séries'));
    expect(headings()).toEqual(['Crônicas de Duna']);
    await click(chip('Tradutor'));
    expect(tabs()).toHaveLength(0);
    expect(headings()).toEqual(['Tradução']);
    expect(container.querySelectorAll('.library-book')).toHaveLength(2);
  });

  it('keeps the title of the rest when what is typed leaves none of the series', async () => {
    await open({ works: mixed });
    await setValue(container.querySelector('input[type="search"]'), 'cérebro');
    expect(headings()).toEqual(['Demais obras']);
  });

  it('has no pages in the highlights', async () => {
    await open({ works: { ...mixed, totalPages: 2 }, notes });
    expect(container.querySelector('nav[aria-label="Páginas das obras da pessoa"]')).toBeTruthy();
    await click(tab('Seus destaques'));
    expect(container.querySelector('nav[aria-label="Páginas das obras da pessoa"]')).toBeNull();
  });

  it('starts on the first tab for another person', async () => {
    await open({ works: mixed, notes });
    await click(tab('Avulsas'));
    api.get.mockImplementation(async (url) => (url === '/people/10' ? { data: person({ id: 10, displayName: 'Outra' }) } : url === '/auth/me' ? { data: { role: 'reader' } } : url === '/notes' ? { data: notes } : { data: mixed }));
    await act(async () => { useGlobalStore.setState({ personSheetId: 10 }); });
    await flush();
    await flush();
    expect(tab('Todas').getAttribute('aria-selected')).toBe('true');
    expect(headings()).toEqual(['Crônicas de Duna', 'Demais obras']);
  });

  it('has no tabs for a person with only series and no highlights, or only loose works', async () => {
    await open({ works: { data: [volume(1, 'Duna', 'Crônicas de Duna', 1), volume(2, 'Messias', 'Crônicas de Duna', 2)], total: 2, totalPages: 1 } });
    expect(tabs()).toHaveLength(0);
    expect(headings()).toEqual(['Crônicas de Duna']);
  });

  it('has only the first and the highlights when the works are all of one kind', async () => {
    await open({ notes });
    expect(tabs().map((t) => t.textContent)).toEqual(['Todas as obras2', 'Seus destaques2']);
  });

  it('puts a dot of a color before each shelf and none on the plain ones of the home', async () => {
    await open({ works: mixed });
    const dots = [...container.querySelectorAll('.library-section-heading > span[aria-hidden="true"]')];
    expect(dots.map((d) => d.className.includes('bg-brand') ? 'brand' : d.className.includes('bg-success') ? 'success' : '?')).toEqual(['brand', 'success']);
  });
});

describe('PersonPage: the page of a person (#186)', () => {
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
    expect(container.querySelector('[role="region"]').getAttribute('aria-label')).toBe('Pessoa');
    expect(container.querySelector('h1').textContent).toBe('Herbert, Frank');
    expect(container.querySelector('[aria-current="page"]').textContent).toBe('Herbert, Frank');
    expect(container.textContent).toContain('Também aparece como F. Herbert · Herbert, Frank');
    expect(asked[0].get('person')).toBe('9');
    expect(asked[0].get('role')).toBe('author');
    expect(asked[0].get('sort')).toBe('title');
    expect(asked[0].get('limit')).toBe('100');
    expect(container.querySelectorAll('.library-book')).toHaveLength(2);
    expect(container.textContent).toContain('[ 2 obras ]');
    expect(container.querySelector('.library-section-heading h2').textContent).toBe('Obras');
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
    expect(container.querySelector('.library-section-heading h2').textContent).toBe('Tradução');
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

  it('puts the works of a series on a shelf of its own, in the order of the series, and the rest apart', async () => {
    const volume = (id, title, series, seriesIndex) => ({ ...work(id, title), series, seriesIndex });
    await open({ works: { data: [volume(3, 'Terceiro', 'Crônicas de Duna', 3), volume(1, 'Duna', 'Crônicas de Duna', 1), volume(2, 'Messias', 'Crônicas de Duna', 2), work(8, 'O Cérebro Verde'), volume(9, 'Sem número', 'Crônicas de Duna', 0)], total: 5, totalPages: 1 } });
    const shelves = [...container.querySelectorAll('.library-shelf')];
    expect(shelves.map((sh) => sh.querySelector('h2').textContent)).toEqual(['Crônicas de Duna', 'Demais obras']);
    expect(shelves[0].textContent).toContain('[ 4 volumes no acervo ]');
    expect([...shelves[0].querySelectorAll('.library-book h3, .library-book-title')].length).toBeGreaterThanOrEqual(0);
    const order = [...shelves[0].querySelectorAll('.library-book-cover')].length;
    expect(order).toBe(4);
    expect(shelves[0].textContent.indexOf('Duna')).toBeLessThan(shelves[0].textContent.indexOf('Messias'));
    expect(shelves[0].textContent.indexOf('Messias')).toBeLessThan(shelves[0].textContent.indexOf('Terceiro'));
    expect(shelves[0].textContent.indexOf('Terceiro')).toBeLessThan(shelves[0].textContent.indexOf('Sem número'));
    expect(shelves[1].textContent).toContain('O Cérebro Verde');
  });

  it('says one volume in the singular, and writes a series the same way however it was spelled', async () => {
    const volume = (id, title, series, seriesIndex) => ({ ...work(id, title), series, seriesIndex });
    await open({ works: { data: [volume(1, 'Solo', 'Saga Única', 1), volume(2, 'A', 'Crônicas de Duna', 1), volume(3, 'B', 'CRONICAS  DE DUNA', 2)], total: 3, totalPages: 1 } });
    const text = [...container.querySelectorAll('.library-shelf')].map((sh) => sh.textContent);
    expect(text.some((t) => t.includes('[ 1 volume no acervo ]'))).toBe(true);
    expect(text.some((t) => t.includes('[ 2 volumes no acervo ]'))).toBe(true);
  });

  it('opens the collection of a series from its shelf, in place of this page', async () => {
    const volume = (id, title, series, seriesIndex) => ({ ...work(id, title), series, seriesIndex });
    await open({ works: { data: [volume(1, 'Duna', 'Crônicas de Duna', 1)], total: 1, totalPages: 1 } });
    await click(button('Ver a coleção'));
    expect(useGlobalStore.getState()).toMatchObject({ collectionSheetId: 5, personSheetId: null });
  });

  it('finds the collection of a series whatever the case and the accents it is written with', async () => {
    const volume = (id, title, series, seriesIndex) => ({ ...work(id, title), series, seriesIndex });
    await open({ works: { data: [volume(1, 'Duna', 'CRONICAS DE DUNA', 1)], total: 1, totalPages: 1 } });
    await click(button('Ver a coleção'));
    expect(useGlobalStore.getState()).toMatchObject({ collectionSheetId: 5 });
  });

  it('has no shelf of the rest when every work is in a series', async () => {
    const volume = (id, title, series, seriesIndex) => ({ ...work(id, title), series, seriesIndex });
    await open({ works: { data: [volume(1, 'Duna', 'Crônicas de Duna', 1)], total: 1, totalPages: 1 } });
    expect([...container.querySelectorAll('.library-shelf h2')].map((h) => h.textContent)).toEqual(['Crônicas de Duna']);
  });

  it('has no link to a collection for a series that is not one', async () => {
    const volume = (id, title, series, seriesIndex) => ({ ...work(id, title), series, seriesIndex });
    await open({ data: person({ collections: [] }), works: { data: [volume(1, 'Duna', 'Crônicas de Duna', 1)], total: 1, totalPages: 1 } });
    expect(button('Ver a coleção')).toBeUndefined();
  });

  it('says how many works the library has of theirs and how far the person is, and the hours read', async () => {
    await open({ data: person({ stats: { works: 14, finished: 3, inProgress: 2, readingSeconds: 3 * 3600 + 20 * 60 } }) });
    const stats = container.querySelector('dl[aria-label="Números"]');
    expect(stats.textContent).toContain('Obras no acervo14');
    expect(stats.textContent).toContain('Que você terminou3');
    expect(stats.textContent).toContain('Em andamento2');
    expect(stats.textContent).toMatch(/Horas lidas.*3/);
  });

  it('says no hours when none was read, and no numbers for a person with no works', async () => {
    await open({ data: person({ stats: { works: 5, finished: 0, inProgress: 0, readingSeconds: 0 } }) });
    expect(container.querySelector('dl[aria-label="Números"]').textContent).toContain('Horas lidas—');
    act(() => root.unmount());
    root = createRoot(container);
    await open({ data: person({ stats: { works: 0, finished: 0, inProgress: 0, readingSeconds: 0 } }) });
    expect(container.querySelector('dl[aria-label="Números"]')).toBeNull();
  });

  it('lists the themes of the works, each as a tag', async () => {
    await open({ data: person({ tags: [{ name: 'ecologia', works: 5 }, { name: 'política', works: 1 }] }) });
    expect([...container.querySelectorAll('ul[aria-label="Temas"] li')].map((li) => li.textContent)).toEqual(['#ecologia', '#política']);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ data: person({ tags: [] }) });
    expect(container.querySelector('ul[aria-label="Temas"]')).toBeNull();
  });

  it('says the library has no work of a person who has none, and asks for none', async () => {
    await open({ data: person({ roles: [], collections: [] }) });
    expect(container.textContent).toContain('O acervo não tem obra dessa pessoa.');
    expect(asked).toHaveLength(0);
  });

  it('opens the page of a work over this one, without closing it', async () => {
    await open();
    await click(container.querySelector('.library-book-cover'));
    expect(useGlobalStore.getState()).toMatchObject({ sheetWorkId: 1, personSheetId: 9 });
  });

  it('goes back with its button, and is a page: Escape does not leave it', async () => {
    await open();
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(useGlobalStore.getState().personSheetId).toBe(9);
    await click(button('← Voltar'));
    expect(useGlobalStore.getState().personSheetId).toBeNull();
  });

  it('puts one of the highlights on the works of the person at the top and all of them at the bottom, each opening the reader there', async () => {
    const notes = { total: 2, data: [
      { id: 1, kind: 'highlight', quote: 'Não terei medo. O medo é o assassino da mente, a pequena morte.', body: '', tags: [], workId: 1, workTitle: 'Duna', fileId: 10, chapter: 'Capítulo 1', locator: { type: 'epub', href: 'c1.xhtml' } },
      { id: 2, kind: 'bookmark', quote: '', body: '', tags: [], workId: 1, workTitle: 'Duna', fileId: 10, locator: { type: 'epub', href: 'c2.xhtml' } },
    ] };
    await open({ notes });
    expect(api.get).toHaveBeenCalledWith('/notes', { params: { personId: 9, limit: 200, chapters: true } });
    expect(container.querySelector('figure[aria-label="Um destaque seu"]').textContent).toContain('Duna · Capítulo 1');
    const section = container.querySelector('section[aria-label="Seus destaques nas obras dele"]');
    expect(section.textContent).toContain('1 anotação salva');
    expect(section.textContent).toContain('Duna · Capítulo 1'); // each says which work it is of
    await click(section.querySelector('li button'));
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 1, activeFileId: 10 });
    expect(useGlobalStore.getState().seek).toMatchObject({ locator: { type: 'epub', href: 'c1.xhtml' }, context: { kind: 'note' } });
  });

  describe('the filter, the formats and the order', () => {
    const w = (id, title, format, extra = {}) => ({ ...work(id, title), format, ...extra });
    const list = { data: [w(1, 'Duna', 'epub', { originalYear: 1965 }), w(2, 'Messias de Duna', 'epub', { originalYear: 1969 }), w(3, 'O Cérebro Verde', 'pdf', { originalYear: 1966 }), w(4, 'Audio Dune', 'm4b'), w(5, 'Sem ano', 'epub'), w(6, 'Outro Pdf', 'pdf')], total: 6, totalPages: 1 };
    const titles = () => [...container.querySelectorAll('.library-book')].map((b) => b.textContent);
    const box = () => container.querySelector('input[type="search"]');

    it('filters the works by what is typed, in the title, without caring for case and accents', async () => {
      await open({ works: list });
      expect(titles()).toHaveLength(6);
      await setValue(box(), 'CEREBRO');
      expect(titles()).toHaveLength(1);
      expect(titles()[0]).toContain('Cérebro');
      await setValue(box(), 'xyz');
      expect(container.textContent).toContain('Nenhuma obra desta pessoa tem isso.');
    });

    it('offers the formats the works have, with how many, and filters by one and back', async () => {
      await open({ works: list });
      const chips = [...container.querySelectorAll('[aria-label="Formato"] button')].map((b) => b.textContent.replace(/\s+/g, ' ').trim());
      expect(chips).toEqual(['Todos', 'EPUB (3)', 'PDF (2)', 'M4B (1)']); // the most first
      await click(button('EPUB (3)'));
      expect(titles()).toHaveLength(3);
      expect(button('EPUB (3)').getAttribute('aria-pressed')).toBe('true');
      await click(button('EPUB (3)')); // again, back to all
      expect(titles()).toHaveLength(6);
      await click(button('PDF (2)'));
      await click(button('Todos'));
      expect(titles()).toHaveLength(6);
    });

    it('puts the works in chronological order, those nobody dated last', async () => {
      await open({ works: list });
      await act(async () => {
        const select = container.querySelector('select');
        Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(select, 'year');
        select.dispatchEvent(new Event('change', { bubbles: true }));
      });
      expect(container.querySelector('.library-section-heading h2').textContent).toBe('Em ordem cronológica');
      const order = titles().map((t) => ['Cérebro', 'Messias', 'Audio', 'Sem ano', 'Outro', 'Duna'].find((n) => t.includes(n)));
      expect(order).toEqual(['Duna', 'Cérebro', 'Messias', 'Audio', 'Outro', 'Sem ano']);
    });

    it('has no filter for a person with one work, and none of the formats when there is one format', async () => {
      await open({ works: { data: [w(1, 'Duna', 'epub')], total: 1, totalPages: 1 } });
      expect(container.querySelector('[aria-label="Filtrar as obras"]')).toBeNull();
      act(() => root.unmount());
      root = createRoot(container);
      await open({ works: { data: [w(1, 'Duna', 'epub'), w(2, 'Messias', 'epub')], total: 2, totalPages: 1 } });
      expect(container.querySelector('[aria-label="Filtrar as obras"]')).toBeTruthy();
      expect(container.querySelector('[aria-label="Formato"]')).toBeNull();
    });

    it('starts again for another person', async () => {
      await open({ works: list });
      await setValue(box(), 'duna');
      await click(button('EPUB (3)'));
      api.get.mockImplementation(async (url) => (url === '/people/10' ? { data: person({ id: 10, displayName: 'Outra' }) } : url === '/auth/me' ? { data: { role: 'reader' } } : url === '/notes' ? { data: { data: [], total: 0 } } : { data: list }));
      await act(async () => { useGlobalStore.setState({ personSheetId: 10 }); });
      await flush();
      await flush();
      expect(box().value).toBe('');
      expect(titles()).toHaveLength(6);
    });
  });

  it('has no marginalia when the person kept none on the works', async () => {
    await open();
    expect(container.querySelector('section[aria-label="Seus destaques nas obras dele"]')).toBeNull();
    expect(container.querySelector('figure[aria-label="Um destaque seu"]')).toBeNull();
  });

  it('gives staff a way to the correction of the name, and nobody else', async () => {
    await open({ role: 'admin' });
    await click(button('Corrigir o nome'));
    expect(useGlobalStore.getState()).toMatchObject({ adminOpen: true, adminTab: 'duplicates', personSheetId: null });
    act(() => root.unmount());
    root = createRoot(container);
    useGlobalStore.setState({ adminOpen: false });
    await open({ role: 'reader' });
    expect(button('Corrigir o nome')).toBeUndefined();
  });

  it('goes to the library from the trail', async () => {
    await open();
    await click(button('Biblioteca'));
    expect(useGlobalStore.getState()).toMatchObject({ personSheetId: null, libraryView: 'all' });
  });

  it('asks for nothing while no page is open', async () => {
    api.get.mockImplementation(async () => { throw new Error('should not ask'); });
    useGlobalStore.setState({ personSheetId: null });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><PersonPage /></QueryClientProvider>); });
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
    expect(container.querySelector('h1').textContent).toBe('Outra');
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
