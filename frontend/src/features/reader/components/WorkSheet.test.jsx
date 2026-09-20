import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkSheet } from './WorkSheet';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const work = {
  id: 7, title: 'Duna', author: 'Frank Herbert', coverUrl: '/covers/7.jpg', fileId: 10,
  metadata: { series: 'Crônicas de Duna', seriesIndex: 1, description: 'Uma sinopse.' },
  editions: [
    {
      id: 1, language: 'en', publisher: 'Ace', publicationDate: '1990', isPrimary: true,
      files: [{ id: 10, format: 'epub', sizeBytes: 3 * 1024 * 1024, availability: 'available', url: '/file/10', percentComplete: 40, completed: false }],
    },
    {
      id: 2, language: 'pt-BR', publisher: 'Aleph', isPrimary: false,
      files: [
        { id: 20, format: 'pdf', availability: 'available', url: '/file/20', percentComplete: 0, completed: false, needsOcr: true },
        { id: 21, format: 'cbz', availability: 'missing', percentComplete: 0, completed: false },
        { id: 22, format: 'mp3', availability: 'available', url: '/file/22', percentComplete: 100, completed: true },
        { id: 23, format: 'txt', availability: 'available', url: '/file/23', percentComplete: 0, completed: false, started: true },
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
        <WorkSheet />
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

describe('WorkSheet', () => {
  it('shows nothing until a work is chosen', async () => {
    const client = new QueryClient();
    useGlobalStore.setState({ sheetWorkId: null });
    await act(async () => { root.render(<QueryClientProvider client={client}><WorkSheet /></QueryClientProvider>); });
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

  it('closes without opening anything', async () => {
    await open();
    await act(async () => { container.querySelector('[aria-label="Fechar"]').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ sheetWorkId: null, activeBookId: null });
  });
});
