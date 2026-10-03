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
let queueTotal = 0;
let providerList = [{ id: 'openlibrary', name: 'Open Library', enabled: true, sends: ['title'] }];
beforeEach(() => {
  vi.clearAllMocks();
  queueTotal = 0;
  providerList = [{ id: 'openlibrary', name: 'Open Library', enabled: true, sends: ['title'] }];
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/jobs') return { data: { data: [], counts: {} } };
    if (url === '/admin/trash') return { data: { items: [], totalBytes: 0, policy: { enabled: false, days: 30 } } };
    if (url === '/admin/storage/roots') return { data: { roots: [], managed: '/data' } };
    if (url === '/admin/storage/cleanups' || url === '/admin/storage/orphans') return { data: { data: [] } };
    if (url === '/admin/storage/referenced') return { data: { data: [], total: 0, summary: { ok: 0, missing: 0, conflict: 0 } } };
    if (url === '/admin/backup') return { data: { lastBackup: null } };
    if (url === '/admin/metadata-providers') return { data: { data: providerList } };
    if (url === '/admin/suggestions') return { data: { data: [], total: queueTotal } };
    if (url === '/admin/duplicates' || url === '/admin/ocr' || url === '/users') return { data: { data: [] } };
    if (url === '/admin/ldap') return { data: { configured: false, host: '', baseDN: '', linkedAccounts: 0, policy: { allowCreate: false, revalidateHours: 24 } } };
    if (url === '/admin/ocr/settings') return { data: { enabled: false, language: 'por+eng', available: true, engine: 'tesseract', engineVersion: '5.5.0', languages: ['eng', 'por'], state: 'idle', error: '' } };
    if (url === '/admin/embeddings') return { data: { enabled: false, available: true, state: 'idle', model: 'sentence-transformers/LaBSE' } };
    if (url === '/admin/dictionaries') return { data: { packages: [{ id: 'wikt-pt', name: 'Wikcionário em português', description: 'd', installable: true, downloadBytes: 37158613, state: 'available', progress: 0, languages: [{ code: 'pt', level: 'complete' }], license: 'CC', licenseUrl: 'https://x', source: 's', sourceUrl: 'https://y' }] } };
    throw new Error(`unexpected GET ${url}`);
  });
});
afterEach(() => view.unmount());

describe('AdminPage: the queue of suggestions (#70)', () => {
  it('has the tab, which says how many works wait when some do, and shows the queue', async () => {
    queueTotal = 12;
    view = await mount(<AdminPage isOwner={false} />);
    expect(view.button('Sugestões (12)')).toBeTruthy();
    await view.click(view.button('Sugestões (12)'));
    expect(view.text()).toContain('Nenhuma obra com sugestões esperando.');
  });

  it('names the tab plainly when nothing waits', async () => {
    view = await mount(<AdminPage isOwner />);
    expect(view.button('Sugestões')).toBeTruthy();
    expect(view.text()).not.toContain('Sugestões (');
  });
});

describe('AdminPage: the external providers (#68)', () => {
  it('has the tab for the owner and the admin, and the owner can reach it from the queue', async () => {
    providerList = [{ id: 'openlibrary', name: 'Open Library', enabled: false, sends: ['title'] }];
    view = await mount(<AdminPage isOwner />);
    expect(view.button('Provedores')).toBeTruthy();
    await view.click(view.button('Sugestões'));
    await view.click(view.button('Escolher os provedores'));
    expect(view.text()).toContain('Provedores de metadados');
    expect(view.text()).toContain('Todos começam desligados');
    view.unmount();

    view = await mount(<AdminPage isOwner={false} />);
    await view.click(view.button('Provedores'));
    expect(view.text()).toContain('Só o dono do acervo liga ou desliga os provedores.');
  });
});

describe('AdminPage', () => {
  it('opens on the jobs and switches between the areas', async () => {
    view = await mount(<AdminPage isOwner />);
    expect(view.text()).toContain('O que o sistema está fazendo');

    await view.click(view.button('Armazenamento'));
    expect(view.text()).toContain('Importar uma pasta');
    expect(view.text()).toContain('Reorganizar o acervo');

    await view.click(view.button('Lixeira'));
    expect(view.text()).toContain('A lixeira está vazia');

    await view.click(view.button('Duplicatas'));
    expect(view.text()).toContain('Nenhuma sugestão pendente');
    expect(view.text()).not.toContain('PDFs com páginas sem texto'); // OCR has a tab of its own

    await view.click(view.button('OCR'));
    expect(view.text()).toContain('Leitura de páginas escaneadas (OCR)');
    expect(view.text()).toContain('Nenhum PDF precisa de OCR');

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

describe('AdminPage: the dictionaries (#109)', () => {
  it('has the tab for the whole staff, and the owner can install from it', async () => {
    view = await mount(<AdminPage isOwner />);
    await view.click(view.button('Dicionários'));
    expect(view.text()).toContain('Estes arquivos não são do Códice');
    expect(view.text()).toContain('Wikcionário em português');
    expect([...view.container.querySelectorAll('li[aria-label] button')].map((b) => b.textContent)).toEqual(['Instalar']);
  });

  it('shows an admin who is not the owner the same list, with nothing to press', async () => {
    view = await mount(<AdminPage isOwner={false} />);
    await view.click(view.button('Dicionários'));
    expect(view.text()).toContain('Wikcionário em português');
    expect(view.text()).toContain('Só o dono do acervo');
    expect(view.container.querySelectorAll('li[aria-label] button').length).toBe(0);
  });

  it('puts the tab with the others that are for the staff, before the accounts', async () => {
    view = await mount(<AdminPage isOwner />);
    const labels = [...view.container.querySelectorAll('[role="tab"]')].map((t) => t.textContent);
    expect(labels.indexOf('Dicionários')).toBe(labels.indexOf('OCR') + 1);
    expect(labels.indexOf('Dicionários')).toBeLessThan(labels.indexOf('Contas'));
  });
});
