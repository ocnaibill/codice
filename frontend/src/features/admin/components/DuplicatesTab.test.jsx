import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { DuplicatesTab } from './DuplicatesTab';

const pair = {
  id: 3, reason: 'title_author',
  a: { id: 10, title: 'Duna', author: 'Frank Herbert', formats: ['epub'] },
  b: { id: 11, title: 'Duna (PDF)', author: 'Frank Herbert', formats: ['pdf'] },
};
let view;

async function open(pairs = [pair], ocr = []) {
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/duplicates') return { data: { data: pairs } };
    if (url === '/admin/ocr') return { data: { data: ocr } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({});
  view = await mount(<DuplicatesTab />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('DuplicatesTab', () => {
  it('shows both works and why they were suggested', async () => {
    await open();
    expect(view.text()).toContain('Mesmo título e autor');
    expect(view.text()).toContain('Duna (PDF)');
    expect(view.text()).toContain('pdf');
  });

  it('dismisses a pair with one click', async () => {
    await open();
    await view.click(view.button('Não é duplicata'));
    expect(api.post).toHaveBeenCalledWith('/admin/duplicates/3/dismiss');
  });

  it('links only after confirming, keeping the work that was chosen', async () => {
    await open();
    await view.click(view.buttonMatching(/mantendo “Duna \(PDF\)” \(B\)/));
    expect(api.post).not.toHaveBeenCalled();
    expect(view.dialog().textContent).toContain('não pode ser desfeito');

    await view.click(view.button('Unir obras'));
    expect(api.post).toHaveBeenCalledWith('/admin/duplicates/3/link', { keep: 11, confirm: true });
  });

  it('does not link when the confirmation is cancelled', async () => {
    await open();
    await view.click(view.buttonMatching(/\(A\)/));
    await view.click(view.button('Cancelar'));
    expect(api.post).not.toHaveBeenCalled();
  });

  it('queues a comparison of the whole library on request', async () => {
    await open([]);
    expect(view.text()).toContain('Nenhuma sugestão pendente');
    await view.click(view.button('Comparar o acervo agora'));
    expect(api.post).toHaveBeenCalledWith('/admin/duplicates/scan');
  });

  it('lists the PDFs that have pages without text', async () => {
    await open([], [
      { workId: 1, title: 'Escaneado', fileId: 4, pageCount: 5, pagesWithoutText: [1, 2, 3, 4, 5] },
      { workId: 2, title: 'Misto', fileId: 6, pageCount: 10, pagesWithoutText: [3, 4] },
    ]);
    expect(view.text()).toContain('Todas as 5 páginas são imagem');
    expect(view.text()).toContain('2 de 10 páginas sem texto (3, 4)');
  });
});
