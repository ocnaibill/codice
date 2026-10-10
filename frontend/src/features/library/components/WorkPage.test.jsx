import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkPage } from './WorkPage';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const work = {
  id: 7, title: 'Duna', author: 'Frank Herbert', coverUrl: '/covers/7.jpg', fileId: 10,
  metadata: { series: 'Crônicas de Duna', seriesIndex: 1, description: 'Uma sinopse.' },
  editions: [
    {
      id: 1, language: 'en', publisher: 'Ace', publicationDate: '1990', isPrimary: true,
      files: [{ id: 10, format: 'epub', sizeBytes: 3 * 1024 * 1024, availability: 'available', url: '/file/10', percentComplete: 40, completed: false, textStatus: 'ready', textSegments: 120 }],
    },
    {
      id: 2, language: 'pt-BR', publisher: 'Aleph', isPrimary: false,
      files: [
        { id: 20, format: 'pdf', availability: 'available', url: '/file/20', percentComplete: 0, completed: false, needsOcr: true },
        { id: 21, format: 'cbz', availability: 'missing', percentComplete: 0, completed: false },
        { id: 22, format: 'mp3', availability: 'available', url: '/file/22', percentComplete: 100, completed: true },
        { id: 23, format: 'txt', availability: 'available', url: '/file/23', percentComplete: 0, completed: false, started: true, textStatus: 'failed' },
      ],
    },
  ],
};

let container;
let root;

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const buttons = (text) => [...container.querySelectorAll('button')].filter((b) => b.textContent.trim() === text);
const rowOf = (format) => [...container.querySelectorAll('li')].find((li) => li.textContent.toLowerCase().includes(format));

async function open() {
  api.get.mockResolvedValue({ data: work });
  useGlobalStore.setState({ sheetWorkId: 7, activeBookId: null, activeFileId: null, fromStart: false });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => {
    root.render(
      <QueryClientProvider client={client}>
        <WorkPage />
      </QueryClientProvider>
    );
  });
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
  useGlobalStore.setState({ sheetWorkId: null, activeBookId: null, activeFileId: null, fromStart: false });
});

describe('WorkPage: a work that is only partly processed (RN-018)', () => {
  const file = (id, format, extra = {}) => ({ id, format, availability: 'available', url: `/file/${id}`, percentComplete: 0, completed: false, ...extra });
  const withFiles = (...files) => ({ ...work, editions: [{ id: 2, language: 'pt', isPrimary: true, files }] });
  const rows = () => [...container.querySelectorAll('li')].filter((li) => li.querySelector('button'));

  async function show(w) {
    api.get.mockResolvedValue({ data: w });
    useGlobalStore.setState({ sheetWorkId: 7, activeBookId: null, activeFileId: null, fromStart: false });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><WorkPage /></QueryClientProvider>); });
    await flush();
  }

  it('says the file can be opened while its text is still waiting to be read', async () => {
    await show(withFiles(file(30, 'pdf', { textStatus: '' })));
    expect(rows()[0].textContent).toContain('texto na fila');
    expect(rows()[0].textContent).toContain('Ler'); // readable now, searchable later
  });

  it('says a file read with no text has none to find, and one read with segments is indexed', async () => {
    await show(withFiles(file(31, 'epub', { textStatus: 'ready', textSegments: 0 }), file(32, 'txt', { textStatus: 'empty' }), file(33, 'md', { textStatus: 'ready', textSegments: 9 })));
    const text = rows().map((r) => r.textContent);
    expect(text[0]).toContain('nenhum texto pesquisável');
    expect(text[1]).toContain('sem texto pesquisável');
    expect(text[2]).toContain('texto indexado');
  });

  it('says nothing of text for a comic or an audiobook that has not been read, and nothing for a kind that has none', async () => {
    await show(withFiles(file(34, 'cbz', { textStatus: '' }), file(35, 'mp3', { textStatus: 'unsupported' })));
    for (const row of rows()) expect(row.textContent).not.toContain('texto');
  });

  it('says a PDF that asks for a password is that, in the place of "text not read", with a warning', async () => {
    await show(withFiles(file(40, 'pdf', { textStatus: 'failed', protected: true }), file(41, 'pdf', { textStatus: 'failed' })));
    expect(rows()[0].textContent).toContain('PDF com senha');
    expect(rows()[0].textContent).not.toContain('texto não lido');
    expect([...rows()[0].querySelectorAll('span')].find((s) => s.textContent === 'PDF com senha').className).toContain('text-warning');
    expect(rows()[0].textContent).toContain('Ler'); // it opens, with the password
    expect(rows()[1].textContent).toContain('texto não lido'); // one that only failed
    expect(rows()[1].textContent).not.toContain('PDF com senha');
  });

  it('leaves a scan to the note of the OCR and does not say it has no text twice', async () => {
    await show(withFiles(file(36, 'pdf', { textStatus: 'empty', needsOcr: true })));
    expect(rows()[0].textContent).toContain('Páginas sem texto');
    expect(rows()[0].textContent).not.toContain('sem texto pesquisável');
  });

  it('tells the tone of each: warning for a failure, plain for waiting, success for indexed', async () => {
    await show(withFiles(file(37, 'pdf', { textStatus: 'failed' }), file(38, 'epub', { textStatus: '' }), file(39, 'txt', { textStatus: 'ready', textSegments: 3 })));
    const chip = (i, text) => [...rows()[i].querySelectorAll('span')].find((s) => s.textContent === text);
    expect(chip(0, 'texto não lido').className).toContain('text-warning');
    expect(chip(1, 'texto na fila').className).toContain('text-ink-soft');
    expect(chip(2, 'texto indexado').className).toContain('text-success');
  });
});

describe('WorkPage: the pages of a scan (#24)', () => {
  const withOcr = (ocr, extra = {}) => ({
    ...work,
    editions: [{ id: 2, language: 'pt-BR', isPrimary: true, files: [{ id: 20, format: 'pdf', availability: 'available', url: '/file/20', percentComplete: 0, completed: false, needsOcr: true, ...(ocr ? { ocr } : {}), ...extra }] }],
  });
  const note = () => [...container.querySelectorAll('li span')].find((s) => /sem texto|OCR/.test(s.textContent));

  async function show(w) {
    api.get.mockResolvedValue({ data: w });
    useGlobalStore.setState({ sheetWorkId: 7, activeBookId: null, activeFileId: null, fromStart: false });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><WorkPage /></QueryClientProvider>); });
    await flush();
  }

  it('says the pages are being read, and how many', async () => {
    await show(withOcr({ pages: 300, read: 12, failed: 0, state: 'reading' }));
    expect(note().textContent).toBe('Lendo as páginas sem texto (12 de 300)');
    expect(container.textContent).not.toContain('OCR ainda não roda');
  });

  it('says they are waiting in line', async () => {
    await show(withOcr({ pages: 5, read: 0, failed: 0, state: 'queued' }));
    expect(note().textContent).toBe('Páginas sem texto na fila para leitura');
  });

  it('says the text was recognised, and that it may have mistakes', async () => {
    await show(withOcr({ pages: 5, read: 5, failed: 0 }));
    expect(note().textContent).toBe('Texto reconhecido por OCR');
    expect(note().getAttribute('title')).toContain('pode ter erros');
  });

  it('says some pages failed', async () => {
    await show(withOcr({ pages: 10, read: 7, failed: 3 }));
    expect(note().textContent).toBe('Texto reconhecido por OCR em 7 de 10 páginas; 3 falharam');
    expect(note().className).toContain('text-warning');
  });

  it('says the pages have no text when nothing was read, and says nothing for a file that has none without text', async () => {
    await show(withOcr({ pages: 5, read: 0, failed: 0 }));
    expect(note().textContent).toBe('Páginas sem texto');
    act(() => root.unmount());
    root = createRoot(container);
    await show(withOcr(undefined, { needsOcr: false }));
    expect(note()).toBeUndefined();
  });
});

describe('WorkPage', () => {
  it('shows nothing until a work is chosen', async () => {
    const client = new QueryClient();
    useGlobalStore.setState({ sheetWorkId: null });
    await act(async () => { root.render(<QueryClientProvider client={client}><WorkPage /></QueryClientProvider>); });
    expect(container.textContent).toBe('');
  });

  it('lists every edition with its language and every file with its own progress', async () => {
    await open();
    const text = container.textContent;
    expect(text).toContain('Duna');
    expect(text).toContain('Inglês · Ace · 1990');
    expect(text).toContain('Português (Brasil) · Aleph');
    expect(text).toContain('Principal');
    expect(rowOf('epub').textContent).toContain('40% lido');
    expect(rowOf('pdf').textContent).toContain('Não iniciado');
    expect(rowOf('mp3').textContent).toContain('Concluído');
    expect(rowOf('pdf').textContent).toContain('Páginas sem texto');
    // What became of the text, for the search: indexed, or not read.
    expect(rowOf('epub').textContent).toContain('texto indexado');
    expect(rowOf('txt').textContent).toContain('texto não lido');
    expect(rowOf('mp3').textContent).not.toContain('texto');
    // A position with no percentage (an EPUB or text viewer) is still a started file.
    expect(rowOf('txt').textContent).toContain('Em andamento');
    expect(rowOf('txt').querySelector('button').textContent).toBe('Continuar');
  });

  it("continues a file from its own position, and can start it over", async () => {
    await open();
    await act(async () => { buttons('Continuar')[0].click(); });
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 10, fromStart: false, sheetWorkId: null });

    await open();
    await act(async () => { buttons('Do começo')[0].click(); });
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 10, fromStart: true });
  });

  it('uses the current version for the prominent reading action', async () => {
    work.inProgress = true;
    work.continue = { fileId: 20, format: 'pdf', language: 'pt-BR', percentComplete: 12 };
    try {
      await open();
      await act(async () => { buttons('Continuar leitura')[0].click(); });
      expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 20, fromStart: false });
    } finally {
      delete work.inProgress; delete work.continue;
    }
  });

  it('does not promise continuation when the current file is unavailable', async () => {
    work.inProgress = true;
    work.continue = { fileId: 21, format: 'cbz', language: 'pt-BR' };
    try {
      await open();
      expect(buttons('Continuar leitura')).toHaveLength(0);
      await act(async () => { [...container.querySelectorAll('button')].find((b) => b.textContent.startsWith('Abrir leitor')).click(); });
      expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 10, fromStart: false });
    } finally {
      delete work.inProgress; delete work.continue;
    }
  });

  it('opens a file that was never read from its start, with no "from the beginning" option', async () => {
    await open();
    expect(rowOf('pdf').querySelector('button').textContent).toBe('Ler');
    expect(rowOf('pdf').textContent).not.toContain('Do começo');
    await act(async () => { rowOf('pdf').querySelector('button').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 20, fromStart: false });
  });

  it('does not offer to read a file that is gone', async () => {
    await open();
    const row = rowOf('cbz');
    expect(row.textContent).toContain('Arquivo ausente');
    expect(row.querySelectorAll('button').length).toBe(0);
  });

  it('marks a file finished, or reopens it, without opening it', async () => {
    api.put.mockResolvedValue({ data: {} });
    await open();
    await act(async () => { [...rowOf('pdf').querySelectorAll('button')].find((b) => b.textContent === 'Marcar como concluído').click(); });
    expect(api.put).toHaveBeenCalledWith('/progress/files/20/completion', { completed: true });
    expect(useGlobalStore.getState().activeBookId).toBeNull();

    // A file that is finished offers to reopen it instead.
    expect([...rowOf('mp3').querySelectorAll('button')].some((b) => b.textContent === 'Marcar como concluído')).toBe(false);
    await act(async () => { [...rowOf('mp3').querySelectorAll('button')].find((b) => b.textContent === 'Reabrir').click(); });
    expect(api.put).toHaveBeenLastCalledWith('/progress/files/22/completion', { completed: false });
  });

  it('says where the person is, how many times they finished it, and offers to finish the whole work', async () => {
    work.inProgress = true;
    work.continue = { fileId: 10, format: 'pdf', language: 'pt-BR', percentComplete: 42, completed: false };
    work.completions = { total: 3, byFormat: { epub: 2, pdf: 1 } };
    api.put.mockResolvedValue({ data: {} });
    try {
      await open();
      const summary = container.querySelector('[aria-label="Sua leitura"]').textContent;
      expect(summary).toContain('Você está em 42% no PDF (Português (Brasil))');
      expect(summary).toContain('Terminada 3 vezes: 2 em EPUB, 1 em PDF');
      await act(async () => { [...container.querySelectorAll('button')].find((b) => b.textContent === 'Marcar a obra toda como finalizada').click(); });
      expect(api.put).toHaveBeenCalledWith('/progress/works/7/finished', { finished: true });
    } finally {
      delete work.inProgress; delete work.continue; delete work.completions;
    }
  });

  it('shows a work marked as finished, and can undo it', async () => {
    work.finished = true;
    api.put.mockResolvedValue({ data: {} });
    try {
      await open();
      expect(container.querySelector('[aria-label="Sua leitura"]').textContent).toContain('marcou a obra toda como finalizada');
      await act(async () => { [...container.querySelectorAll('button')].find((b) => b.textContent === 'Desfazer').click(); });
      expect(api.put).toHaveBeenCalledWith('/progress/works/7/finished', { finished: false });
    } finally {
      delete work.finished;
    }
  });

  it('rereads a finished file: reopens it from scratch, then opens it from the start', async () => {
    api.put.mockResolvedValue({ data: {} });
    await open();
    await act(async () => { [...rowOf('mp3').querySelectorAll('button')].find((b) => b.textContent === 'Reler').click(); });
    await flush();
    expect(api.put).toHaveBeenCalledWith('/progress/files/22/completion', { completed: false, restart: true });
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 22, fromStart: true });
  });

  it('goes back without opening anything', async () => {
    await open();
    await act(async () => { buttons('← Voltar')[0].click(); });
    expect(useGlobalStore.getState()).toMatchObject({ sheetWorkId: null, activeBookId: null });
  });

  it('is a page: Escape belongs to the dialogs and menus over it, and does not leave it', async () => {
    await open();
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(useGlobalStore.getState().sheetWorkId).toBe(7);
  });

  it('says where it is: the library, the category of the work, and the work', async () => {
    api.get.mockResolvedValue({ data: { ...work, metadata: { ...work.metadata, categories: [{ id: 5, name: 'Seinen', path: 'Mangá › Seinen' }] } } });
    useGlobalStore.setState({ sheetWorkId: 7 });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><WorkPage /></QueryClientProvider>); });
    await flush();
    const crumbs = container.querySelector('nav[aria-label="Onde você está"]');
    expect(crumbs.textContent).toContain('Biblioteca');
    expect(crumbs.textContent).toContain('Mangá › Seinen');
    expect(crumbs.querySelector('[aria-current="page"]').textContent).toBe('Duna');
    await act(async () => { buttons('Mangá › Seinen')[0].click(); });
    expect(useGlobalStore.getState()).toMatchObject({ categoryPageId: 5, sheetWorkId: null });
  });
});

describe('WorkPage: the page of the work (DEC-149)', () => {
  async function show(extra = {}) {
    api.get.mockResolvedValue({ data: { ...work, ...extra } });
    api.post.mockResolvedValue({ data: {} });
    api.delete.mockResolvedValue({ data: {} });
    useGlobalStore.setState({ sheetWorkId: 7, activeBookId: null, activeFileId: null, fromStart: false });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><WorkPage /></QueryClientProvider>); });
    await flush();
  }
  const reading = {
    inProgress: true, formatCount: 2,
    continue: { fileId: 10, format: 'epub', language: 'en', position: 'epubcfi(/6/4)', percentComplete: 68, completed: false, chapter: 'Cap. 14: O Despertar', unitIndex: 4819, unitTotal: 7100 },
  };

  it('says where the person stopped: the chapter, how far through and which position of how many', async () => {
    await show(reading);
    const text = container.textContent;
    expect(text).toContain('Retomando:');
    expect(text).toContain('Cap. 14: O Despertar');
    expect(text).toContain('68% lido');
    expect(text).toContain('Pos. 4.819 de 7.100');
    expect(text).toContain('2 formatos disponíveis');
    expect(buttons('Continuar leitura')).toHaveLength(1);
  });

  it('says none of that of a work that was not begun', async () => {
    await show({ continue: null, inProgress: false, formatCount: 1 });
    const text = container.textContent;
    expect(text).not.toContain('Retomando');
    expect(text).not.toContain('formatos disponíveis');
    expect(container.querySelector('[aria-label="Seu progresso"]')).toBeNull();
  });

  it('names the format on the button that opens the reader, when there is nothing to continue', async () => {
    await show({ continue: null, inProgress: false });
    expect([...container.querySelectorAll('button')].map((b) => b.textContent)).toContain('Abrir leitor EPUB');
  });

  it('lists the tags of the work', async () => {
    await show({ tags: ['ficção científica', 'space opera'] });
    expect([...container.querySelectorAll('ul[aria-label="Etiquetas"] li')].map((li) => li.textContent)).toEqual(['#ficção científica', '#space opera']);
  });

  it('shortens a long synopsis and gives the rest on a press; a short one has nothing to open', async () => {
    await show({ metadata: { ...work.metadata, description: 'Muito longa. '.repeat(60) } });
    const more = buttons('Ler mais')[0];
    expect(more.getAttribute('aria-expanded')).toBe('false');
    await act(async () => { more.click(); });
    expect(buttons('Mostrar menos')[0].getAttribute('aria-expanded')).toBe('true');
    act(() => root.unmount());
    root = createRoot(container);
    await show();
    expect(buttons('Ler mais')).toHaveLength(0);
  });

  it('keeps the work among the favorites with the heart, and takes it out again', async () => {
    await show({ isFavorite: false });
    const heart = container.querySelector('button[aria-label="Adicionar aos favoritos"]');
    expect(heart.getAttribute('aria-pressed')).toBe('false');
    await act(async () => { heart.click(); });
    expect(api.post).toHaveBeenCalledWith('/works/7/favorite');
    act(() => root.unmount());
    root = createRoot(container);
    await show({ isFavorite: true });
    const filled = container.querySelector('button[aria-label="Remover dos favoritos"]');
    expect(filled.getAttribute('aria-pressed')).toBe('true');
    await act(async () => { filled.click(); });
    expect(api.delete).toHaveBeenCalledWith('/works/7/favorite');
  });

  it('clamps a long synopsis to six lines until it is opened', async () => {
    await show({ metadata: { ...work.metadata, description: 'Muito longa. '.repeat(60) } });
    const text = () => container.querySelector('p.whitespace-pre-line');
    expect(text().style.webkitLineClamp).toBe('6');
    await act(async () => { buttons('Ler mais')[0].click(); });
    expect(text().style.webkitLineClamp).toBe('');
  });

  it('does not promise the chapter or the place when the file it was in cannot be opened any more', async () => {
    const gone = {
      ...reading,
      editions: [
        { id: 1, isPrimary: true, language: 'en', files: [{ id: 10, format: 'epub', availability: 'missing', percentComplete: 68, completed: false }] },
        { id: 2, isPrimary: false, language: 'pt', files: [{ id: 20, format: 'pdf', availability: 'available', url: '/file/20', percentComplete: 0, completed: false }] },
      ],
    };
    await show(gone);
    const text = container.textContent;
    expect(text).not.toContain('Retomando');
    expect(text).not.toContain('Pos. 4.819');
    expect(buttons('Continuar leitura')).toHaveLength(0);
    expect(text).toContain('68% lido'); // how far it went is still true
  });

  it('says a finished file is finished, and not a percentage', async () => {
    await show({ ...reading, inProgress: false, continue: { ...reading.continue, completed: true, percentComplete: 100 } });
    const progress = container.querySelector('[aria-label="Seu progresso"]');
    expect(progress.textContent).toContain('Concluído');
    expect(progress.textContent).not.toContain('100% lido');
  });

  it('has no list of tags for a work that has none', async () => {
    await show({ tags: [] });
    expect(container.querySelector('ul[aria-label="Etiquetas"]')).toBeNull();
  });

  it('goes back to what was under it, with the page of the collection it came from still open', async () => {
    await show();
    useGlobalStore.setState({ collectionSheetId: 3, libraryView: 'ebooks' });
    await act(async () => { buttons('← Voltar')[0].click(); });
    expect(useGlobalStore.getState()).toMatchObject({ sheetWorkId: null, collectionSheetId: 3, libraryView: 'ebooks' });
    useGlobalStore.setState({ collectionSheetId: null, libraryView: 'all' });
  });

  it('says the year of a publication date that was kept whole, and a date that is not one as it is', async () => {
    await show({
      editions: [
        { id: 1, language: 'pt', publisher: 'Seguinte', publicationDate: '2018-05-25T03:00:00+00:00', isPrimary: true, files: [{ id: 10, format: 'epub', availability: 'available', url: '/f/10', percentComplete: 0, completed: false }] },
        { id: 2, language: 'en', publisher: 'Ace', publicationDate: 'primavera de 1990', isPrimary: false, files: [] },
      ],
    });
    const chips = [...container.querySelectorAll('[aria-label="Dados da edição em foco"] span')].map((c) => c.textContent);
    expect(chips).toContain('2018');
    expect(container.textContent).not.toContain('T03:00:00');
    expect(container.textContent).toContain('primavera de 1990');
  });

  it('shows the table of contents of the file it leads with and opens the reader at a chapter', async () => {
    const outline = {
      fileId: 10, current: 1,
      chapters: [
        { title: 'Capítulo 1', depth: 0, part: 'body', hasChildren: false, locator: { type: 'epub', href: 'c1.xhtml' }, percent: 0 },
        { title: 'Capítulo 2', depth: 0, part: 'body', hasChildren: false, locator: { type: 'epub', href: 'c2.xhtml' }, percent: 40 },
      ],
    };
    api.get.mockImplementation(async (url) => ({ data: url.endsWith('/outline') ? outline : { ...work } }));
    useGlobalStore.setState({ sheetWorkId: 7, activeBookId: null, activeFileId: null, fromStart: false });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><WorkPage /></QueryClientProvider>); });
    await flush();
    await flush(); // the outline is asked for when the work has arrived
    expect(api.get).toHaveBeenCalledWith('/progress/files/10/outline');
    const section = container.querySelector('section[aria-label="Sumário da obra"]');
    expect(section.textContent).toContain('2 capítulos');
    await act(async () => { [...section.querySelectorAll('button')].find((b) => b.textContent.includes('Capítulo 1')).click(); });
    expect(useGlobalStore.getState()).toMatchObject({ activeBookId: 7, activeFileId: 10, fromStart: false });
    expect(useGlobalStore.getState().seek).toMatchObject({ locator: { type: 'epub', href: 'c1.xhtml' }, context: { kind: 'chapter', quote: 'Capítulo 1' } });
  });

  it('asks for no table of contents of a file that has no text to have one (a comic, an audiobook)', async () => {
    api.get.mockResolvedValue({ data: { ...work, editions: [{ id: 1, isPrimary: true, language: 'en', files: [{ id: 30, format: 'cbz', availability: 'available', url: '/f/30', percentComplete: 0, completed: false }] }] } });
    useGlobalStore.setState({ sheetWorkId: 7 });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    api.get.mockClear();
    await act(async () => { root.render(<QueryClientProvider client={client}><WorkPage /></QueryClientProvider>); });
    await flush();
    expect(api.get.mock.calls.map((c) => c[0]).filter((u) => String(u).includes('outline'))).toEqual([]);
    expect(container.querySelector('section[aria-label="Sumário da obra"]')).toBeNull();
  });

  it('is left by a search: the results are what the person asked for', () => {
    useGlobalStore.setState({ sheetWorkId: 7, searchQuery: '' });
    useGlobalStore.getState().setSearchQuery('duna');
    expect(useGlobalStore.getState().sheetWorkId).toBeNull();
    useGlobalStore.setState({ sheetWorkId: 7 });
    useGlobalStore.getState().setSearchQuery('   '); // nothing typed is no search
    expect(useGlobalStore.getState().sheetWorkId).toBe(7);
    useGlobalStore.getState().setSearchQuery('');
  });
});
