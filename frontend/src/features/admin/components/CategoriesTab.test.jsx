import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { CategoriesTab } from './CategoriesTab';

const tree = [
  { id: 1, parentId: null, name: 'Ficção científica', works: 3, own: 3 },
  { id: 2, parentId: null, name: 'Mangá', works: 5, own: 1 },
  { id: 3, parentId: 2, name: 'Seinen', works: 4, own: 4 },
];
let view;
let current;

async function open(categories = tree) {
  current = categories;
  api.get.mockImplementation(async (url) => {
    if (url === '/categories') return { data: { data: current } };
    if (url === '/admin/categories/rules') return { data: { data: [] } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: { id: 9, name: 'Nova' } });
  api.put.mockResolvedValue({ data: {} });
  api.delete.mockResolvedValue({});
  view = await mount(<CategoriesTab />);
}
const rowOf = (name) => [...document.body.querySelectorAll('ul[aria-label="Categorias"] > li')].find((li) => li.textContent.includes(name));
const inRow = (name, label) => [...rowOf(name).querySelectorAll('button')].find((b) => b.textContent.trim() === label);
const field = (label) => document.body.querySelector(`[aria-label="${label}"]`);
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('CategoriesTab', () => {
  it('offers the list to start from while it is empty, and to add from after, and has the rules only once there are categories', async () => {
    await open([]);
    expect(view.button('Começar de uma lista sugerida')).toBeTruthy();
    expect(view.text()).not.toContain('Regras');
    view.unmount();
    await open();
    expect(view.button('Adicionar da lista sugerida')).toBeTruthy();
    expect(view.button('Começar de uma lista sugerida')).toBeUndefined();
    expect(document.body.querySelector('h2')).not.toBeNull();
    expect([...document.body.querySelectorAll('h2')].map((h) => h.textContent)).toEqual(['Categorias', 'Regras']);
  });

  it('starts empty and says so', async () => {
    await open([]);
    expect(view.text()).toContain('Nenhuma categoria ainda. Crie a primeira acima.');
    expect(view.text()).not.toContain('categorias.');
    expect(view.button('Criar categoria').disabled).toBe(true);
  });

  it('shows the tree in reading order with how many works each has, and how many directly when they are not the same', async () => {
    await open();
    const lines = [...document.body.querySelectorAll('ul[aria-label="Categorias"] > li')].map((li) => li.textContent.replace(/(Editar|Apagar).*$/, '').trim());
    expect(lines).toEqual(['Ficção científica3 obras', 'Mangá5 obras (1 direto)', 'Seinen4 obras']);
    expect(view.text()).toContain('3 categorias.');
    expect(rowOf('Seinen').style.paddingLeft).toBe('20px');
    expect(rowOf('Mangá').style.paddingLeft).toBe('0px');
  });

  it('says one work and one category in the singular', async () => {
    await open([{ id: 1, parentId: null, name: 'Única', works: 1, own: 1 }]);
    expect(view.text()).toContain('1 obra');
    expect(view.text()).not.toContain('1 obras');
    expect(view.text()).toContain('1 categoria.');
  });

  it('creates a category at the top with the name tidied, and says so', async () => {
    await open();
    await view.type(field('Nome da nova categoria'), '  Terror ');
    await view.click(view.button('Criar categoria'));
    expect(api.post).toHaveBeenCalledWith('/admin/categories', { name: 'Terror', parentId: null });
    expect(view.text()).toContain('“Nova” foi criada.');
    expect(field('Nome da nova categoria').value).toBe('');
  });

  it('stops saying a category was created as soon as another is tried', async () => {
    await open();
    await view.type(field('Nome da nova categoria'), 'Terror');
    await view.click(view.button('Criar categoria'));
    expect(view.text()).toContain('foi criada');
    api.post.mockRejectedValueOnce({ response: { status: 409, data: 'A category with that name already exists there' } });
    await view.type(field('Nome da nova categoria'), 'Mangá');
    await view.click(view.button('Criar categoria'));
    expect(view.text()).not.toContain('foi criada');
  });

  it('creates one under another, offering only the places that leave room for it', async () => {
    await open([...tree, { id: 4, parentId: 3, name: 'Dark', works: 1, own: 1 }]);
    const options = [...field('Onde fica a nova categoria').querySelectorAll('option')].map((o) => o.textContent);
    expect(options).toEqual(['No topo', 'Ficção científica', 'Mangá', 'Mangá › Seinen']);
    await view.type(field('Nome da nova categoria'), 'Terror');
    const select = field('Onde fica a nova categoria');
    select.value = '2';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await view.click(view.button('Criar categoria'));
    expect(api.post).toHaveBeenCalledWith('/admin/categories', { name: 'Terror', parentId: 2 });
  });

  it('says what the server refused, in Portuguese, and keeps the name typed', async () => {
    await open();
    api.post.mockRejectedValueOnce({ response: { status: 409, data: 'A category with that name already exists there' } });
    await view.type(field('Nome da nova categoria'), 'Mangá');
    await view.click(view.button('Criar categoria'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Já existe uma categoria com esse nome nesse lugar.');
    expect(field('Nome da nova categoria').value).toBe('Mangá');
    expect(view.text()).not.toContain('foi criada');
  });

  it('edits the name and the place of a category, and says it was saved', async () => {
    await open();
    await view.click(inRow('Seinen', 'Editar'));
    await view.type(field('Nome de Seinen'), ' Seinen adulto ');
    const select = field('Onde fica Seinen');
    expect([...select.querySelectorAll('option')].map((o) => o.textContent)).toEqual(['No topo', 'Ficção científica', 'Mangá']);
    select.value = '1';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await view.click(view.button('Salvar'));
    expect(api.put).toHaveBeenCalledWith('/admin/categories/3', { name: 'Seinen adulto', parentId: 1 });
    expect(view.text()).toContain('“Seinen adulto” foi salva.');
    expect(field('Nome de Seinen')).toBeNull();
  });

  it('starts the editing from what the category is, and does not offer itself nor what is under it', async () => {
    await open();
    await view.click(inRow('Mangá', 'Editar'));
    expect(field('Nome de Mangá').value).toBe('Mangá');
    expect(field('Onde fica Mangá').value).toBe('');
    expect([...field('Onde fica Mangá').querySelectorAll('option')].map((o) => o.textContent)).toEqual(['No topo', 'Ficção científica']);
    view.unmount();
    await open();
    await view.click(inRow('Seinen', 'Editar'));
    expect(field('Onde fica Seinen').value).toBe('2');
  });

  it('forgets the place chosen too when the editing is cancelled', async () => {
    await open();
    await view.click(inRow('Seinen', 'Editar'));
    const select = field('Onde fica Seinen');
    select.value = '1';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await view.click(view.button('Cancelar'));
    await view.click(inRow('Seinen', 'Editar'));
    expect(field('Onde fica Seinen').value).toBe('2');
  });

  it('does not save a category with no name', async () => {
    await open();
    await view.click(inRow('Mangá', 'Editar'));
    expect(view.button('Salvar').disabled).toBe(false);
    await view.type(field('Nome de Mangá'), '   ');
    expect(view.button('Salvar').disabled).toBe(true);
  });

  it('forgets what was typed when the editing is cancelled', async () => {
    await open();
    await view.click(inRow('Mangá', 'Editar'));
    await view.type(field('Nome de Mangá'), 'Outro');
    await view.click(view.button('Cancelar'));
    expect(api.put).not.toHaveBeenCalled();
    await view.click(inRow('Mangá', 'Editar'));
    expect(field('Nome de Mangá').value).toBe('Mangá');
  });

  it('keeps the editing open and says why when the server refuses', async () => {
    await open();
    api.put.mockRejectedValueOnce({ response: { status: 400, data: 'A category cannot go inside itself' } });
    await view.click(inRow('Mangá', 'Editar'));
    await view.click(view.button('Salvar'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Uma categoria não pode ficar dentro dela mesma, nem do que há nela.');
    expect(field('Nome de Mangá')).not.toBeNull();
  });

  it('asks before deleting, and says how many works stop being in it, none of them deleted', async () => {
    await open();
    await view.click(inRow('Ficção científica', 'Apagar'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(view.dialog().textContent).toContain('“Ficção científica” será apagada.');
    expect(view.dialog().textContent).toContain('3 obras deixam de estar nela, mas nenhuma obra é apagada.');
    await view.click(view.button('Apagar categoria'));
    expect(api.delete).toHaveBeenCalledWith('/admin/categories/1');
    expect(view.text()).toContain('“Ficção científica” foi apagada.');
  });

  it('says it in the singular, and says when no work is in it', async () => {
    await open([{ id: 1, parentId: null, name: 'Uma', works: 1, own: 1 }, { id: 2, parentId: null, name: 'Vazia', works: 0, own: 0 }]);
    await view.click(inRow('Uma', 'Apagar'));
    expect(view.dialog().textContent).toContain('1 obra deixa de estar nela');
    await view.click(view.button('Cancelar'));
    await view.click(inRow('Vazia', 'Apagar'));
    expect(view.dialog().textContent).toContain('Nenhuma obra está nela.');
  });

  it('deletes nothing when it is cancelled', async () => {
    await open();
    await view.click(inRow('Ficção científica', 'Apagar'));
    await view.click(view.button('Cancelar'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(view.dialog()).toBeNull();
  });

  it('does not offer to delete one that has subcategories, and says why', async () => {
    await open();
    expect(inRow('Mangá', 'Apagar').disabled).toBe(true);
    expect(inRow('Mangá', 'Apagar').title).toBe('Mova ou apague as subcategorias primeiro');
    expect(inRow('Seinen', 'Apagar').disabled).toBe(false);
    expect(inRow('Seinen', 'Apagar').title).toBe('');
  });

  it('keeps the buttons still while something is saved', async () => {
    await open();
    api.delete.mockImplementationOnce(() => new Promise(() => {}));
    await view.click(inRow('Seinen', 'Apagar'));
    await view.click(view.button('Apagar categoria'));
    expect(inRow('Ficção científica', 'Editar').disabled).toBe(true);
    expect(inRow('Ficção científica', 'Apagar').disabled).toBe(true);
    expect(view.button('Criar categoria').disabled).toBe(true);
  });

  it('shows that the categories could not be loaded', async () => {
    api.get.mockRejectedValue({ response: { status: 500 } });
    view = await mount(<CategoriesTab />);
    expect(view.text()).toContain('Não foi possível carregar as categorias.');
  });
});
