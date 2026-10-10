import React, { act } from 'react';
import axe from 'axe-core';
import { describe, it, expect, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { mount, flush } from '../../admin/testUtils';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { CollectionPage } from './CollectionPage';
import { CollectionsGrid } from './CollectionsGrid';

// What axe finds on the screens of the collections (the same rules as the other screens: names of the controls, roles, lists).
async function audit(element) {
  const result = await axe.run(element, {
    runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
    rules: { 'color-contrast': { enabled: false }, region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
  });
  return result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.html.slice(0, 90)).join(' | ')}`);
}

const works = [
  { entryId: 10, id: 1, title: 'Pedra Filosofal', author: 'J. K. Rowling', coverUrl: '/c/1.jpg', position: 1, completed: true, available: true },
  { entryId: 20, id: 2, title: 'Câmara Secreta', author: 'J. K. Rowling', coverUrl: '/c/2.jpg', position: 2, completed: false, available: true },
];
let view;
afterEach(() => {
  view?.unmount();
  useGlobalStore.setState({ collectionSheetId: null });
});
const serve = () => api.get.mockImplementation(async (url) => {
  if (url === '/auth/me') return { data: { role: 'admin' } };
  if (url === '/collections/5') return { data: { collection: { id: 5, kind: 'official', name: 'Harry Potter', workCount: 2, completedCount: 1, coverUrl: '/c/1.jpg' }, works } };
  if (url === '/collections') return { data: { data: [{ id: 5, kind: 'official', name: 'Harry Potter', workCount: 2, completedCount: 1, coverUrl: '/c/1.jpg' }], total: 1, totalPages: 1 } };
  if (url === '/works') return { data: { data: [] } };
  throw new Error(`unexpected GET ${url}`);
});

describe('what axe finds on the collections', () => {
  it('on the page of a collection, for the staff, with the rename form, the question of taking one out and the search', async () => {
    serve();
    useGlobalStore.setState({ collectionSheetId: 5 });
    view = await mount(<CollectionPage />);
    await flush();
    expect(await audit(document.body)).toEqual([]);
    await view.click(view.button('Renomear'));
    expect(await audit(document.body)).toEqual([]);
    await view.click(view.button('Renomear'));
    await view.click(view.button('Acrescentar obra'));
    expect(await audit(document.body)).toEqual([]);
    await view.click(view.button('Acrescentar obra'));
    await view.click(document.body.querySelector('[aria-label="Tirar “Câmara Secreta” da coleção"]'));
    expect(await audit(document.body)).toEqual([]);
    await view.click(view.button('Cancelar'));
    await view.click(view.button('Aposentar'));
    expect(await audit(document.body)).toEqual([]);
  });

  it('on the grid of collections, for the staff, with the form of a new one', async () => {
    serve();
    view = await mount(<CollectionsGrid />);
    await flush();
    expect(await audit(document.body)).toEqual([]);
    await view.click(view.button('Nova coleção'));
    expect(await audit(document.body)).toEqual([]);
  });

  it('on the page of a list of the person, with a work that left the library, the forms and the questions', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/auth/me') return { data: { role: 'reader' } };
      if (url === '/collections/7') {
        return { data: { collection: { id: 7, kind: 'personal', name: 'Para ler', workCount: 1, completedCount: 0, coverUrl: '/c/1.jpg' }, works: [
          { entryId: 10, id: 1, title: 'Duna', author: 'Frank Herbert', coverUrl: '/c/1.jpg', position: 1, completed: false, available: true },
          { entryId: 20, id: 0, title: 'Obra que saiu', author: 'x', coverUrl: '/covers/placeholder.svg', position: 2, completed: false, available: false },
        ] } };
      }
      if (url === '/works') return { data: { data: [] } };
      throw new Error(`unexpected GET ${url}`);
    });
    useGlobalStore.setState({ collectionSheetId: 7 });
    view = await mount(<CollectionPage />);
    await flush();
    expect(await audit(document.body)).toEqual([]);
    await view.click(view.button('Renomear'));
    expect(await audit(document.body)).toEqual([]);
    await view.click(view.button('Renomear'));
    await view.click(document.body.querySelector('[aria-label="Tirar “Obra que saiu” da lista"]'));
    expect(await audit(document.body)).toEqual([]);
    await view.click(view.button('Cancelar'));
    await view.click(view.button('Aposentar'));
    expect(await audit(document.body)).toEqual([]);
  });

  it('on the grid of the lists of the person, with the form of a new one', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/auth/me') return { data: { role: 'reader' } };
      return { data: { data: [{ id: 3, kind: 'personal', name: 'Para ler', workCount: 1, completedCount: 0, coverUrl: '/c.jpg' }], total: 1, totalPages: 1 } };
    });
    view = await mount(<CollectionsGrid kind="personal" />);
    await flush();
    expect(await audit(document.body)).toEqual([]);
    await view.click(view.button('Nova lista'));
    expect(await audit(document.body)).toEqual([]);
  });
});
