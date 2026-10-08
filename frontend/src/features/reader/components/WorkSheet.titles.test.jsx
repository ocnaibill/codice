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
import { WorkSheet } from './WorkSheet';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const work = (alternativeTitles, contributors, extraMeta = {}, over = {}) => ({
  id: 7, title: 'Duna', author: 'Frank Herbert', coverUrl: '/covers/7.jpg', fileId: 10,
  metadata: { description: 'Uma sinopse.', alternativeTitles, contributors, ...extraMeta },
  editions: [{ id: 1, title: 'Duna', language: 'pt', isPrimary: true, files: [{ id: 10, format: 'epub', availability: 'available', url: '/file/10', percentComplete: 0, completed: false }] }],
  ...over,
});

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

async function open(titles, contributors, extraMeta, series = { collection: null, next: null }, over = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work(titles, contributors, extraMeta, over) };
    if (url === '/works/7/candidates') return { data: { data: [] } };
    if (url === '/works/7/series') return { data: series };
    if (url === '/auth/me') return { data: { role: 'reader' } };
    throw new Error(`unexpected GET ${url}`);
  });
  useGlobalStore.setState({ sheetWorkId: 7 });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><WorkSheet /></QueryClientProvider>); });
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
  useGlobalStore.setState({ sheetWorkId: null });
});

describe('WorkSheet: the people on the work, each opening a page (#186)', () => {
  const credits = [
    { personId: 1, name: 'Frank Herbert', displayName: 'Herbert, Frank', role: 'author', position: 0 },
    { personId: 2, name: 'Brian Herbert', displayName: 'Brian Herbert', role: 'author', position: 1 },
    { personId: 3, name: 'Maria Tradutora', displayName: 'Maria Tradutora', role: 'translator', position: 0 },
  ];
  const named = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent === text);

  it('shows each author by the name the account is shown, and opens the page of the one that is pressed', async () => {
    await open([], credits);
    expect(named('Herbert, Frank')).toBeTruthy();
    expect(named('Brian Herbert')).toBeTruthy();
    expect(named('Frank Herbert')).toBeUndefined();
    await act(async () => { named('Brian Herbert').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ personSheetId: 2, sheetWorkId: null });
  });

  it('puts a comma between the authors, and opens the page of a translator too', async () => {
    await open([], credits);
    const line = [...container.querySelectorAll('p')].find((p) => p.textContent.startsWith('Herbert, Frank'));
    expect(line.textContent).toBe('Herbert, Frank, Brian Herbert');
    await act(async () => { named('Maria Tradutora').click(); });
    expect(useGlobalStore.getState().personSheetId).toBe(3);
  });

  it('says the author as the card says it when the work has no credits', async () => {
    await open([], []);
    expect([...container.querySelectorAll('p')].some((p) => p.textContent === 'Frank Herbert')).toBe(true);
    expect(named('Frank Herbert')).toBeUndefined();
  });

  it('falls back to the stored name when there is no name to show', async () => {
    await open([], [{ personId: 1, name: 'Frank Herbert', role: 'author', position: 0 }]);
    expect(named('Frank Herbert')).toBeTruthy();
  });
});

describe('WorkSheet: the other names of the work (#185)', () => {
  const otherLines = () => [...container.querySelectorAll('[aria-label="Outros títulos"] p')].map((p) => p.textContent);
  const heading = () => container.querySelector('h3').textContent;
  // Two editions: the Portuguese one, and an English one that someone gave a title and that the person is reading.
  const twoEditions = (english) => ({
    editions: [
      { id: 1, title: 'Duna', titleSet: false, language: 'pt', isPrimary: true, files: [{ id: 10, format: 'epub', availability: 'available', url: '/file/10', percentComplete: 0, completed: false }] },
      { id: 2, title: 'Dune', titleSet: true, language: 'en', isPrimary: false, files: [{ id: 20, format: 'epub', availability: 'available', url: '/file/20', percentComplete: 40, completed: false, started: true }], ...english },
    ],
    continue: { fileId: 20, format: 'epub', language: 'en', percentComplete: 40, completed: false },
    inProgress: true,
  });

  it('puts the other names under the title, each on a line, smaller, with its language, and no longer as "also known as"', async () => {
    await open([{ id: 3, title: 'Arrakis', language: '', source: 'manual' }, { id: 0, title: 'Dune', language: 'en', source: 'edition', editionId: 2 }]);
    expect(otherLines()).toEqual(['Arrakis', 'DuneInglês']);
    expect(container.textContent).not.toContain('Também conhecida como');
    const line = container.querySelector('[aria-label="Outros títulos"] p');
    expect(line.className).toContain('text-xl');
    expect(heading()).toBe('Duna');
  });

  it('goes by the title written for the edition being read, and the other names are the rest, the main title first', async () => {
    await open([{ id: 3, title: 'Arrakis', language: '', source: 'manual' }, { id: 0, title: 'Dune', language: 'en', source: 'edition', editionId: 2 }], [], {}, undefined, twoEditions());
    expect(heading()).toBe('Dune');
    expect(container.querySelector('h2').textContent).toBe('Dune');
    expect(otherLines()).toEqual(['Duna', 'Arrakis']); // not "Dune" again
  });

  it('goes by the main title when the edition being read has only the title its file brought', async () => {
    await open([], [], {}, undefined, twoEditions({ titleSet: false, title: 'cloud.epub' }));
    expect(heading()).toBe('Duna');
    expect(otherLines()).toEqual([]);
  });

  it('goes by the title written for the primary edition when nothing was read yet', async () => {
    await open([], [], {}, undefined, {
      editions: [{ id: 1, title: 'Duna (pt)', titleSet: true, language: 'pt', isPrimary: true, files: [{ id: 10, format: 'epub', availability: 'available', url: '/file/10', percentComplete: 0, completed: false }] }],
    });
    expect(heading()).toBe('Duna (pt)');
    expect(otherLines()).toEqual([]); // "Duna (pt)" and "Duna" are the same name
  });

  it('does not repeat a name that is the one it goes by, with other capitals, accents or a note in parentheses', async () => {
    await open([{ id: 3, title: 'DÚNA', language: '', source: 'manual' }, { id: 4, title: 'duna (edição de bolso)', language: 'pt', source: 'manual' }, { id: 5, title: 'Arrakis', language: '', source: 'manual' }]);
    expect(otherLines()).toEqual(['Arrakis']);
  });

  it('shows three of the other names and keeps the rest one press away', async () => {
    const many = ['Um', 'Dois', 'Três', 'Quatro', 'Cinco'].map((title, id) => ({ id: id + 1, title, language: '', source: 'manual' }));
    await open(many);
    expect(otherLines()).toEqual(['Um', 'Dois', 'Três']);
    const more = [...container.querySelectorAll('button')].find((b) => b.textContent === 'Mais 2');
    expect(more.getAttribute('aria-expanded')).toBe('false');
    await act(async () => { more.click(); });
    expect(otherLines()).toEqual(['Um', 'Dois', 'Três', 'Quatro', 'Cinco']);
    const less = [...container.querySelectorAll('button')].find((b) => b.textContent === 'Mostrar menos');
    expect(less.getAttribute('aria-expanded')).toBe('true');
    await act(async () => { less.click(); });
    expect(otherLines()).toEqual(['Um', 'Dois', 'Três']);
  });

  it('has no language mark on a name that has no language', async () => {
    await open([{ id: 1, title: 'Arrakis', language: '', source: 'manual' }, { id: 2, title: 'Dune', language: 'en', source: 'manual' }]);
    const lines = [...container.querySelectorAll('[aria-label="Outros títulos"] p')];
    expect(lines[0].querySelector('span')).toBeNull();
    expect(lines[1].querySelector('span').textContent).toBe('Inglês');
  });

  it('shows three again when another work is opened after the rest of one was shown, even one that was already loaded', async () => {
    const many = (names) => names.map((title, id) => ({ id: id + 1, title, language: '', source: 'manual' }));
    await open(many(['Um', 'Dois', 'Três', 'Quatro', 'Cinco']));
    api.get.mockImplementation(async (url) => {
      if (url === '/works/7') return { data: work(many(['Um', 'Dois', 'Três', 'Quatro', 'Cinco'])) };
      if (url === '/works/8') return { data: { ...work(many(['A', 'B', 'C', 'D', 'E', 'F'])), id: 8, title: 'Outra' } };
      if (url.endsWith('/series')) return { data: { collection: null, next: null } };
      if (url.endsWith('/candidates')) return { data: { data: [] } };
      if (url === '/auth/me') return { data: { role: 'reader' } };
      throw new Error(`unexpected GET ${url}`);
    });
    const show = async (id) => { await act(async () => { useGlobalStore.setState({ sheetWorkId: id }); }); await flush(); await flush(); };
    await show(8); // loaded once, so that it is there at once the next time
    await show(7);
    await act(async () => { [...container.querySelectorAll('button')].find((b) => b.textContent === 'Mais 2').click(); });
    expect(otherLines()).toHaveLength(5);
    await show(8);
    expect(heading()).toBe('Outra');
    expect(otherLines()).toEqual(['A', 'B', 'C']);
  });

  it('has no button for the rest when there are three or fewer', async () => {
    await open(['Um', 'Dois', 'Três'].map((title, id) => ({ id: id + 1, title, language: '', source: 'manual' })));
    expect([...container.querySelectorAll('button')].some((b) => /^Mais \d|Mostrar menos$/.test(b.textContent))).toBe(false);
  });

  it('says nothing when the server says nothing of it', async () => {
    await open(undefined);
    expect(container.querySelector('[aria-label="Outros títulos"]')).toBeNull();
    expect(heading()).toBe('Duna');
  });

  it('says who else is credited besides the authors, by role', async () => {
    await open([], [
      { personId: 1, name: 'Frank Herbert', role: 'author', position: 0 },
      { personId: 2, name: 'Tradutora', role: 'translator', position: 0 },
      { personId: 3, name: 'Narrador', role: 'narrator', position: 0 },
    ]);
    const line = [...container.querySelectorAll('p')].find((p) => p.textContent.startsWith('Tradução:'));
    expect(line.textContent).toBe('Tradução: Tradutora · Narração: Narrador');
  });

  it('says nothing more when only authors are credited', async () => {
    await open([], [{ personId: 1, name: 'Frank Herbert', role: 'author', position: 0 }]);
    expect([...container.querySelectorAll('p')].some((p) => p.textContent.includes('Tradução:') || p.textContent.includes('Narração:'))).toBe(false);
  });
});

describe('WorkSheet: what a comic or manga work is of its series (#187)', () => {
  const seriesLine = () => [...container.querySelectorAll('p')].find((p) => p.textContent.startsWith('One Piece'));

  it('says the unit and the number in the series, and whether it is a manga', async () => {
    await open([], [], { series: 'One Piece', seriesIndex: 27.5, unit: 'chapter', comicKind: 'manga' });
    expect(seriesLine().textContent).toBe('One Piece · Cap. 27,5');
    expect([...container.querySelectorAll('span')].some((s) => s.textContent === 'Mangá')).toBe(true);
  });

  it('says a volume, and a one-shot with no number', async () => {
    await open([], [], { series: 'One Piece', seriesIndex: 3, unit: 'volume', comicKind: 'comic' });
    expect(seriesLine().textContent).toBe('One Piece · Vol. 3');
    expect([...container.querySelectorAll('span')].some((s) => s.textContent === 'Quadrinho')).toBe(true);
    act(() => root.unmount());
    root = createRoot(container);
    await open([], [], { series: 'One Piece', seriesIndex: 0, unit: 'oneshot' });
    expect(seriesLine().textContent).toBe('One Piece · Único');
  });

  it('says a dash for a number that is not there, and not zero', async () => {
    await open([], [], { series: 'One Piece', seriesIndex: 0, unit: 'volume' });
    expect(seriesLine().textContent).toBe('One Piece · Vol. —');
  });

  it('keeps saying "Livro" for a series whose works have no unit', async () => {
    await open([], [], { series: 'One Piece', seriesIndex: 2 });
    expect(seriesLine().textContent).toBe('One Piece · Livro 2');
    expect([...container.querySelectorAll('span')].some((s) => s.textContent === 'Mangá' || s.textContent === 'Quadrinho')).toBe(false);
  });
});

describe('WorkSheet: the name of the series opens the series (#187)', () => {
  const inSeries = { collection: { id: 4, name: 'Scythe' }, next: null };
  const line = () => [...container.querySelectorAll('p')].find((p) => p.textContent.startsWith('Scythe'));
  const link = () => [...container.querySelectorAll('button')].find((b) => b.textContent === 'Scythe');

  it('makes the name a button when the work is in an official collection, with the number after it as before', async () => {
    await open([], [], { series: 'Scythe', seriesIndex: 2 }, inSeries);
    expect(link()).toBeTruthy();
    expect(link().title).toBe('Ver as outras obras da série');
    expect(line().textContent).toBe('Scythe · Livro 2');
  });

  it('opens the collection in the place of the sheet', async () => {
    await open([], [], { series: 'Scythe', seriesIndex: 2 }, inSeries);
    useGlobalStore.setState({ collectionSheetId: null });
    await act(async () => { link().click(); });
    expect(useGlobalStore.getState()).toMatchObject({ collectionSheetId: 4, sheetWorkId: null });
  });

  it('keeps the unit of a series of volumes and chapters after the name', async () => {
    await open([], [], { series: 'Scythe', unit: 'volume', seriesIndex: 3 }, inSeries);
    expect(link()).toBeTruthy();
    expect(line().textContent).toBe('Scythe · Vol. 3');
  });

  it('is only text when the work is in no collection, or the collection is not there to open', async () => {
    await open([], [], { series: 'Scythe', seriesIndex: 2 });
    expect(link()).toBeUndefined();
    expect(line().textContent).toBe('Scythe · Livro 2');
  });

  it('has no line for the series when the work says none', async () => {
    await open([], [], {}, inSeries);
    expect(line()).toBeUndefined();
    expect(link()).toBeUndefined();
  });
});

