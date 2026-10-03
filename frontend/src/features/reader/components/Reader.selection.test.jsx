import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
vi.mock('../copyText', () => ({ copyText: vi.fn() }));
vi.mock('../../../lib/download', () => ({ downloadFile: vi.fn() }));
// A viewer that tells of a selection when asked, and says what it was given about the highlights to underline.
const sent = { selection: null, marks: null, cleared: 0 };
const stub = (kind) => ({
  default: ({ onSelection, marks }) => {
    sent.marks = marks;
    return (
      <div data-testid="viewer" data-kind={kind} data-has-handler={String(typeof onSelection === 'function')}>
        <button onClick={() => onSelection(sent.selection)}>select</button>
        <button onClick={() => onSelection(null)}>deselect</button>
      </div>
    );
  },
});
vi.mock('./viewers/PdfViewer', () => stub('pdf'));
vi.mock('./viewers/EpubViewer', () => stub('epub'));
vi.mock('./viewers/TextViewer', () => stub('txt'));
vi.mock('./viewers/MarkdownViewer', () => stub('md'));
vi.mock('./viewers/MangaViewer', () => stub('cbz'));
vi.mock('./viewers/AudioViewer', () => stub('mp3'));

import { api } from '../../../lib/api';
import { copyText } from '../copyText';
import { useToasts } from '../../../components/ui/toast';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';
import { getHighlightColor, saveHighlightColor, setPreferenceOwner } from '../preferences';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const work = (format, language = 'pt') => ({
  id: 7, title: 'Duna', author: 'Frank Herbert', fileId: 10, fileUrl: '/f/10', format, finished: false,
  editions: [{ id: 1, language, files: [
    { id: 10, format, availability: 'available', url: '/f/10' },
    { id: 11, format, availability: 'available', url: '/f/11' },
  ] }],
});
const RANGE = 'epubcfi(/6/4!/4/2,/1:0,/1:20)';
const notes = [
  { id: 1, kind: 'highlight', fileId: 10, quote: 'a', locator: { type: 'epub', cfi: RANGE } },
  { id: 2, kind: 'note', fileId: 10, quote: 'b', body: 'x', locator: { type: 'epub', cfi: 'epubcfi(/6/6!/4/2,/1:3,/1:9)' } },
  { id: 3, kind: 'bookmark', fileId: 10, locator: { type: 'epub', cfi: 'epubcfi(/6/8!/4/2,/1:0,/1:1)' } }, // a bookmark is not underlined
  { id: 4, kind: 'highlight', fileId: 11, locator: { type: 'epub', cfi: 'epubcfi(/6/2!/4/2,/1:0,/1:5)' } }, // another file
  { id: 5, kind: 'highlight', fileId: 10, locator: { type: 'epub', cfi: 'epubcfi(/6/4!/4/2)' } }, // a point, not a passage
  { id: 6, kind: 'highlight', fileId: 10, locator: { type: 'pdf', page: 3 } },
  { id: 7, kind: 'highlight', fileId: 10 },
].map((n) => ({ tags: [], body: '', quote: '', ...n }));

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const menu = () => container.querySelector('[role="toolbar"]');
const labels = () => [...menu().querySelectorAll('button')].map((b) => b.textContent).filter(Boolean); // the colors have no text, only a name
const colorButton = (name) => menu().querySelector(`button[aria-label="Destacar em ${name}"]`);
const press = (label, scope = container) => act(async () => { [...scope.querySelectorAll('button')].find((b) => b.textContent === label).click(); });
const toasts = () => useToasts.getState().items;

async function open(format, { saved = null, language = 'pt' } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work(format, language) };
    if (url === '/dictionary/languages') return { data: { words: ['pt', 'en', 'ja', 'ko'], definitions: ['pt', 'en'] } };
    if (url === '/dictionary') return { data: { word: 'x', lang: 'pt', installed: true, items: [], sources: [] } };
    if (url.startsWith('/progress/files/')) return { data: { revision: 1, position: '', locator: saved } };
    if (url === '/notes') return { data: { data: notes, total: notes.length } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: { id: 99 } });
  api.put.mockResolvedValue({ data: {} });
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek: null });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
}
const selectIt = async () => { await press('select'); };

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  setPreferenceOwner('ana');
  useToasts.getState().clear();
  sent.selection = { text: 'um trecho escolhido', rect: { left: 100, top: 300, width: 100, height: 20, bottom: 320, right: 200 }, touch: false, clear: vi.fn() };
  sent.marks = null;
  copyText.mockResolvedValue(true);
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('Reader: the menu by a selection', () => {
  it.each(['pdf', 'epub', 'txt', 'md'])('is given to the %s viewer, which can tell of a selection', async (format) => {
    await open(format);
    expect(container.querySelector('[data-testid="viewer"]').dataset.hasHandler).toBe('true');
  });

  it.each(['cbz', 'mp3'])('is not given to the %s viewer, which has no text to select', async (format) => {
    await open(format);
    expect(container.querySelector('[data-testid="viewer"]').dataset.hasHandler).toBe('false');
  });

  it('shows the menu when a text is selected, and takes it away when the selection goes', async () => {
    await open('txt');
    expect(menu()).toBeNull();
    await selectIt();
    expect(menu()).not.toBeNull();
    await press('deselect');
    expect(menu()).toBeNull();
  });

  it('copies the passage, says so, clears the selection and closes the menu', async () => {
    await open('txt');
    await selectIt();
    await press('Copiar', menu());
    expect(copyText).toHaveBeenCalledWith('um trecho escolhido');
    expect(sent.selection.clear).toHaveBeenCalled();
    expect(menu()).toBeNull();
    expect(toasts().map((t) => [t.tone, t.title])).toEqual([['success', 'Trecho copiado']]);
  });

  it('says so when the browser does not let it copy', async () => {
    copyText.mockResolvedValue(false);
    await open('txt');
    await selectIt();
    await press('Copiar', menu());
    expect(toasts()).toHaveLength(1);
    expect(toasts()[0]).toMatchObject({ tone: 'error', title: 'Não foi possível copiar' });
    expect(toasts()[0].message).toContain('menu do sistema');
  });

  it('highlights the passage: a highlight with the quote, tied to the file and the place of the reader', async () => {
    await open('pdf', { saved: { type: 'pdf', page: 4 } });
    await selectIt();
    await press('Destacar', menu());
    await flush();
    expect(api.post).toHaveBeenCalledWith('/works/7/notes', { kind: 'highlight', quote: 'um trecho escolhido', color: 'terracotta', fileId: 10, locator: { type: 'pdf', page: 4 } });
    expect(sent.selection.clear).toHaveBeenCalled();
    expect(menu()).toBeNull();
    expect(toasts().map((t) => [t.tone, t.title])).toEqual([['success', 'Destaque salvo']]);
  });

  it('paints the highlight with the color that was touched, and keeps it for the next', async () => {
    await open('txt');
    await selectIt();
    expect(colorButton('terracota').getAttribute('aria-current')).toBe('true');
    await act(async () => { colorButton('sálvia').click(); });
    await flush();
    expect(api.post.mock.calls.at(-1)[1]).toMatchObject({ kind: 'highlight', color: 'sage' });
    expect(getHighlightColor()).toBe('sage');
    await selectIt();
    expect(colorButton('sálvia').getAttribute('aria-current')).toBe('true');
    await press('Destacar', menu());
    await flush();
    expect(api.post.mock.calls.at(-1)[1]).toMatchObject({ color: 'sage' });
  });

  it('starts from the color the account used last time', async () => {
    saveHighlightColor('indigo');
    await open('txt');
    await selectIt();
    expect(colorButton('índigo').getAttribute('aria-current')).toBe('true');
    await press('Destacar', menu());
    await flush();
    expect(api.post.mock.calls.at(-1)[1]).toMatchObject({ color: 'indigo' });
  });

  it('highlights with the place the viewer gave of the passage (a range of an EPUB), and not with the reader\'s', async () => {
    sent.selection.locator = { type: 'epub', cfi: RANGE, excerpt: 'um trecho' };
    await open('epub', { saved: { type: 'epub', cfi: 'epubcfi(/6/2!/4)' } });
    await selectIt();
    await press('Destacar', menu());
    await flush();
    expect(api.post.mock.calls.at(-1)[1]).toEqual({ kind: 'highlight', quote: 'um trecho escolhido', color: 'terracotta', fileId: 10, locator: { type: 'epub', cfi: RANGE, excerpt: 'um trecho' } });
  });

  it('highlights without a place when the reader has none yet', async () => {
    await open('txt');
    await selectIt();
    await press('Destacar', menu());
    await flush();
    expect(api.post.mock.calls.at(-1)[1]).toEqual({ kind: 'highlight', quote: 'um trecho escolhido', color: 'terracotta' });
  });

  it('says why when it could not save the highlight', async () => {
    await open('txt');
    api.post.mockRejectedValue({ response: { status: 400, data: 'a quote is needed' } });
    await selectIt();
    await press('Destacar', menu());
    await flush();
    expect(toasts()).toHaveLength(1);
    expect(toasts()[0]).toMatchObject({ tone: 'error', title: 'Não foi possível salvar o destaque' });
  });

  it('writes a note on the passage: the notes panel opens with it in the quote field, and the menu is gone', async () => {
    await open('txt');
    await selectIt();
    await press('Nota', menu());
    await flush();
    const panel = container.querySelector('aside[aria-label="Anotações"]');
    expect(panel).not.toBeNull();
    expect(panel.querySelector('[aria-label="Trecho do livro"]').value).toBe('um trecho escolhido');
    expect(menu()).toBeNull();
    expect(sent.selection.clear).toHaveBeenCalled();
    expect(api.post.mock.calls.filter(([url]) => url.includes('/notes'))).toEqual([]); // nothing is saved until the person saves the note
  });

  it('ties the note to the place of the passage it was asked on, and the place of the passage is not kept after the note', async () => {
    sent.selection.locator = { type: 'epub', cfi: RANGE };
    await open('epub', { saved: { type: 'epub', cfi: 'epubcfi(/6/2!/4)' } });
    await selectIt();
    await press('Nota', menu());
    await flush();
    const panel = container.querySelector('aside[aria-label="Anotações"]');
    const body = panel.querySelector('[aria-label="Sua anotação"]');
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(body, 'minha ideia');
      body.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await press('Salvar nota', panel);
    await flush();
    expect(api.post).toHaveBeenCalledWith('/works/7/notes', expect.objectContaining({ kind: 'note', quote: 'um trecho escolhido', body: 'minha ideia', fileId: 10, locator: { type: 'epub', cfi: RANGE } }));
  });

  it('does not bring the passage back when the notes panel is opened again later', async () => {
    await open('txt');
    await selectIt();
    await press('Nota', menu());
    await flush();
    await press('✕', container.querySelector('aside[aria-label="Anotações"]'));
    await flush();
    expect(container.querySelector('aside[aria-label="Anotações"]')).toBeNull();
    await act(async () => { container.querySelector('button[aria-label="Notas, destaques e marcadores deste livro"]').click(); });
    await flush();
    expect(container.querySelector('aside [aria-label="Trecho do livro"]').value).toBe('');
  });

  it('does not show the menu over the notes panel', async () => {
    await open('txt');
    await selectIt();
    await press('Nota', menu());
    await flush();
    await selectIt();
    expect(menu()).toBeNull();
  });

  it('closes the menu with Escape, clearing the selection', async () => {
    await open('txt');
    await selectIt();
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(menu()).toBeNull();
    expect(sent.selection.clear).toHaveBeenCalled();
  });

  it('takes the menu away when another file is opened', async () => {
    await open('txt');
    await selectIt();
    expect(menu()).not.toBeNull();
    await act(async () => { useGlobalStore.setState({ activeFileId: 11 }); });
    await flush();
    expect(menu()).toBeNull();
  });

  it('copes with a selection that has nothing to clear', async () => {
    delete sent.selection.clear;
    await open('txt');
    await selectIt();
    await press('Copiar', menu());
    expect(menu()).toBeNull();
  });
});

describe('Reader: the passages underlined on an EPUB', () => {
  it('gives the viewer the passages of this file that were highlighted or written on, by the range of the book', async () => {
    await open('epub');
    expect(sent.marks.map((m) => m.cfi)).toEqual([RANGE, 'epubcfi(/6/6!/4/2,/1:3,/1:9)']);
  });

  it('gives it the color of each', async () => {
    notes[0].color = 'indigo';
    notes[1].color = 'sepia';
    try {
      await open('epub');
      expect(sent.marks.map((m) => m.color)).toEqual(['indigo', 'sepia']);
    } finally {
      delete notes[0].color;
      delete notes[1].color;
    }
  });

  it('gives it no color for the highlights of a server that does not say, which are terracotta', async () => {
    await open('epub');
    expect(sent.marks.map((m) => m.color)).toEqual([undefined, undefined]);
  });
});

describe('Reader: looking a word up in the dictionary', () => {
  const dictionaryCalls = () => api.get.mock.calls.filter(([url]) => url === '/dictionary');
  const card = () => container.querySelector('[role="dialog"][aria-label^="Dicionário"]');

  it('offers the dictionary for a word, and opens the card with it, in the language of the file', async () => {
    sent.selection.text = 'correram';
    await open('txt', { language: 'pt-BR' });
    await selectIt();
    expect(labels()).toEqual(['Copiar', 'Destacar', 'Nota', 'Dicionário']);
    await press('Dicionário', menu());
    await flush();
    expect(card().getAttribute('aria-label')).toBe('Dicionário: correram');
    expect(dictionaryCalls().at(-1)[1]).toEqual({ params: { word: 'correram', lang: 'pt', prefer: 'pt' } });
    expect(menu()).toBeNull();
    expect(sent.selection.clear).toHaveBeenCalled();
    expect(api.post.mock.calls.filter(([url]) => url.includes('/notes'))).toEqual([]); // nothing is saved
  });

  it('looks a word up in the language of its script when that is not the file\'s, and in the file\'s for Latin letters', async () => {
    sent.selection.text = '走る';
    await open('txt', { language: 'pt-BR' });
    await selectIt();
    await press('Dicionário', menu());
    await flush();
    expect(dictionaryCalls().at(-1)[1].params.lang).toBe('ja');
    expect(container.querySelector('select[aria-label="Idioma da palavra"]').value).toBe('ja');
  });

  it('does not take Han alone for Japanese: the file\'s language stands', async () => {
    sent.selection.text = '猫';
    await open('txt', { language: 'zh' });
    await selectIt();
    await press('Dicionário', menu());
    await flush();
    expect(dictionaryCalls().at(-1)[1].params.lang).toBe('zh');
  });

  it('looks a word up in the language of the file, whichever it is', async () => {
    sent.selection.text = 'house';
    await open('epub', { language: 'en-GB' });
    await selectIt();
    await press('Dicionário', menu());
    await flush();
    expect(dictionaryCalls().at(-1)[1]).toEqual({ params: { word: 'house', lang: 'en', prefer: 'pt' } });
  });

  it('starts in Portuguese for a file whose language is not one the dictionary has, or is not said', async () => {
    sent.selection.text = 'neno';
    await open('txt', { language: 'sw' });
    await selectIt();
    await press('Dicionário', menu());
    await flush();
    expect(dictionaryCalls().at(-1)[1].params.lang).toBe('pt');
  });

  it('does not offer the dictionary for a passage', async () => {
    sent.selection.text = 'Era uma vez um texto longo demais para uma palavra';
    await open('txt');
    await selectIt();
    expect(labels()).toEqual(['Copiar', 'Destacar', 'Nota']);
  });

  it('closes the card, and takes it away when another file is opened', async () => {
    sent.selection.text = 'casa';
    await open('txt');
    await selectIt();
    await press('Dicionário', menu());
    await flush();
    await act(async () => { container.querySelector('button[aria-label="Fechar o dicionário"]').click(); });
    expect(card()).toBeNull();
    await selectIt();
    await press('Dicionário', menu());
    await flush();
    expect(card()).not.toBeNull();
    await act(async () => { useGlobalStore.setState({ activeFileId: 11 }); });
    await flush();
    expect(card()).toBeNull();
  });

  it('shows the card of the new word when another is selected while one is open', async () => {
    sent.selection.text = 'casa';
    await open('txt');
    await selectIt();
    await press('Dicionário', menu());
    await flush();
    sent.selection = { ...sent.selection, text: 'livro' };
    await selectIt();
    await press('Dicionário', menu());
    await flush();
    expect(card().getAttribute('aria-label')).toBe('Dicionário: livro');
  });
});
