import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn() } }));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { mount } from '../testUtils';
import { SuggestionsTab } from './SuggestionsTab';

let view;
async function open(payload) {
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/suggestions') {
      if (payload instanceof Error) throw payload;
      return { data: payload };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  view = await mount(<SuggestionsTab />);
}
beforeEach(() => {
  vi.clearAllMocks();
  useGlobalStore.setState({ metadataWorkId: null });
});
afterEach(() => view.unmount());

const queued = [
  { workId: 7, title: 'Duna', author: 'Frank Herbert', pending: 3, fields: ['author', 'isbn', 'tags'] },
  { workId: 9, title: 'Fundação', author: '', pending: 1, fields: ['publisher'] },
];

describe('SuggestionsTab: the queue of suggestions (#70)', () => {
  it('lists each work with how many suggestions wait and for which fields, in the order it was given', async () => {
    await open({ data: queued, total: 2 });
    const text = view.text();
    expect(text).toContain('Duna');
    expect(text).toContain('Frank Herbert');
    expect(text).toContain('3 sugestões: Autor, ISBN, Etiquetas');
    expect(text).toContain('1 sugestão: Editora');
    expect(text).toContain('Sem autor');
    expect(text.indexOf('Duna')).toBeLessThan(text.indexOf('Fundação'));
    expect(text).not.toContain('Mostrando');
  });

  it('opens the metadata of the work on its suggestions when asked to review', async () => {
    await open({ data: queued, total: 2 });
    const buttons = [...document.body.querySelectorAll('button')].filter((b) => b.textContent === 'Revisar');
    expect(buttons).toHaveLength(2);
    await view.click(buttons[1]);
    expect(useGlobalStore.getState()).toMatchObject({ metadataWorkId: 9, metadataTab: 'suggestions' });
  });

  it('names a field it does not know as it came', async () => {
    await open({ data: [{ workId: 1, title: 'X', author: 'Y', pending: 1, fields: ['novo_campo'] }], total: 1 });
    expect(view.text()).toContain('1 sugestão: novo_campo');
  });

  it('says when it shows only part of the queue', async () => {
    await open({ data: queued, total: 5 });
    expect(view.text()).toContain('Mostrando 2 de 5 obras');
  });

  it('says so when there is nothing waiting, and when it could not load', async () => {
    await open({ data: [], total: 0 });
    expect(view.text()).toContain('Nenhuma obra com sugestões esperando.');
    view.unmount();
    await open(new Error('offline'));
    expect(view.text()).toContain('Não foi possível carregar a fila.');
    expect(view.text()).not.toContain('Nenhuma obra com sugestões');
  });
});
