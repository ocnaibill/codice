import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn() } }));

import { api } from '../../../lib/api';
import { DictionaryCard } from './DictionaryCard';
import { getDictionaryTarget, setPreferenceOwner } from '../preferences';

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
const targetSelect = () => container.querySelector('label select');
const choose = async (el, value) => {
  const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
  await act(async () => { set.call(el, value); el.dispatchEvent(new Event('change', { bubbles: true })); });
  await flush();
};
const click = (el) => act(async () => { el.click(); });

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  setPreferenceOwner('ana');
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
    expect(api.get).toHaveBeenCalledWith('/dictionary', { params: { word: 'correram', lang: 'pt', prefer: 'pt' } });
    expect(card().getAttribute('aria-label')).toBe('Dicionário: correram');
    expect(card().textContent).toContain('correram');
    expect(select().value).toBe('pt');
  });

  it('offers the languages of the catalog, and asks again when another is chosen', async () => {
    await open({ word: '走る', language: 'ja' });
    expect([...select().options].map((o) => o.value)).toEqual(['de', 'zh', 'ko', 'ku', 'es', 'fr', 'el', 'nl', 'id', 'en', 'it', 'ja', 'ms', 'pl', 'pt', 'ru', 'th', 'cs', 'tr', 'vi']);
    expect(select().value).toBe('ja');
    const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
    await act(async () => { set.call(select(), 'zh'); select().dispatchEvent(new Event('change', { bubbles: true })); });
    await flush();
    expect(api.get).toHaveBeenLastCalledWith('/dictionary', { params: { word: '走る', lang: 'zh', prefer: 'pt' } });
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
    expect(api.get).toHaveBeenLastCalledWith('/dictionary', { params: { word: 'livro', lang: 'pt', prefer: 'pt' } });
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

describe('DictionaryCard: the language of the definitions', () => {
  it('asks for the definitions in Portuguese until the reader chooses, and says it', async () => {
    await open({ word: 'casa' });
    expect(targetSelect().value).toBe('pt');
    expect(card().textContent).toContain('Definições primeiro em');
    expect(api.get.mock.calls[0][1].params.prefer).toBe('pt');
  });

  it('asks again with the language chosen, and remembers it for the next word and the next time', async () => {
    await open({ word: 'casa' });
    await choose(targetSelect(), 'fr');
    expect(api.get).toHaveBeenLastCalledWith('/dictionary', { params: { word: 'casa', lang: 'pt', prefer: 'fr' } });
    expect(getDictionaryTarget()).toBe('fr');
    act(() => root.unmount());
    root = createRoot(container);
    await open({ word: 'livro' });
    expect(targetSelect().value).toBe('fr');
    expect(api.get).toHaveBeenLastCalledWith('/dictionary', { params: { word: 'livro', lang: 'pt', prefer: 'fr' } });
  });

  it('keeps the language of the word as it was when the definitions are changed', async () => {
    await open({ word: '走る', language: 'ja' });
    await choose(targetSelect(), 'en');
    expect(select().value).toBe('ja');
  });
});

describe('DictionaryCard: more than one dictionary', () => {
  const two = () => answer([
    { kind: 'entry', entry: { ...entry(1, 'Haus', 'noun', { senses: [{ glosses: ['casa'] }] }), package: 'wikt-it' } },
    { kind: 'entry', entry: entry(2, 'Haus', 'noun', { senses: [{ glosses: ['moradia'] }] }) },
  ], { sources: [source, { ...source, package: 'wikt-it', name: 'Wikcionário em italiano' }] });

  it('says under which dictionary each entry is, when the words are from more than one', async () => {
    await open({ word: 'Haus', language: 'de', data: two() });
    expect([...card().querySelectorAll('section > h2')].map((h) => h.textContent)).toEqual(['Wikcionário em italiano', 'Wikcionário em português']);
    expect(card().querySelectorAll('section article')).toHaveLength(2);
  });

  it('puts what the same dictionary says together under one name', async () => {
    const items = [
      { kind: 'entry', entry: { ...entry(1, 'Haus', 'noun', { senses: [{ glosses: ['a'] }] }), package: 'wikt-it' } },
      { kind: 'entry', entry: { ...entry(2, 'Haus', 'verb', { senses: [{ glosses: ['b'] }] }), package: 'wikt-it' } },
      { kind: 'entry', entry: entry(3, 'Haus', 'noun', { senses: [{ glosses: ['c'] }] }) },
    ];
    await open({ word: 'Haus', language: 'de', data: answer(items, { sources: [source, { ...source, package: 'wikt-it', name: 'Wikcionário em italiano' }] }) });
    expect([...card().querySelectorAll('section')].map((s) => s.querySelectorAll('article').length)).toEqual([2, 1]);
  });

  it('names no dictionary when there is only one', async () => {
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'x', 'noun', { senses: [{ glosses: ['a'] }] }) }, { kind: 'entry', entry: entry(2, 'y', 'noun', { senses: [{ glosses: ['b'] }] }) }]) });
    expect(card().querySelector('section > h2')).toBeNull();
  });

  it('says the id of the dictionary when its name is not known', async () => {
    const data = two();
    data.sources = [source];
    await open({ word: 'Haus', language: 'de', data });
    expect([...card().querySelectorAll('section > h2')].map((h) => h.textContent)).toEqual(['wikt-it', 'Wikcionário em português']);
  });
});

describe('DictionaryCard: translations', () => {
  const withTranslations = (translations) => answer([{ kind: 'entry', entry: entry(1, 'casa', 'noun', { senses: [{ glosses: ['moradia'] }], translations }) }]);

  it('shows what an entry translates to, by language, with the language of the reader first', async () => {
    await open({ data: withTranslations([{ lang: 'en', word: 'house' }, { lang: 'fr', word: 'maison' }, { lang: 'en', word: 'home' }]) });
    await choose(targetSelect(), 'fr');
    const blocks = [...card().querySelectorAll('article > p:last-child > span.block')].map((b) => b.textContent);
    expect(blocks).toEqual(['Francês: maison', 'Inglês: house, home']);
  });

  it('shows no translation when an entry has none', async () => {
    await open({ data: answer([{ kind: 'entry', entry: entry(1, 'x', 'noun', { senses: [{ glosses: ['a'] }] }) }]) });
    expect(card().textContent).not.toContain('Tradução');
  });

  it('says how many more there are when a language has more than fit', async () => {
    const many = ['a', 'b', 'c', 'd', 'e', 'f', 'g'].map((word) => ({ lang: 'en', word }));
    await open({ data: withTranslations(many) });
    expect(card().textContent).toContain('Inglês: a, b, c, d, e e mais 2');
  });
});

describe('DictionaryCard: the bridge through English', () => {
  const bridge = (extra = {}) => ({ via: 'en', from: 'ja', to: 'pt', available: true, candidates: [{ english: 'run', words: ['correr', 'andar'] }, { english: 'race', words: ['competir'] }], ...extra });
  const jaEntry = () => ({ kind: 'entry', entry: { ...entry(1, '走る', 'verb', { senses: [{ glosses: ['はしる'] }] }), package: 'wikt-ja', lang: 'ja' } });
  const bridged = (b, items = [jaEntry()]) => answer(items, { bridge: b, sources: [{ ...source, package: 'wikt-ja', name: 'Wikcionário em japonês' }] });
  const section = () => card().querySelector('section[aria-label="Via inglês"]');

  it('says the candidates are by way of English, an approximation and not the answer', async () => {
    await open({ word: '走る', language: 'ja', data: bridged(bridge()) });
    expect(section().querySelector('h2').textContent).toBe('Via inglês');
    expect(section().textContent).toContain('não ligam “走る” a Português diretamente');
    expect(section().textContent).toContain('candidatos');
    expect(section().textContent).toContain('aproximação');
  });

  it('shows each English word with the words of the language the reader wants', async () => {
    await open({ word: '走る', language: 'ja', data: bridged(bridge()) });
    expect([...section().querySelectorAll('li')].map((li) => li.textContent)).toEqual(['run → correr, andar', 'race → competir']);
  });

  it('shows a single candidate as a list too', async () => {
    await open({ word: '走る', language: 'ja', data: bridged(bridge({ candidates: [{ english: 'run', words: ['correr'] }] })) });
    expect([...section().querySelectorAll('li')].map((li) => li.textContent)).toEqual(['run → correr']);
    expect(section().textContent).toContain('candidatos');
    expect(section().textContent).not.toContain('Nem pela ponte');
  });

  it('does not say the English dictionary is missing when it is installed', async () => {
    await open({ word: '走る', language: 'ja', data: bridged(bridge()) });
    expect(section().textContent).not.toContain('não está instalado');
  });

  it('says the English dictionary would make it better, and where to install it, when it is not installed', async () => {
    await open({ word: '走る', language: 'ja', data: bridged(bridge({ available: false })) });
    expect(section().textContent).toContain('O dicionário de inglês não está instalado');
    expect(section().textContent).toContain('Administração → Dicionários');
    expect(section().querySelectorAll('li')).toHaveLength(2);
  });

  it('says it found nothing through English either, in the language the reader wants', async () => {
    await open({ word: '食べる', language: 'ja', data: bridged(bridge({ candidates: [], to: 'fr' })) });
    expect(section().textContent).toContain('Nem pela ponte do inglês achei “食べる” em Francês.');
    expect(section().querySelectorAll('li')).toHaveLength(0);
    expect(section().textContent).not.toContain('candidatos');
  });

  it('reads a bridge with no list of candidates as one with none', async () => {
    await open({ word: 'x', language: 'ja', data: bridged({ via: 'en', from: 'ja', to: 'pt', available: true }) });
    expect(section().textContent).toContain('Nem pela ponte do inglês');
  });

  it('shows nothing of the bridge when the lookup did not try it', async () => {
    await open({ word: 'casa', data: answer([{ kind: 'entry', entry: entry(1, 'casa', 'noun', { senses: [{ glosses: ['moradia'] }] }) }]) });
    expect(section()).toBeNull();
  });

  it('says the word was not found only when the bridge has nothing either', async () => {
    await open({ word: '走る', language: 'ja', data: bridged(bridge(), []) });
    expect(card().textContent).not.toContain('Não achei');
    expect(section().querySelectorAll('li')).toHaveLength(2);
    await open({ word: '食べる', language: 'ja', data: bridged(bridge({ candidates: [] }), []) });
    expect(card().textContent).toContain('Não achei “食べる” em Japonês.');
  });

  it('puts the bridge after what the dictionaries say, and the sources at the foot', async () => {
    await open({ word: '走る', language: 'ja', data: bridged(bridge()) });
    const body = card().querySelector('.overflow-y-auto');
    expect(body.lastElementChild).toBe(section());
    expect(card().querySelector('footer').textContent).toContain('Wikcionário em japonês');
  });
});
