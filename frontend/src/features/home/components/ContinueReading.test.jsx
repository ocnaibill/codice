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

describe('ContinueReading: where the person is, in words (DEC-148)', () => {
  const epub = { ...work, continue: { ...work.continue, chapter: 'Capítulo 3: O Deserto', unitIndex: 1204, unitTotal: 9120 } };
  const pdf = {
    ...work, id: 12, title: 'Manual', continue: { fileId: 30, format: 'pdf', position: '42', percentComplete: 13, completed: false, chapter: 'Parte II', unitIndex: 42, unitTotal: 310 },
  };

  it('says the chapter and, for an EPUB, the position of how many (it has no pages)', async () => {
    await render([epub]);
    const text = card('Duna').textContent;
    expect(text).toContain('Capítulo 3: O Deserto');
    expect(text).toContain('Pos. 1.204 de 9.120');
    expect(text).not.toContain('Página');
  });

  it('says the whole word to who hovers the position of an EPUB', async () => {
    await render([epub]);
    expect([...card('Duna').querySelectorAll('li')].map((li) => li.getAttribute('title'))).toContain('Posição 1.204 de 9.120');
  });

  it('says the page of how many for a PDF and for a comic', async () => {
    const comic = { ...plain, continue: { fileId: 40, format: 'cbz', position: '4', percentComplete: 50, completed: false, unitIndex: 5, unitTotal: 32 } };
    await render([pdf, comic]);
    expect(card('Manual').textContent).toContain('Página 42 de 310');
    expect(card('Manual').textContent).toContain('Parte II');
    expect(card('Outro').textContent).toContain('Página 5 de 32');
  });

  it('keeps what an older save gives: the page of a PDF or a comic, and nothing for the rest', async () => {
    const old = { ...pdf, continue: { fileId: 30, format: 'pdf', position: '42', percentComplete: 13, completed: false } };
    const text = { ...work, id: 13, title: 'Texto', continue: { fileId: 50, format: 'txt', position: '1534', percentComplete: 8, completed: false } };
    await render([old, text, work]);
    expect(card('Manual').textContent).toContain('Página 42');
    expect(card('Manual').textContent).not.toContain(' de ');
    // the position of a plain text is a number of characters, and an EPUB's is a place in its code: neither is a page
    expect(card('Texto').textContent).not.toContain('Página');
    expect(card('Duna').textContent).not.toMatch(/Página|Pos\./);
  });

  it('does not say a place that is past the end of the file', async () => {
    const wrong = { ...pdf, continue: { ...pdf.continue, position: '42', unitIndex: 400, unitTotal: 310 } };
    await render([wrong]);
    expect(card('Manual').textContent).not.toContain('400');
    expect(card('Manual').textContent).toContain('Página 42'); // what the older save says
  });

  it('says a long chapter whole to who reads it, and shortened to who sees it', async () => {
    const long = 'Capítulo 12: ' + 'uma travessia muito longa '.repeat(8);
    await render([{ ...epub, continue: { ...epub.continue, chapter: long } }]);
    const item = card('Duna').querySelector('li[title]');
    expect(item.getAttribute('title')).toBe(long);
    expect(item.querySelector('span:last-child').className).toContain('truncate');
  });

  it('says no chapter for an audio file', async () => {
    const audio = { ...work, id: 14, title: 'Audio', continue: { fileId: 60, format: 'm4b', position: '3600', percentComplete: 20, completed: false, chapter: 'Faixa 2' } };
    await render([audio]);
    expect(card('Audio').textContent).not.toContain('Faixa 2');
  });
});
