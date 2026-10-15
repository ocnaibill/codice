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
import { WikiPage } from './WikiPage';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const WORK = 12;
const PROJECT = 3;

const chapters = (n, upto) => Array.from({ length: n }, (_, i) => ({ chapter: i + 1, title: i + 1 <= upto ? `Título ${i + 1}` : '', part: '', read: i + 1 <= upto }));

const wiki = (extra = {}) => ({
  project: { id: PROJECT, workId: WORK, collectionId: null, state: 'ready', review: 'none', provider: 'deepseek', model: 'deepseek-v4-flash', remote: true, generatedAt: '2026-10-15T12:00:00Z', books: [] },
  books: [{ position: 0, workId: WORK, fileId: 30, side: false, title: 'O Ceifador', chapters: chapters(10, 4) }],
  cursor: { book: 0, chapter: 4, side: false, label: 'O Ceifador · Título 4', source: 'progress' },
  own: { book: 0, chapter: 4, side: false, label: 'O Ceifador · Título 4', source: 'progress' },
  index: [
    { slug: 'citra', name: 'Citra Terranova', kind: 'person', page: true, appearances: 4, aliases: ['Citra'] },
    { slug: 'rowan', name: 'Rowan Damisch', kind: 'person', page: true, appearances: 3, aliases: [] },
    { slug: 'faraday', name: 'Ceifador Faraday', kind: 'person', page: false, appearances: 2, aliases: ['Honorável Ceifador Faraday'] },
    { slug: 'ceifa', name: 'Ceifa', kind: 'organization', page: false, appearances: 4, aliases: [] },
    { slug: 'anel', name: 'Anel da Ceifa', kind: 'object', page: false, appearances: 3, aliases: [] },
  ],
  hasSide: false,
  ...extra,
});

const S = (id, text, refs = [1]) => ({ id, text, refs });
const page = (extra = {}) => ({
  slug: 'citra', name: 'Citra Terranova', kind: 'person', page: true, aliases: ['Citra'],
  books: [{ book: 0, title: 'O Ceifador', side: false, chapters: 4 }],
  situation: { book: 0, chapter: 4, label: 'O Ceifador · Título 4', text: 'Aprendiz de Faraday, treina na casa dele.' },
  who: { book: 0, part: 'Parte 1', label: 'O Ceifador · Título 2', paragraphs: [[S(1, 'Citra é aprendiz de ceifador.'), S(2, 'Mora com a família.', [1, 2])]] },
  biography: [{ book: 0, title: 'O Ceifador', side: false, parts: [{ part: 'Parte 1', chapterFrom: 1, chapterTo: 2, paragraphs: [[S(3, 'Faraday a escolhe como aprendiz.', [2])]] }] }],
  inProgress: { book: 0, part: 'Parte 2', chapterFrom: 3, chapterTo: 6, states: [
    { book: 0, chapter: 3, label: 'O Ceifador · Título 3', text: 'Chega à casa de Faraday.' },
    { book: 0, chapter: 4, label: 'O Ceifador · Título 4', text: 'Começa o treino.' },
  ] },
  hidden: { chapterFrom: 5, chapterTo: 10, laterBooks: 0 },
  personality: [S(4, 'É teimosa.', [1])],
  relations: [
    { group: 'mentors', items: [{ slug: 'faraday', name: 'Ceifador Faraday', role: 'mestre' }] },
    { group: 'things', items: [{ name: 'Anel da Ceifa', role: 'símbolo' }] },
  ],
  timeline: [
    { book: 0, chapter: 1, label: 'O Ceifador · Título 1', text: 'Conhece Faraday.' },
    { book: 0, chapter: 4, label: 'O Ceifador · Título 4', text: 'Começa o treino.' },
  ],
  references: [
    { n: 1, book: 0, chapter: 1, label: 'O Ceifador · Título 1', items: [{ fact: 'Faraday visita a família', quote: 'O ceifador entrou sem bater.', sequence: 5, locator: { type: 'epub', href: 'c1.xhtml', progression: 0.2 } }] },
    { n: 2, book: 0, chapter: 2, label: 'O Ceifador · Título 2', items: [{ fact: 'Ela aceita', quote: '' }] },
  ],
  ...extra,
});

const graph = {
  cursor: wiki().cursor,
  nodes: [
    { slug: 'citra', name: 'Citra Terranova', kind: 'person', page: true, appearances: 4 },
    { slug: 'faraday', name: 'Ceifador Faraday', kind: 'person', page: false, appearances: 2 },
    { slug: 'ceifa', name: 'Ceifa', kind: 'organization', page: false, appearances: 4 },
    { slug: 'rowan', name: 'Rowan Damisch', kind: 'person', page: true, appearances: 3 },
  ],
  edges: [
    { a: 'citra', b: 'faraday', weight: 4, labels: [{ label: 'aprendiz de', book: 0, chapter: 1 }] },
    { a: 'citra', b: 'rowan', weight: 2, labels: [{ label: 'colega de treino', book: 0, chapter: 3 }] },
    { a: 'faraday', b: 'ceifa', weight: 1, labels: [] },
  ],
};

const review = {
  review: 'pending',
  changes: [
    { slug: 'recem', name: 'Recém-chegado', kind: 'person', page: true, new: true, labels: ['Quem é, até Parte I'] },
    { slug: 'citra', name: 'Citra Terranova', kind: 'person', page: true, new: false, labels: ['Biografia, Parte 2'], added: [] },
    { slug: 'rowan', name: 'Rowan Damisch', kind: 'person', page: true, new: false, labels: ['Biografia, Parte 3'], undone: true },
  ],
};

let rules = [];
let manual = [];

let container;
let root;
let asked;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const click = async (el) => { await act(async () => { el.dispatchEvent(new MouseEvent('click', { bubbles: true })); }); await flush(); };
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim().startsWith(text));
const text = () => container.textContent;

async function open({ role = 'reader', summary = wiki(), entity = page(), state = {} } = {}) {
  asked = [];
  api.get.mockImplementation(async (url, config) => {
    asked.push({ url, params: config?.params ?? {} });
    if (url === '/auth/me') return { data: { id: 'u', role } };
    if (url === `/works/${WORK}/wiki`) return { data: { wiki: typeof summary === 'function' ? summary(config?.params ?? {}) : summary } };
    if (url.startsWith(`/wiki/${PROJECT}/entities/`)) {
      if (entity instanceof Error) throw entity;
      return { data: { cursor: wiki().cursor, page: entity } };
    }
    if (url === `/wiki/${PROJECT}/graph`) return { data: graph };
    if (url === `/wiki/${PROJECT}/review`) return { data: review };
    if (url === `/wiki/${PROJECT}/identity`) return { data: { rules } };
    if (url === `/wiki/${PROJECT}/manual`) return { data: { items: manual } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: { hidden: 2, slug: 'citra' } });
  api.delete.mockResolvedValue({});
  api.put.mockResolvedValue({});
  useGlobalStore.setState({ wikiWorkId: WORK, wikiEntity: null, wikiGraph: false, wikiFrom: null, activeBookId: null, seek: null, ...state });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  // A second opening in the same test starts from nothing: the queries of the first one belong to its client.
  act(() => root.unmount());
  root = createRoot(container);
  await act(async () => { root.render(<QueryClientProvider client={client}><WikiPage /></QueryClientProvider>); });
  await flush();
  await flush();
}

beforeEach(() => {
  vi.clearAllMocks();
  rules = [];
  manual = [];
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  sessionStorage.clear();
  act(() => root.unmount());
  container.remove();
  useGlobalStore.getState().goHome();
});

describe('WikiPage: the index of what appeared up to the reader', () => {
  it('says up to where it shows and where that comes from, and how much is kept back', async () => {
    await open();
    const bar = container.querySelector('[aria-label="Até onde a wiki mostra"]');
    expect(bar.textContent).toContain('Mostrando até O Ceifador · Título 4');
    expect(bar.textContent).toContain('do seu progresso no leitor');
    expect(bar.textContent).toContain('6 capítulos deste livro ficam ocultos');
    expect(container.querySelector('h1').textContent).toBe('O Ceifador');
  });

  it('filters by kind with the counts, and searches by any name', async () => {
    await open();
    const filters = container.querySelector('[aria-label="Tipo de entidade"]');
    expect([...filters.querySelectorAll('button')].map((b) => b.textContent)).toEqual(['Todas5', 'Pessoas3', 'Organizações1', 'Objetos1']);
    await click(button('Objetos'));
    expect([...container.querySelectorAll('[aria-label="Índice da wiki"] li')].map((li) => li.textContent)).toEqual([expect.stringContaining('Anel da Ceifa')]);
    await click(button('Todas'));
    const search = container.querySelector('input[type="search"]');
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(search, 'honoravel');
      search.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect([...container.querySelectorAll('[aria-label="Índice da wiki"] li')].map((li) => li.querySelector('.font-display').textContent)).toEqual(['Ceifador Faraday']);
  });

  it('puts those with a page first, and opens an entity in the address', async () => {
    await open();
    const names = [...container.querySelectorAll('[aria-label="Índice da wiki"] li .font-display')].map((n) => n.textContent);
    expect(names.slice(0, 2)).toEqual(['Citra Terranova', 'Rowan Damisch']);
    await click(button('Página completa') ?? container.querySelector('[aria-label="Índice da wiki"] li button'));
    expect(useGlobalStore.getState().wikiEntity).toBe('citra');
  });

  it('asks the server for another point, and warns when it goes past what was read', async () => {
    await open();
    const select = container.querySelector('select');
    expect([...select.options].map((o) => o.textContent).slice(0, 6)).toEqual([
      'Onde estou (O Ceifador · Título 4)', '1. Título 1', '2. Título 2', '3. Título 3', '4. Título 4', 'Capítulo 5 (não lido)',
    ]);
    await act(async () => {
      select.value = '0:7';
      select.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await flush();
    expect(asked.filter((a) => a.url === `/works/${WORK}/wiki`).at(-1).params).toEqual({ book: 0, chapter: 7 });
    await open({ summary: (params) => (params.chapter === 7 ? wiki({ cursor: { book: 0, chapter: 7, side: false, label: 'O Ceifador · 7', source: 'manual' } }) : wiki()) });
    await act(async () => {
      const s = container.querySelector('select');
      s.value = '0:7';
      s.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await flush();
    expect(text()).toContain('Você escolheu ir além do que leu');
  });

  it('keeps the point chosen through a reload of the page, in this tab only', async () => {
    await open();
    await act(async () => {
      const s = container.querySelector('select');
      s.value = '0:7';
      s.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await flush();
    expect(JSON.parse(sessionStorage.getItem(`codice:wiki-cursor:${WORK}`))).toMatchObject({ manual: true, book: 0, chapter: 7 });
    await open();
    expect(container.querySelector('select').value).toBe('0:7');
    expect(asked.find((a) => a.url === `/works/${WORK}/wiki`).params).toEqual({ book: 0, chapter: 7 });
    // Back to the reader's own point, nothing is kept.
    await act(async () => {
      const s = container.querySelector('select');
      s.value = 'own';
      s.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(sessionStorage.getItem(`codice:wiki-cursor:${WORK}`)).toBeNull();
    sessionStorage.setItem(`codice:wiki-cursor:${WORK}`, '{not json');
    await open();
    expect(container.querySelector('select').value).toBe('own');
  });

  it('chooses a book and then a chapter of it, in a series', async () => {
    const series = wiki({ books: [
      { position: 0, workId: WORK, fileId: 30, side: false, title: 'O Ceifador', chapters: chapters(10, 4) },
      { position: 1, workId: 13, fileId: 31, side: false, title: 'A Nuvem', chapters: chapters(8, 0) },
    ] });
    await open({ summary: series });
    const book = container.querySelector('select[aria-label="Livro"]');
    expect([...book.options].map((o) => o.textContent)).toEqual(['Onde estou (O Ceifador · Título 4)', 'O Ceifador', 'A Nuvem']);
    expect(container.querySelector('select[aria-label="Capítulo"]')).toBeNull();
    await act(async () => {
      book.value = '1';
      book.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await flush();
    expect(asked.filter((a) => a.url === `/works/${WORK}/wiki`).at(-1).params).toEqual({ book: 1, chapter: 8 });
    const chapter = container.querySelector('select[aria-label="Capítulo"]');
    expect(chapter.value).toBe('1:8');
    expect(chapter.options[7].textContent).toBe('Capítulo 8 (fim do livro) · não lido');
    await act(async () => {
      chapter.value = '1:3';
      chapter.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await flush();
    expect(asked.filter((a) => a.url === `/works/${WORK}/wiki`).at(-1).params).toEqual({ book: 1, chapter: 3 });
  });

  it('offers the side books only when the wiki has them, and asks for them', async () => {
    await open();
    expect(text()).not.toContain('Incluir contos paralelos');
    await open({ summary: wiki({ hasSide: true }) });
    const box = container.querySelector('input[type="checkbox"]');
    await click(box);
    expect(asked.filter((a) => a.url === `/works/${WORK}/wiki`).at(-1).params).toEqual({ side: 1 });
  });

  it('says when the reader has not started', async () => {
    await open({ summary: wiki({ cursor: { book: -1, chapter: 0, side: false, label: '', source: 'none' }, own: { book: -1, chapter: 0, source: 'none', label: '' }, index: [] }) });
    expect(text()).toContain('Você ainda não começou a ler');
    expect(text()).toContain('Ninguém apareceu ainda até este ponto');
    expect(text()).toContain('1 livro posterior fica oculto');
  });

  it('says when the work has no wiki', async () => {
    await open({ summary: null });
    expect(text()).toContain('Esta obra ainda não tem wiki.');
  });

  it('tells who wrote it, with the real provider and model, and whether the text left the server', async () => {
    await open();
    const note = container.querySelector('[aria-label="Sobre esta wiki"]').textContent;
    expect(note).toContain('Experimental');
    expect(note).toContain('DeepSeek');
    expect(note).toContain('deepseek-v4-flash');
    expect(note).toContain('saiu deste servidor');
    await open({ summary: wiki({ project: { ...wiki().project, provider: 'ollama', remote: false } }) });
    expect(container.querySelector('[aria-label="Sobre esta wiki"]').textContent).toContain('não saiu deste servidor');
  });

  it('says the state of the generation, and the review only as such to staff', async () => {
    await open({ summary: wiki({ project: { ...wiki().project, state: 'partial', review: 'pending' } }) });
    expect(text()).toContain('Esta wiki está parcial');
    expect(text()).toContain('está em revisão');
    await open({ role: 'admin', summary: wiki({ project: { ...wiki().project, review: 'pending' } }) });
    expect(text()).toContain('falta revisar');
  });
});

describe('WikiPage: the page of an entity', () => {
  it('shows the situation, "Quem é" with its point, the biography of the parts read and the part in progress', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    expect(container.querySelector('h1').textContent).toBe('Citra Terranova');
    expect(text()).toContain('“Aprendiz de Faraday, treina na casa dele.”');
    expect(text()).toContain('atualizado ao fim de Título 2');
    expect(text()).toContain('Citra é aprendiz de ceifador.');
    expect(text()).toContain('Faraday a escolhe como aprendiz.');
    expect(text()).toContain('Parte 2: em andamento');
    const current = container.querySelector('[aria-current="step"]');
    expect(current.textContent).toContain('Título 4');
    expect(current.textContent).toContain('ponto atual');
    expect(text()).toContain('Os próximos 6 capítulos estão ocultos');
    expect(text()).toContain('você está em Título 4');
    // The breadcrumb names it.
    expect(container.querySelector('nav [aria-current="page"]').textContent).toBe('Citra Terranova');
  });

  it('numbers the references of each sentence and lists them with the quote', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    const marks = [...container.querySelectorAll('sup button')].map((b) => b.textContent);
    expect(marks.slice(0, 3)).toEqual(['[1]', '[1]', '[2]']);
    const refs = container.querySelector('#wiki-ref-1');
    expect(refs.textContent).toContain('“O ceifador entrou sem bater.”');
    expect(refs.textContent).toContain('Faraday visita a família');
    // Without a place in the file, no button to open it.
    expect(container.querySelector('#wiki-ref-2').textContent).not.toContain('Abrir no leitor');
  });

  it('opens the reader at the paragraph only to look, keeping the wiki under it', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    await click(button('Abrir no leitor neste parágrafo'));
    const s = useGlobalStore.getState();
    expect(s).toMatchObject({ activeBookId: WORK, activeFileId: 30, wikiWorkId: WORK, wikiEntity: 'citra' });
    expect(s.seek).toMatchObject({ locator: { type: 'epub', href: 'c1.xhtml' }, peek: true, context: { kind: 'wiki', quote: 'O ceifador entrou sem bater.' } });
  });

  it('comes back from the book at the reference it opened', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    await click(button('Abrir no leitor neste parágrafo'));
    useGlobalStore.getState().closeBook();
    await open({ state: { wikiEntity: 'citra' } });
    await flush();
    expect(document.activeElement?.id).toBe('wiki-ref-1');
    // Another time the page opens at its name.
    await open({ state: { wikiEntity: 'citra' } });
    await flush();
    expect(document.activeElement?.hasAttribute('data-wiki-heading')).toBe(true);
  });

  it('links the relations that have a page, and groups the ficha', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    const ficha = container.querySelector('[aria-labelledby="wiki-ficha"]');
    expect(ficha.textContent).toContain('Mentores');
    expect(ficha.textContent).toContain('Objetos, conceitos e eventos');
    expect([...ficha.querySelectorAll('button')].map((b) => b.textContent)).toEqual(['Ceifador Faraday']);
    await click(button('Ceifador Faraday'));
    expect(useGlobalStore.getState().wikiEntity).toBe('faraday');
  });

  it('draws the minigraph from the links, and expands it', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    const mini = container.querySelector('[aria-labelledby="wiki-minigraph"]');
    expect([...mini.querySelectorAll('g[role="button"]')].map((g) => g.getAttribute('aria-label'))).toEqual([
      'Ceifador Faraday (Pessoa): aprendiz de',
      'Rowan Damisch (Pessoa): colega de treino',
    ]);
    await click(button('Expandir grafo'));
    expect(useGlobalStore.getState()).toMatchObject({ wikiGraph: true, wikiEntity: 'citra' });
  });

  it('keeps the timeline closed until asked', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    const timeline = [...container.querySelectorAll('details')].find((d) => d.textContent.includes('Linha do tempo'));
    expect(timeline.open).toBe(false);
    expect(timeline.textContent).toContain('2 registros');
  });

  it('says an entity beyond the point is not there, without telling more', async () => {
    const notFound = Object.assign(new Error('404'), { response: { status: 404 } });
    await open({ state: { wikiEntity: 'vilao' }, entity: notFound });
    expect(text()).toContain('Esta página não existe, ou fica além do ponto em que você está.');
  });

  it('goes back to the reader where the person was, when it came from there', async () => {
    await open({ state: { wikiEntity: 'citra', wikiFrom: { workId: WORK, fileId: 30 } } });
    expect(text()).toContain('Você veio do leitor · O Ceifador · Título 4');
    await click(button('Retomar leitura no parágrafo'));
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: WORK, activeFileId: 30, seek: null, wikiWorkId: null, wikiFrom: null });
  });

  it('goes back from a page to the index, and from the index to the work', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    await click(button('← Voltar'));
    expect(useGlobalStore.getState()).toMatchObject({ wikiWorkId: WORK, wikiEntity: null });
    await click(button('← Voltar'));
    expect(useGlobalStore.getState()).toMatchObject({ wikiWorkId: null, sheetWorkId: WORK });
  });

  it('has no accessibility violation (axe)', async () => {
    await open({ role: 'admin', state: { wikiEntity: 'citra' }, entity: page({ suggestions: [{ id: 1, kind: 'split', entity: 'citra', evidence: 'x' }] }) });
    const result = await axe.run(document.body, {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
      rules: { 'color-contrast': { enabled: false }, region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
    });
    expect(result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.html.slice(0, 90)).join(' | ')}`)).toEqual([]);
  });
});

describe('WikiPage: the graph', () => {
  it('draws every kind, and lists the links of the node chosen', async () => {
    await open({ state: { wikiGraph: true } });
    const nodes = [...container.querySelectorAll('[aria-label="Entidades e vínculos"] g[role="button"]')].map((g) => g.getAttribute('aria-label'));
    expect(nodes).toEqual(['Citra Terranova (Pessoa)', 'Ceifador Faraday (Pessoa)', 'Ceifa (Organização)', 'Rowan Damisch (Pessoa)']);
    await click(container.querySelector('g[aria-label="Citra Terranova (Pessoa)"]'));
    const side = container.querySelector('[aria-label="Entidade escolhida"]');
    expect(side.textContent).toContain('2 vínculos');
    expect(side.textContent).toContain('aprendiz de');
    await click(button('Abrir a página'));
    expect(useGlobalStore.getState()).toMatchObject({ wikiEntity: 'citra', wikiGraph: false });
  });

  it('leaves out the weak links, and the nodes left alone, when asked', async () => {
    await open({ state: { wikiGraph: true } });
    const select = [...container.querySelectorAll('select')].find((s) => s.closest('[aria-label="Grafo da wiki"]'));
    await act(async () => {
      select.value = '2';
      select.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.querySelectorAll('[aria-label="Entidades e vínculos"] g[role="button"]')).toHaveLength(3);
    expect(text()).toContain('1 sem vínculo neste filtro');
  });

  it('opens a big graph without the links seen only once', async () => {
    const many = Array.from({ length: 301 }, (_, i) => ({ a: 'citra', b: `n${i}`, weight: i === 0 ? 2 : 1, labels: [] }));
    const big = { ...graph, nodes: [...graph.nodes, ...many.map((e) => ({ slug: e.b, name: e.b, kind: 'other', page: false, appearances: 1 }))], edges: many };
    api.get.mockImplementation(async (url) => {
      if (url === '/auth/me') return { data: { id: 'u', role: 'reader' } };
      if (url === `/works/${WORK}/wiki`) return { data: { wiki: wiki() } };
      if (url === `/wiki/${PROJECT}/graph`) return { data: big };
      throw new Error(`unexpected GET ${url}`);
    });
    useGlobalStore.setState({ wikiWorkId: WORK, wikiEntity: null, wikiGraph: true });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><WikiPage /></QueryClientProvider>); });
    await flush();
    await flush();
    expect(container.querySelectorAll('[aria-label="Entidades e vínculos"] g[role="button"]')).toHaveLength(2);
    expect([...container.querySelectorAll('select')].find((s) => s.closest('[aria-label="Grafo da wiki"]')).value).toBe('2');
  });
});

describe('WikiPage: the review of an update and the sentences marked wrong (DEC-180, RF-052)', () => {
  const pending = (over = {}) => wiki({ project: { ...wiki().project, review: 'pending' }, ...over });
  const flag = () => [...container.querySelectorAll('button[aria-label="Marcar esta frase como errada"]')];

  it('shows the staff what the update changed, and undoes what one entity gained', async () => {
    await open({ role: 'admin', summary: pending() });
    const banner = container.querySelector('[aria-label="Revisão da atualização"]');
    expect(banner.textContent).toContain('1 página nova e 2 páginas que ganharam seções');
    expect(banner.textContent).not.toContain('Recém-chegado'); // closed until asked: the names of a later book are spoilers
    await click(button('Ver o que mudou'));
    const list = banner.querySelector('[aria-label="O que mudou"]');
    expect([...list.querySelectorAll('li')].map((li) => li.querySelector('button').textContent)).toEqual(['Recém-chegado', 'Citra Terranova', 'Rowan Damisch']);
    expect(list.textContent).toContain('Ganhou: Biografia, Parte 2');
    expect(list.textContent).toContain('desfeito');
    await click([...list.querySelectorAll('button')].find((b) => b.textContent === 'Desfazer'));
    expect(api.post).toHaveBeenCalledWith(`/wiki/${PROJECT}/review/undo`, { slug: 'recem' });
    await click(button('Marcar como revisada'));
    expect(api.post).toHaveBeenCalledWith(`/wiki/${PROJECT}/review/approve`);
    await click(list.querySelector('button'));
    expect(useGlobalStore.getState().wikiEntity).toBe('recem');
  });

  it('shows no review to a reader, who is only told it is being reviewed', async () => {
    await open({ role: 'reader', summary: pending() });
    expect(container.querySelector('[aria-label="Revisão da atualização"]')).toBeNull();
    expect(text()).toContain('está em revisão');
    expect(api.get.mock.calls.some(([url]) => url.endsWith('/review'))).toBe(false);
  });

  it('lets the staff mark a sentence as wrong, and show it again; a reader has no button', async () => {
    await open({ role: 'admin', state: { wikiEntity: 'citra' } });
    expect(flag().length).toBeGreaterThan(0);
    await click(flag()[0]);
    expect(api.put).toHaveBeenCalledWith(`/wiki/${PROJECT}/sentences/1/hidden`, { hidden: true });
    const struck = page({ who: { ...page().who, paragraphs: [[{ ...page().who.paragraphs[0][0], hidden: true }, page().who.paragraphs[0][1]]] } });
    await open({ role: 'admin', state: { wikiEntity: 'citra' }, entity: struck });
    expect(container.querySelector('del').textContent).toBe('Citra é aprendiz de ceifador.');
    await click(button('Mostrar de novo'));
    expect(api.put).toHaveBeenLastCalledWith(`/wiki/${PROJECT}/sentences/1/hidden`, { hidden: false });
    await open({ role: 'reader', state: { wikiEntity: 'citra' } });
    expect(flag()).toHaveLength(0);
  });
});

describe('WikiPage: who is who (DEC-181)', () => {
  const merge = { id: 1, kind: 'merge', entity: 'citra', other: 'faraday', otherName: 'Ceifador Faraday', evidence: 'O modelo achou que são a mesma, sem prova.' };
  const select = (label) => [...container.querySelectorAll('label')].find((l) => l.textContent.includes(label)).querySelector('select');
  const choose = (el, value) => act(async () => {
    el.value = value;
    el.dispatchEvent(new Event('change', { bubbles: true }));
  });
  const identity = () => container.querySelector('[aria-labelledby="wiki-identity"]');

  it('is only for the staff, and shows what the generator could not prove', async () => {
    await open({ role: 'reader', state: { wikiEntity: 'citra' }, entity: page({ suggestions: [merge] }) });
    expect(identity()).toBeNull();
    await open({ role: 'admin', state: { wikiEntity: 'citra' }, entity: page({ suggestions: [merge] }) });
    const list = identity().querySelector('[aria-label="Sugestões de identidade"]');
    expect(list.textContent).toContain('Pode ser a mesma que Ceifador Faraday');
    expect(list.textContent).toContain('sem prova');
  });

  it('keeps two entities apart, in one click', async () => {
    await open({ role: 'admin', state: { wikiEntity: 'citra' }, entity: page({ suggestions: [merge] }) });
    await click(button('Manter distintas'));
    expect(api.post).toHaveBeenCalledWith(`/wiki/${PROJECT}/identity`, { action: 'distinct', entity: 'citra', other: 'faraday', suggestion: 1 });
  });

  it('asks again before joining, since it cannot be undone in this wiki, and goes to the page that stayed', async () => {
    await open({ role: 'admin', state: { wikiEntity: 'citra' }, entity: page({ suggestions: [merge] }) });
    api.post.mockResolvedValueOnce({ data: { slug: 'faraday' } });
    await click(button('Confirmar junção'));
    expect(api.post).not.toHaveBeenCalledWith(`/wiki/${PROJECT}/identity`, expect.anything());
    const dialog = container.querySelector('[role="alertdialog"]');
    expect(dialog.textContent).toContain('Para separar de novo é preciso gerar a wiki outra vez');
    await click([...dialog.querySelectorAll('button')].find((b) => b.textContent === 'Juntar'));
    expect(api.post).toHaveBeenCalledWith(`/wiki/${PROJECT}/identity`, { action: 'merge', entity: 'citra', other: 'faraday', suggestion: 1, label: 'Ceifador Faraday' });
    expect(useGlobalStore.getState().wikiEntity).toBe('faraday');
  });

  it('joins with an entity chosen from the index, and takes a mention away', async () => {
    await open({ role: 'admin', state: { wikiEntity: 'citra' } });
    await choose(select('Juntar com outra entidade'), 'rowan');
    await click([...identity().querySelectorAll('button')].find((b) => b.textContent === 'Juntar'));
    await click([...container.querySelectorAll('[role="alertdialog"] button')].find((b) => b.textContent === 'Juntar'));
    expect(api.post).toHaveBeenLastCalledWith(`/wiki/${PROJECT}/identity`, expect.objectContaining({ action: 'merge', entity: 'citra', other: 'rowan' }));
    await choose(select('Separar uma menção'), 'Citra');
    await click(button('Separar'));
    expect(api.post).toHaveBeenLastCalledWith(`/wiki/${PROJECT}/identity`, { action: 'mention', entity: 'citra', mention: 'Citra' });
  });

  it('lists what was decided and takes it back for the next generations', async () => {
    rules = [{ id: 4, kind: 'same', a: 'Bea', b: 'Beatriz' }, { id: 5, kind: 'mention', a: 'Luc', b: 'o Mestre dos Mares' }];
    await open({ role: 'admin', state: { wikiEntity: 'citra' } });
    const list = identity().querySelector('details');
    expect(list.textContent).toContain('Juntas: Bea e Beatriz');
    expect(list.textContent).toContain('“o Mestre dos Mares” não é de Luc');
    await click([...list.querySelectorAll('button')].find((b) => b.textContent === 'Esquecer'));
    expect(api.delete).toHaveBeenCalledWith(`/wiki/${PROJECT}/identity/4`);
  });
});

describe('WikiPage: written by hand (DEC-186)', () => {
  const panel = () => container.querySelector('[aria-labelledby="wiki-manual"]');
  const type = (el, value) => act(async () => {
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, value);
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
  const submit = async (form) => { await act(async () => { form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); }); await flush(); };

  it('is only for the staff', async () => {
    await open({ role: 'reader', state: { wikiEntity: 'faraday' }, entity: page({ slug: 'faraday', name: 'Ceifador Faraday', page: false, who: null }) });
    expect(panel()).toBeNull();
    await open({ role: 'admin', state: { wikiEntity: 'faraday' }, entity: page({ slug: 'faraday', name: 'Ceifador Faraday', page: false, who: null }) });
    expect(panel()).not.toBeNull();
    expect(button('Escrever a página')).toBeTruthy();
  });

  it('writes the page of who has none, from a chapter, one paragraph at a time', async () => {
    await open({ role: 'admin', state: { wikiEntity: 'faraday' }, entity: page({ slug: 'faraday', name: 'Ceifador Faraday', page: false, who: null }) });
    await click(button('Escrever a página'));
    const form = panel().querySelector('form');
    expect(form.querySelector('button[type="submit"]').disabled).toBe(true);
    await type(form.querySelector('textarea'), 'Faraday é um ceifador.\n\nÉ mestre de Citra.');
    await submit(form);
    expect(api.post).toHaveBeenCalledWith(`/wiki/${PROJECT}/manual`, {
      items: [{ kind: 'section', entity: 'faraday', book: 0, chapter: 4, data: { kind: 'who', part: '', paragraphs: ['Faraday é um ceifador.', 'É mestre de Citra.'] } }],
    });
  });

  it('records a moment and a tie, and ties it to another entity in the graph', async () => {
    await open({ role: 'admin', state: { wikiEntity: 'citra' } });
    await click(button('Registrar um momento'));
    await type(panel().querySelector('textarea'), 'Corta o cabelo.');
    await submit(panel().querySelector('form'));
    expect(api.post).toHaveBeenLastCalledWith(`/wiki/${PROJECT}/manual`, { items: [{ kind: 'state', entity: 'citra', book: 0, chapter: 4, data: { text: 'Corta o cabelo.' } }] });

    await click(button('Ligar a outra entidade'));
    const form = panel().querySelector('form');
    expect(form.querySelector('button[type="submit"]').disabled).toBe(true);
    const select = [...form.querySelectorAll('select')].find((x) => [...x.options].some((o) => o.value === 'rowan'));
    await act(async () => { select.value = 'rowan'; select.dispatchEvent(new Event('change', { bubbles: true })); });
    await type(form.querySelector('input'), 'colega de treino');
    await submit(form);
    expect(api.post).toHaveBeenLastCalledWith(`/wiki/${PROJECT}/manual`, {
      items: [{ kind: 'edge', entity: 'citra', other: 'rowan', book: 0, chapter: 4, data: { label: 'colega de treino' } }],
    });
  });

  it('lists what was written, corrects it and takes it away after asking', async () => {
    manual = [
      { id: 7, kind: 'state', entity: 'citra', data: { text: 'Corta o cabelo.' }, book: 0, chapter: 3, author: 'ana', createdAt: '2026-10-15T12:00:00Z' },
      { id: 8, kind: 'entity', entity: 'citra', data: { kind: 'person', aliases: [] }, book: 0, chapter: 1, author: 'ana', createdAt: '2026-10-15T12:00:00Z' },
    ];
    await open({ role: 'admin', state: { wikiEntity: 'citra' } });
    const list = panel().querySelector('[aria-label="Escrito à mão"]');
    expect(list.textContent).toContain('Momento · cap. 3 · por ana');
    expect(list.textContent).toContain('Corta o cabelo.');
    await click(button('Corrigir'));
    await type(panel().querySelector('textarea'), 'Corta o cabelo bem curto.');
    await submit(panel().querySelector('li form'));
    expect(api.put).toHaveBeenCalledWith(`/wiki/${PROJECT}/manual/7`, { book: 0, chapter: 3, data: { text: 'Corta o cabelo bem curto.' } });
    await click(button('Apagar'));
    expect(api.delete).not.toHaveBeenCalled();
    await click([...container.querySelectorAll('[role="alertdialog"] button')].find((b) => b.textContent === 'Apagar'));
    expect(api.delete).toHaveBeenCalledWith(`/wiki/${PROJECT}/manual/7`);
  });

  it('creates a new entity from the index, with its text and who took part, in one request', async () => {
    await open({ role: 'admin' });
    api.post.mockResolvedValue({ data: { ids: [1, 2, 3], slugs: ['o-baile', '', ''] } });
    await click(button('Nova entidade'));
    const form = container.querySelector('form[aria-label="Nova entidade"]');
    expect(form.querySelector('button[type="submit"]').disabled).toBe(true);
    await type(form.querySelector('input'), 'O Baile');
    await type(form.querySelector('textarea'), 'Um baile na praça.');
    const box = [...form.querySelectorAll('label')].find((l) => l.textContent.includes('Citra Terranova')).querySelector('input');
    await click(box);
    await submit(form);
    expect(api.post).toHaveBeenCalledWith(`/wiki/${PROJECT}/manual`, {
      items: [
        { kind: 'entity', name: 'O Baile', book: 0, chapter: 4, data: { kind: 'event', aliases: [] } },
        { kind: 'section', entity: '@0', book: 0, chapter: 4, data: { kind: 'who', part: '', paragraphs: ['Um baile na praça.'] } },
        { kind: 'edge', entity: '@0', other: 'citra', book: 0, chapter: 4, data: { label: 'participou' } },
      ],
    });
    expect(useGlobalStore.getState().wikiEntity).toBe('o-baile');
  });

  it('gives a reader no way to create an entity', async () => {
    await open({ role: 'reader' });
    expect(button('Nova entidade')).toBeUndefined();
  });
});
