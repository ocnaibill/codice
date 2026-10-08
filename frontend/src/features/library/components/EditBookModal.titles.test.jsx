import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axe from 'axe-core';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { EditBookModal } from './EditBookModal';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const record = (alternativeTitles) => ({
  id: 7, title: 'Duna', author: 'Frank Herbert', tags: [], format: 'epub',
  metadata: { firstAuthor: 'Frank Herbert', locks: {}, sources: {}, alternativeTitles },
});
const kept = { id: 3, title: 'Arrakis', language: '', source: 'manual' };
const english = { id: 4, title: 'Dune', language: 'en', source: 'openlibrary' };
const edition = { id: 0, title: 'Dune Messiah', language: 'en', source: 'edition' };

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

async function open(titles = [kept, english, edition]) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: record(titles) };
    if (url === '/works/7/candidates') return { data: { data: [] } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: {} });
  api.delete.mockResolvedValue({});
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
    expect(items).toHaveLength(3);
    expect(items[0]).toContain('Arrakis');
    expect(items[0]).toContain('Você');
    expect(items[1]).toContain('Dune');
    expect(items[1]).toContain('Inglês · openlibrary');
    expect(items[2]).toContain('Dune Messiah');
    expect(items[2]).toContain('Inglês · Edição');
  });

  it('lets the ones that were kept be taken away, and not the title of an edition', async () => {
    await open();
    expect(container.querySelector('[aria-label="Tirar o título “Dune Messiah”"]')).toBeNull();
    expect(container.textContent).toContain('da edição');
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
