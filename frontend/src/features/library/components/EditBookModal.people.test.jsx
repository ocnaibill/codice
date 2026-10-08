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

const p = (personId, name, role, position = 0) => ({ personId, name, role, position });
const sample = [p(1, 'Frank Herbert', 'author', 0), p(2, 'Brian Herbert', 'author', 1), p(3, 'Kevin Anderson', 'author', 2), p(4, 'Tradutora', 'translator'), p(5, 'Narrador', 'narrator')];
const twoTranslators = [p(1, 'Frank Herbert', 'author', 0), p(6, 'Tradutora A', 'translator', 0), p(7, 'Tradutora B', 'translator', 1)];
const record = (contributors) => ({
  id: 7, title: 'Duna', author: 'Frank Herbert, Brian Herbert', tags: [], format: 'epub',
  metadata: { firstAuthor: 'Frank Herbert', locks: {}, sources: {}, alternativeTitles: [], contributors },
});

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const click = async (el) => { await act(async () => { el.click(); }); await flush(); };
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const labelled = (label) => container.querySelector(`[aria-label="${label}"]`);
const setValue = (el, value) => act(async () => {
  const proto = el.tagName === 'SELECT' ? HTMLSelectElement.prototype : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, value);
  el.dispatchEvent(new Event(el.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true }));
});
const field = (label) => [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith(label)).querySelector('input, select');
const section = (label) => container.querySelector(`section[aria-label="${label}"]`);

async function open(contributors = sample) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: record(contributors) };
    if (url === '/works/7/candidates') return { data: { data: [] } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: {} });
  api.put.mockResolvedValue({});
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

describe('EditBookModal: the people credited on a work (#185)', () => {
  it('lists each role that has someone, in order, and the first author is said to be the main one', async () => {
    await open();
    const headings = [...container.querySelectorAll('section[aria-label]')].map((s) => s.getAttribute('aria-label')).filter((l) => ['Autores', 'Tradução', 'Narração', 'Edição', 'Ilustração'].includes(l));
    expect(headings).toEqual(['Autores', 'Tradução', 'Narração']);
    const authors = [...section('Autores').querySelectorAll('li')].map((li) => li.textContent);
    expect(authors[0]).toContain('Frank Herbert');
    expect(authors[0]).toContain('principal');
    expect(authors[1]).not.toContain('principal');
    expect(authors).toHaveLength(3);
    // only the first author is the main one: the first of a translation or a narration is not
    expect(section('Tradução').textContent).not.toContain('principal');
    expect(section('Narração').textContent).not.toContain('principal');
  });

  it('moves a person up and down by sending the people of the role in the new order', async () => {
    await open();
    expect(labelled('Subir Frank Herbert').disabled).toBe(true);
    expect(labelled('Descer Kevin Anderson').disabled).toBe(true);
    await click(labelled('Descer Frank Herbert'));
    expect(api.put).toHaveBeenCalledWith('/works/7/contributors/order', { role: 'author', personIds: [2, 1, 3] });
    await click(labelled('Subir Kevin Anderson'));
    expect(api.put).toHaveBeenLastCalledWith('/works/7/contributors/order', { role: 'author', personIds: [1, 3, 2] });
  });

  it('moves the people of another role by their own role', async () => {
    await open(twoTranslators);
    await click(labelled('Descer Tradutora A'));
    expect(api.put).toHaveBeenCalledWith('/works/7/contributors/order', { role: 'translator', personIds: [7, 6] });
  });

  it('waits while an order is being sent: nothing else can be pressed', async () => {
    await open();
    api.put.mockReturnValue(new Promise(() => {}));
    await click(labelled('Descer Frank Herbert'));
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });
    expect(labelled('Subir Brian Herbert').disabled).toBe(true);
    expect(labelled('Tirar Narrador de narração').disabled).toBe(true);
    expect(button('Creditar').disabled).toBe(true);
  });

  it('waits while a person is being taken away: nothing else can be pressed', async () => {
    await open();
    api.delete.mockReturnValue(new Promise(() => {}));
    await click(labelled('Tirar Narrador de narração'));
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });
    expect(labelled('Descer Frank Herbert').disabled).toBe(true);
    expect(labelled('Tirar Tradutora de tradução').disabled).toBe(true);
  });

  it('has no arrows for a role with one person', async () => {
    await open();
    expect(labelled('Subir Tradutora')).toBeNull();
    expect(labelled('Descer Narrador')).toBeNull();
  });

  it('takes a person away from a role, naming the role', async () => {
    await open();
    await click(labelled('Tirar Brian Herbert de autores'));
    expect(api.delete).toHaveBeenCalledWith('/works/7/contributors/2/author');
    await click(labelled('Tirar Narrador de narração'));
    expect(api.delete).toHaveBeenLastCalledWith('/works/7/contributors/5/narrator');
  });

  it('credits a person with a role, and clears the name', async () => {
    await open();
    expect(button('Creditar').disabled).toBe(true);
    await setValue(field('Nome da pessoa'), '  Ilustra Dora ');
    await setValue(field('Função'), 'illustrator');
    await click(button('Creditar'));
    expect(api.post).toHaveBeenCalledWith('/works/7/contributors', { name: '  Ilustra Dora ', role: 'illustrator' });
    expect(field('Nome da pessoa').value).toBe('');
  });

  it('offers the five roles, author first', async () => {
    await open();
    const options = [...field('Função').options].map((o) => [o.value, o.textContent]);
    expect(options).toEqual([['author', 'Autor'], ['translator', 'Tradutor'], ['narrator', 'Narrador'], ['editor', 'Editor'], ['illustrator', 'Ilustrador']]);
    expect(field('Função').value).toBe('author');
  });

  it('says what the server refused, and keeps what was typed', async () => {
    await open();
    api.post.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 409, data: 'Essa pessoa já tem esse papel na obra.' } }));
    await setValue(field('Nome da pessoa'), 'Brian Herbert');
    await click(button('Creditar'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('Essa pessoa já tem esse papel na obra.');
    expect(field('Nome da pessoa').value).toBe('Brian Herbert');
  });

  it('says what the server refused when taking away and when ordering', async () => {
    await open();
    api.delete.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 404, data: 'Contributor not found' } }));
    await click(labelled('Tirar Tradutora de tradução'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('Essa pessoa não está mais creditada nessa função.');
    api.put.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 400, data: 'A lista deve ter cada pessoa desse papel uma vez, e só elas.' } }));
    await click(labelled('Descer Frank Herbert'));
    expect(container.querySelector('[role="alert"]').textContent).toContain('A lista deve ter cada pessoa desse papel');
  });

  it('does not send a name that is only spaces, not even with the form', async () => {
    await open();
    await setValue(field('Nome da pessoa'), '   ');
    await act(async () => { [...container.querySelectorAll('form')].pop().dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    expect(api.post).not.toHaveBeenCalled();
  });

  it('waits for the answer: nothing can be typed or pressed while a person is being credited', async () => {
    await open();
    api.post.mockReturnValue(new Promise(() => {}));
    await setValue(field('Nome da pessoa'), 'Alguém');
    await click(button('Creditar'));
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });
    expect(field('Nome da pessoa').disabled).toBe(true);
    expect(field('Função').disabled).toBe(true);
    expect(button('Creditar').disabled).toBe(true);
    expect(labelled('Tirar Narrador de narração').disabled).toBe(true);
  });

  it('says nothing is credited yet, and still offers to credit', async () => {
    await open([]);
    expect(section('Autores')).toBeNull();
    expect(button('Creditar')).toBeTruthy();
  });

  it('has no accessibility violation (axe), with the people and the form', async () => {
    await open();
    const result = await axe.run(document.body, {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
      rules: { 'color-contrast': { enabled: false }, region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
    });
    expect(result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.html.slice(0, 90)).join(' | ')}`)).toEqual([]);
  });
});
