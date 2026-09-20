import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import { useGlobalStore } from '../../../store/useGlobalStore';
import { LibraryGrid } from './LibraryGrid';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const card = (over) => ({
  id: 1, title: 'Obra', author: 'A', coverUrl: '/c.jpg', tags: [], format: 'epub', fileUrl: '/f', fileId: 10,
  fileCount: 1, inProgress: false, continue: null, ...over,
});

let container;
let root;
const render = async (items) => { await act(async () => { root.render(<LibraryGrid items={items} isLoading={false} />); }); };
const readButton = () => container.querySelector('article button[title]:not([title="Ver edições e arquivos"])');
const state = () => useGlobalStore.getState();

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.getState().closeBook();
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
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
