import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import { useGlobalStore } from '../../../store/useGlobalStore';
import { FavoriteSeries, readingLine } from './FavoriteSeries';

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
});
