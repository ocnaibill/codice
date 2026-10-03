import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn() } }));

import { api } from '../../../lib/api';
import { DictionaryCard } from './DictionaryCard';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let client;
let onClose;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const source = { package: 'wikt-pt', name: 'Wikcionário em português', license: 'CC BY-SA 4.0 e GFDL', licenseUrl: 'https://creativecommons.org/licenses/by-sa/4.0/', source: 's', sourceUrl: 'https://kaikki.org/dictionary/rawdata.html' };
const entry = (id, word, pos, data) => ({ id, package: 'wikt-pt', lang: 'pt', word, pos, data });
const answer = (items, extra = {}) => ({ word: 'x', lang: 'pt', installed: true, items, sources: items.length ? [source] : [], ...extra });

async function open({ word = 'correram', language = 'pt', data = answer([]) } = {}) {
  api.get.mockResolvedValue({ data });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><DictionaryCard word={word} language={language} onClose={onClose} /></QueryClientProvider>); });
  await flush();
}
const card = () => container.querySelector('[role="dialog"]');
const select = () => container.querySelector('select[aria-label="Idioma da palavra"]');
const click = (el) => act(async () => { el.click(); });

beforeEach(() => {
  vi.clearAllMocks();
  onClose = vi.fn();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('DictionaryCard: asking', () => {
  it('asks the server for the word, in the language of the file, and says which word it is', async () => {
    await open({ word: 'correram', language: 'pt' });
    expect(api.get).toHaveBeenCalledTimes(1);
    expect(api.get).toHaveBeenCalledWith('/dictionary', { params: { word: 'correram', lang: 'pt' } });
    expect(card().getAttribute('aria-label')).toBe('Dicionário: correram');
    expect(card().textContent).toContain('correram');
    expect(select().value).toBe('pt');
  });

  it('offers the languages of the library, and asks again when another is chosen', async () => {
    await open({ word: '走る', language: 'ja' });
    expect([...select().options].map((o) => o.value)).toEqual(['pt', 'en', 'es', 'fr', 'de', 'it', 'ja', 'zh']);
    expect(select().value).toBe('ja');
    const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
    await act(async () => { set.call(select(), 'zh'); select().dispatchEvent(new Event('change', { bubbles: true })); });
    await flush();
    expect(api.get).toHaveBeenLastCalledWith('/dictionary', { params: { word: '走る', lang: 'zh' } });
    expect(select().value).toBe('zh');
  });

  it('starts again in the language of the file when it is a new word', async () => {
    await open({ word: 'casa', language: 'pt' });
    const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
    await act(async () => { set.call(select(), 'fr'); select().dispatchEvent(new Event('change', { bubbles: true })); });
    await flush();
    await act(async () => { root.render(<QueryClientProvider client={client}><DictionaryCard word="livro" language="pt" onClose={onClose} /></QueryClientProvider>); });
    await flush();
    expect(select().value).toBe('pt');
    expect(api.get).toHaveBeenLastCalledWith('/dictionary', { params: { word: 'livro', lang: 'pt' } });
  });

  it('shows a placeholder while it asks, and says it could not when it fails', async () => {
    api.get.mockReturnValue(new Promise(() => {}));
    client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><DictionaryCard word="x" language="pt" onClose={onClose} /></QueryClientProvider>); });
    expect(container.querySelector('[role="status"]').getAttribute('aria-label')).toBe('Procurando a palavra');
    act(() => root.unmount());
    root = createRoot(container);
    api.get.mockRejectedValue(new Error('x'));
    await act(async () => { root.render(<QueryClientProvider client={client}><DictionaryCard word="y" language="pt" onClose={onClose} /></QueryClientProvider>); });
    await flush();
    expect(container.querySelector('[role="alert"]').textContent).toBe('Não foi possível consultar o dicionário.');
  });

  it('asks nothing again for a word it already has, when the language is changed and back', async () => {
    await open({ word: 'casa', language: 'pt' });
    const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
    for (const lang of ['en', 'pt']) {
      await act(async () => { set.call(select(), lang); select().dispatchEvent(new Event('change', { bubbles: true })); });
      await flush();
    }
    expect(api.get).toHaveBeenCalledTimes(2); // pt, en; and pt again is kept
  });
});

describe('DictionaryCard: what it says', () => {
  it('says there is no dictionary installed, and where to install one', async () => {
    await open({ data: answer([], { installed: false }) });
    expect(card().textContent).toContain('Nenhum dicionário está instalado neste servidor.');
    expect(card().textContent).toContain('Administração → Dicionários');
    expect(card().querySelector('footer')).toBeNull();
  });

  it('says it did not find the word, in the language it looked in, and what to try', async () => {
    await open({ word: 'xyzzy', language: 'fr', data: answer([]) });
    expect(card().textContent).toContain('Não achei “xyzzy” em Francês.');
    expect(card().textContent).toContain('troque o idioma');
    expect(card().querySelector('footer')).toBeNull();
  });

  it('shows an entry: the word, its class in Portuguese, the pronunciation and its senses numbered', async () => {
    await open({ word: 'livro', data: answer([{ kind: 'entry', entry: entry(1, 'livro', 'noun', {
      ipa: '/ˈli.vɾu/',
      senses: [
        { glosses: ['objeto feito de várias folhas'], examples: [{ text: 'Com verba pública, livro técnico ainda é restrito.', translation: 'Public money.' }] },
        { glosses: ['obra literária'], tags: ['figurado'] },
      ],
    }) }]) });
    const article = card().querySelector('article');
    expect(article.querySelector('h3').textContent).toContain('livro');
    expect(article.querySelector('h3').textContent).toContain('substantivo');
    expect(article.querySelector('h3').textContent).toContain('/ˈli.vɾu/');
    const senses = [...article.querySelectorAll('ol > li')];
    expect(senses).toHaveLength(2);
    expect(senses[0].textContent).toContain('objeto feito de várias folhas');
    expect(senses[0].textContent).toContain('“Com verba pública, livro técnico ainda é restrito.” — Public money.');
    expect(senses[1].textContent).toContain('figurado');
    expect(senses[1].textContent).toContain('obra literária');
  });

  it('joins the glosses of a sense, and shows an example without a translation as it is', async () => {
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'x', 'verb', { senses: [{ glosses: ['correr', 'fugir'], examples: [{ text: 'Corre.' }] }] }) }]) });
    const sense = card().querySelector('ol > li');
    expect(sense.textContent).toContain('correr; fugir');
    expect(sense.textContent).toContain('“Corre.”');
    expect(sense.textContent).not.toContain('—');
  });

  it('says the tags in Portuguese, and not the ones that only say it is a form', async () => {
    await open({ data: answer([
      { kind: 'entry', entry: entry(1, 'correram', 'verb', { senses: [{ glosses: ['do verbo correr'], tags: ['form-of', 'plural'] }] }) },
      { kind: 'lemma', form: 'correram', tags: ['plural', 'third-person', 'plural'], entry: entry(2, 'correr', 'verb', { senses: [{ glosses: ['a'] }] }) },
    ]) });
    const [first, second] = card().querySelectorAll('article');
    expect(first.querySelector('ol > li').textContent).toBe('pluraldo verbo correr');
    expect(second.querySelector('p').textContent).toBe('“correram” vem de(plural, 3ª pessoa)');
  });

  it('says a sense that points to its word is a form of it', async () => {
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'correram', 'verb', { senses: [{ glosses: [], form_of: [{ word: 'correr' }, { word: 'corrar' }] }] }) }]) });
    expect(card().querySelector('ol > li').textContent).toBe('forma de correr, corrar');
  });

  it('shows the first senses and the rest on request', async () => {
    const senses = Array.from({ length: 6 }, (_, i) => ({ glosses: [`sentido ${i + 1}`] }));
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'x', 'noun', { senses }) }]) });
    expect(card().querySelectorAll('ol > li').length).toBe(4);
    const more = [...card().querySelectorAll('button')].find((b) => b.textContent === 'Mostrar mais 2 sentidos');
    await click(more);
    expect(card().querySelectorAll('ol > li').length).toBe(6);
    await click([...card().querySelectorAll('button')].find((b) => b.textContent === 'Mostrar menos'));
    expect(card().querySelectorAll('ol > li').length).toBe(4);
  });

  it('says "1 sentido" when there is one more', async () => {
    const senses = Array.from({ length: 5 }, (_, i) => ({ glosses: [`s${i}`] }));
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'x', 'noun', { senses }) }]) });
    expect([...card().querySelectorAll('button')].some((b) => b.textContent === 'Mostrar mais 1 sentido')).toBe(true);
  });

  it('shows no button when there is nothing more to show', async () => {
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'x', 'noun', { senses: [{ glosses: ['a'] }] }) }]) });
    expect([...card().querySelectorAll('button')].map((b) => b.textContent)).toEqual(['✕']);
  });

  it('says where a word comes from when it is a form of another, with which form it is', async () => {
    await open({ word: 'corro', data: answer([
      { kind: 'lemma', form: 'corro', tags: ['first-person', 'present'], entry: entry(2, 'correr', 'verb', { senses: [{ glosses: ['mover-se com rapidez'] }] }) },
    ]) });
    const note = card().querySelector('article p');
    expect(note.textContent.startsWith('“corro” vem de')).toBe(true);
    expect(note.querySelector('span').textContent).toBe('(1ª pessoa, presente)');
    expect(card().querySelector('article h3').textContent).toContain('correr');
  });

  it('says a word comes from another one even when the form is not known', async () => {
    await open({ data: answer([{ kind: 'lemma', form: '', entry: entry(2, 'correr', 'verb', { senses: [{ glosses: ['a'] }] }) }]) });
    expect(card().querySelector('article p').textContent).toBe('Vem de');
  });

  it('says a word is listed as a translation of another, and which meaning', async () => {
    await open({ word: '走る', language: 'ja', data: answer([
      { kind: 'translation', via: '走る', sense: 'mover-se com rapidez', entry: entry(3, 'correr', 'verb', { senses: [{ glosses: ['mover-se com rapidez'] }] }) },
      { kind: 'translation', via: '走る', sense: '', entry: entry(4, 'fugir', 'verb', { senses: [{ glosses: ['escapar'] }] }) },
    ]) });
    const notes = [...card().querySelectorAll('article > p')];
    expect(notes.map((p) => p.textContent.startsWith('“走る” está listado como tradução de'))).toEqual([true, true]);
    expect(notes.map((p) => p.querySelector('span')?.textContent ?? null)).toEqual(['(mover-se com rapidez)', null]);
  });

  it('shows what comes first first: the word, then what it comes from, then what it translates', async () => {
    await open({ data: answer([
      { kind: 'entry', entry: entry(1, 'correram', 'verb', { senses: [{ glosses: [], form_of: [{ word: 'correr' }] }] }) },
      { kind: 'lemma', form: 'correram', entry: entry(2, 'correr', 'verb', { senses: [{ glosses: ['a'] }] }) },
    ]) });
    expect([...card().querySelectorAll('article h3 > span:first-child')].map((h) => h.textContent)).toEqual(['correram', 'correr']);
  });

  it('reads the data of an entry that came as text', async () => {
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'x', 'noun', JSON.stringify({ senses: [{ glosses: ['em texto'] }] })) }]) });
    expect(card().textContent).toContain('em texto');
  });

  it('says where it comes from, with the license, in links that open elsewhere', async () => {
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'x', 'noun', { senses: [{ glosses: ['a'] }] }) }]) });
    const footer = card().querySelector('footer');
    expect(footer.textContent).toContain('Fonte: Wikcionário em português (CC BY-SA 4.0 e GFDL)');
    const [src, lic] = footer.querySelectorAll('a');
    expect(src.getAttribute('href')).toBe('https://kaikki.org/dictionary/rawdata.html');
    expect(lic.getAttribute('href')).toBe('https://creativecommons.org/licenses/by-sa/4.0/');
    for (const a of [src, lic]) {
      expect(a.getAttribute('target')).toBe('_blank');
      expect(a.getAttribute('rel')).toBe('noopener noreferrer');
    }
  });

  it('says every source when there are more than one', async () => {
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'x', 'noun', { senses: [{ glosses: ['a'] }] }) }], { sources: [source, { ...source, package: 'wikt-fr', name: 'Wikcionário em francês' }] }) });
    expect(card().querySelector('footer').textContent).toContain('Wikcionário em português (CC BY-SA 4.0 e GFDL); Wikcionário em francês');
  });
});

describe('DictionaryCard: closing', () => {
  it('closes by its button and by Escape, and not by another key', async () => {
    await open();
    await click(container.querySelector('button[aria-label="Fechar o dicionário"]'));
    expect(onClose).toHaveBeenCalledTimes(1);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' })); });
    expect(onClose).toHaveBeenCalledTimes(1);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it('does not listen to the keyboard once it is gone', async () => {
    await open();
    act(() => root.unmount());
    onClose.mockClear();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(onClose).not.toHaveBeenCalled();
    root = createRoot(container);
  });

  it('is above the menu by a selection and the toolbars of the viewers', async () => {
    await open();
    expect(Number(/\bz-\[(\d+)\]/.exec(card().className)[1])).toBeGreaterThan(60);
  });
});
