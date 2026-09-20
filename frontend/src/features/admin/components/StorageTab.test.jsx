import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { StorageTab } from './StorageTab';

const plan = {
  hash: 'abc123', unchanged: 4, skipped: [],
  moves: [{ fileId: 1, workId: 1, title: 'Duna', from: 'Duna.epub', to: 'Frank Herbert/Duna/Duna.epub' }],
};
let view;

let lastBackup = null;
async function open({ isOwner = true, cleanups = [], orphans = [], last = null } = {}) {
  lastBackup = last;
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/storage/roots') return { data: { roots: [{ id: 2, path: '/mnt/livros' }], managed: '/data/uploads' } };
    if (url === '/admin/storage/cleanups') return { data: { data: cleanups } };
    if (url === '/admin/storage/orphans') return { data: { data: orphans } };
    if (url === '/admin/storage/reorganize') return { data: plan };
    if (url === '/admin/backup') return { data: { lastBackup } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: { moved: 1, failures: [] } });
  api.delete.mockResolvedValue({});
  view = await mount(<StorageTab isOwner={isOwner} />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('StorageTab backup', () => {
  const hoursAgo = (h) => new Date(Date.now() - h * 3600 * 1000).toISOString();

  it('says so, in red, when no backup was ever made', async () => {
    await open();
    expect(view.text()).toContain('Nenhum backup registrado');
  });

  it('shows the latest backup and what it holds', async () => {
    await open({ last: { at: hoursAgo(3), bytes: 2048, files: 12, includesFiles: true, encrypted: true } });
    expect(view.text()).toContain('Último backup');
    expect(view.text()).toContain('com 12 arquivo(s)');
    expect(view.text()).toContain('criptografado');
    expect(view.text()).not.toContain('Faz mais de dois dias');
  });

  it('calls out a backup older than two days, since the goal is one a day', async () => {
    await open({ last: { at: hoursAgo(60), bytes: 2048, files: 0, includesFiles: false, encrypted: false } });
    expect(view.text()).toContain('Faz mais de dois dias');
    expect(view.text()).toContain('sem criptografia');
  });
});

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
});
