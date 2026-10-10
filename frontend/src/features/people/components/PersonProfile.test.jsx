import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), delete: vi.fn() },
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

let searchReply = null; // what GET .../profile/search answers: null is "none was made" (404), or a function giving the answer
async function show(p, role = 'reader') {
  api.get.mockImplementation(async (url) => {
    if (url === '/auth/me') return { data: { id: 'u', role } };
    if (url === '/admin/people/9/profile/search') {
      const answer = typeof searchReply === 'function' ? searchReply() : searchReply;
      if (!answer) throw Object.assign(new Error('x'), { response: { status: 404 } });
      return { data: answer };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: {} });
  api.post.mockResolvedValue({ data: {} });
  api.delete.mockResolvedValue({});
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><PersonProfile person={person(p)} /></QueryClientProvider>); });
  await flush();
  await flush();
}

beforeEach(() => {
  vi.clearAllMocks();
  searchReply = null;
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('PersonProfile: what Wikidata and Wikipedia say about an author (DEC-146)', () => {
  it('says nothing to a reader when nothing was read', async () => {
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

describe('PersonProfile: written by hand, by owner and admin (DEC-167)', () => {
  const typeInto = async (el, value) => {
    const proto = el.tagName === 'TEXTAREA' ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
    await act(async () => {
      Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, value);
      el.dispatchEvent(new Event('input', { bubbles: true }));
    });
  };
  const field = (label) => [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith(label)).querySelector('input, textarea');
  const choose = async (input, file) => {
    Object.defineProperty(input, 'files', { value: [file], configurable: true });
    await act(async () => { input.dispatchEvent(new Event('change', { bubbles: true })); });
  };
  const photo = (type = 'image/jpeg', size = 100) => new File([new Uint8Array(size)], 'foto.jpg', { type });

  it('tells the staff, and only them, that a person has no profile, with the way to write it', async () => {
    await show(null, 'admin');
    expect(container.textContent).toContain('Sem perfil');
    expect(button('Escrever o perfil')).toBeTruthy();
    act(() => root.unmount());
    root = createRoot(container);
    await show(null, 'reader');
    expect(container.textContent).toBe('');
  });

  it('writes a profile for a person who has none: the texts in one request, the empty ones too', async () => {
    await show(null, 'owner');
    await click(button('Escrever o perfil'));
    await typeInto(field('Descrição curta'), 'Escritor brasileiro');
    await typeInto(field('Nascimento'), '1980-05');
    await typeInto(field('Local de nascimento'), 'São Paulo');
    await typeInto(field('Biografia'), 'Primeiro parágrafo.');
    await click(button('Salvar'));
    expect(api.put).toHaveBeenCalledWith('/admin/people/9/profile', {
      description: 'Escritor brasileiro', born: '1980-05', died: '', bornPlace: 'São Paulo', bio: 'Primeiro parágrafo.', imageCredit: '', imageLicense: '',
    });
    expect(api.post).not.toHaveBeenCalled();
    expect(container.querySelector('form')).toBeNull(); // done: it closes
  });

  it('starts from what the profile says, so only what is changed changes', async () => {
    await show(profile(), 'admin');
    await click(button('Editar o perfil'));
    expect(field('Descrição curta').value).toBe('escritor de ficção científica americano (1920-1986)');
    expect(field('Nascimento').value).toBe('1920-10-08');
    expect(field('Biografia').value).toBe('Frank Herbert foi um escritor americano.');
    expect(field('Crédito da foto').value).toBe('Unknown photographer');
    await typeInto(field('Morte'), '1986');
    await click(button('Salvar'));
    expect(api.put.mock.calls[0][1]).toMatchObject({ died: '1986', born: '1920-10-08', imageCredit: 'Unknown photographer', imageLicense: 'Public domain' });
  });

  it('sends the photo as a file, with its credit and license, after the texts', async () => {
    await show(null, 'admin');
    await click(button('Escrever o perfil'));
    await typeInto(field('Descrição curta'), 'Escritor');
    const file = photo();
    await choose(container.querySelector('input[type="file"]'), file);
    await typeInto(field('Crédito da foto'), 'Arquivo da família');
    await typeInto(field('Licença da foto'), 'Uso autorizado');
    await click(button('Salvar'));
    // With a new photo, the credit and the license go with it and not with the texts.
    expect(api.put.mock.calls[0][1]).not.toHaveProperty('imageCredit');
    const [url, form] = api.post.mock.calls[0];
    expect(url).toBe('/admin/people/9/profile/photo');
    expect(form.get('image')).toBe(file);
    expect(form.get('credit')).toBe('Arquivo da família');
    expect(form.get('license')).toBe('Uso autorizado');
    expect(api.put.mock.invocationCallOrder[0]).toBeLessThan(api.post.mock.invocationCallOrder[0]);
  });

  it('refuses a file that is not a photo or is too big before sending anything', async () => {
    await show(null, 'admin');
    await click(button('Escrever o perfil'));
    const input = container.querySelector('input[type="file"]');
    await choose(input, photo('image/gif'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('A foto deve ser JPEG, PNG ou WebP.');
    await choose(input, photo('image/jpeg', 5 * 1024 * 1024 + 1));
    expect(container.querySelector('[role="alert"]').textContent).toBe('A foto tem até 5 MB.');
    await click(button('Salvar'));
    expect(api.post).not.toHaveBeenCalled();
  });

  it('says what the server refused and keeps the form', async () => {
    await show(null, 'admin');
    await click(button('Escrever o perfil'));
    api.put.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 400, data: 'A data deve ser 1962, 1962-11 ou 1962-11-12.' } }));
    await typeInto(field('Nascimento'), '12/11/1962');
    await click(button('Salvar'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('A data deve ser 1962, 1962-11 ou 1962-11-12.');
    expect(container.querySelector('form')).toBeTruthy();
  });

  it('closes without sending anything on cancel', async () => {
    await show(null, 'admin');
    await click(button('Escrever o perfil'));
    await click(button('Cancelar'));
    expect(container.querySelector('form')).toBeNull();
    expect(api.put).not.toHaveBeenCalled();
  });

  it('offers a reader no editor, and tells only the staff that it was written by hand', async () => {
    await show(profile({ manual: true, wikidataId: '' }), 'reader');
    expect(button('Editar o perfil')).toBeUndefined();
    expect(button('Descartar o perfil')).toBeUndefined();
    expect(container.textContent).not.toContain('Escrito à mão');
    act(() => root.unmount());
    root = createRoot(container);
    await show(profile({ manual: true, wikidataId: '' }), 'admin');
    expect(container.textContent).toContain('Escrito à mão');
  });

  it('asks before throwing the profile away, and says it is read again when the person has an identifier', async () => {
    await show(profile({ manual: true }), 'admin');
    await click(button('Descartar o perfil'));
    const dialog = container.querySelector('[role="alertdialog"]');
    expect(dialog.textContent).toContain('o perfil da Wikidata é lido de novo');
    expect(api.delete).not.toHaveBeenCalled();
    await click(button('Descartar'));
    expect(api.delete).toHaveBeenCalledWith('/admin/people/9/profile');
  });

  it('does not promise a new reading when there is no identifier', async () => {
    await show(profile({ manual: true, wikidataId: '' }), 'admin');
    await click(button('Descartar o perfil'));
    expect(container.querySelector('[role="alertdialog"]').textContent).not.toContain('Wikidata');
  });
});

describe('PersonProfile: looking the person up on Wikidata (DEC-168)', () => {
  const found = [
    { id: 'Q6984190', label: 'Neal Shusterman', description: 'Escritor norte-americano', born: '1962-11-12', photo: true, wikipedia: true },
    { id: 'Q111', label: 'Neal S.', description: '', photo: false, wikipedia: false },
  ];
  const typeInto = async (el, value) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set.call(el, value);
      el.dispatchEvent(new Event('input', { bubbles: true }));
    });
  };
  const open = async (p = null, role = 'admin') => {
    await show(p, role);
    await click(button('Buscar o perfil'));
  };
  const field = () => container.querySelector('section[aria-label="Buscar o perfil"] input');

  it('is offered to the staff, with or without a profile, and not to a reader', async () => {
    await show(null, 'admin');
    expect(button('Buscar o perfil')).toBeTruthy();
    act(() => root.unmount());
    root = createRoot(container);
    await show(profile(), 'owner');
    expect(button('Buscar o perfil')).toBeTruthy();
    act(() => root.unmount());
    root = createRoot(container);
    await show(profile(), 'reader');
    expect(button('Buscar o perfil')).toBeUndefined();
  });

  it('asks for the search by the name of the person, and says it is waiting', async () => {
    api.post.mockResolvedValueOnce({ data: { state: 'pending', query: 'Herbert, Frank', results: [] } });
    await open();
    expect(field().value).toBe('Herbert, Frank');
    await click(button('Buscar'));
    expect(api.post).toHaveBeenCalledWith('/admin/people/9/profile/search', { query: 'Herbert, Frank' });
    expect(container.textContent).toContain('Buscando na Wikidata…');
    expect(button('Buscar').disabled).toBe(true);
  });

  it('asks for what was typed instead, and shows the candidates with what tells them apart', async () => {
    await open();
    api.post.mockImplementationOnce(async () => {
      searchReply = { state: 'done', query: 'Neal Shusterman', results: found };
      return { data: { state: 'pending', query: 'Neal Shusterman', results: [] } };
    });
    await typeInto(field(), 'Neal Shusterman');
    await click(button('Buscar'));
    await act(async () => { await new Promise((r) => setTimeout(r, 1700)); });
    expect(api.post).toHaveBeenCalledWith('/admin/people/9/profile/search', { query: 'Neal Shusterman' });
    const list = container.querySelector('ul[aria-label="Pessoas encontradas"]');
    const rows = [...list.querySelectorAll('li')].map((li) => li.textContent);
    expect(rows[0]).toContain('Neal Shusterman');
    expect(rows[0]).toContain('Escritor norte-americano');
    expect(rows[0]).toContain('nasc. 12 de novembro de 1962');
    expect(rows[0]).toContain('com foto · com Wikipédia');
    expect(rows[1]).toContain('Sem descrição');
    const link = list.querySelector('a');
    expect(link.href).toBe('https://www.wikidata.org/wiki/Q6984190');
    expect(link.rel).toContain('noopener');
  }, 10000);

  it('ties the person to the one chosen, and says the profile is on its way', async () => {
    searchReply = { state: 'done', query: 'Herbert, Frank', results: found };
    api.post.mockResolvedValueOnce({ data: { profile: 'queued' } });
    await open();
    await click(container.querySelector('button[aria-label^="É esta pessoa: Neal Shusterman"]'));
    expect(api.post).toHaveBeenCalledWith('/admin/people/9/profile/link', { wikidataId: 'Q6984190' });
    expect(container.textContent).toContain('o perfil dessa pessoa chega em instantes');
  });

  it('says what happened to a profile that was written by hand, and what the server refused', async () => {
    searchReply = { state: 'done', query: 'x', results: found };
    api.post.mockResolvedValueOnce({ data: { profile: 'kept' } });
    await open();
    await click(container.querySelector('button[aria-label^="É esta pessoa: Neal Shusterman"]'));
    expect(container.textContent).toContain('O perfil escrito à mão continua. Descarte-o');
    api.post.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 400, data: 'Escolha uma das pessoas da busca.' } }));
    await click(container.querySelector('button[aria-label^="É esta pessoa: Neal S."]'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('Escolha uma das pessoas da busca.');
  });

  it('says why there is nothing: Wikidata off, no answer, or nobody by that name', async () => {
    searchReply = { state: 'off', query: 'x', results: [] };
    await open();
    expect(container.textContent).toContain('A Wikidata está desligada');
    act(() => root.unmount());
    root = createRoot(container);
    searchReply = { state: 'failed', query: 'x', results: [] };
    await open();
    expect(container.textContent).toContain('A Wikidata não respondeu');
    act(() => root.unmount());
    root = createRoot(container);
    searchReply = { state: 'done', query: 'x', results: [] };
    await open();
    expect(container.textContent).toContain('Ninguém com esse nome na Wikidata. Tente outra grafia, ou escreva o perfil à mão.');
  });

  it('closes, and shows the panel under the profile of a person that has one', async () => {
    searchReply = { state: 'done', query: 'x', results: found };
    await open(profile());
    expect(container.querySelector('section[aria-label="Perfil"]')).toBeTruthy();
    expect(container.querySelector('section[aria-label="Buscar o perfil"]')).toBeTruthy();
    await click(button('Fechar'));
    expect(container.querySelector('section[aria-label="Buscar o perfil"]')).toBeNull();
  });
});
