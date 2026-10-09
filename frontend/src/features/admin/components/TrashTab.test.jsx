import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { TrashTab } from './TrashTab';

const item = { id: 5, kind: 'file', workTitle: 'Duna', originalPath: 'Frank Herbert/Duna/Duna.epub', sizeBytes: 2048,
  trashedAt: '2026-09-01T10:00:00Z', purgeAfter: null };
let view;

const retired = (over = {}) => ({
  id: 7, title: 'Neuromancer', author: 'William Gibson', retiredAt: '2026-10-08T10:00:00Z', retiredBy: 'Ana',
  files: 2, formats: ['EPUB', 'PDF'], inTrash: 0, sizeBytes: 3 * 1024 * 1024, ...over,
});
let retiredPages = {};

async function open({ isOwner = true, items = [item], policy = { enabled: false, days: 30 }, works = [], total = works.length, totalPages = total > 0 ? 1 : 0 } = {}) {
  retiredPages = { 1: { data: works, total, page: 1, limit: 20, totalPages } };
  api.get.mockImplementation(async (url, options) => {
    if (url === '/admin/retired-works') return { data: retiredPages[options?.params?.page] ?? { data: [], total, page: options?.params?.page, limit: 20, totalPages } };
    if (url === '/admin/trash') return { data: { items, totalBytes: 2048, policy } };
    if (url === '/admin/trash/policy/preview') return { data: { days: 10, items: 4, alreadyDue: 2 } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: {} });
  api.put.mockResolvedValue({});
  api.delete.mockResolvedValue({});
  view = await mount(<TrashTab isOwner={isOwner} />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('TrashTab', () => {
  it('shows what is in the trash and the space it takes', async () => {
    await open();
    expect(view.text()).toContain('Duna');
    expect(view.text()).toContain('2,0 KB');
  });

  it('restores an item without asking, since nothing is lost', async () => {
    await open();
    await view.click(view.button('Recuperar'));
    expect(api.post).toHaveBeenCalledWith('/admin/trash/5/restore');
  });

  it('deletes for good only after a confirmation that names the item', async () => {
    await open();
    await view.click(view.button('Apagar de vez'));
    expect(view.dialog().textContent).toContain('Duna');
    expect(api.delete).not.toHaveBeenCalled();

    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(api.delete).toHaveBeenCalledWith('/admin/trash/5', { params: { confirm: true } });
  });

  it('does not delete when the confirmation is cancelled', async () => {
    await open();
    await view.click(view.button('Apagar de vez'));
    await view.click(view.button('Cancelar'));
    expect(api.delete).not.toHaveBeenCalled();
  });

  it('empties the trash only after confirming', async () => {
    await open();
    await view.click(view.button('Esvaziar'));
    expect(api.post).not.toHaveBeenCalled();
    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(api.post).toHaveBeenCalledWith('/admin/trash/empty', { confirm: true });
  });

  it('lets only the owner change the automatic cleanup', async () => {
    await open({ isOwner: false });
    expect(view.container.querySelector('input[type="checkbox"]')).toBeNull();
    view.unmount();

    await open({ isOwner: true });
    expect(view.container.querySelector('input[type="checkbox"]')).toBeTruthy();
  });

  it('previews the policy before applying it to the items already there', async () => {
    await open({ policy: { enabled: true, days: 10 } });
    await view.click(view.button('Ver o efeito nos itens atuais'));
    expect(view.text()).toContain('apagaria de vez 2');

    await view.click(view.button('Aplicar aos itens atuais'));
    expect(api.post).not.toHaveBeenCalled();
    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(api.post).toHaveBeenCalledWith('/admin/trash/policy/apply', { days: 10, confirm: true });
  });
});

describe('TrashTab: the works that were retired (#89)', () => {
  const rows = () => [...document.body.querySelectorAll('ul[aria-label="Obras retiradas"] li')];
  const restoreOf = (title) => document.body.querySelector(`[aria-label="Restaurar “${title}”"]`);
  const purgeOf = (title) => document.body.querySelector(`[aria-label="Apagar de vez “${title}”"]`);
  const dialogText = () => document.body.querySelector('[role="dialog"]')?.textContent ?? '';
  const dialogButton = (text) => [...document.body.querySelector('[role="dialog"]').querySelectorAll('button')].find((b) => b.textContent === text);

  it('has a section for the retired works above the files in the trash, each saying what it holds', async () => {
    await open({ works: [retired()] });
    const headings = [...document.body.querySelectorAll('h2')].map((h) => h.textContent);
    expect(headings).toEqual(['Obras retiradas', 'Arquivos na lixeira', 'Limpeza automática']);
    expect(view.text()).toContain('Obras que saíram do acervo');
    expect(view.text()).toContain('Arquivos de obras que você apagou de vez');
  });

  it('says the title, the author, when and by whom it was retired, and the files it keeps', async () => {
    await open({ works: [retired()] });
    expect(rows()).toHaveLength(1);
    const text = rows()[0].textContent;
    expect(text).toContain('Neuromancer');
    expect(text).toContain('William Gibson · retirada em');
    expect(text).toContain('por Ana');
    expect(text).toContain('EPUB + PDF · 2 arquivos · 3,0 MB no disco.');
    expect(view.text()).toContain('1 obra retirada.');
  });

  it('counts the works in the plural, and says when there are none', async () => {
    await open({ works: [retired(), retired({ id: 8, title: 'Duna' })], total: 2 });
    expect(view.text()).toContain('2 obras retiradas.');
    view.unmount();
    await open({ works: [] });
    expect(view.text()).toContain('Nenhuma obra retirada.');
    expect(rows()).toHaveLength(0);
  });

  it('says it without the name of who retired it when that is not known, and without an author', async () => {
    await open({ works: [retired({ retiredBy: '', author: '' })] });
    const text = rows()[0].textContent;
    expect(text).toMatch(/^Neuromancer(retirada em|\s)/);
    expect(text).not.toContain(' por ');
    expect(text).not.toContain('·  ·');
  });

  it('says the files in one line for each state: one file, none, some in the trash, all in the trash', async () => {
    await open({
      works: [
        retired({ id: 1, title: 'Um', files: 1, formats: ['EPUB'], sizeBytes: 2048 }),
        retired({ id: 2, title: 'Dois', files: 0, formats: [], sizeBytes: 0 }),
        retired({ id: 3, title: 'Três', files: 3, formats: ['CBZ'], inTrash: 1, sizeBytes: 1024 }),
        retired({ id: 4, title: 'Quatro', files: 2, formats: ['PDF'], inTrash: 2, sizeBytes: 0 }),
      ],
    });
    const lines = rows().map((li) => li.querySelectorAll('p')[2].textContent);
    expect(lines).toEqual([
      'EPUB · 1 arquivo · 2,0 KB no disco.',
      'Sem arquivo guardado.',
      'CBZ · 3 arquivos (1 na lixeira dos arquivos, logo abaixo) · 1,0 KB no disco.',
      'PDF · 2 arquivos, na lixeira dos arquivos, logo abaixo.',
    ]);
  });

  it('restores a work without asking, since nothing is lost, and says it came back', async () => {
    await open({ works: [retired()] });
    await view.click(restoreOf('Neuromancer'));
    expect(api.post).toHaveBeenCalledWith('/works/7/restore');
    expect(document.body.querySelector('[role="status"]').textContent).toBe('“Neuromancer” voltou ao acervo.');
  });

  it('does not restore a work whose files are in the trash, and says why', async () => {
    await open({ works: [retired({ inTrash: 2 })] });
    expect(restoreOf('Neuromancer').disabled).toBe(true);
    expect(restoreOf('Neuromancer').title).toBe('Recupere os arquivos na lixeira, logo abaixo, para restaurar a obra');
    expect(rows()[0].textContent).toContain('Falta apagar os arquivos, abaixo.');
    expect(purgeOf('Neuromancer')).toBeNull(); // the next step is in the list below
  });

  it('keeps both buttons for a work that has some files left, and one with no file at all', async () => {
    await open({ works: [retired({ id: 1, title: 'Parcial', files: 3, inTrash: 1 }), retired({ id: 2, title: 'Vazia', files: 0, formats: [] })] });
    expect(restoreOf('Parcial').disabled).toBe(true); // a file is in the trash
    expect(purgeOf('Parcial')).not.toBeNull();
    expect(restoreOf('Vazia').disabled).toBe(false);
    expect(purgeOf('Vazia')).not.toBeNull();
    expect(rows()[1].textContent).not.toContain('Falta apagar');
  });

  it('asks before deleting for good, and says where the files go', async () => {
    await open({ works: [retired()] });
    await view.click(purgeOf('Neuromancer'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(dialogText()).toContain('Apagar esta obra de vez?');
    expect(dialogText()).toContain('Os 2 arquivos de “Neuromancer” (3,0 MB) vão para a lixeira, logo abaixo');
    expect(dialogText()).toContain('ainda dá para recuperá-los');
    expect(dialogText()).toContain('quando eles forem apagados do disco');
    await view.click(dialogButton('Apagar de vez'));
    expect(api.delete).toHaveBeenCalledWith('/works/7', { params: { purge: true } });
  });

  it('says in the singular for one file, and that a work with no file is deleted at once', async () => {
    await open({ works: [retired({ id: 1, title: 'Um', files: 1, sizeBytes: 1024 }), retired({ id: 2, title: 'Vazia', files: 0, formats: [] })] });
    await view.click(purgeOf('Um'));
    expect(dialogText()).toContain('O arquivo de “Um” (1,0 KB) vai para a lixeira, logo abaixo, de onde ainda dá para recuperá-lo.');
    expect(dialogText()).toContain('quando ele for apagado do disco');
    expect(dialogText()).not.toContain('Os 1 ');
  });

  it('says a work with no file is deleted at once', async () => {
    await open({ works: [retired({ id: 2, title: 'Vazia', files: 0, formats: [] })] });
    await view.click(purgeOf('Vazia'));
    expect(dialogText()).toContain('não tem arquivo guardado aqui: a obra será apagada de vez');
  });

  it('leaves the work alone when the question is cancelled', async () => {
    await open({ works: [retired()] });
    await view.click(purgeOf('Neuromancer'));
    await view.click(view.button('Cancelar'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
  });

  it('says what happened after deleting for good: the files went to the trash, or the work is gone', async () => {
    await open({ works: [retired()] });
    api.delete.mockResolvedValueOnce({ data: { trashed: 2, deleted: false } });
    await view.click(purgeOf('Neuromancer'));
    await view.click(dialogButton('Apagar de vez'));
    expect(document.body.querySelector('[role="status"]').textContent).toContain('Os arquivos de “Neuromancer” foram para a lixeira, logo abaixo.');
    expect(document.body.querySelector('[role="status"]').textContent).toContain('quando eles forem apagados do disco');
    api.delete.mockResolvedValueOnce({ data: { trashed: 1, deleted: false } });
    await view.click(purgeOf('Neuromancer'));
    await view.click(dialogButton('Apagar de vez'));
    expect(document.body.querySelector('[role="status"]').textContent).toContain('O arquivo de “Neuromancer” foi para a lixeira, logo abaixo. A obra sai desta lista quando ele for apagado do disco.');
    api.delete.mockResolvedValueOnce({ data: { trashed: 0, deleted: true } });
    await view.click(purgeOf('Neuromancer'));
    await view.click(dialogButton('Apagar de vez'));
    expect(document.body.querySelector('[role="status"]').textContent).toBe('“Neuromancer” foi apagada de vez.');
  });

  it('says what the server refused', async () => {
    await open({ works: [retired()] });
    api.post.mockRejectedValueOnce(Object.assign(new Error('x'), { response: { status: 404, data: 'Book not found' } }));
    await view.click(restoreOf('Neuromancer'));
    expect(document.body.querySelector('[role="alert"]').textContent.length).toBeGreaterThan(0);
    expect(document.body.querySelector('[role="status"]')).toBeNull();
  });

  it('turns the pages of the list, a page asks for the next one, and the first has no way back', async () => {
    await open({ works: [retired()], total: 45, totalPages: 3 });
    retiredPages[2] = { data: [retired({ id: 9, title: 'Segunda' })], total: 45, page: 2, limit: 20, totalPages: 3 };
    const nav = () => document.body.querySelector('nav[aria-label="Páginas das obras retiradas"]');
    expect(nav().textContent).toContain('Página 1 de 3');
    expect(view.button('Anterior').disabled).toBe(true);
    await view.click(view.button('Próxima'));
    expect(api.get).toHaveBeenCalledWith('/admin/retired-works', { params: { page: 2, limit: 20 } });
    expect(rows()[0].textContent).toContain('Segunda');
    expect(nav().textContent).toContain('Página 2 de 3');
    expect(view.button('Anterior').disabled).toBe(false);
  });

  it('has no way forward from the last page', async () => {
    await open({ works: [retired()], total: 25, totalPages: 2 });
    retiredPages[2] = { data: [retired({ id: 9, title: 'Última' })], total: 25, page: 2, limit: 20, totalPages: 2 };
    expect(view.button('Próxima').disabled).toBe(false);
    await view.click(view.button('Próxima'));
    expect(view.button('Próxima').disabled).toBe(true);
    expect(view.button('Anterior').disabled).toBe(false);
  });

  it('has no pages when everything fits in one', async () => {
    await open({ works: [retired()] });
    expect(document.body.querySelector('nav[aria-label="Páginas das obras retiradas"]')).toBeNull();
  });

  it('goes back to the last page that there is when the one on screen was emptied', async () => {
    await open({ works: [retired()], total: 21, totalPages: 2 });
    retiredPages[2] = { data: [retired({ id: 9, title: 'Última' })], total: 21, page: 2, limit: 20, totalPages: 2 };
    await view.click(view.button('Próxima'));
    expect(rows()[0].textContent).toContain('Última');
    retiredPages[2] = { data: [], total: 20, page: 2, limit: 20, totalPages: 1 };
    retiredPages[1] = { data: [retired({ id: 1, title: 'Primeira' })], total: 20, page: 1, limit: 20, totalPages: 1 };
    await view.click(restoreOf('Última'));
    expect(rows()[0].textContent).toContain('Primeira');
  });

  it('goes back to the last page that there is, not to the first, when the one on screen was emptied', async () => {
    await open({ works: [retired()], total: 45, totalPages: 3 });
    retiredPages[2] = { data: [retired({ id: 8, title: 'Segunda' })], total: 45, page: 2, limit: 20, totalPages: 3 };
    retiredPages[3] = { data: [retired({ id: 9, title: 'Terceira' })], total: 45, page: 3, limit: 20, totalPages: 3 };
    await view.click(view.button('Próxima'));
    await view.click(view.button('Próxima'));
    expect(rows()[0].textContent).toContain('Terceira');
    retiredPages[3] = { data: [], total: 40, page: 3, limit: 20, totalPages: 2 };
    retiredPages[2] = { data: [retired({ id: 8, title: 'Segunda' })], total: 40, page: 2, limit: 20, totalPages: 2 };
    await view.click(restoreOf('Terceira'));
    expect(rows()[0].textContent).toContain('Segunda');
  });

  it('says in Portuguese that the list could not be loaded, and tries again', async () => {
    await open({ works: [retired()] });
    api.get.mockImplementation(async (url) => {
      if (url === '/admin/retired-works') throw Object.assign(new Error('x'), { response: { status: 500, data: 'boom' } });
      if (url === '/admin/trash') return { data: { items: [], totalBytes: 0, policy: { enabled: false, days: 30 } } };
      throw new Error(`unexpected GET ${url}`);
    });
    view.unmount();
    view = await mount(<TrashTab isOwner />);
    expect(view.text()).toContain('Não foi possível carregar as obras retiradas.');
  });
});

