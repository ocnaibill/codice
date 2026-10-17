import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }));

import { api } from '../../../lib/api';
import { mount, flush } from '../../admin/testUtils';
import { ImportNotes } from './ImportNotes';

const PREVIEW = {
  format: 'kindle',
  entries: 5,
  skipped: { bookmarks: 1 },
  groups: [
    { key: 'duna|herbert frank', title: 'Duna', author: 'Frank Herbert', highlights: 3, notes: 1, suggested: 7, candidates: [{ workId: 7, title: 'Duna', author: 'Herbert, Frank', score: 4 }] },
    { key: 'rosa|eco', title: 'O Nome da Rosa', author: 'Eco', highlights: 1, notes: 0, suggested: 0, candidates: [{ workId: 8, title: 'O Nome da Rosa', author: 'Umberto Eco', score: 2 }, { workId: 9, title: 'O Nome da Rosa (HQ)', author: '', score: 1 }] },
    { key: 'x|y', title: 'Outro', author: 'Fulano', highlights: 1, notes: 0, suggested: 0, candidates: [] },
  ],
};

let view;
let onClose;
async function open() {
  onClose = vi.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  view = await mount(<QueryClientProvider client={client}><ImportNotes onClose={onClose} /></QueryClientProvider>);
}
const pick = async (name = 'My Clippings.txt') => {
  const input = document.body.querySelector('input[type="file"]');
  const file = new File(['x'], name, { type: 'text/plain' });
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  await act(async () => { input.dispatchEvent(new Event('change', { bubbles: true })); });
  await flush();
  return file;
};
const select = (title) => [...document.body.querySelectorAll('li')].find((li) => li.textContent.includes(title)).querySelector('select');
const choose = (el, value) => act(async () => {
  el.value = value;
  el.dispatchEvent(new Event('change', { bubbles: true }));
});

beforeEach(() => {
  vi.clearAllMocks();
  api.post.mockImplementation(async (url) => (url === '/notes/import/preview' ? { data: PREVIEW } : { data: { created: 4, duplicates: 1, located: 3, unlocated: 1, ignored: 1 } }));
});
afterEach(() => view.unmount());

describe('ImportNotes', () => {
  it('says how to get the file of each device, and reads nothing before one is chosen', async () => {
    await open();
    const text = view.text();
    expect(text).toContain('My Clippings.txt');
    expect(text).toContain('Exportar destaques');
    expect(api.post).not.toHaveBeenCalled();
    expect(view.button('Importar').disabled).toBe(true);
  });

  it('shows the books of the file with the work chosen where the server was sure, and what was left out', async () => {
    await open();
    await pick();
    expect(api.post).toHaveBeenCalledWith('/notes/import/preview', expect.any(FormData));
    const text = view.text();
    expect(text).toContain('5 anotações em 3 livros (Kindle).');
    expect(text).toContain('Ficou de fora: 1 marcadores, que não têm texto.');
    expect(select('Duna').value).toBe('7');
    expect(select('O Nome da Rosa').value).toBe('0'); // not sure: the person chooses
    expect([...select('Outro').options].map((o) => o.textContent)).toEqual(['Nenhuma obra parecida: não será importado']);
    expect(view.button('Importar 1 livro').disabled).toBe(false);
  });

  it('sends the file and only the books that were matched, and says what came of it', async () => {
    await open();
    const file = await pick();
    await choose(select('O Nome da Rosa'), '8');
    expect(view.button('Importar 2 livros')).toBeTruthy();
    await act(async () => { view.button('Importar 2 livros').click(); });
    await flush();
    const [url, data] = api.post.mock.calls.at(-1);
    expect(url).toBe('/notes/import');
    expect(data.get('file')).toBe(file);
    expect(JSON.parse(data.get('mapping'))).toEqual({ 'duna|herbert frank': 7, 'rosa|eco': 8 });
    const text = view.text();
    expect(text).toContain('4 anotações importadas.');
    expect(text).toContain('1 já estavam aqui.');
    expect(text).toContain('3 com o lugar no livro; 1 só no livro');
    expect(text).toContain('1 de livros que você deixou de fora.');
    await act(async () => { view.button('Pronto').click(); });
    expect(onClose).toHaveBeenCalled();
  });

  it('says why a file was refused', async () => {
    api.post.mockRejectedValueOnce({ response: { status: 400, data: 'Não achei nenhum destaque ou nota neste arquivo.' } });
    await open();
    await pick('vazio.txt');
    expect(document.body.querySelector('[role="alert"]')?.textContent).toContain('Não achei nenhum destaque');
    expect(view.button('Importar').disabled).toBe(true);
  });
});
