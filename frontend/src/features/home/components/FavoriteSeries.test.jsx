import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import { useGlobalStore } from '../../../store/useGlobalStore';
import { FavoriteSeries, readingLine, SHOWN } from './FavoriteSeries';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const collection = {
  kind: 'collection', collectionId: 5, collectionKind: 'official', title: 'Harry Potter', author: '', coverUrl: '/c.jpg',
  workCount: 7, completedCount: 2,
};
const list = { ...collection, collectionId: 6, collectionKind: 'personal', title: 'Para ler', workCount: 1, completedCount: 0 };
const loose = { kind: 'work', workId: 9, title: 'Solo', author: 'Alguém', coverUrl: '/c.jpg', workCount: 0, completedCount: 0, completed: false };

let container;
let root;
const render = async (items, extra = {}) => {
  await act(async () => { root.render(<FavoriteSeries items={items} total={items.length} isLoading={false} {...extra} />); });
};
const card = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.includes(text));

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.getState().closeBook();
  useGlobalStore.getState().closeSheet();
  useGlobalStore.getState().closeCollection();
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('FavoriteSeries: the favorites of the home', () => {
  it('shows a collection as one card, with its name and how much of it was read', async () => {
    await render([collection]);
    expect(container.querySelectorAll('button')).toHaveLength(1);
    const c = card('Harry Potter');
    expect(c.textContent).toContain('Coleção · 7 obras · Leu 2 de 7');
    expect(c.textContent).not.toContain('—');
  });

  it('says a list is a list', async () => {
    await render([list]);
    expect(card('Para ler').textContent).toContain('Lista · 1 obra');
  });

  it('opens the page of a collection, and the sheet of a work', async () => {
    await render([collection, loose]);
    await act(async () => { card('Harry Potter').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ collectionSheetId: 5, sheetWorkId: null });
    await act(async () => { card('Solo').click(); });
    expect(useGlobalStore.getState().sheetWorkId).toBe(9);
  });

  it('tells a work as read or not, with its author', async () => {
    await render([loose, { ...loose, workId: 10, title: 'Lido', completed: true }]);
    expect(card('Solo').textContent).toContain('Solo — Alguém');
    expect(card('Solo').textContent).toContain('Ainda não lido');
    expect(card('Lido').textContent).toContain('Já lido');
  });

  it('reads the line of each kind', () => {
    expect(readingLine(collection)).toBe('7 obras · Leu 2 de 7');
    expect(readingLine({ ...collection, workCount: 0, completedCount: 0 })).toBe('Nenhuma obra ainda');
    expect(readingLine(loose)).toBe('Ainda não lido');
    expect(readingLine({ ...loose, completed: true })).toBe('Já lido');
  });

  it('keeps a collection and a work with the same number apart', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    await render([collection, { ...loose, workId: collection.collectionId }, list]);
    expect(container.querySelectorAll('button')).toHaveLength(3);
    expect(error).not.toHaveBeenCalled(); // React complains of two items with the same key
    error.mockRestore();
  });

  it('says there are none, and counts', async () => {
    await render([]);
    expect(container.textContent).toContain('Ainda sem favoritos. Marque um livro, uma coleção ou uma lista como favorito');
    await render([collection, loose]);
    expect(container.textContent).toContain('2 no total');
  });

  it('shows placeholders while it loads', async () => {
    await render([], { isLoading: true });
    expect(container.textContent).not.toContain('no total');
    expect(container.querySelectorAll('.animate-pulse')).toHaveLength(2);
  });

  describe('when there are more than the home shows', () => {
    const many = (n) => Array.from({ length: n }, (_, i) => ({ ...loose, workId: 100 + i, title: `Obra ${i + 1}` }));
    const seeAll = () => [...container.querySelectorAll('button')].find((b) => b.textContent.startsWith('Ver todos'));
    const click = async (el) => { await act(async () => { el.click(); }); };

    it('shows four and a way to see all of them, with how many', async () => {
      expect(SHOWN).toBe(4);
      await render(many(15));
      expect([...container.querySelectorAll('p.truncate')].map((p) => p.textContent)).toHaveLength(4);
      expect(container.textContent).toContain('Obra 4 —');
      expect(container.textContent).not.toContain('Obra 5 —');
      expect(seeAll().textContent).toContain('Ver todos (15)');
      expect(container.textContent).toContain('15 no total');
    });

    it('has no such button for four, or for fewer', async () => {
      await render(many(4));
      expect(seeAll()).toBeUndefined();
      await render(many(1));
      expect(seeAll()).toBeUndefined();
      await render(many(5));
      expect(seeAll()).toBeDefined();
    });

    it('lists all of them in a window, with a way out', async () => {
      await render(many(15));
      await click(seeAll());
      const dialog = document.body.querySelector('[role="dialog"]');
      expect(dialog.getAttribute('aria-label')).toBe('Todos os favoritos');
      expect(dialog.textContent).toContain('15 no total');
      expect([...dialog.querySelectorAll('p.truncate')]).toHaveLength(15);
      expect(dialog.textContent).toContain('Obra 15 —');
      await click(dialog.querySelector('[aria-label="Fechar"]'));
      expect(document.body.querySelector('[role="dialog"]')).toBeNull();
    });

    it('opens a work from the window and closes it, and a collection too', async () => {
      await render([collection, ...many(6)]);
      await click(seeAll());
      const dialog = document.body.querySelector('[role="dialog"]');
      const row = [...dialog.querySelectorAll('button')].find((b) => b.textContent.includes('Obra 6'));
      await click(row);
      expect(useGlobalStore.getState().sheetWorkId).toBe(105);
      expect(document.body.querySelector('[role="dialog"]')).toBeNull();
      await click(seeAll());
      const again = [...document.body.querySelector('[role="dialog"]').querySelectorAll('button')].find((b) => b.textContent.includes('Harry Potter'));
      await click(again);
      expect(useGlobalStore.getState().collectionSheetId).toBe(5);
    });

    it('closes the window with Escape', async () => {
      await render(many(6));
      await click(seeAll());
      await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
      expect(document.body.querySelector('[role="dialog"]')).toBeNull();
    });
  });
});
