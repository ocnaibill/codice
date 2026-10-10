import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { EditBookModal } from './EditBookModal';
import { useToasts } from '../../../components/ui/toast';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const record = {
  id: 7, title: 'Duna', author: 'Herbert, Frank; Brian Herbert', tags: ['Sci-Fi'], format: 'epub',
  metadata: {
    firstAuthor: 'Frank Herbert',
    series: 'Crônicas de Duna', seriesIndex: 1, isbn: '9788576573135', publisher: 'Aleph',
    language: 'pt', publicationDate: '2017', description: 'Sinopse original',
    locks: { title: true, publisher: false }, sources: {},
  },
};
const isbn = { id: 3, field: 'isbn', value: '111', source: 'Open Library', current: '', keys: [] };

let container;
let root;
let onClose;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const tab = (name) => [...container.querySelectorAll('[role="tab"]')].find((t) => t.textContent.startsWith(name));
const button = (text, within = document.body) => [...within.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const input = (value) => [...container.querySelectorAll('input, textarea')].find((i) => i.value === value);
const click = async (el) => { await act(async () => { el.click(); }); await flush(); };
const lock = (label) => [...container.querySelector('fieldset').querySelectorAll('label')].find((l) => l.textContent.trim() === label).querySelector('input');
const authorInput = () => [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith('Primeiro autor')).querySelector('input');
const setValue = (el, value) => act(async () => {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, value);
  el.dispatchEvent(new Event('input', { bubbles: true }));
});
const key = (k) => act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true })); });

async function render({ candidates = [], tab: initial, work = record } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: work };
    if (url === '/works/7/candidates') return { data: { data: candidates } };
    if (url === '/admin/works/7/metadata-refresh') return { data: { job: null } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({});
  api.post.mockResolvedValue({});
  api.delete.mockResolvedValue({});
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  await act(async () => {
    root.render(<QueryClientProvider client={client}><EditBookModal workId={7} tab={initial} onClose={onClose} /></QueryClientProvider>);
  });
  await flush();
  await flush();
}

beforeEach(() => {
  vi.clearAllMocks();
  onClose = vi.fn();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.setState({ sheetWorkId: 7 });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  useGlobalStore.setState({ sheetWorkId: null });
});

describe('EditBookModal: the metadata of a work (#70)', () => {
  it('puts the button that searches the providers again on the suggestions, above them, and not on the other tabs', async () => {
    await render({ candidates: [isbn] });
    const search = button('Buscar metadados de novo');
    expect(search).toBeTruthy();
    expect(container.querySelector('section[aria-label="Sugestões dos provedores"]').compareDocumentPosition(search) & Node.DOCUMENT_POSITION_PRECEDING).toBeTruthy();
    await click(tab('Editar'));
    expect(button('Buscar metadados de novo')).toBeUndefined();
  });

  it('opens on the suggestions, says how many wait, and says so when none does', async () => {
    await render({ candidates: [isbn] });
    expect(tab('Sugestões (1)').getAttribute('aria-selected')).toBe('true');
    expect(container.textContent).toContain('Duna');
    expect(container.textContent).toContain('111');
    act(() => root.unmount());
    root = createRoot(container);
    await render({ candidates: [] });
    expect(tab('Sugestões').textContent).toBe('Sugestões');
    expect(container.textContent).toContain('Nenhuma sugestão esperando decisão.');
  });

  it('opens on the part it was asked for and moves between the two', async () => {
    await render({ candidates: [isbn], tab: 'edit' });
    expect(tab('Editar').getAttribute('aria-selected')).toBe('true');
    expect(input('Aleph')).toBeTruthy();
    expect(container.textContent).not.toContain('Nada muda até você aceitar');
    await click(tab('Sugestões'));
    expect(container.textContent).toContain('Nada muda até você aceitar');
    expect(input('Aleph')).toBeUndefined();
  });

  it('decides a suggestion through the API', async () => {
    await render({ candidates: [isbn] });
    await click(button('Aceitar', container));
    expect(api.post).toHaveBeenCalledWith('/works/7/candidates/3/accept');
  });

  it('starts the form from the full record, with the locks as they are', async () => {
    await render({ tab: 'edit' });
    for (const v of ['Duna', 'Frank Herbert', 'Crônicas de Duna', '9788576573135', 'Aleph', 'pt', '2017', 'Sinopse original', 'Sci-Fi']) {
      expect(input(v), v).toBeTruthy();
    }
    expect(lock('Título').checked).toBe(true);
    expect(lock('Editora').checked).toBe(false);
  });

  it('saves without erasing what was not touched, and what was changed goes with its locks', async () => {
    await render({ tab: 'edit' });
    await click(button('Salvar'));
    expect(api.put).toHaveBeenCalledTimes(1);
    const [url, body] = api.put.mock.calls[0];
    expect(url).toBe('/works/7');
    expect(body).toMatchObject({
      title: 'Duna', series: 'Crônicas de Duna', series_index: 1, isbn: '9788576573135',
      publisher: 'Aleph', language: 'pt', publication_date: '2017', description: 'Sinopse original', tags: ['Sci-Fi'],
      title_lock: true, publisher_lock: false, cover_lock: false, language_lock: false, publication_date_lock: false,
    });
    expect(body).not.toHaveProperty('author');
    expect(onClose).toHaveBeenCalled();
  });

  it('writes the year the work was first published, apart from the date of the edition, and locks it', async () => {
    await render({ tab: 'edit', work: { ...record, metadata: { ...record.metadata, originalYear: 1965, locks: { ...record.metadata.locks, original_year: false } } } });
    const year = [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith('Ano da primeira publicação')).querySelector('input');
    expect(year.value).toBe('1965');
    expect(input('2017')).toBeTruthy(); // the date of the edition is its own field
    await setValue(year, '1964');
    await act(async () => { lock('Ano original').click(); });
    await click(button('Salvar'));
    expect(api.put.mock.calls[0][1]).toMatchObject({ original_year: '1964', original_year_lock: true, publication_date: '2017' });
  });

  it('leaves the year empty when nobody knows it, and sends it empty (which clears)', async () => {
    await render({ tab: 'edit' });
    const year = [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith('Ano da primeira publicação')).querySelector('input');
    expect(year.value).toBe('');
    await click(button('Salvar'));
    expect(api.put.mock.calls[0][1]).toMatchObject({ original_year: '', original_year_lock: false });
  });

  it('starts the author from the first author as it is stored, never from what the sheet shows for the work', async () => {
    await render({ tab: 'edit' });
    expect(input('Frank Herbert')).toBeTruthy();
    expect(input('Herbert, Frank; Brian Herbert')).toBeUndefined();
  });

  it('sends the author only when it was changed, and then only that name', async () => {
    await render({ tab: 'edit' });
    await setValue(authorInput(), 'Franklin Herbert');
    await click(button('Salvar'));
    expect(api.put.mock.calls[0][1].author).toBe('Franklin Herbert');
  });

  it('sends what was edited: tags split by commas and trimmed, the lock that was ticked, the series number as a number', async () => {
    await render({ tab: 'edit' });
    const set = async (el, value) => act(async () => {
      const proto = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement : HTMLInputElement;
      Object.getOwnPropertyDescriptor(proto.prototype, 'value').set.call(el, value);
      el.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await set(input('Sci-Fi'), ' Fantasia ,, Humor ');
    await set(input('1'), '2.5');
    await act(async () => { lock('Editora').click(); lock('Capa').click(); lock('Data').click(); });
    await click(button('Salvar'));
    expect(api.put.mock.calls[0][1]).toMatchObject({ tags: ['Fantasia', 'Humor'], series_index: 2.5, publisher_lock: true, cover_lock: true, publication_date_lock: true });
  });

  it('does not ask for an author: a work may have none, and one with none sends none', async () => {
    await render({ tab: 'edit', work: { ...record, author: 'Unknown Author', metadata: { ...record.metadata, firstAuthor: 'Unknown Author' } } });
    expect(input('Unknown Author')).toBeUndefined();
    await click(button('Salvar'));
    expect(api.put.mock.calls[0][1]).not.toHaveProperty('author');
  });

  it('lets the author be cleared', async () => {
    await render({ tab: 'edit' });
    await setValue(authorInput(), '');
    await click(button('Salvar'));
    expect(api.put.mock.calls[0][1].author).toBe('');
  });

  it('has no form to save before the record has loaded', async () => {
    let release;
    api.get.mockImplementation((url) => {
      if (url === '/works/7') return new Promise((res) => { release = () => res({ data: record }); });
      return Promise.resolve({ data: { data: [] } });
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><EditBookModal workId={7} tab="edit" onClose={onClose} /></QueryClientProvider>); });
    expect(button('Salvar')).toBeUndefined();
    expect(container.textContent).toContain('Carregando a obra');
    await act(async () => { release(); });
    await flush();
    expect(button('Salvar')).toBeTruthy();
  });

  it('shows what the server said when saving fails, and keeps the modal open', async () => {
    await render({ tab: 'edit' });
    api.put.mockRejectedValue({ response: { data: 'Title is required' } });
    await click(button('Salvar'));
    expect(container.querySelector('[role="alert"]').textContent).toBe('Informe o título.');
    expect(onClose).not.toHaveBeenCalled();
  });

  it('retires only after a confirmation, then closes the sheet and the modal', async () => {
    await render({ tab: 'edit' });
    await click(button('Retirar do acervo'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain('pode ser restaurada na Lixeira');
    await click(button('Cancelar', document.querySelector('[aria-label="Retirar esta obra do acervo?"]')));
    expect(api.delete).not.toHaveBeenCalled();

    await click(button('Retirar do acervo'));
    await click(button('Retirar do acervo', document.querySelector('[aria-label="Retirar esta obra do acervo?"]')));
    expect(api.delete).toHaveBeenCalledWith('/works/7');
    expect(onClose).toHaveBeenCalled();
    expect(useGlobalStore.getState().sheetWorkId).toBeNull();
  });

  it('says where the retired work went, and the notice takes the person to the trash of the administration', async () => {
    useToasts.getState().clear();
    useGlobalStore.setState({ adminOpen: false, adminTab: 'jobs' });
    await render({ tab: 'edit' });
    await click(button('Retirar do acervo'));
    await click(button('Retirar do acervo', document.querySelector('[aria-label="Retirar esta obra do acervo?"]')));
    const [notice] = useToasts.getState().items;
    expect(notice.tone).toBe('success');
    expect(notice.title).toBe('Obra retirada');
    expect(notice.message).toContain('está guardada na Lixeira, em Administração');
    expect(notice.action.label).toBe('Ver na lixeira');
    await act(async () => { notice.action.onClick(); });
    expect(useGlobalStore.getState()).toMatchObject({ adminOpen: true, adminTab: 'trash' });
  });

  it('says nothing of the trash when the retirement did not work', async () => {
    useToasts.getState().clear();
    api.delete.mockRejectedValueOnce({ response: { status: 500, data: 'x' } });
    await render({ tab: 'edit' });
    await click(button('Retirar do acervo'));
    await click(button('Retirar do acervo', document.querySelector('[aria-label="Retirar esta obra do acervo?"]')));
    expect(useToasts.getState().items).toHaveLength(0);
  });

  it('closes on Escape, but a confirmation on top takes the Escape for itself', async () => {
    await render({ tab: 'edit' });
    await click(button('Retirar do acervo'));
    await key('Escape');
    expect(document.querySelector('[aria-label="Retirar esta obra do acervo?"]')).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
    await key('Escape');
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('says so when the work cannot be opened', async () => {
    api.get.mockImplementation(async (url) => { if (url === '/works/7') throw new Error('gone'); return { data: { data: [] } }; });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><EditBookModal workId={7} onClose={onClose} /></QueryClientProvider>); });
    await flush();
    expect(container.textContent).toContain('Não foi possível abrir esta obra.');
  });

  it('says what a comic or manga work is, by hand, and saves it with the rest (#187)', async () => {
    await render({ tab: 'edit' });
    const unit = [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith('Unidade na série')).querySelector('select');
    const kind = [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith('Quadrinho ou mangá')).querySelector('select');
    expect([...unit.options].map((o) => [o.value, o.textContent])).toEqual([['', 'Não informada'], ['volume', 'Volume'], ['chapter', 'Capítulo'], ['oneshot', 'Único']]);
    expect([...kind.options].map((o) => [o.value, o.textContent])).toEqual([['', 'Não informado'], ['manga', 'Mangá'], ['comic', 'Quadrinho']]);
    expect(unit.value).toBe('');
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(unit, 'chapter');
      unit.dispatchEvent(new Event('change', { bubbles: true }));
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(kind, 'manga');
      kind.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await click(button('Salvar'));
    expect(api.put).toHaveBeenCalledWith('/works/7', expect.objectContaining({ unit: 'chapter', comic_kind: 'manga' }));
  });

  it('starts from what the work already is', async () => {
    await render({ tab: 'edit', work: { ...record, metadata: { ...record.metadata, unit: 'volume', comicKind: 'comic' } } });
    const select = (label) => [...container.querySelectorAll('label')].find((l) => l.textContent.startsWith(label)).querySelector('select');
    expect(select('Unidade na série').value).toBe('volume');
    expect(select('Quadrinho ou mangá').value).toBe('comic');
    await click(button('Salvar'));
    expect(api.put).toHaveBeenCalledWith('/works/7', expect.objectContaining({ unit: 'volume', comic_kind: 'comic' }));
  });
});
