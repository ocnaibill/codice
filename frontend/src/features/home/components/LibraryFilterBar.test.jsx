import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { LibraryFilterBar } from './LibraryFilterBar';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let container;
let root;
beforeEach(() => { container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container); });
afterEach(() => { act(() => root.unmount()); container.remove(); });

const render = (props) => act(async () => { root.render(<LibraryFilterBar worksTotal={3} activeFilter="all" onFilterChange={() => {}} viewMode="grid" onViewModeChange={() => {}} {...props} />); });

describe('LibraryFilterBar: sorting', () => {
  it('offers newest first, title and author, with the one in use selected', async () => {
    await render({ sort: 'author', onSortChange: vi.fn() });
    const select = container.querySelector('select[aria-label="Ordenar o acervo"]');
    expect([...select.options].map((o) => [o.value, o.textContent])).toEqual([['added', 'Mais recentes'], ['title', 'Título'], ['author', 'Autor']]);
    expect(select.value).toBe('author');
  });

  it('tells which one was picked', async () => {
    const onSortChange = vi.fn();
    await render({ sort: 'added', onSortChange });
    const select = container.querySelector('select');
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
      setter.call(select, 'title');
      select.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(onSortChange).toHaveBeenCalledWith('title');
  });

  it('has no sort control when nobody listens to it', async () => {
    await render({});
    expect(container.querySelector('select')).toBeNull();
  });
});

describe('LibraryFilterBar: the kinds of work that the library has', () => {
  const labels = () => [...container.querySelectorAll('[aria-label="Filtrar acervo"] button')].map((b) => b.textContent);

  it('counts each kind, and offers only those with at least one work; Todos is always there', async () => {
    await render({ worksTotal: 9, breakdown: { livros: 5, mangas: 0, audio: 4 } });
    expect(labels()).toEqual(['Todos[09]', 'Livros digitais[05]', 'Audiolivros[04]']);
  });

  it('offers only Todos for an empty library, and no kind before the counts are known', async () => {
    await render({ worksTotal: 0, breakdown: { livros: 0, mangas: 0, audio: 0 } });
    expect(labels()).toEqual(['Todos[00]']);
    await render({ worksTotal: undefined, breakdown: undefined });
    expect(labels()).toEqual(['Todos']);
  });

  it('picks a kind, and marks the one on screen', async () => {
    const onFilterChange = vi.fn();
    await render({ worksTotal: 3, breakdown: { livros: 3, mangas: 0, audio: 0 }, activeFilter: 'ebooks', onFilterChange });
    const buttons = [...container.querySelectorAll('[aria-label="Filtrar acervo"] button')];
    expect(buttons.map((b) => b.getAttribute('aria-pressed'))).toEqual(['false', 'true']);
    await act(async () => buttons[0].click());
    expect(onFilterChange).toHaveBeenCalledWith('all');
  });

  it('goes back to all of the works when the kind on screen has none left', async () => {
    const onFilterChange = vi.fn();
    await render({ worksTotal: 3, breakdown: { livros: 3, mangas: 0, audio: 0 }, activeFilter: 'comics', onFilterChange });
    expect(onFilterChange).toHaveBeenCalledWith('all');
  });

  it('does not go back while the counts are unknown, nor from a kind that has works, nor from all', async () => {
    const onFilterChange = vi.fn();
    await render({ worksTotal: undefined, breakdown: undefined, activeFilter: 'comics', onFilterChange });
    await render({ worksTotal: 4, breakdown: { livros: 0, mangas: 4, audio: 0 }, activeFilter: 'comics', onFilterChange });
    await render({ worksTotal: 0, breakdown: { livros: 0, mangas: 0, audio: 0 }, activeFilter: 'all', onFilterChange });
    expect(onFilterChange).not.toHaveBeenCalled();
  });
});

describe('LibraryFilterBar: collections', () => {
  const labels = () => [...container.querySelectorAll('[aria-label="Filtrar acervo"] button')].map((b) => b.textContent);

  it('offers them after the kinds of work, with their count, once there is one', async () => {
    await render({ worksTotal: 9, breakdown: { livros: 5, mangas: 0, audio: 4 }, collectionsTotal: 3 });
    expect(labels()).toEqual(['Todos[09]', 'Livros digitais[05]', 'Audiolivros[04]', 'Coleções[03]']);
  });

  it('does not offer them while there is none, to a reader', async () => {
    await render({ worksTotal: 9, breakdown: { livros: 9, mangas: 0, audio: 0 }, collectionsTotal: 0 });
    expect(labels()).toEqual(['Todos[09]', 'Livros digitais[09]']);
  });

  it('offers them to the staff even with none, so that the first is made', async () => {
    await render({ worksTotal: 9, breakdown: { livros: 9, mangas: 0, audio: 0 }, collectionsTotal: 0, canManageCollections: true });
    expect(labels()).toEqual(['Todos[09]', 'Livros digitais[09]', 'Coleções[00]']);
  });

  it('picks them, and is not sent back to Todos for not being a kind of work', async () => {
    const onFilterChange = vi.fn();
    await render({ worksTotal: 9, breakdown: { livros: 9, mangas: 0, audio: 0 }, collectionsTotal: 2, activeFilter: 'collections', onFilterChange });
    const buttons = [...container.querySelectorAll('[aria-label="Filtrar acervo"] button')];
    expect(buttons.map((b) => b.getAttribute('aria-pressed'))).toEqual(['false', 'false', 'true']);
    expect(onFilterChange).not.toHaveBeenCalled();
    await act(async () => buttons[2].click());
    expect(onFilterChange).toHaveBeenCalledWith('collections');
  });
});

describe('LibraryFilterBar: the lists of the person', () => {
  const labels = () => [...container.querySelectorAll('[aria-label="Filtrar acervo"] button')].map((b) => b.textContent);

  it('offers them last, with their count, once it is known: anybody can keep lists', async () => {
    await render({ worksTotal: 9, breakdown: { livros: 9, mangas: 0, audio: 0 }, collectionsTotal: 2, listsTotal: 4 });
    expect(labels()).toEqual(['Todos[09]', 'Livros digitais[09]', 'Coleções[02]', 'Minhas listas[04]']);
  });

  it('offers them with none, too, so that the first is made', async () => {
    await render({ worksTotal: 9, breakdown: { livros: 9, mangas: 0, audio: 0 }, listsTotal: 0 });
    expect(labels()).toEqual(['Todos[09]', 'Livros digitais[09]', 'Minhas listas[00]']);
  });

  it('does not offer them before their number is known', async () => {
    await render({ worksTotal: 9, breakdown: { livros: 9, mangas: 0, audio: 0 } });
    expect(labels()).toEqual(['Todos[09]', 'Livros digitais[09]']);
  });

  it('picks them, and is not sent back to Todos for not being a kind of work', async () => {
    const onFilterChange = vi.fn();
    await render({ worksTotal: 9, breakdown: { livros: 9, mangas: 0, audio: 0 }, listsTotal: 1, activeFilter: 'lists', onFilterChange });
    const buttons = [...container.querySelectorAll('[aria-label="Filtrar acervo"] button')];
    expect(buttons.map((b) => b.getAttribute('aria-pressed'))).toEqual(['false', 'false', 'true']);
    expect(onFilterChange).not.toHaveBeenCalled();
    await act(async () => buttons[2].click());
    expect(onFilterChange).toHaveBeenCalledWith('lists');
  });
});
