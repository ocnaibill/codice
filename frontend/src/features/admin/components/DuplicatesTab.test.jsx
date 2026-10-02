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

async function open(pairs = [pair]) {
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/duplicates') return { data: { data: pairs } };
    if (url === '/admin/people/merges') return { data: { data: [] } };
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

  it('says it is the same text, and how much of each is in the other, for a pair found by the words', async () => {
    await open([{ ...pair, id: 4, reason: 'content', evidence: { content: { ofA: 0.858, ofB: 0.892 } } }]);
    expect(view.text()).toContain('O mesmo texto, em outro arquivo');
    expect(view.text()).toContain('86% do texto de “Duna” está em “Duna (PDF)”, e 89% do texto de “Duna (PDF)” está em “Duna”.');
  });

  it('says nothing about the text for a pair that was found by the title and the author', async () => {
    await open();
    expect(view.text()).not.toContain('do texto de');
  });

  it('adds what the words say to a pair that was also found by the title and the author', async () => {
    await open([{ ...pair, evidence: { content: { ofA: 0.99, ofB: 0.97 } } }]);
    expect(view.text()).toContain('Mesmo título e autor');
    expect(view.text()).toContain('99% do texto de “Duna”');
  });

  it('says it looks like the same book in another language, and what reading them against each other found', async () => {
    await open([{ ...pair, id: 5, reason: 'translation', evidence: { translation: { hits: 14, samples: 60, order: 1, languageA: 'pt', languageB: 'en' } } }]);
    expect(view.text()).toContain('Parece a mesma obra em outra língua');
    expect(view.text()).toContain('14 de 60 passagens de uma foram achadas na outra, 100% delas na ordem do livro. Idiomas: português e inglês.');
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

});
