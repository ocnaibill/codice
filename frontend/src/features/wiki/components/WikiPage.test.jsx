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
    throw new Error(`unexpected GET ${url}`);
  });
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
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
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
    expect(text()).toContain('falta revisar o que mudou');
  });
});

describe('WikiPage: the page of an entity', () => {
  it('shows the situation, "Quem é" with its point, the biography of the parts read and the part in progress', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    expect(container.querySelector('h1').textContent).toBe('Citra Terranova');
    expect(text()).toContain('“Aprendiz de Faraday, treina na casa dele.”');
    expect(text()).toContain('atualizado ao fim de O Ceifador · Título 2');
    expect(text()).toContain('Citra é aprendiz de ceifador.');
    expect(text()).toContain('Faraday a escolhe como aprendiz.');
    expect(text()).toContain('Parte 2: em andamento');
    const current = container.querySelector('[aria-current="step"]');
    expect(current.textContent).toContain('Título 4');
    expect(current.textContent).toContain('ponto atual');
    expect(text()).toContain('Capítulos 5 a 10 ocultos');
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

  it('shows the identity suggestions only when the server sends them (to staff)', async () => {
    await open({ state: { wikiEntity: 'citra' } });
    expect(text()).not.toContain('Identidade a conferir');
    await open({ role: 'admin', state: { wikiEntity: 'citra' }, entity: page({ suggestions: [{ id: 1, kind: 'merge', entity: 'citra', other: 'ceifadora-anastasia', otherName: 'Ceifadora Anastásia', evidence: 'Cap. 30: o nome dado na ordenação.' }] }) });
    expect(text()).toContain('Pode ser a mesma entidade que Ceifadora Anastásia');
    expect(text()).toContain('Cap. 30: o nome dado na ordenação.');
  });

  it('says an entity beyond the point is not there, without telling more', async () => {
    const notFound = Object.assign(new Error('404'), { response: { status: 404 } });
    await open({ state: { wikiEntity: 'vilao' }, entity: notFound });
    expect(text()).toContain('Esta página não existe, ou fica além do ponto em que você está.');
  });

  it('goes back to the reader where the person was, when it came from there', async () => {
    await open({ state: { wikiEntity: 'citra', wikiFrom: { workId: WORK, fileId: 30, label: 'Capítulo 4' } } });
    expect(text()).toContain('Você veio do leitor · Capítulo 4');
    await click(button('Retomar leitura no parágrafo'));
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: WORK, activeFileId: 30, seek: null });
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
