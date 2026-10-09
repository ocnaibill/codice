import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { CategoryRules } from './CategoryRules';

const tree = [
  { id: 1, parentId: null, name: 'Ficção científica', works: 3, own: 3 },
  { id: 2, parentId: null, name: 'Mangá', works: 7, own: 1 },
  { id: 3, parentId: 2, name: 'Seinen', works: 6, own: 6 },
];
const rules = [
  { id: 10, categoryId: 1, term: 'science fiction' },
  { id: 11, categoryId: 1, term: 'sci-fi' },
  { id: 12, categoryId: 3, term: 'seinen' },
];
const preview = {
  categories: [
    { id: 1, name: 'Ficção científica', matched: 5, fresh: 4 },
    { id: 3, name: 'Seinen', matched: 1, fresh: 1 },
  ],
  links: 5, works: 4, withoutNow: 9, withoutAfter: 5,
};
let view;
let current;

async function open({ list = rules, previewAnswer = preview } = {}) {
  current = { list, previewAnswer };
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/categories/rules') return { data: { data: current.list } };
    if (url === '/admin/categories/rules/preview') return { data: current.previewAnswer };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockImplementation(async (url) => (url.endsWith('/apply') ? { data: { links: 5, works: 4 } } : { data: { id: 99, categoryId: 1, term: 'x' } }));
  api.delete.mockResolvedValue({});
  view = await mount(<CategoryRules categories={tree} />);
}
const field = (label) => document.body.querySelector(`[aria-label="${label}"]`);
const select = (el, value) => { el.value = String(value); el.dispatchEvent(new Event('change', { bubbles: true })); };
const dialogButton = (label) => [...view.dialog().querySelectorAll('button')].find((b) => b.textContent.trim() === label);
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('CategoryRules: the terms', () => {
  it('lists the terms of each category that has some, in the order of the tree, and none for the others', async () => {
    await open();
    expect([...document.body.querySelectorAll('ul[aria-label="Regras por categoria"] > li > p')].map((p) => p.textContent)).toEqual(['Ficção científica', 'Mangá › Seinen']);
    expect([...document.body.querySelectorAll('ul[aria-label="Termos de Ficção científica"] > li')].map((li) => li.firstChild.textContent)).toEqual(['science fiction', 'sci-fi']);
    expect(document.body.querySelector('ul[aria-label="Termos de Mangá"]')).toBeNull();
  });

  it('offers every category by its path for a new term, the first one chosen', async () => {
    await open();
    const options = [...field('Categoria do novo termo').querySelectorAll('option')].map((o) => o.textContent);
    expect(options).toEqual(['Ficção científica', 'Mangá', 'Mangá › Seinen']);
    expect(field('Categoria do novo termo').value).toBe('1');
  });

  it('adds a term to the category chosen, with the spaces tidied, and empties the field', async () => {
    await open();
    select(field('Categoria do novo termo'), 3);
    await view.type(field('Novo termo'), '  slice of life ');
    await view.click(view.button('Adicionar termo'));
    expect(api.post).toHaveBeenCalledWith('/admin/categories/3/rules', { term: 'slice of life' });
    expect(field('Novo termo').value).toBe('');
  });

  it('adds to the first category when none was chosen', async () => {
    await open();
    await view.type(field('Novo termo'), 'space opera');
    await view.click(view.button('Adicionar termo'));
    expect(api.post).toHaveBeenCalledWith('/admin/categories/1/rules', { term: 'space opera' });
  });

  it('does not add an empty term', async () => {
    await open();
    expect(view.button('Adicionar termo').disabled).toBe(true);
    await view.type(field('Novo termo'), '   ');
    expect(view.button('Adicionar termo').disabled).toBe(true);
  });

  it('says why a term was refused, in Portuguese, and keeps what was typed', async () => {
    await open();
    api.post.mockRejectedValueOnce({ response: { status: 409, data: 'That term is already a rule of this category' } });
    await view.type(field('Novo termo'), 'sci-fi');
    await view.click(view.button('Adicionar termo'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Esse termo já é uma regra desta categoria.');
    expect(field('Novo termo').value).toBe('sci-fi');
  });

  it('takes a term off a category', async () => {
    await open();
    await view.click(document.body.querySelector('[aria-label="Tirar o termo sci-fi de Ficção científica"]'));
    expect(api.delete).toHaveBeenCalledWith('/admin/categories/rules/11');
  });

  it('says what went wrong when a term cannot be taken off', async () => {
    await open();
    api.delete.mockRejectedValueOnce({ response: { status: 404, data: 'Rule not found' } });
    await view.click(document.body.querySelector('[aria-label="Tirar o termo seinen de Seinen"]'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Essa regra não existe mais.');
  });

  it('says there are no rules yet, and has nothing to preview', async () => {
    await open({ list: [] });
    expect(view.text()).toContain('Nenhuma regra ainda.');
    expect(view.button('Ver o que as regras fariam')).toBeUndefined();
  });

  it('says it could not load the rules', async () => {
    api.get.mockRejectedValue({ response: { status: 500 } });
    view = await mount(<CategoryRules categories={tree} />);
    expect(view.text()).toContain('Não foi possível carregar as regras.');
  });
});

describe('CategoryRules: what they would do, and doing it', () => {
  it('shows what applying them would do, per category, and how many works stay without one', async () => {
    await open();
    await view.click(view.button('Ver o que as regras fariam'));
    expect(api.get).toHaveBeenCalledWith('/admin/categories/rules/preview');
    const text = document.body.querySelector('[aria-label="Prévia das regras"]').textContent;
    expect(text).toContain('5 novos lugares em 4 obras.');
    expect(text).toContain('Sem categoria: 9 obras agora, 5 depois.');
    const lines = [...document.body.querySelectorAll('ul[aria-label="O que cada categoria receberia"] li')].map((li) => li.textContent);
    expect(lines).toEqual(['Ficção científica5 obras casam, 4 novas', 'Mangá › Seinen1 obra casa, 1 nova']);
  });

  it('lists only the categories the rules reach, in the order of the tree, and counts the ones they do not', async () => {
    await open({ previewAnswer: {
      categories: [
        { id: 1, name: 'Ficção científica', matched: 0, fresh: 0 },
        { id: 3, name: 'Seinen', matched: 1, fresh: 1 },
        { id: 2, name: 'Mangá', matched: 2, fresh: 1 },
      ],
      links: 2, works: 2, withoutNow: 3, withoutAfter: 1,
    } });
    await view.click(view.button('Ver o que as regras fariam'));
    const lines = [...document.body.querySelectorAll('ul[aria-label="O que cada categoria receberia"] li')].map((li) => li.textContent);
    expect(lines).toEqual(['Mangá2 obras casam, 1 nova', 'Mangá › Seinen1 obra casa, 1 nova']);
    expect(view.text()).toContain('Mais 1 categoria com regras não casa com nenhuma obra.');
  });

  it('says how many categories the rules do not reach, in the plural', async () => {
    await open({ previewAnswer: { categories: [{ id: 1, name: 'Ficção científica', matched: 0, fresh: 0 }, { id: 2, name: 'Mangá', matched: 0, fresh: 0 }, { id: 3, name: 'Seinen', matched: 1, fresh: 1 }], links: 1, works: 1, withoutNow: 1, withoutAfter: 0 } });
    await view.click(view.button('Ver o que as regras fariam'));
    expect(view.text()).toContain('Mais 2 categorias com regras não casam com nenhuma obra.');
  });

  it('says nothing about categories not reached when the rules reach them all', async () => {
    await open();
    await view.click(view.button('Ver o que as regras fariam'));
    expect(view.text()).not.toContain('com regras não casa');
  });

  it('says it in the singular for one', async () => {
    await open({ previewAnswer: { categories: [{ id: 1, name: 'Ficção científica', matched: 1, fresh: 1 }], links: 1, works: 1, withoutNow: 1, withoutAfter: 0 } });
    await view.click(view.button('Ver o que as regras fariam'));
    const text = document.body.querySelector('[aria-label="Prévia das regras"]').textContent;
    expect(text).toContain('1 novo lugar em 1 obra.');
    expect(text).toContain('Sem categoria: 1 obra agora, 0 depois.');
  });

  it('says there is nothing to do, and does not offer to apply', async () => {
    await open({ previewAnswer: { categories: [{ id: 1, name: 'Ficção científica', matched: 2, fresh: 0 }], links: 0, works: 0, withoutNow: 3, withoutAfter: 3 } });
    await view.click(view.button('Ver o que as regras fariam'));
    expect(view.text()).toContain('As regras não têm nada novo a fazer.');
    expect(view.button('Aplicar as regras').disabled).toBe(true);
  });

  it('closes the preview', async () => {
    await open();
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(view.button('Fechar a prévia'));
    expect(document.body.querySelector('[aria-label="Prévia das regras"]')).toBeNull();
  });

  it('asks before applying, and applies nothing when it is cancelled', async () => {
    await open();
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(view.button('Aplicar as regras'));
    expect(api.post).not.toHaveBeenCalled();
    expect(view.dialog().textContent).toContain('4 obras entram em categorias, com 5 novos lugares no total.');
    expect(view.dialog().textContent).toContain('Nenhuma obra sai de nenhuma categoria');
    await view.click(dialogButton('Cancelar'));
    expect(api.post).not.toHaveBeenCalled();
    expect(view.dialog()).toBeNull();
    expect(document.body.querySelector('[aria-label="Prévia das regras"]')).not.toBeNull();
  });

  it('says it in the singular in the question', async () => {
    await open({ previewAnswer: { categories: [], links: 1, works: 1, withoutNow: 1, withoutAfter: 0 } });
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(view.button('Aplicar as regras'));
    expect(view.dialog().textContent).toContain('1 obra entra em categorias, com 1 novo lugar no total.');
  });

  it('applies what the preview showed, says how many places were made, and closes the preview', async () => {
    await open();
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(view.button('Aplicar as regras'));
    await view.click(dialogButton('Aplicar as regras'));
    expect(api.post).toHaveBeenCalledWith('/admin/categories/rules/apply', { links: 5 });
    expect(view.text()).toContain('5 novos lugares foram criados, em 4 obras.');
    expect(document.body.querySelector('[aria-label="Prévia das regras"]')).toBeNull();
  });

  it('says it in the singular when one place was made', async () => {
    await open();
    api.post.mockResolvedValueOnce({ data: { links: 1, works: 1 } });
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(view.button('Aplicar as regras'));
    await view.click(dialogButton('Aplicar as regras'));
    expect(view.text()).toContain('1 novo lugar foi criado, em 1 obra.');
  });

  it('says the library changed when the preview is no longer true, and asks to look again', async () => {
    await open();
    api.post.mockRejectedValueOnce({ response: { status: 409, data: 'The library changed since the preview: look at it again' } });
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(view.button('Aplicar as regras'));
    await view.click(dialogButton('Aplicar as regras'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('O acervo mudou desde a prévia: veja de novo o que as regras fariam.');
    expect(document.body.querySelector('[aria-label="Prévia das regras"]')).toBeNull();
    expect(view.text()).not.toContain('foram criados');
  });

  it('forgets the preview when a term is added or taken off, since it is no longer what the rules say', async () => {
    await open();
    await view.click(view.button('Ver o que as regras fariam'));
    await view.type(field('Novo termo'), 'space opera');
    await view.click(view.button('Adicionar termo'));
    expect(document.body.querySelector('[aria-label="Prévia das regras"]')).toBeNull();
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(document.body.querySelector('[aria-label="Tirar o termo sci-fi de Ficção científica"]'));
    expect(document.body.querySelector('[aria-label="Prévia das regras"]')).toBeNull();
  });

  it('forgets the sentence of the last time when something is previewed again', async () => {
    await open();
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(view.button('Aplicar as regras'));
    await view.click(dialogButton('Aplicar as regras'));
    expect(view.text()).toContain('foram criados');
    api.get.mockImplementation(() => new Promise(() => {}));
    await view.click(view.button('Ver o que as regras fariam'));
    expect(view.text()).not.toContain('foram criados');
  });

  it('forgets the sentence of the last time when a term is added', async () => {
    await open();
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(view.button('Aplicar as regras'));
    await view.click(dialogButton('Aplicar as regras'));
    expect(view.text()).toContain('foram criados');
    await view.type(field('Novo termo'), 'space opera');
    await view.click(view.button('Adicionar termo'));
    expect(view.text()).not.toContain('foram criados');
  });

  it('keeps the buttons still while the rules are being applied', async () => {
    await open();
    api.post.mockImplementationOnce(() => new Promise(() => {}));
    await view.click(view.button('Ver o que as regras fariam'));
    await view.click(view.button('Aplicar as regras'));
    await view.click(dialogButton('Aplicar as regras'));
    expect(view.button('Aplicar as regras').disabled).toBe(true);
    expect(view.button('Ver o que as regras fariam').disabled).toBe(true);
  });

  it('keeps the buttons still while the preview is being made', async () => {
    await open();
    api.get.mockImplementation(() => new Promise(() => {}));
    await view.click(view.button('Ver o que as regras fariam'));
    expect(view.button('Ver o que as regras fariam').disabled).toBe(true);
  });

  it('says what went wrong when the preview fails', async () => {
    await open();
    api.get.mockImplementation(async (url) => {
      if (url === '/admin/categories/rules') return { data: { data: rules } };
      throw { response: { status: 500, data: 'Error previewing the rules' } };
    });
    await view.click(view.button('Ver o que as regras fariam'));
    expect(view.container.querySelector('[role="alert"]')).not.toBeNull();
  });
});
