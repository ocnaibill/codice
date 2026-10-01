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

  it('says why a pair was proposed when the same key of a reference source is the reason, and shows the key each holds', async () => {
    const keyed = {
      id: 9,
      reason: 'authority',
      evidence: { scheme: 'openlibrary', value: 'OL79034A' },
      a: { id: 1, name: 'Frank Herbert', aliases: [], works: 2, titles: ['Duna'], authorities: ['openlibrary:OL79034A'] },
      b: { id: 2, name: 'Frank P. Herbert', aliases: [], works: 1, titles: ['Messias'], authorities: ['comicvine:4050-1', 'openlibrary:OL79034A'] },
    };
    await open([keyed]);
    const text = view.text();
    expect(text).toContain('Mesma chave no Open Library (OL79034A): muito provavelmente a mesma pessoa.');
    expect(text).toContain('Chave: Open Library OL79034A');
    expect(text).toContain('Chave: ComicVine 4050-1; Open Library OL79034A');
  });

  it('names the identifier of a reference authority that two people share, and the ones each holds', async () => {
    const shared = {
      id: 11, reason: 'authority', evidence: { scheme: 'wikidata', value: 'Q7934' },
      a: { id: 1, name: 'Frank Herbert', aliases: [], works: 2, titles: ['Duna'], authorities: ['isni:0000000121347853', 'openlibrary:OL79034A', 'viaf:59083797', 'wikidata:Q7934'] },
      b: { id: 2, name: 'F. P. Herbert', aliases: [], works: 1, titles: ['Messias'], authorities: ['openlibrary:OL5A', 'wikidata:Q7934'] },
    };
    await open([shared]);
    const text = view.text();
    expect(text).toContain('Mesma chave no Wikidata (Q7934): muito provavelmente a mesma pessoa.');
    expect(text).toContain('Chave: ISNI 0000000121347853; Open Library OL79034A; VIAF 59083797; Wikidata Q7934');
    expect(text).toContain('Chave: Open Library OL5A; Wikidata Q7934');
  });

  it('gives no reason to a pair proposed only for sharing words, and no key line to a person without one', async () => {
    await open();
    expect(view.text()).not.toContain('Mesma chave');
    expect(view.text()).not.toContain('Chave:');
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
