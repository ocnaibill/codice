import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import { useGlobalStore } from '../../../store/useGlobalStore';
import { ContinueReading } from './ContinueReading';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

// A work whose primary file is a PDF, read last in its EPUB.
const work = {
  id: 7, title: 'Duna', author: 'Frank Herbert', coverUrl: '/c.jpg', tags: [], isFavorite: false,
  format: 'pdf', readingProgress: '3', percentComplete: 5, fileId: 10,
  continue: { fileId: 11, format: 'epub', language: 'en', position: 'epubcfi(/6/2)', percentComplete: 30, completed: false },
};
const plain = { id: 8, title: 'Outro', author: 'X', coverUrl: '/c.jpg', tags: [], format: 'cbz', readingProgress: '4', percentComplete: 50, fileId: 20, continue: null };

let container;
let root;
const render = async (items) => {
  await act(async () => { root.render(<ContinueReading items={items} isLoading={false} />); });
};
const card = (title) => [...container.querySelectorAll('article')].find((a) => a.textContent.includes(title));

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

describe('ContinueReading', () => {
  it('shows the file read last, not the primary one, and opens exactly that file', async () => {
    await render([work]);
    const c = card('Duna');
    expect(c.textContent).toContain('EPUB · EN'); // the edition, so two EPUBs are told apart
    expect(c.textContent).not.toContain('PDF');
    expect(c.textContent).toContain('30%');
    expect(c.textContent).not.toContain('5%');
    await act(async () => { c.querySelector('button').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 11 });
  });

  it('falls back to the card itself when there is no recorded last file', async () => {
    await render([plain]);
    const c = card('Outro');
    expect(c.textContent).toContain('CBZ');
    expect(c.textContent).toContain('50%');
    expect(c.textContent).toContain('Página 5'); // a comic position counts from 0
    await act(async () => { c.querySelector('button').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 8, activeFileId: null });
  });

  it('links the author of a card to the page of the person, and says the text when it has no authors one by one (#186)', async () => {
    await render([{ ...work, authors: [{ id: 3, name: 'Frank Herbert' }] }, { ...plain, authors: [] }]);
    const linked = card('Duna');
    await act(async () => { [...linked.querySelectorAll('button')].find((b) => b.textContent === 'Frank Herbert').click(); });
    expect(useGlobalStore.getState().personSheetId).toBe(3);
    expect([...card('Outro').querySelectorAll('button')].some((b) => b.textContent === 'X')).toBe(false);
    expect(card('Outro').textContent).toContain('X');
  });
});

describe('ContinueReading: the title of the edition being read (#185)', () => {
  const english = { ...work, continue: { ...work.continue, title: 'Dune' } };

  it('says the title written for the edition read last, and keeps the main title otherwise', async () => {
    await render([english, plain, { ...work, id: 9, title: 'Sem título escrito', continue: { ...work.continue, title: '' } }]);
    const titles = [...container.querySelectorAll('article h3')].map((h) => h.textContent);
    expect(titles).toEqual(['Dune', 'Outro', 'Sem título escrito']);
  });

  it('still opens the work and the file it was reading', async () => {
    await render([english]);
    await act(async () => { card('Dune').querySelector('button').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 11 });
  });

  it('puts the title in the cover when there is none, so that the placeholder says the name read', async () => {
    await render([{ ...english, coverUrl: '' }]);
    expect(container.querySelector('article [role="img"]').getAttribute('aria-label')).toBe('Dune');
  });
});
