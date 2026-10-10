import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ authenticatedUrl: (u) => u, api: { post: vi.fn(), delete: vi.fn() } }));

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { LibraryGrid } from './LibraryGrid';
import { formatBadge } from '../utils/format';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const card = (over) => ({
  id: 1, title: 'Obra', author: 'A', coverUrl: '/c.jpg', tags: [], format: 'epub', fileUrl: '/f', fileId: 10,
  fileCount: 1, inProgress: false, continue: null, ...over,
});

let container;
let root;
let queryClient;
const render = async (items) => {
  await act(async () => {
    root.render(<QueryClientProvider client={queryClient}><LibraryGrid items={items} isLoading={false} /></QueryClientProvider>);
  });
};
const readButton = () => container.querySelector('article button[title]:not([title="Ver edições e arquivos"])');
const state = () => useGlobalStore.getState();

beforeEach(() => {
  vi.clearAllMocks();
  api.post.mockResolvedValue({});
  api.delete.mockResolvedValue({});
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.getState().closeBook();
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('the cover and the name of a card open the page of the work', () => {
  beforeEach(() => { useGlobalStore.setState({ sheetWorkId: null, collectionSheetId: null }); });

  it('opens it from the cover', async () => {
    await render([card({ id: 5, title: 'Duna' })]);
    await act(async () => { container.querySelector('button.library-book-cover').click(); });
    expect(state()).toMatchObject({ sheetWorkId: 5, activeBookId: null });
  });

  it('opens it from the name, which is a button with the name of the work, and does not open the reader', async () => {
    await render([card({ id: 5, title: 'Duna', inProgress: true, continue: { fileId: 11, format: 'epub', completed: false } })]);
    const name = container.querySelector('h3 button.library-book-title');
    expect(name.textContent).toBe('Duna');
    expect(name.getAttribute('aria-label')).toBe('Abrir a página: Duna');
    await act(async () => { name.click(); });
    expect(state()).toMatchObject({ sheetWorkId: 5, activeBookId: null });
  });

  it('keeps the title attribute of the name out of the way of the Read button, which is the one with a title of its own', async () => {
    await render([card({ id: 5, title: 'Duna' })]);
    expect(container.querySelector('h3 button.library-book-title').hasAttribute('title')).toBe(false);
    expect(readButton().getAttribute('aria-label')).toMatch(/^.*: Duna$/);
  });

  it('opens the collection from the name of a series, as from its cover', async () => {
    const series = { collectionId: 4, name: 'One Piece', volumes: 3, chapters: 0, oneShots: 0, coverUrl: '/s.jpg', continue: null };
    await render([card({ id: 9, title: 'One Piece 3', collapsed: series })]);
    await act(async () => { container.querySelector('article[data-series="true"] h3 button').click(); });
    expect(state()).toMatchObject({ collectionSheetId: 4, sheetWorkId: null });
  });
});

describe('the Read button of a card (DEC-081)', () => {
  it('continues the version that counts when the work is in progress', async () => {
    await render([card({ fileCount: 3, inProgress: true, continue: { fileId: 22, format: 'pdf', completed: false } })]);
    expect(readButton().title).toBe('Continuar de onde parou');
    await act(async () => { readButton().click(); });
    expect(state()).toMatchObject({ activeBookId: 1, activeFileId: 22, sheetWorkId: null });
  });

  it('opens the sheet the first time when there is something to choose', async () => {
    await render([card({ fileCount: 2 })]);
    expect(readButton().title).toBe('Escolher versão para ler');
    await act(async () => { readButton().click(); });
    expect(state()).toMatchObject({ sheetWorkId: 1, activeBookId: null });
  });

  it('opens the only file of a work that has one', async () => {
    await render([card()]);
    expect(readButton().title).toBe('Ler');
    await act(async () => { readButton().click(); });
    expect(state()).toMatchObject({ activeBookId: 1, activeFileId: null });
  });

  it('opens the sheet of a work that was finished, where the count and "read again" are', async () => {
    await render([card({ continue: { fileId: 10, completed: true } })]);
    await act(async () => { readButton().click(); });
    expect(state()).toMatchObject({ sheetWorkId: 1, activeBookId: null });
  });
});

describe('the corner of a cover says the format, or how many formats there are', () => {
  it('names the format of a work that has one', () => {
    expect(formatBadge(card({ format: 'epub', formatCount: 1 }))).toBe('EPUB');
    expect(formatBadge(card({ format: 'cbz' }))).toBe('CBZ');
  });
  it('says how many when there is more than one', () => {
    expect(formatBadge(card({ format: 'epub', formatCount: 2 }))).toBe('2 formatos');
    expect(formatBadge(card({ format: 'pdf', formatCount: 3 }))).toBe('3 formatos');
  });
  it('shows it on the card', async () => {
    await render([card({ format: 'epub', formatCount: 2 }), card({ id: 2, format: 'cbz', formatCount: 1 })]);
    const corners = [...container.querySelectorAll('article .library-book-cover span')].map((s) => s.textContent);
    expect(corners).toEqual(['2 formatos', 'CBZ']);
  });
});

describe('the heart of a card (#179)', () => {
  const heart = () => container.querySelector('button.library-favorite');
  const settle = () => act(async () => { await Promise.resolve(); });

  it('is there for every card, and says what it does by the title of the work', async () => {
    await render([card({ id: 1, title: 'Duna' }), card({ id: 2, title: 'Neuromancer', isFavorite: true })]);
    const hearts = [...container.querySelectorAll('button.library-favorite')];
    expect(hearts.map((h) => h.getAttribute('aria-label'))).toEqual([
      'Adicionar aos favoritos: Duna', 'Remover dos favoritos: Neuromancer']);
    expect(hearts.map((h) => h.getAttribute('aria-pressed'))).toEqual(['false', 'true']);
  });

  it('favorites a work without opening it', async () => {
    await render([card({ id: 7 })]);
    await act(async () => { heart().click(); });
    await settle();
    expect(api.post).toHaveBeenCalledWith('/works/7/favorite');
    expect(api.delete).not.toHaveBeenCalled();
    expect(state()).toMatchObject({ activeBookId: null, sheetWorkId: null });
  });

  it('takes a work off the favorites', async () => {
    await render([card({ id: 7, isFavorite: true })]);
    await act(async () => { heart().click(); });
    await settle();
    expect(api.delete).toHaveBeenCalledWith('/works/7/favorite');
    expect(api.post).not.toHaveBeenCalled();
  });

  it('asks the library to be read again once it is done, so that the heart changes', async () => {
    const spy = vi.spyOn(queryClient, 'invalidateQueries');
    await render([card({ id: 7 })]);
    await act(async () => { heart().click(); });
    await settle();
    expect(spy.mock.calls.map((c) => c[0].queryKey[0])).toEqual(expect.arrayContaining(['works', 'favorites']));
  });

  it('is a button of its own, not a part of the one that reads', async () => {
    await render([card({ id: 7 })]);
    expect(heart().closest('.library-book-actions')).not.toBeNull();
    expect(heart().type).toBe('button');
    expect(readButton()).not.toBe(heart());
  });

  it('shows the author of a card as a link to the page of the person, and the text when the card has no authors one by one (#186)', async () => {
    await render([
      card({ id: 1, title: 'Duna', author: 'Frank Herbert, Brian Herbert', authors: [{ id: 3, name: 'Frank Herbert' }, { id: 4, name: 'Brian Herbert' }] }),
      card({ id: 2, title: 'Outra', author: 'Sem ids', authors: [] }),
    ]);
    const first = container.querySelectorAll('article')[0].querySelector('.library-author');
    expect([...first.querySelectorAll('button')].map((b) => b.textContent)).toEqual(['Frank Herbert', 'Brian Herbert']);
    await act(async () => { first.querySelectorAll('button')[1].click(); });
    expect(state().personSheetId).toBe(4);
    const second = container.querySelectorAll('article')[1].querySelector('.library-author');
    expect(second.textContent).toBe('Sem ids');
    expect(second.querySelector('button')).toBeNull();
  });
});

describe('a series in the grid (#187)', () => {
  const series = (over = {}) => ({
    collectionId: 4, name: 'One Piece', volumes: 3, chapters: 121, oneShots: 0, coverUrl: '/s.jpg',
    continue: { id: 9, title: 'One Piece 28', unit: 'chapter', position: 28, started: false, begun: true }, ...over,
  });
  const seriesCard = (over = {}, extra = {}) => card({ id: 9, title: 'One Piece 121', coverUrl: '/s.jpg', collapsed: series(over), ...extra });
  const article = () => container.querySelector('article[data-series="true"]');
  const text = () => article().textContent;

  it('is one card with the name of the series, what it has by unit, and a mark that it is a series', async () => {
    await render([seriesCard()]);
    expect(article().querySelector('h3').textContent).toBe('One Piece');
    expect(text()).toContain('3 volumes · 121 capítulos');
    expect(text()).toContain('SÉRIE');
    expect(article().querySelector('img').getAttribute('alt')).toBe('One Piece');
  });

  it('says how many are new, in the singular too, and says nothing when none is', async () => {
    await render([seriesCard({ newCount: 3 })]);
    const badge = () => [...article().querySelectorAll('span')].find((e) => /novo/.test(e.textContent));
    expect(badge().textContent).toBe('3 novos');
    expect(badge().title).toBe('Chegaram na última semana e você ainda não terminou');
    await render([seriesCard({ newCount: 1 })]);
    expect(badge().textContent).toBe('1 novo');
    await render([seriesCard({ newCount: 0 })]);
    expect(badge()).toBeUndefined();
    await render([seriesCard({ newCount: undefined })]);
    expect(badge()).toBeUndefined();
  });

  it('opens the collection from the cover, and does not open the sheet of the work that carries it', async () => {
    await render([seriesCard()]);
    useGlobalStore.setState({ collectionSheetId: null, sheetWorkId: null });
    await act(async () => { article().querySelector('[aria-label="Abrir a série One Piece"]').click(); });
    expect(state().collectionSheetId).toBe(4);
    expect(state().sheetWorkId).toBeNull();
  });

  it('goes on with the series from the button, in the reader', async () => {
    await render([seriesCard()]);
    const go = article().querySelector('.library-series-go');
    expect(go.textContent).toBe('Próximo: Cap. 28');
    await act(async () => { go.click(); });
    expect(state().activeBookId).toBe(9);
    expect(state().collectionSheetId).toBeNull();
  });

  it('says it all was read when there is nothing to go on with, and has no button', async () => {
    await render([seriesCard({ continue: null })]);
    expect(article().querySelector('.library-series-go')).toBeNull();
    expect(text()).toContain('Tudo lido');
  });

  it('leaves a card of a work as it was, next to a series', async () => {
    await render([card({ id: 1, title: 'Duna' }), seriesCard()]);
    expect(container.querySelectorAll('article')).toHaveLength(2);
    expect(container.querySelectorAll('article[data-series="true"]')).toHaveLength(1);
    expect(container.querySelector('article:not([data-series]) h3').textContent).toBe('Duna');
  });

  it('counts items, not works, when told to', async () => {
    await act(async () => {
      root.render(<QueryClientProvider client={queryClient}><LibraryGrid items={[seriesCard()]} total={5} countWord="itens" /></QueryClientProvider>);
    });
    expect(container.textContent).toContain('[ 5 itens ]');
  });

  it('says one in the singular: "[ 1 obra ]" and "[ 1 item ]"', async () => {
    await act(async () => {
      root.render(<QueryClientProvider client={queryClient}><LibraryGrid items={[card({ id: 1, title: 'Duna' })]} total={1} /></QueryClientProvider>);
    });
    expect(container.textContent).toContain('[ 1 obra ]');
    expect(container.textContent).not.toContain('1 obras');
    await act(async () => {
      root.render(<QueryClientProvider client={queryClient}><LibraryGrid items={[seriesCard()]} total={1} countWord="itens" /></QueryClientProvider>);
    });
    expect(container.textContent).toContain('[ 1 item ]');
  });
});

describe('the title of the edition being read on a card (#185)', () => {
  const reading = (over = {}) => card({ id: 3, title: 'A Nuvem 2', continue: { fileId: 22, format: 'epub', title: 'The Cloud 2', completed: false }, inProgress: true, ...over });
  const heading = () => container.querySelector('article h3');

  it('says the title written for that edition, in the heading, its hint and the names of the buttons', async () => {
    await render([reading()]);
    expect(heading().textContent).toBe('The Cloud 2');
    expect(heading().title).toBe('The Cloud 2');
    expect(container.querySelector('[aria-label^="Continuar"]').getAttribute('aria-label')).toContain('The Cloud 2');
    expect(container.querySelector('[aria-label^="Baixar"]').getAttribute('aria-label')).toBe('Baixar: The Cloud 2');
    expect(container.querySelector('[aria-label^="Adicionar aos favoritos"]').getAttribute('aria-label')).toContain('A Nuvem 2'); // the heart is of the work
  });

  it('keeps the main title when nobody wrote one for the edition, or nothing was read', async () => {
    await render([reading({ continue: { fileId: 22, format: 'epub', completed: false } }), card({ id: 4, title: 'Duna', continue: null })]);
    expect([...container.querySelectorAll('article h3')].map((h) => h.textContent)).toEqual(['A Nuvem 2', 'Duna']);
  });

  it('puts the title in the cover when there is none', async () => {
    await render([reading({ coverUrl: '' })]);
    expect(container.querySelector('article [role="img"]').getAttribute('aria-label')).toBe('The Cloud 2');
  });
});
