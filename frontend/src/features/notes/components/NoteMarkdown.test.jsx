import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';

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

// Formulas (DEC-111): between $$ and $$, drawn by KaTeX, which is told not to trust what it is given.
const drawn = () => vi.waitFor(() => expect(document.querySelector('.katex, .katex-error')).not.toBeNull());
const tex = () => [...document.querySelectorAll('annotation')].map((a) => a.textContent);

describe('NoteMarkdown, formulas', () => {
  it('draws a formula in the line, with the text around it as it is', async () => {
    view = await render('Vale $$x^2$$ aqui', {});
    await drawn();
    expect(tex()).toEqual(['x^2']);
    expect(document.querySelector('.katex-display')).toBeNull();
    expect(document.querySelector('p').textContent).toContain('Vale ');
    expect(document.querySelector('p').textContent).toContain(' aqui');
  });

  it('draws a block of formula, centred, from two lines of $$', async () => {
    view = await render('antes\n\n$$\n\\frac{a}{b}\n$$\n\ndepois', {});
    await drawn();
    expect(document.querySelector('.katex-display')).not.toBeNull();
    expect(tex()).toEqual(['\\frac{a}{b}']);
  });

  it('reads a single dollar as a dollar sign: a price is no formula', async () => {
    view = await render('custa R$ 5 e R$ 10, ou $x$ mesmo', {});
    await new Promise((r) => setTimeout(r, 20));
    expect(view.text()).toBe('custa R$ 5 e R$ 10, ou $x$ mesmo');
    expect(document.querySelector('.katex')).toBeNull();
  });

  it('reads a single dollar as a dollar sign even when the note has a formula', async () => {
    view = await render('custa R$ 5 e R$ 10, $x$ não é fórmula, mas $$y$$ é', {});
    await drawn();
    expect(tex()).toEqual(['y']);
    expect(view.text()).toContain('custa R$ 5 e R$ 10, $x$ não é fórmula, mas ');
  });

  it('draws a formula that has accents in it, as a person writing in Portuguese will have', async () => {
    view = await render('$$área = b h$$ e $$\\text{ação}$$', {});
    await drawn();
    expect(document.querySelector('.katex-error')).toBeNull();
    expect(tex()).toEqual(['área = b h', '\\text{ação}']);
  });

  it('shows a wrong formula as written, in red, with the reason, and the rest of the note stays', async () => {
    view = await render('Antes $$\\frac{a$$ depois', {});
    await drawn();
    const bad = document.querySelector('.katex-error');
    expect(bad.textContent).toBe('\\frac{a');
    expect(bad.title).toContain('KaTeX parse error');
    expect(view.text()).toContain('Antes ');
    expect(view.text()).toContain(' depois');
  });

  it.each([
    ['a link', '$$\\href{javascript:alert(1)}{x}$$', '\\href'],
    ['an address', '$$\\url{http://exemplo.org}$$', '\\url'],
    ['an image', '$$\\includegraphics{http://exemplo.org/a.png}$$', '\\includegraphics'],
  ])('does not draw %s: it is red text, never a link or a fetch', async (_what, text, command) => {
    view = await render(text, {});
    await drawn();
    expect(document.querySelector('a, img')).toBeNull();
    expect(document.querySelector('[href], [src]')).toBeNull();
    // The text of the command stays in the formula's source, as text: nothing in the page holds an address.
    expect(document.querySelector('.katex-html').textContent).toBe(command);
    expect(document.querySelector('.katex-html [style*="rgb(204, 0, 0)"]')).not.toBeNull();
  });

  it('cuts short a formula that expands without end', async () => {
    view = await render('$$\\def\\a{\\a\\a}\\a$$', {});
    await drawn();
    expect(document.querySelector('.katex-error').title).toContain('Too many expansions');
  });

  it('limits the size a formula can ask for', async () => {
    view = await render('$$\\rule{1000em}{1000em}$$', {});
    await drawn();
    const html = document.body.innerHTML;
    expect(html).toContain('10em');
    expect(html).not.toContain('1000em"');
    expect(html).not.toContain('1000em;');
  });

  it('does not interpret HTML written in a formula', async () => {
    view = await render('$$<img src=x onerror=alert(1)>$$', {});
    await drawn();
    expect(document.querySelector('img')).toBeNull();
    expect(document.querySelector('[onerror]')).toBeNull();
  });

  it('keeps a [[link]] outside a formula a link, and one inside it part of the formula', async () => {
    view = await render('[[A]] e $$[[B]]$$', { A: concept('A'), B: concept('B', '', 6) });
    await drawn();
    expect([...document.querySelectorAll('[data-concept]')].map((m) => m.textContent)).toEqual(['A']);
    expect(tex()).toEqual(['[[B]]']);
  });

  it('draws a formula in a note that is changed while it is shown', async () => {
    view = await render('sem nada', {});
    expect(document.querySelector('.katex')).toBeNull();
    await act(async () => {
      view.unmount();
    });
    view = await render('agora $$x$$', {});
    await drawn();
    expect(tex()).toEqual(['x']);
  });
});
