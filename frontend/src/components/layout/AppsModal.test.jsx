import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../lib/api';
import { mount, flush } from '../../features/admin/testUtils';
import { AppsModal } from './AppsModal';
import { md5 } from '../../lib/md5';

let view;
const SECRET = 'cdc_s3cr3t-token-value';

const KOREADER = { id: 't1', name: 'KOReader', createdAt: '2026-09-01T10:00:00Z', lastUsedAt: '2026-09-20T18:30:00Z' };
const NEVER = { id: 't2', name: 'Moon+ Reader', createdAt: '2026-09-25T10:00:00Z' };

async function open(tokens = [], onClose = vi.fn()) {
  let list = tokens;
  api.get.mockImplementation(async (url) => {
    if (url === '/auth/me') return { data: { id: 'u1', username: 'maria', role: 'reader' } };
    if (url === '/auth/app-tokens') return { data: list };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockImplementation(async (url, body) => {
    list = [{ id: 't9', name: body.name, createdAt: '2026-10-01T10:00:00Z' }, ...list];
    return { data: { id: 't9', name: body.name, scope: 'app', token: SECRET } };
  });
  api.delete.mockResolvedValue({});
  view = await mount(<AppsModal onClose={onClose} />);
  return onClose;
}
const input = (label) => document.body.querySelector(`input[aria-label="${label}"]`);
const nameInput = () => [...document.body.querySelectorAll('input')].find((i) => !i.readOnly);
const dialogs = () => [...document.body.querySelectorAll('[role="dialog"]')];

beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('AppsModal', () => {
  it('explains what an access is and says there is none yet', async () => {
    await open([]);
    const text = view.dialog().textContent;
    expect(text).toContain('KOReader');
    expect(text).toContain('serve apenas para ler');
    expect(text).toContain('Nenhum ainda');
  });

  it('lists the accesses with when they were created and last used, or that they never were', async () => {
    await open([KOREADER, NEVER]);
    const text = view.dialog().textContent;
    expect(text).toContain('KOReader');
    expect(text).toContain('usado pela última vez em');
    expect(text).toContain('Moon+ Reader');
    expect(text).toContain('ainda não usado');
    expect(text).not.toContain('Nenhum ainda');
  });

  it('starts from a name already filled in, so creating takes one click', async () => {
    await open();
    expect(nameInput().value).toBe('KOReader');
    await view.click(view.button('Criar acesso'));
    expect(api.post).toHaveBeenCalledWith('/auth/app-tokens', { name: 'KOReader', scope: 'koreader' });
  });

  it('takes a suggestion or any other name, and does not create one without a name', async () => {
    await open();
    await view.click(view.button('Librera'));
    expect(nameInput().value).toBe('Librera');
    await view.type(nameInput(), '   ');
    expect(view.button('Criar acesso').disabled).toBe(true);
    await view.type(nameInput(), '  Leitor da sala ');
    await view.click(view.button('Criar acesso'));
    expect(api.post).toHaveBeenCalledWith('/auth/app-tokens', { name: 'Leitor da sala', scope: 'app' });
  });

  it('shows address, username and the password to type into the app, once', async () => {
    await open();
    await view.click(view.button('Criar acesso'));
    expect(input('Endereço').value).toBe(`${window.location.origin}/opds/v1.2/catalog`);
    expect(input('Usuário').value).toBe('maria');
    expect(input('Senha').value).toBe(SECRET);
    expect(view.dialog().textContent).toContain('aparece só agora');
    expect(view.dialog().textContent).not.toContain(SECRET); // only the field holds it, never the text of the page
    await view.click(view.button('Já copiei'));
    expect(document.body.innerHTML).not.toContain(SECRET);
    expect(view.dialog().textContent).toContain('Qual aplicativo vai usar?');
    expect(view.dialog().textContent).toContain('KOReader'); // now in the list, by name
  });

  it('warns that localhost does not work on a phone', async () => {
    await open();
    await view.click(view.button('Criar acesso'));
    expect(view.dialog().textContent).toContain('só funciona neste computador');
  });

  it('copies each value and says it did', async () => {
    const writeText = vi.fn().mockResolvedValue();
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    await open();
    await view.click(view.button('Criar acesso'));
    await view.click(view.container.ownerDocument.body.querySelector('button[aria-label="Copiar senha"]'));
    expect(writeText).toHaveBeenCalledWith(SECRET);
    await view.click(view.container.ownerDocument.body.querySelector('button[aria-label="Copiar usuário"]'));
    expect(writeText).toHaveBeenLastCalledWith('maria');
    expect(view.dialog().textContent).toContain('Copiado');
  });

  it('keeps the values on screen to copy by hand when the browser has no clipboard', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: vi.fn().mockRejectedValue(new Error('no')) }, configurable: true });
    await open();
    await view.click(view.button('Criar acesso'));
    await view.click(document.body.querySelector('button[aria-label="Copiar senha"]'));
    expect(view.dialog().textContent).not.toContain('Copiado');
    expect(input('Senha').value).toBe(SECRET);
  });

  it('tells the person when it could not create', async () => {
    await open();
    api.post.mockRejectedValue(new Error('boom'));
    await view.click(view.button('Criar acesso'));
    expect(view.dialog().textContent).toContain('Não foi possível criar o acesso.');
    expect(view.dialog().textContent).not.toContain('Pronto');
  });

  it('asks before removing, and the app loses access only once confirmed', async () => {
    await open([KOREADER, NEVER]);
    await view.click(document.body.querySelector('button[aria-label="Remover o acesso KOReader"]'));
    expect(dialogs()).toHaveLength(2);
    expect(dialogs()[1].textContent).toContain('perde o acesso na hora');
    await view.click(view.button('Cancelar'));
    expect(api.delete).not.toHaveBeenCalled();

    await view.click(document.body.querySelector('button[aria-label="Remover o acesso KOReader"]'));
    await view.click(view.button('Remover acesso'));
    expect(api.delete).toHaveBeenCalledTimes(1);
    expect(api.delete).toHaveBeenCalledWith('/auth/app-tokens/t1');
  });

  it('says so when it could not remove', async () => {
    await open([KOREADER]);
    api.delete.mockRejectedValue(new Error('boom'));
    await view.click(document.body.querySelector('button[aria-label="Remover o acesso KOReader"]'));
    await view.click(view.button('Remover acesso'));
    expect(view.dialog().textContent).toContain('Não foi possível remover o acesso.');
  });

  it('closes with Escape, but with a question open only the question closes', async () => {
    const onClose = await open([KOREADER]);
    await view.click(document.body.querySelector('button[aria-label="Remover o acesso KOReader"]'));
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await flush();
    expect(dialogs()).toHaveLength(1);
    expect(onClose).not.toHaveBeenCalled();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await flush();
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('does not keep the list once it closes, so another account never sees it', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    api.get.mockImplementation(async (url) => (url === '/auth/me' ? { data: { id: 'u1', username: 'maria', role: 'reader' } } : { data: [KOREADER] }));
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);
    await act(async () => { root.render(<QueryClientProvider client={client}><AppsModal onClose={() => {}} /></QueryClientProvider>); });
    await flush();
    expect(client.getQueryData(['app-tokens'])).toEqual([KOREADER]);
    await act(async () => { root.render(<QueryClientProvider client={client}><div /></QueryClientProvider>); });
    await flush();
    expect(client.getQueryData(['app-tokens'])).toBeUndefined();
    act(() => root.unmount());
    container.remove();
  });
});

describe('AppsModal, how it appears', () => {
  it('fades the backdrop in and lets the dialog rise, with the motion of the system', async () => {
    await open();
    expect(view.dialog().className).toContain('animate-pop-in');
    expect(view.dialog().parentElement.className).toContain('animate-fade-in');
  });

  it('asks for the long password by the name of another app, and for the short one by the name of KOReader', async () => {
    await open();
    expect(document.body.querySelector('input[type="checkbox"]').checked).toBe(true); // KOReader is the first suggestion
    await view.click(view.button('Librera'));
    expect(document.body.querySelector('input[type="checkbox"]').checked).toBe(false);
    await view.click(view.button('KOReader'));
    expect(document.body.querySelector('input[type="checkbox"]').checked).toBe(true);
    await view.click(document.body.querySelector('input[type="checkbox"]'));
    await view.click(view.button('Criar acesso'));
    expect(api.post).toHaveBeenLastCalledWith('/auth/app-tokens', { name: 'KOReader', scope: 'app' });
  });

  it('shows the short password in groups, says it can be typed without the spaces, and gives the file of the catalog', async () => {
    await open();
    api.post.mockResolvedValueOnce({ data: { id: 't9', name: 'KOReader', scope: 'koreader', token: '7k2m 9xqa r4bd h8tw' } });
    const click = vi.fn();
    const created = [];
    URL.createObjectURL = vi.fn((blob) => { created.push(blob); return 'blob:x'; });
    URL.revokeObjectURL = vi.fn();
    const original = HTMLAnchorElement.prototype.click;
    HTMLAnchorElement.prototype.click = function () { click(this.download); };
    try {
      await view.click(view.button('Criar acesso'));
      expect(document.body.querySelector('[aria-label="Senha do KOReader"]').textContent).toBe('7k2m 9xqa r4bd h8tw');
      expect(view.dialog().textContent).toContain('sem os espaços');
      expect(input('Usuário').value).toBe('maria');
      expect(view.dialog().textContent).toContain('substitui o que já estiver lá');
      await view.click(view.button('Baixar opds.lua'));
      expect(click).toHaveBeenCalledWith('opds.lua');
      await view.click(view.button('Baixar kosync.lua'));
      expect(click).toHaveBeenLastCalledWith('kosync.lua');
      const sync = await created[1].text();
      expect(sync).toContain('["custom_server"] = "' + window.location.origin + '/kosync"');
      expect(sync).toContain('["username"] = "maria"');
      expect(sync).toContain('["userkey"] = "' + md5('7k2m9xqar4bdh8tw') + '"');
      expect(sync).toContain('["auto_sync"] = true');
      expect(sync).not.toContain('7k2m');
      const text = await created[0].text();
      expect(text).toContain('["password"] = "7k2m9xqar4bdh8tw"');
      expect(text).toContain('["username"] = "maria"');
      expect(text).toContain(`["url"] = "${window.location.origin}/opds/v1.2/catalog"`);
    } finally {
      HTMLAnchorElement.prototype.click = original;
    }
  });

  it('says that a short password is one, and how long ago the device synced', async () => {
    const recent = new Date(Date.now() - 3 * 24 * 3600 * 1000).toISOString();
    await open([{ id: 't3', name: 'Kindle da sala', scope: 'koreader', createdAt: '2026-09-25T10:00:00Z', lastUsedAt: recent }]);
    const text = view.dialog().textContent;
    expect(text).toContain('Kindle da sala');
    expect(text).toContain('senha curta');
    expect(text).toContain('(há 3 dias)');
  });
});
