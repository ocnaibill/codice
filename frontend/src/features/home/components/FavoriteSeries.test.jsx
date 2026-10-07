import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import { useGlobalStore } from '../../../store/useGlobalStore';
import { FavoriteSeries, readingLine } from './FavoriteSeries';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const series = {
  kind: 'series', workId: 3, title: 'Harry Potter', author: 'J. K. Rowling', coverUrl: '/c.jpg',
  seriesLabel: 'Harry Potter', seriesTotal: 7, seriesCompleted: 2, favoriteCount: 3,
};
const loose = {
  kind: 'work', workId: 9, title: 'Solo', author: 'Alguém', coverUrl: '/c.jpg',
  seriesLabel: 'Solo', seriesTotal: 1, seriesCompleted: 0, favoriteCount: 1,
};

let container;
let root;
const render = async (items) => {
  await act(async () => { root.render(<FavoriteSeries items={items} total={items.length} isLoading={false} />); });
};
const card = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.includes(text));

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.getState().closeBook();
  useGlobalStore.getState().closeSheet();
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('FavoriteSeries', () => {
  it('shows a series as one card with the name of the series and how much of it was read', async () => {
    await render([series]);
    expect(container.querySelectorAll('button')).toHaveLength(1);
    const c = card('Harry Potter');
    expect(c.textContent).toContain('Harry Potter — J. K. Rowling');
    expect(c.textContent).toContain('Leu 2 de 7 · 3 favoritos');
    expect(readingLine({ ...series, favoriteCount: 2 })).toBe('Leu 2 de 7 · 2 favoritos');
  });

  it('opens the work that stands for the series', async () => {
    await render([series, loose]);
    await act(async () => { card('Harry Potter').click(); });
    expect(useGlobalStore.getState().sheetWorkId).toBe(3);
  });

  it('tells a loose work as read or not, never as "0 de 1"', async () => {
    await render([loose, { ...loose, workId: 10, title: 'Lido', seriesCompleted: 1 }]);
    expect(card('Solo').textContent).toContain('Ainda não lido');
    expect(card('Lido').textContent).toContain('Já lido');
    expect(container.textContent).not.toContain('de 1');
  });

  it('does not count favorites when there is only one', () => {
    expect(readingLine({ ...series, favoriteCount: 1 })).toBe('Leu 2 de 7');
  });

  it('keeps a series and a work with the same id apart', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    await render([series, { ...loose, workId: series.workId }]);
    expect(container.querySelectorAll('button')).toHaveLength(2);
    expect(error).not.toHaveBeenCalled(); // React complains of two items with the same key
    error.mockRestore();
  });
});
