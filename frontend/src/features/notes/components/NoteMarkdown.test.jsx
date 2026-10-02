import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { post: vi.fn() } }));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { NoteMarkdown } from './NoteMarkdown';

let view;
const render = (body, links) => mount(<NoteMarkdown body={body} links={links} />);
const concept = (name, description = '', conceptId = 5) => ({ conceptId, name, description });
beforeEach(() => vi.clearAllMocks());
afterEach(() => view?.unmount());

describe('NoteMarkdown, a [[link]] to a concept the person has', () => {
  it('is marked, with the text as written and a hint of the concept', async () => {
    view = await render('Sobre [[Poder|o poder]] aqui.', { Poder: concept('Poder', 'Quem manda.') });
    const mark = document.querySelector('[data-concept="5"]');
    expect(mark.textContent).toBe('o poder');
    expect(mark.title).toBe('Poder: Quem manda.');
    expect(view.text()).toBe('Sobre o poder aqui.');
    expect(view.button('o poder')).toBeUndefined();
  });

  it('names the concept in the hint when it has no description, and is looked up by the name as written', async () => {
    view = await render('[[Estoicismo (escola)]]', { 'Estoicismo (escola)': concept('Estoicismo (escola)') });
    const mark = document.querySelector('[data-concept]');
    expect(mark.title).toBe('Estoicismo (escola)');
    expect(mark.textContent).toBe('Estoicismo (escola)');
  });

  it('shows the text as text: nothing in it is read as Markdown', async () => {
    view = await render('[[A|*x* _y_ `z` <b>]]', { A: concept('A') });
    expect(document.querySelector('[data-concept]').textContent).toBe('*x* _y_ `z` <b>');
    expect(document.querySelector('em, strong, code, b')).toBeNull();
  });
});

describe('NoteMarkdown, a [[link]] to a concept that does not exist', () => {
  it('keeps the text, as a mark that offers to create the concept, and creates nothing by itself', async () => {
    view = await render('Fala de [[Destino]] hoje', { Destino: null });
    expect(view.text()).toBe('Fala de Destino hoje');
    expect(view.button('Destino').title).toBe('Esse conceito ainda não existe');
    expect(view.buttonMatching(/Criar conceito/)).toBeUndefined();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('creates the concept, with the name as written, when the person says so', async () => {
    api.post.mockResolvedValue({ data: {} });
    view = await render('[[Destino|o fado]]', { Destino: null });
    await view.click(view.button('o fado'));
    expect(view.text()).toContain('Ainda não há um conceito “Destino”.');
    await view.click(view.button('Criar conceito'));
    expect(api.post).toHaveBeenCalledWith('/concepts', { name: 'Destino' });
  });

  it('does not ask twice while the concept is being created', async () => {
    api.post.mockReturnValue(new Promise(() => {}));
    view = await render('[[Destino]]', { Destino: null });
    await view.click(view.button('Destino'));
    await view.click(view.button('Criar conceito'));
    expect(view.button('Criar conceito').disabled).toBe(true);
  });

  it('says why when it cannot create it', async () => {
    api.post.mockRejectedValue({ response: { status: 409, data: 'Você chegou ao limite de conceitos.' } });
    view = await render('[[Destino]]', { Destino: null });
    await view.click(view.button('Destino'));
    await view.click(view.button('Criar conceito'));
    expect(view.text()).toContain('Você chegou ao limite de conceitos.');
  });

  it('closes the offer with a second click', async () => {
    view = await render('[[Destino]]', { Destino: null });
    await view.click(view.button('Destino'));
    expect(view.button('Criar conceito')).toBeDefined();
    await view.click(view.button('Destino'));
    expect(view.button('Criar conceito')).toBeUndefined();
  });
});

describe('NoteMarkdown, the rest of the text', () => {
  it('says nothing about a link whose state the server did not say', async () => {
    view = await render('[[Destino]] e [[Poder]]', undefined);
    expect(view.text()).toBe('Destino e Poder');
    expect(document.querySelector('button, [data-concept]')).toBeNull();
  });

  it('leaves a link in code as it was written', async () => {
    view = await render('`[[A]]` e\n\n```\n[[A]]\n```', { A: concept('A') });
    expect(view.text()).toContain('[[A]] e');
    expect(document.querySelectorAll('[data-concept]')).toHaveLength(0);
    expect(document.querySelector('pre').textContent).toContain('[[A]]');
  });

  it('still shows ordinary links and Markdown, and does not interpret HTML or a script address', async () => {
    view = await render('[site](https://exemplo.org) **forte** <b>cru</b> [mau](javascript:alert(1))', {});
    const links = [...document.querySelectorAll('a')];
    expect(links[0].getAttribute('href')).toBe('https://exemplo.org');
    expect(document.querySelector('strong').textContent).toBe('forte');
    expect(document.querySelector('b')).toBeNull();
    expect(view.text()).toContain('<b>cru</b>');
    expect(links.map((a) => a.getAttribute('href')).join(' ')).not.toContain('javascript');
  });

  it('keeps the same link in two places, and two names of one concept, each as written', async () => {
    view = await render('[[Poder]] e [[dominação]] e [[Poder]]', { Poder: concept('Poder'), dominação: concept('Poder') });
    expect([...document.querySelectorAll('[data-concept]')].map((m) => m.textContent)).toEqual(['Poder', 'dominação', 'Poder']);
  });
});
