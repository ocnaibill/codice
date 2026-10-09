import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { act } from 'react';
import { mount, flush } from '../testUtils';
import { StorageTab } from './StorageTab';

const plan = {
  hash: 'abc123', unchanged: 4, skipped: [],
  moves: [{ fileId: 1, workId: 1, title: 'Duna', from: 'Duna.epub', to: 'Frank Herbert/Duna/Duna.epub' }],
};
let view;

async function open({ isOwner = true, cleanups = [], orphans = [], root = {} } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/storage/roots') return { data: { roots: [{ id: 2, path: '/mnt/livros', files: 0, retiredFiles: 0, retiredWorks: 0, purgeableWorks: 0, ...root }], managed: '/data/uploads' } };
    if (url === '/admin/storage/cleanups') return { data: { data: cleanups } };
    if (url === '/admin/storage/orphans') return { data: { data: orphans } };
    if (url === '/admin/jobs') return { data: { data: [], counts: {} } };
    if (url === '/admin/storage/referenced') return { data: { data: [], total: 0, summary: { ok: 0, missing: 0, conflict: 0 } } };
    if (url === '/admin/storage/reorganize') return { data: plan };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: { moved: 1, failures: [] } });
  api.delete.mockResolvedValue({});
  view = await mount(<StorageTab isOwner={isOwner} />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('StorageTab', () => {
  it('scans an authorised folder', async () => {
    await open();
    await view.click(view.button('Varrer'));
    expect(api.post).toHaveBeenCalledWith('/admin/library/scan', { rootId: 2 });
  });

  it('lets only the owner authorise or remove a folder', async () => {
    await open({ isOwner: false });
    expect(view.button('Autorizar pasta')).toBeUndefined();
    expect(view.button('Remover')).toBeUndefined();
    view.unmount();

    await open({ isOwner: true });
    expect(view.button('Autorizar pasta')).toBeTruthy();
    expect(view.button('Remover')).toBeTruthy();
  });

  it('asks before removing a folder', async () => {
    await open();
    await view.click(view.button('Remover'));
    expect(api.delete).not.toHaveBeenCalled();
    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(api.delete).toHaveBeenCalledWith('/admin/storage/roots/2');
  });

  it('shows the reorganization plan first and moves nothing until confirmed with its hash', async () => {
    await open();
    await view.click(view.button('Ver plano'));
    expect(view.text()).toContain('Duna.epub → Frank Herbert/Duna/Duna.epub');
    expect(api.post).not.toHaveBeenCalled();

    await view.click(view.buttonMatching(/^Mover 1 arquivo/));
    expect(api.post).not.toHaveBeenCalled();
    await view.click(view.button('Reorganizar'));
    expect(api.post).toHaveBeenCalledWith('/admin/storage/reorganize', { hash: 'abc123' });
  });

  it('sends the chosen orphans to the (recoverable) trash after confirming', async () => {
    await open({ orphans: [{ path: 'sobra.epub', sizeBytes: 100 }, { path: 'outra.pdf', sizeBytes: 200 }] });
    const box = view.container.querySelector('input[aria-label="Selecionar sobra.epub"]');
    await view.click(box);
    await view.click(view.buttonMatching(/^Enviar 1 para a lixeira/));
    expect(api.post).not.toHaveBeenCalled();

    await view.click(view.button('Enviar para a lixeira'));
    expect(api.post).toHaveBeenCalledWith('/admin/storage/orphans/trash', { paths: ['sobra.epub'] });
  });

  it('lists originals that could not be removed and retries them', async () => {
    await open({ cleanups: [{ id: 1, path: '/origem/a.epub', reason: 'sem permissão' }] });
    expect(view.text()).toContain('sem permissão');
    await view.click(view.button('Tentar apagar de novo'));
    expect(api.post).toHaveBeenCalledWith('/admin/storage/cleanups/retry');
  });

  it('says in Portuguese why an original could not be removed, with what the system said', async () => {
    await open({ cleanups: [{ id: 1, path: '/origem/a.epub', reason: 'the original could not be removed: remove /origem/a.epub: permission denied' }] });
    expect(view.text()).toContain('Não foi possível apagar o original: remove /origem/a.epub: permission denied');
  });

  it('has the referenced files with the folders, and the backup is no longer here (it is in the Sistema tab)', async () => {
    await open();
    const text = view.text();
    expect(text.indexOf('Pastas autorizadas')).toBeLessThan(text.indexOf('Arquivos referenciados'));
    expect(text).not.toContain('Último backup');
    expect(text).not.toContain('Nenhum backup registrado');
  });
});

describe('StorageTab: the retired works of a folder (#230)', () => {
  const retired = { files: 21, retiredFiles: 2931, retiredWorks: 2930, purgeableWorks: 2930 };

  it('says what the catalog keeps in a folder, the active files apart from the ones of retired works', async () => {
    await open({ root: retired });
    expect(view.text()).toContain('21 arquivos no acervo · 2931 arquivos de 2930 obras retiradas');
    view.unmount();
    await open({ root: { files: 1, retiredFiles: 1, retiredWorks: 1, purgeableWorks: 1 } });
    expect(view.text()).toContain('1 arquivo no acervo · 1 arquivo de 1 obra retirada');
    view.unmount();
    await open();
    expect(view.text()).not.toContain('retirada');
  });

  it('offers to delete the retired works for good only when there are some the folder can delete', async () => {
    await open();
    expect(view.buttonMatching(/^Apagar/)).toBeUndefined();
    view.unmount();
    await open({ root: { retiredFiles: 3, retiredWorks: 2, purgeableWorks: 0 } });
    expect(view.buttonMatching(/^Apagar/)).toBeUndefined();
    expect(view.text()).toContain('2 obras retiradas guardam também arquivos no servidor: apague-as na Lixeira.');
    expect(view.text()).toContain('3 arquivos de 2 obras retiradas');
    expect(view.text()).not.toContain('0 arquivo');
    view.unmount();
    await open({ root: { retiredFiles: 1, retiredWorks: 1, purgeableWorks: 0 } });
    expect(view.text()).toContain('1 obra retirada guarda também arquivos no servidor: apague-a na Lixeira.');
    view.unmount();
    await open({ root: retired });
    expect(view.button('Apagar 2930 obras retiradas')).toBeTruthy();
    expect(view.text()).not.toContain('guardam também');
    view.unmount();
    await open({ root: { retiredFiles: 1, retiredWorks: 1, purgeableWorks: 1 } });
    expect(view.button('Apagar 1 obra retirada')).toBeTruthy();
  });

  it('lets an admin delete them too, while only the owner removes the folder', async () => {
    await open({ isOwner: false, root: retired });
    expect(view.button('Apagar 2930 obras retiradas')).toBeTruthy();
    expect(view.button('Remover')).toBeUndefined();
  });

  it('asks first, and says that the files of the folder stay where they are', async () => {
    await open({ root: retired });
    await view.click(view.button('Apagar 2930 obras retiradas'));
    expect(api.post).not.toHaveBeenCalled();
    const text = view.dialog().textContent;
    expect(text).toContain('2930 obras retiradas com arquivo em /mnt/livros serão apagadas de vez');
    expect(text).toContain('os registros delas saem do Códice: os arquivos da pasta continuam onde estão');
    expect(text).toContain('Isso não tem volta.');
    await view.click(view.dialog().querySelectorAll('button')[0]);
    expect(api.post).not.toHaveBeenCalled();
    expect(view.dialog()).toBeNull();
  });

  it('says it in the singular for one work', async () => {
    await open({ root: { retiredFiles: 1, retiredWorks: 1, purgeableWorks: 1 } });
    await view.click(view.button('Apagar 1 obra retirada'));
    expect(view.dialog().textContent).toContain('1 obra retirada com arquivo em /mnt/livros será apagada de vez. Só o registro dela sai do Códice');
  });

  it('asks the server again until none waits, shows how far it got, and says how many it deleted', async () => {
    await open({ root: retired });
    let release;
    const second = new Promise((resolve) => { release = resolve; });
    api.post
      .mockResolvedValueOnce({ data: { purged: 200, remaining: 2730 } })
      .mockImplementationOnce(() => second)
      .mockResolvedValue({ data: { purged: 0, remaining: 0 } });
    await view.click(view.button('Apagar 2930 obras retiradas'));
    await view.click(view.button('Apagar de vez'));
    expect(api.post).toHaveBeenCalledWith('/admin/storage/roots/2/purge-retired');
    expect(view.text()).toContain('Apagando as obras retiradas… 200 obras apagadas até agora.');
    expect(view.button('Apagar 2930 obras retiradas').disabled).toBe(true);
    await act(async () => { release({ data: { purged: 2730, remaining: 0 } }); });
    await flush();
    expect(api.post).toHaveBeenCalledTimes(2);
    expect(view.text()).toContain('2930 obras retiradas foram apagadas de vez. Os arquivos de /mnt/livros não foram tocados.');
    expect(view.text()).not.toContain('Apagando as obras retiradas');
  });

  it('says it in the singular for one deleted', async () => {
    await open({ root: { retiredFiles: 1, retiredWorks: 1, purgeableWorks: 1 } });
    api.post.mockResolvedValueOnce({ data: { purged: 1, remaining: 0 } });
    await view.click(view.button('Apagar 1 obra retirada'));
    await view.click(view.button('Apagar de vez'));
    expect(view.text()).toContain('1 obra retirada foi apagada de vez.');
  });

  it('starts again from nothing: the number and the sentence of the last time are not shown while it runs', async () => {
    await open({ root: { retiredFiles: 5, retiredWorks: 5, purgeableWorks: 5 } });
    api.post.mockResolvedValueOnce({ data: { purged: 5, remaining: 0 } });
    await view.click(view.button('Apagar 5 obras retiradas'));
    await view.click(view.button('Apagar de vez'));
    expect(view.text()).toContain('5 obras retiradas foram apagadas de vez.');
    let release;
    api.post.mockImplementationOnce(() => new Promise((resolve) => { release = resolve; }));
    await view.click(view.button('Apagar 5 obras retiradas'));
    await view.click(view.button('Apagar de vez'));
    expect(view.text()).toContain('Apagando as obras retiradas… 0 obras apagadas até agora.');
    expect(view.text()).not.toContain('foram apagadas de vez');
    await act(async () => { release({ data: { purged: 0, remaining: 0 } }); });
    await flush();
  });

  it('takes away the refusal to remove the folder once the works it was about are deleted', async () => {
    await open({ root: { files: 2, retiredFiles: 3, retiredWorks: 3, purgeableWorks: 3 } });
    api.delete.mockRejectedValueOnce({ response: { status: 409, data: 'Files in this directory are still catalogued: 2 in the catalog, 3 from retired works' } });
    await view.click(view.button('Remover'));
    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('Ainda há 2 arquivos desta pasta no acervo e 3 arquivos de obras retiradas');
    api.post.mockResolvedValueOnce({ data: { purged: 3, remaining: 0 } });
    await view.click(view.button('Apagar 3 obras retiradas'));
    await view.click(view.button('Apagar de vez'));
    expect(view.container.querySelector('[role="alert"]')).toBeNull();
    expect(view.text()).toContain('3 obras retiradas foram apagadas de vez.');
  });

  it('stops when a call deletes nothing, whatever it says still waits', async () => {
    await open({ root: retired });
    api.post.mockResolvedValue({ data: { purged: 0, remaining: 7 } });
    await view.click(view.button('Apagar 2930 obras retiradas'));
    await view.click(view.button('Apagar de vez'));
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(view.text()).toContain('0 obras retiradas foram apagadas de vez.');
  });

  it('says what went wrong and asks the lists again, since what was deleted before stays deleted', async () => {
    await open({ root: retired });
    api.post
      .mockResolvedValueOnce({ data: { purged: 200, remaining: 2730 } })
      .mockRejectedValueOnce({ response: { status: 500, data: 'Error deleting the book' } });
    const reads = () => api.get.mock.calls.filter(([url]) => url === '/admin/storage/roots').length;
    const before = reads();
    await view.click(view.button('Apagar 2930 obras retiradas'));
    await view.click(view.button('Apagar de vez'));
    expect(api.post).toHaveBeenCalledTimes(2);
    expect(view.container.querySelector('[role="alert"]')).not.toBeNull();
    expect(view.text()).not.toContain('foram apagadas de vez');
    expect(reads()).toBeGreaterThan(before);
  });
});
