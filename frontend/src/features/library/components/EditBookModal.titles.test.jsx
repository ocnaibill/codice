import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axe from 'axe-core';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { EditBookModal } from './EditBookModal';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const record = (alternativeTitles, editions) => ({
  id: 7, title: 'Duna', author: 'Frank Herbert', tags: [], format: 'epub', editions,
  metadata: { firstAuthor: 'Frank Herbert', locks: {}, sources: {}, alternativeTitles },
});
// An edition has the title its file brought until someone writes one.
const fromFile = { id: 11, title: 'Duna - Frank Herbert.epub', titleSet: false, language: 'pt', isPrimary: true, files: [{ id: 21, format: 'epub' }] };
const written = { id: 12, title: 'Dune Messiah', titleSet: true, language: 'en', isPrimary: false, files: [{ id: 22, format: 'pdf' }, { id: 23, format: 'pdf' }] };
const kept = { id: 3, title: 'Arrakis', language: '', source: 'manual' };
const english = { id: 4, title: 'Dune', language: 'en', source: 'openlibrary' };
const edition = { id: 0, title: 'Dune Messiah', language: 'en', source: 'edition', editionId: 12 };

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const click = async (el) => { await act(async () => { el.click(); }); await flush(); };
const setValue = (el, value) => act(async () => {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, value);
  el.dispatchEvent(new Event('input', { bubbles: true }));
});
const field = (label) => [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith(label)).querySelector('input');

async function open(titles = [kept, english, edition], editions = [fromFile, written]) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: record(titles, editions) };
    if (url === '/works/7/candidates') return { data: { data: [] } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: {} });
  api.delete.mockResolvedValue({});
  api.patch.mockResolvedValue({ data: {} });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><EditBookModal workId={7} tab="titles" onClose={vi.fn()} /></QueryClientProvider>); });
  await flush();
  await flush();
}

beforeEach(() => {
  vi.clearAllMocks();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.setState({ sheetWorkId: 7 });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  useGlobalStore.setState({ sheetWorkId: null });
});

describe('EditBookModal: the titles of a work (#185)', () => {
  it('is a tab of its own, shown the way it was asked for', async () => {
    await open();
    const tabs = [...container.querySelectorAll('[role="tab"]')].map((t) => t.textContent);
    expect(tabs).toEqual(['Sugestões', 'Editar', 'Títulos e autores']);
    expect(container.querySelector('[role="tab"][aria-selected="true"]').textContent).toBe('Títulos e autores');
  });

  it('shows the main title, and each other name with its language and where it came from', async () => {
    await open();
    expect(container.textContent).toContain('Título principal');
    const items = [...container.querySelectorAll('section[aria-label="Títulos alternativos"] li')].map((li) => li.textContent);
    // The title of an edition is written in the list of the editions, not in this one.
    expect(items).toHaveLength(2);
    expect(items[0]).toContain('Arrakis');
    expect(items[0]).toContain('Você');
    expect(items[1]).toContain('Dune');
    expect(items[1]).toContain('Inglês · openlibrary');
    expect(items.join()).not.toContain('Dune Messiah');
  });

  it('lets the ones that were kept be taken away', async () => {
    await open();
    expect(container.querySelector('[aria-label="Tirar o título “Dune Messiah”"]')).toBeNull();
    await click(container.querySelector('[aria-label="Tirar o título “Arrakis”"]'));
    expect(api.delete).toHaveBeenCalledWith('/works/7/titles/3');
    await click(container.querySelector('[aria-label="Tirar o título “Dune”"]'));
    expect(api.delete).toHaveBeenLastCalledWith('/works/7/titles/4');
  });

  it('adds a title with its language, and clears the form', async () => {
    await open([]);
    expect(container.textContent).toContain('A obra ainda não tem outro título.');
    expect(button('Acrescentar').disabled).toBe(true);
    await setValue(field('Outro título'), '  Dune ');
    await setValue(field('Idioma'), ' en ');
    await click(button('Acrescentar'));
    expect(api.post).toHaveBeenCalledWith('/works/7/titles', { title: '  Dune ', language: 'en' });
    expect(field('Outro título').value).toBe('');
    expect(field('Idioma').value).toBe('');
  });

  it('adds a title with no language', async () => {
    await open([]);
    await setValue(field('Outro título'), 'Arrakis');
    await click(button('Acrescentar'));
    expect(api.post).toHaveBeenCalledWith('/works/7/titles', { title: 'Arrakis', language: '' });
  });

  it('says what the server refused, and keeps what was typed', async () => {
    await open([]);
    api.post.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 409, data: 'A obra já tem esse título.' } }));
    await setValue(field('Outro título'), 'Duna');
    await click(button('Acrescentar'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('A obra já tem esse título.');
    expect(field('Outro título').value).toBe('Duna');
    api.delete.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 404, data: 'Title not found' } }));
  });

  it('says what the server refused when taking one away', async () => {
    await open();
    api.delete.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 404, data: 'Title not found' } }));
    await click(container.querySelector('[aria-label="Tirar o título “Arrakis”"]'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('Esse título não está mais na obra.');
  });

  it('waits for the answer: nothing can be typed or pressed while a title is being added', async () => {
    await open([kept]);
    api.post.mockReturnValue(new Promise(() => {}));
    await setValue(field('Outro título'), 'Dune');
    await click(button('Acrescentar'));
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });
    expect(field('Outro título').disabled).toBe(true);
    expect(field('Idioma').disabled).toBe(true);
    expect(button('Acrescentar').disabled).toBe(true);
    expect(container.querySelector('[aria-label="Tirar o título “Arrakis”"]').disabled).toBe(true);
  });

  it('does not send a title that is only spaces, not even with the form', async () => {
    await open([]);
    await setValue(field('Outro título'), '   ');
    await act(async () => { container.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    expect(api.post).not.toHaveBeenCalled();
  });

  it('has no accessibility violation (axe), with the titles and with the form', async () => {
    await open();
    const result = await axe.run(document.body, {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
      rules: { 'color-contrast': { enabled: false }, region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
    });
    expect(result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.html.slice(0, 90)).join(' | ')}`)).toEqual([]);
  });
});

describe('EditBookModal: the title of each edition (#185)', () => {
  const section = () => container.querySelector('section[aria-label="Títulos das edições"]');
  const rows = () => [...section().querySelectorAll('li')];
  const edit = (title) => container.querySelector(`[aria-label="Editar o título da edição “${title}”"]`);
  const draft = () => section().querySelector('input');

  it('lists every edition with its title, language, formats and whether anyone wrote the title', async () => {
    await open();
    expect(rows()).toHaveLength(2);
    expect(rows()[0].textContent).toContain('Duna - Frank Herbert.epub');
    expect(rows()[0].textContent).toContain('Português · EPUB · veio do arquivo');
    expect(rows()[1].textContent).toContain('Dune Messiah');
    expect(rows()[1].textContent).toContain('Inglês · PDF · escrito por você'); // two files of the same format are said once
  });

  it('has an edit button for each edition, including the one whose title is the main title', async () => {
    await open([], [{ ...fromFile, title: 'Duna' }, written]);
    expect(edit('Duna')).not.toBeNull();
    expect(edit('Dune Messiah')).not.toBeNull();
  });

  it('writes the title of an edition: the form starts with the title it has, and sends it to that edition', async () => {
    await open();
    await click(edit('Duna - Frank Herbert.epub'));
    expect(draft().value).toBe('Duna - Frank Herbert.epub');
    expect(draft().maxLength).toBe(255);
    await setValue(draft(), '  Duna  ');
    await click(button('Guardar'));
    expect(api.patch).toHaveBeenCalledWith('/works/7/editions/11', { title: '  Duna  ' });
    expect(section().querySelector('input')).toBeNull(); // the form is gone
  });

  it('leaves the title as it is on Cancelar, and sends nothing', async () => {
    await open();
    await click(edit('Dune Messiah'));
    await setValue(draft(), 'Outro');
    await click(button('Cancelar'));
    expect(api.patch).not.toHaveBeenCalled();
    expect(rows()[1].textContent).toContain('Dune Messiah');
    expect(section().querySelector('input')).toBeNull();
  });

  it('does not send an empty title, not even with the form', async () => {
    await open();
    await click(edit('Dune Messiah'));
    await setValue(draft(), '   ');
    expect(button('Guardar').disabled).toBe(true);
    await act(async () => { section().querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    expect(api.patch).not.toHaveBeenCalled();
  });

  it('says what the server refused, and keeps the form open with what was typed', async () => {
    await open();
    api.patch.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 400, data: 'O título da edição é obrigatório e tem até 255 caracteres.' } }));
    await click(edit('Dune Messiah'));
    await setValue(draft(), 'Novo');
    await click(button('Guardar'));
    expect(section().querySelector('[role="alert"]').textContent).toBe('O título da edição é obrigatório e tem até 255 caracteres.');
    expect(draft().value).toBe('Novo');
  });

  it('says in Portuguese when the edition is no longer there', async () => {
    await open();
    api.patch.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 404, data: 'Edition not found' } }));
    await click(edit('Dune Messiah'));
    await click(button('Guardar'));
    expect(section().querySelector('[role="alert"]').textContent).toBe('Essa edição não está mais na obra.');
  });

  it('waits for the answer: nothing can be typed or pressed while a title is being written', async () => {
    await open();
    api.patch.mockReturnValue(new Promise(() => {}));
    await click(edit('Dune Messiah'));
    await setValue(draft(), 'Novo');
    await click(button('Guardar'));
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });
    expect(draft().disabled).toBe(true);
    expect(button('Guardar').disabled).toBe(true);
    expect(button('Cancelar').disabled).toBe(true);
    expect(edit('Duna - Frank Herbert.epub').disabled).toBe(true);
  });

  it('has no section when there is no edition to list', async () => {
    await open([kept], []);
    expect(section()).toBeNull();
    await open([kept], undefined);
    expect(container.textContent).toContain('Título principal');
  });

  it('has no accessibility violation (axe), with the form of an edition open', async () => {
    await open();
    await click(edit('Dune Messiah'));
    const result = await axe.run(document.body, {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
      rules: { 'color-contrast': { enabled: false }, region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
    });
    expect(result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.html.slice(0, 90)).join(' | ')}`)).toEqual([]);
  });
});
