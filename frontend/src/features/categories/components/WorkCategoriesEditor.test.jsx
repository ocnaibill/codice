import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { WorkCategoriesEditor } from './WorkCategoriesEditor';

const tree = [
  { id: 1, parentId: null, name: 'Ficção científica', works: 3, own: 3 },
  { id: 2, parentId: null, name: 'Mangá', works: 5, own: 1 },
  { id: 3, parentId: 2, name: 'Seinen', works: 4, own: 4 },
];
const work = { id: 7, metadata: { categories: [{ id: 3, name: 'Seinen', path: 'Mangá › Seinen' }] } };
let view;

async function open(categories = tree, w = work) {
  api.get.mockImplementation(async (url) => {
    if (url === '/categories') return { data: { data: categories } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: { categories: [] } });
  view = await mount(<WorkCategoriesEditor work={w} />);
}
const box = (name) => [...document.body.querySelectorAll('input[type="checkbox"]')].find((b) => b.closest('label').textContent.trim() === name);
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('WorkCategoriesEditor', () => {
  it('shows the tree with the categories the work is in checked, each under its parent', async () => {
    await open();
    expect(box('Seinen').checked).toBe(true);
    expect(box('Mangá').checked).toBe(false);
    expect(box('Ficção científica').checked).toBe(false);
    const items = [...document.body.querySelectorAll('ul[aria-label="Categorias"] > li')];
    expect(items.map((li) => li.textContent)).toEqual(['Ficção científica', 'Mangá', 'Seinen']);
    expect(items[2].style.paddingLeft).toBe('20px');
    expect(view.text()).toContain('Marcar uma subcategoria já a põe nas de cima.');
  });

  it('starts from nothing for a work that is in no category, or whose categories were not sent', async () => {
    await open(tree, { id: 8, metadata: { categories: [] } });
    expect(document.body.querySelectorAll('input[type="checkbox"]:checked')).toHaveLength(0);
    view.unmount();
    await open(tree, { id: 9 });
    expect(document.body.querySelectorAll('input[type="checkbox"]:checked')).toHaveLength(0);
  });

  it('saves the ones checked, and says so', async () => {
    await open();
    await view.click(box('Ficção científica'));
    await view.click(box('Seinen'));
    await view.click(view.button('Salvar categorias'));
    expect(api.put).toHaveBeenCalledWith('/works/7/categories', { ids: [1] });
    expect(view.text()).toContain('Categorias salvas.');
  });

  it('saves none when all are unchecked', async () => {
    await open();
    await view.click(box('Seinen'));
    await view.click(view.button('Salvar categorias'));
    expect(api.put).toHaveBeenCalledWith('/works/7/categories', { ids: [] });
  });

  it('stops saying it was saved as soon as something is changed', async () => {
    await open();
    await view.click(view.button('Salvar categorias'));
    expect(view.text()).toContain('Categorias salvas.');
    await view.click(box('Mangá'));
    expect(view.text()).not.toContain('Categorias salvas.');
  });

  it('stops saying it was saved while the next save runs', async () => {
    await open();
    await view.click(view.button('Salvar categorias'));
    expect(view.text()).toContain('Categorias salvas.');
    api.put.mockImplementationOnce(() => new Promise(() => {}));
    await view.click(view.button('Salvar categorias'));
    expect(view.text()).not.toContain('Categorias salvas.');
  });

  it('keeps the form still while it saves', async () => {
    await open();
    api.put.mockImplementationOnce(() => new Promise(() => {}));
    await view.click(view.button('Salvar categorias'));
    expect(view.button('Salvando…').disabled).toBe(true);
    expect(box('Mangá').disabled).toBe(true);
  });

  it('says what the server refused', async () => {
    await open();
    api.put.mockRejectedValueOnce({ response: { status: 404, data: 'Category not found' } });
    await view.click(view.button('Salvar categorias'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Categoria não encontrada.');
    expect(view.text()).not.toContain('Categorias salvas.');
  });

  it('says there are no categories yet, and where to make them', async () => {
    await open([]);
    expect(view.text()).toContain('Ainda não há categorias. Crie as primeiras em Administração → Categorias.');
    expect(view.button('Salvar categorias')).toBeUndefined();
  });

  it('says it could not load them', async () => {
    api.get.mockRejectedValue({ response: { status: 500 } });
    view = await mount(<WorkCategoriesEditor work={work} />);
    expect(view.text()).toContain('Não foi possível carregar as categorias.');
  });
});
