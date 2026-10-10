import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn() },
  authenticatedUrl: (u) => `${u}?rt=token`,   // the images of the library are asked for with a token that is only good for them
}));

import { api } from '../../../lib/api';
import { PersonProfile } from './PersonProfile';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const profile = (over = {}) => ({
  wikidataId: 'Q7934', description: 'escritor de ficção científica americano (1920-1986)', born: '1920-10-08', died: '1986-02-11',
  bio: 'Frank Herbert foi um escritor americano.', bioSource: { language: 'pt', title: 'Frank Herbert', url: 'https://pt.wikipedia.org/wiki/Frank_Herbert', license: 'CC BY-SA 4.0' },
  image: { url: '/covers/person_Q7934.jpg', credit: 'Unknown photographer', license: 'Public domain', pageUrl: 'https://commons.wikimedia.org/wiki/File:A.jpg' },
  ...over,
});
const person = (p) => ({ id: 9, displayName: 'Herbert, Frank', profile: p });

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const click = async (el) => { await act(async () => { el.click(); }); await flush(); };
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);

async function show(p, role = 'reader') {
  api.get.mockImplementation(async (url) => {
    if (url === '/auth/me') return { data: { id: 'u', role } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: {} });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><PersonProfile person={person(p)} /></QueryClientProvider>); });
  await flush();
  await flush();
}

beforeEach(() => {
  vi.clearAllMocks();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('PersonProfile: what Wikidata and Wikipedia say about an author (DEC-146)', () => {
  it('says nothing when nothing was read', async () => {
    await show(null);
    expect(container.textContent).toBe('');
  });

  it('shows the description, the years, the biography and the photo, each with where it came from', async () => {
    await show(profile());
    const text = container.textContent;
    expect(text).toContain('escritor de ficção científica americano (1920-1986)');
    expect(text).toContain('Nasceu em 8 de outubro de 1920 · Morreu em 11 de fevereiro de 1986');
    expect(text).toContain('Frank Herbert foi um escritor americano.');
    const source = [...container.querySelectorAll('a')].find((a) => a.textContent === 'Wikipédia (pt)');
    expect(source.href).toBe('https://pt.wikipedia.org/wiki/Frank_Herbert');
    expect(source.rel).toContain('noopener');
    expect(text).toContain('Fonte: Wikipédia (pt), CC BY-SA 4.0');
    const img = container.querySelector('img');
    expect(img.getAttribute('src')).toBe('/covers/person_Q7934.jpg?rt=token');
    expect(img.alt).toBe('Foto de Herbert, Frank');
    expect(container.querySelector('figcaption').textContent).toBe('Foto: Unknown photographer · Public domain · Wikimedia Commons');
    expect([...container.querySelectorAll('a')].find((a) => a.textContent === 'Wikimedia Commons').href).toBe('https://commons.wikimedia.org/wiki/File:A.jpg');
  });

  it('says only what there is: no photo, no biography, one of the years', async () => {
    await show(profile({ image: undefined, bio: undefined, bioSource: undefined, died: undefined, description: '' }));
    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toContain('Nasceu em 8 de outubro de 1920');
    expect(container.textContent).not.toContain('Morreu');
    expect(container.textContent).not.toContain('Fonte:');
    await show(profile({ born: undefined, died: '-0384' }));
    expect(container.textContent).toContain('Morreu em 384 a.C.');
    expect(container.textContent).not.toContain('Nasceu');
  });

  it('says where they were born, with the date or alone', async () => {
    await show(profile({ bornPlace: 'Tacoma, Washington' }));
    expect(container.textContent).toContain('Nasceu em 8 de outubro de 1920, em Tacoma, Washington · Morreu em 11 de fevereiro de 1986');
    await show(profile({ born: undefined, bornPlace: 'Tacoma', died: undefined }));
    expect(container.textContent).toContain('Nasceu em Tacoma');
    expect(container.textContent).not.toContain('Morreu');
    await show(profile({ bornPlace: undefined }));
    expect(container.textContent).not.toContain(', em ');
  });

  it('says a photo without credit or license by what it has', async () => {
    await show(profile({ image: { url: '/covers/x.jpg' } }));
    expect(container.querySelector('figcaption').textContent).toBe('Foto');
  });

  it('gives a reader no way to hide it', async () => {
    await show(profile(), 'reader');
    expect(button('Ocultar o perfil')).toBeUndefined();
    expect(button('Ocultar a foto')).toBeUndefined();
  });

  it('lets staff hide the profile, or only the photo, and says it when it is hidden', async () => {
    for (const role of ['admin', 'owner']) {
      await show(profile(), role);
      await click(button('Ocultar o perfil'));
      expect(api.put).toHaveBeenLastCalledWith('/admin/people/9/profile', { hidden: true });
      await click(button('Ocultar a foto'));
      expect(api.put).toHaveBeenLastCalledWith('/admin/people/9/profile', { imageHidden: true });
    }
    await show(profile({ hidden: true, imageHidden: true }), 'admin');
    expect(container.textContent).toContain('O perfil está oculto: só quem administra o vê.');
    await click(button('Mostrar o perfil'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/people/9/profile', { hidden: false });
    await click(button('Mostrar a foto'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/people/9/profile', { imageHidden: false });
  });

  it('asks for the page of the person again after a choice, so that it shows what was chosen', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    api.get.mockImplementation(async () => ({ data: { id: 'u', role: 'admin' } }));
    api.put.mockResolvedValue({ data: {} });
    await act(async () => { root.render(<QueryClientProvider client={client}><PersonProfile person={person(profile())} /></QueryClientProvider>); });
    await flush();
    await flush();
    await click(button('Ocultar o perfil'));
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['person', 9] });
  });

  it('has no button for the photo of a person that has none', async () => {
    await show(profile({ image: undefined }), 'admin');
    expect(button('Ocultar o perfil')).toBeDefined();
    expect(button('Ocultar a foto')).toBeUndefined();
  });

  it('says when the choice could not be saved', async () => {
    await show(profile(), 'admin');
    api.put.mockRejectedValue(new Error('boom'));
    await click(button('Ocultar o perfil'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('Não foi possível salvar a escolha.');
  });
});
