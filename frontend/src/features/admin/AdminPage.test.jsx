import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../lib/api';
import { mount } from './testUtils';
import { AdminPage } from './AdminPage';
import { isStaff } from '../auth/api/useMe';
import { Header } from '../../components/layout/Header';

let view;
beforeEach(() => {
  vi.clearAllMocks();
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/jobs') return { data: { data: [], counts: {} } };
    if (url === '/admin/trash') return { data: { items: [], totalBytes: 0, policy: { enabled: false, days: 30 } } };
    if (url === '/admin/storage/roots') return { data: { roots: [], managed: '/data' } };
    if (url === '/admin/storage/cleanups' || url === '/admin/storage/orphans') return { data: { data: [] } };
    if (url === '/admin/backup') return { data: { lastBackup: null } };
    if (url === '/admin/duplicates' || url === '/admin/ocr' || url === '/users') return { data: { data: [] } };
    if (url === '/admin/ldap') return { data: { configured: false, host: '', baseDN: '', linkedAccounts: 0, policy: { allowCreate: false, revalidateHours: 24 } } };
    if (url === '/admin/embeddings') return { data: { enabled: false, available: true, state: 'idle', model: 'sentence-transformers/LaBSE' } };
    throw new Error(`unexpected GET ${url}`);
  });
});
afterEach(() => view.unmount());

describe('AdminPage', () => {
  it('opens on the jobs and switches between the areas', async () => {
    view = await mount(<AdminPage isOwner />);
    expect(view.text()).toContain('O que o sistema está fazendo');

    await view.click(view.button('Armazenamento'));
    expect(view.text()).toContain('Importar uma pasta');
    expect(view.text()).toContain('Reorganizar o acervo');

    await view.click(view.button('Lixeira'));
    expect(view.text()).toContain('A lixeira está vazia');

    await view.click(view.button('Duplicatas e OCR'));
    expect(view.text()).toContain('Nenhuma sugestão pendente');

    await view.click(view.button('Contas'));
    expect(view.text()).toContain('Nenhuma conta');
  });

  it('offers the external login settings to the owner only', async () => {
    view = await mount(<AdminPage isOwner />);
    expect(view.button('Login externo')).toBeTruthy();
    await view.click(view.button('Login externo'));
    expect(view.text()).toContain('Login pelo diretório');
    view.unmount();

    view = await mount(<AdminPage isOwner={false} />);
    expect(view.button('Login externo')).toBeUndefined();
  });

  it('has a way back to the library, also on narrow screens', async () => {
    const back = vi.fn();
    view = await mount(<AdminPage isOwner onClose={back} />);
    await view.click(view.buttonMatching(/Voltar ao acervo/));
    expect(back).toHaveBeenCalledTimes(1);
  });

  it('marks the tab that is open', async () => {
    view = await mount(<AdminPage isOwner={false} />);
    await view.click(view.button('Lixeira'));
    const tab = view.button('Lixeira');
    expect(tab.getAttribute('aria-selected')).toBe('true');
    expect(view.button('Trabalhos').getAttribute('aria-selected')).toBe('false');
  });
});

describe('who sees the administration', () => {
  it('is for the owner and admins only', () => {
    expect(isStaff({ role: 'owner' })).toBe(true);
    expect(isStaff({ role: 'admin' })).toBe(true);
    expect(isStaff({ role: 'reader' })).toBe(false);
    expect(isStaff(undefined)).toBe(false);
  });

  it('shows the header button only to them, and it opens the area', async () => {
    const open = vi.fn();
    view = await mount(<Header canAdmin onOpenAdmin={open} />);
    await view.click(view.container.querySelector('[aria-label="Minha conta"]'));
    await view.click(view.button('Administração'));
    expect(open).toHaveBeenCalledTimes(1);
    view.unmount();

    view = await mount(<Header canAdmin={false} onOpenAdmin={open} />);
    await view.click(view.container.querySelector('[aria-label="Minha conta"]'));
    expect(view.button('Administração')).toBeUndefined();
  });

  it('does not offer to add books to a reader, who would only get a 403', async () => {
    view = await mount(<Header canAdmin={false} />);
    expect(view.buttonMatching(/Adicionar/)).toBeUndefined();
    view.unmount();

    view = await mount(<Header canAdmin />);
    expect(view.buttonMatching(/Adicionar/)).toBeTruthy();
  });
});
