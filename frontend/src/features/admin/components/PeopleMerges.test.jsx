import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { PeopleMerges } from './PeopleMerges';

const pair = {
  id: 4,
  a: { id: 1, name: 'Herbert, Frank', aliases: ['Herbert, Frank, author'], works: 4, titles: ['Duna', 'Messias', 'Filhos de Duna'] },
  b: { id: 2, name: 'Frank Herbert', aliases: [], works: 1, titles: ['Dune'] },
};
let view;

async function open(pairs = [pair]) {
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/people/merges') return { data: { data: pairs } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({});
  view = await mount(<PeopleMerges />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('PeopleMerges', () => {
  it('shows both names with what each has written and every way it was written', async () => {
    await open();
    const text = view.text();
    expect(text).toContain('Herbert, Frank');
    expect(text).toContain('4 obras: Duna, Messias, Filhos de Duna…');
    expect(text).toContain('Também escrito: Herbert, Frank, author');
    expect(text).toContain('1 obra: Dune');
  });

  it('says so when there is nothing to decide', async () => {
    await open([]);
    expect(view.text()).toContain('Nenhuma sugestão pendente');
  });

  it('merges only after confirming, keeping the one that was chosen', async () => {
    await open();
    await view.click(view.buttonMatching(/mantendo “Frank Herbert”/));
    expect(api.post).not.toHaveBeenCalled();
    expect(view.dialog().textContent).toContain('Todas as obras de “Herbert, Frank” passam a ser de “Frank Herbert”');
    expect(view.dialog().textContent).toContain('não pode ser desfeito');

    await view.click(view.button('Unir pessoas'));
    expect(api.post).toHaveBeenCalledWith('/admin/people/merges/4/merge', { keep: 2, confirm: true });
  });

  it('can keep either side', async () => {
    await open();
    await view.click(view.buttonMatching(/mantendo “Herbert, Frank”/));
    await view.click(view.button('Unir pessoas'));
    expect(api.post).toHaveBeenCalledWith('/admin/people/merges/4/merge', { keep: 1, confirm: true });
  });

  it('cancelling the question changes nothing', async () => {
    await open();
    await view.click(view.buttonMatching(/mantendo “Frank Herbert”/));
    await view.click(view.button('Cancelar'));
    expect(api.post).not.toHaveBeenCalled();
    expect(view.dialog()).toBeNull();
  });

  it('says they are not the same person with one click', async () => {
    await open();
    await view.click(view.button('Não são a mesma pessoa'));
    expect(api.post).toHaveBeenCalledWith('/admin/people/merges/4/dismiss');
  });

  it('shows what the server said when it cannot merge', async () => {
    await open();
    api.post.mockRejectedValue({ response: { status: 404, data: 'Candidate not found\n' } });
    await view.click(view.buttonMatching(/mantendo “Frank Herbert”/));
    await view.click(view.button('Unir pessoas'));
    expect(view.text()).toContain('Candidate not found');
  });

  it('says it could not load when the list fails', async () => {
    api.get.mockRejectedValue(new Error('boom'));
    view = await mount(<PeopleMerges />);
    expect(view.text()).toContain('Não foi possível carregar as sugestões');
  });
});
