import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { StarterList } from './StarterList';

const list = [
  { name: 'Ficção científica', terms: ['science fiction', 'sci-fi'] },
  { name: 'Terror', terms: ['horror'] },
  { name: 'Mangá', terms: ['manga'], children: [{ name: 'Shounen', terms: ['shounen', 'shonen'] }, { name: 'Seinen', terms: ['seinen'] }] },
];
let view;

async function open({ hasCategories = false } = {}) {
  api.get.mockResolvedValue({ data: { data: list } });
  api.post.mockResolvedValue({ data: { categories: 5, rules: 7 } });
  view = await mount(<StarterList hasCategories={hasCategories} />);
}
const dialogButton = (label) => [...view.dialog().querySelectorAll('button')].find((b) => b.textContent.trim().startsWith(label));
const box = (name) => [...view.dialog().querySelectorAll('input[type="checkbox"]')].find((b) => b.closest('label').textContent.startsWith(name));
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('StarterList', () => {
  it('is called "Começar" while there are no categories, and "Adicionar" after', async () => {
    await open();
    expect(view.button('Começar de uma lista sugerida')).toBeTruthy();
    view.unmount();
    await open({ hasCategories: true });
    expect(view.button('Adicionar da lista sugerida')).toBeTruthy();
    expect(view.button('Começar de uma lista sugerida')).toBeUndefined();
  });

  it('does not read the list until it is asked for', async () => {
    await open();
    expect(api.get).not.toHaveBeenCalled();
    await view.click(view.button('Começar de uma lista sugerida'));
    expect(api.get).toHaveBeenCalledWith('/admin/categories/starter');
  });

  it('shows each category with how many terms it brings and its subcategories, all checked', async () => {
    await open();
    await view.click(view.button('Começar de uma lista sugerida'));
    const items = [...view.dialog().querySelectorAll('ul[aria-label="Categorias da lista"] > li')].map((li) => li.textContent);
    expect(items).toEqual(['Ficção científica · 2 termos', 'Terror · 1 termo', 'Mangá · 4 termosShounen, Seinen']);
    expect([...view.dialog().querySelectorAll('input[type="checkbox"]')].every((b) => b.checked)).toBe(true);
    expect(view.dialog().textContent).toContain('Nada é posto em nenhuma obra por aqui');
  });

  it('says how many categories it would make, counting the subcategories, and follows the ones checked', async () => {
    await open();
    await view.click(view.button('Começar de uma lista sugerida'));
    expect(dialogButton('Criar').textContent).toBe('Criar 5 categorias');
    await view.click(box('Mangá'));
    expect(box('Mangá').checked).toBe(false);
    expect(box('Terror').checked).toBe(true);
    expect(dialogButton('Criar').textContent).toBe('Criar 2 categorias');
    await view.click(box('Ficção científica'));
    expect(dialogButton('Criar').textContent).toBe('Criar 1 categoria');
    await view.click(box('Terror'));
    expect(dialogButton('Criar').textContent).toBe('Criar');
    expect(dialogButton('Criar').disabled).toBe(true);
    await view.click(box('Terror'));
    expect(dialogButton('Criar').disabled).toBe(false);
  });

  it('makes only the ones checked, and says how many categories and terms it made', async () => {
    await open();
    await view.click(view.button('Começar de uma lista sugerida'));
    await view.click(box('Terror'));
    await view.click(dialogButton('Criar'));
    expect(api.post).toHaveBeenCalledWith('/admin/categories/starter', { groups: ['Ficção científica', 'Mangá'] });
    expect(view.text()).toContain('5 categorias criadas e 7 termos nas regras.');
    expect(view.dialog()).toBeNull();
  });

  it('says it in the singular', async () => {
    await open();
    api.post.mockResolvedValueOnce({ data: { categories: 1, rules: 1 } });
    await view.click(view.button('Começar de uma lista sugerida'));
    await view.click(dialogButton('Criar'));
    expect(view.text()).toContain('1 categoria criada e 1 termo nas regras.');
  });

  it('makes nothing when it is cancelled', async () => {
    await open();
    await view.click(view.button('Começar de uma lista sugerida'));
    await view.click(dialogButton('Cancelar'));
    expect(api.post).not.toHaveBeenCalled();
    expect(view.dialog()).toBeNull();
  });

  it('says what the server refused', async () => {
    await open();
    api.post.mockRejectedValueOnce({ response: { status: 400, data: 'That category is not in the list' } });
    await view.click(view.button('Começar de uma lista sugerida'));
    await view.click(dialogButton('Criar'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Essa categoria não está na lista sugerida.');
    expect(view.text()).not.toContain('criadas');
  });

  it('says it could not load the list, and does not offer to make anything from it', async () => {
    api.get.mockRejectedValue({ response: { status: 500 } });
    view = await mount(<StarterList hasCategories={false} />);
    await view.click(view.button('Começar de uma lista sugerida'));
    expect(view.dialog().textContent).toContain('Não foi possível carregar a lista.');
    expect(dialogButton('Criar').disabled).toBe(true);
  });

  it('shows that it is loading', async () => {
    api.get.mockReturnValue(new Promise(() => {}));
    view = await mount(<StarterList hasCategories={false} />);
    await view.click(view.button('Começar de uma lista sugerida'));
    expect(view.dialog().textContent).toContain('Carregando a lista…');
  });

  it('keeps the button still while it makes the categories, and forgets the sentence of the last time', async () => {
    await open();
    await view.click(view.button('Começar de uma lista sugerida'));
    await view.click(dialogButton('Criar'));
    expect(view.text()).toContain('categorias criadas');
    api.post.mockImplementationOnce(() => new Promise(() => {}));
    await view.click(view.button('Começar de uma lista sugerida'));
    await view.click(dialogButton('Criar'));
    expect(view.button('Começar de uma lista sugerida').disabled).toBe(true);
    expect(view.text()).not.toContain('categorias criadas');
  });
});
