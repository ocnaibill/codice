import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useToasts } from '../../../components/ui/toast';
import { EditBookModal } from './EditBookModal';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let onClose;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...document.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const click = async (el) => { await act(async () => { el.click(); }); await flush(); };
// the panel has its own "Editora", and so has the form: only the panel's labels
const box = (label) => [...document.querySelectorAll('[aria-label="Levar para a série"] label')].find((l) => l.textContent.trim() === label)?.querySelector('input');
const shareCalls = () => api.post.mock.calls.filter(([url]) => url === '/collections/5/share');

async function open({ seriesCollectionId = 5 } = {}) {
  const metadata = { firstAuthor: 'Kentaro Miura', locks: {}, sources: {}, alternativeTitles: [], seriesCollectionId, publisher: 'Panini' };
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: { id: 7, title: 'Berserk Vol. 1', author: 'Kentaro Miura', tags: [], format: 'cbz', coverUrl: '/c.jpg', editions: [], metadata } };
    if (url === '/works/7/candidates') return { data: { data: [] } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: {} });
  api.post.mockImplementation(async (url, body) => ({ data: body.preview ? { preview: true, works: 38, worksChanged: 26, changed: { publisher: 26 }, kept: { publisher: 12 }, sourceHasNone: [] } : { preview: false, works: 38, worksChanged: 26, changed: { publisher: 26 }, kept: {}, sourceHasNone: [] } }));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  onClose = vi.fn();
  await act(async () => { root.render(<QueryClientProvider client={client}><EditBookModal workId={7} tab="edit" onClose={onClose} /></QueryClientProvider>); });
  await flush();
  await flush();
}

beforeEach(() => {
  vi.clearAllMocks();
  useToasts.getState().clear();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.setState({ sheetWorkId: 7 });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('EditBookModal, taking data to the series (DEC-203)', () => {
  it('offers it only to a work that is in a series', async () => {
    await open({ seriesCollectionId: null });
    expect(document.querySelector('[aria-label="Levar para a série"]')).toBeNull();
    act(() => root.unmount());
    root = createRoot(container);
    await open();
    expect(document.querySelector('[aria-label="Levar para a série"]')).not.toBeNull();
  });

  it('shows where and how only once a field is chosen, with the safe way first', async () => {
    await open();
    expect(document.querySelector('[aria-label="Para quais obras"]')).toBeNull();
    await click(box('Editora'));
    expect(box('Preencher só onde está em branco').checked).toBe(true);
    expect(box('Todas as obras').checked).toBe(true);
    expect(box('Substituir em todas').checked).toBe(false);
  });

  it('saves the work and nothing is sent to the series when nothing was chosen', async () => {
    await open();
    await click(button('Salvar'));
    expect(api.put).toHaveBeenCalledWith('/works/7', expect.any(Object));
    expect(shareCalls()).toHaveLength(0);
    expect(onClose).toHaveBeenCalled();
  });

  it('saves first, shows what the preview says, and only then hands it on', async () => {
    await open();
    await click(box('Editora'));
    await click(box('Só volumes'));
    await click(box('Substituir em todas'));
    await click(button('Salvar'));
    expect(api.put).toHaveBeenCalled();
    expect(shareCalls()).toHaveLength(1);
    expect(shareCalls()[0][1]).toEqual({ sourceWorkId: 7, fields: ['publisher'], units: ['volume'], mode: 'replace', preview: true });
    expect(document.body.textContent).toContain('26 obras mudam, de 38 outras da série.');
    expect(onClose).not.toHaveBeenCalled();

    await click(button('Levar para a série'));
    expect(shareCalls()).toHaveLength(2);
    expect(shareCalls()[1][1].preview).toBe(false);
    expect(useToasts.getState().items[0].title).toBe('Levado para a série');
    expect(onClose).toHaveBeenCalled();
  });

  it('closes without handing anything on when the person says not now', async () => {
    await open();
    await click(box('Editora'));
    await click(button('Salvar'));
    await click(button('Agora não'));
    expect(shareCalls()).toHaveLength(1); // only the preview
    expect(onClose).toHaveBeenCalled();
  });

  it('offers nothing to apply when no work would change', async () => {
    await open();
    api.post.mockResolvedValue({ data: { preview: true, works: 3, worksChanged: 0, changed: {}, kept: { publisher: 3 }, sourceHasNone: [] } });
    await click(box('Editora'));
    await click(button('Salvar'));
    expect(document.body.textContent).toContain('Nenhuma das 3 outras obras da série muda.');
    expect(button('Levar para a série')).toBeUndefined();
    expect(button('Fechar')).toBeTruthy();
  });
});

describe('EditBookModal, what the file contains (DEC-202)', () => {
  const field = (label) => document.querySelector(`input[aria-label="${label}"]`);
  const range = (label) => [...document.querySelectorAll('[aria-label="O que este arquivo contém"] label')].find((l) => l.textContent.trim().startsWith(label))?.querySelector('input');
  const typeInto = async (input, value) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, value);
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
  };

  async function openVolume(extra = {}) {
    const metadata = { firstAuthor: 'Kentaro Miura', locks: {}, sources: {}, alternativeTitles: [], seriesCollectionId: 5, unit: 'volume', seriesIndex: 3, ...extra };
    api.get.mockImplementation(async (url) => {
      if (url === '/works/7') return { data: { id: 7, title: 'Berserk Vol. 3', author: 'Kentaro Miura', tags: [], format: 'cbz', coverUrl: '/c.jpg', editions: [], metadata } };
      if (url === '/works/7/candidates') return { data: { data: [] } };
      if (url === '/collections/5') {
        return { data: { collection: { id: 5 }, works: [
          { id: 1, unit: 'chapter', position: 18, volumeNumber: 3 }, { id: 2, unit: 'chapter', position: 26, volumeNumber: 3 }, { id: 3, unit: 'chapter', position: 27, volumeNumber: 4 },
        ] } };
      }
      throw new Error(`unexpected GET ${url}`);
    });
    api.put.mockResolvedValue({ data: {} });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><EditBookModal workId={7} tab="edit" onClose={vi.fn()} /></QueryClientProvider>); });
    await flush();
    await flush();
  }

  it('asks for it only of a work that has a unit', async () => {
    await open();
    expect(document.querySelector('[aria-label="O que este arquivo contém"]')).toBeNull();
  });

  it('suggests the range of a volume from the chapters that say they are in it, and takes it with one click', async () => {
    await openVolume();
    expect(document.body.textContent).toContain('2 obras da série já dizem “volume 3”: capítulos 18 a 26.');
    await click(button('Usar 18 a 26'));
    expect(range('Do capítulo').value).toBe('18');
    expect(range('Ao capítulo').value).toBe('26');
    expect(document.body.textContent).toContain('= 9 capítulos');
    expect(button('Usar 18 a 26')).toBeUndefined(); // it is what is written now
    await click(button('Salvar'));
    expect(api.put.mock.calls[0][1]).toMatchObject({ covers_from: '18', covers_to: '26' });
  });

  it('gives the last chapter from the first and the quantity', async () => {
    await openVolume();
    await typeInto(range('Do capítulo'), '10');
    await typeInto(field('Quantidade de capítulos'), '5');
    expect(range('Ao capítulo').value).toBe('14');
  });

  it('keeps what the work already says', async () => {
    await openVolume({ coversFrom: 4, coversTo: 6 });
    expect(range('Do capítulo').value).toBe('4');
    expect(range('Ao capítulo').value).toBe('6');
  });

  it('sends both ends empty when there is none, which takes the range away', async () => {
    await openVolume();
    await click(button('Salvar'));
    expect(api.put.mock.calls.at(-1)[1]).toMatchObject({ covers_from: '', covers_to: '' });
  });
});
