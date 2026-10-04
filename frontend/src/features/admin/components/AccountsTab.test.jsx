import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { AccountsTab } from './AccountsTab';

const accounts = [
  { id: 'o1', username: 'boss', email: 'b@x', role: 'owner', blockedAt: null, canBlock: false, canRemove: false, isSelf: true },
  { id: 'r1', username: 'ana', email: 'a@x', role: 'reader', external: 'ldap', blockedAt: null, canBlock: true, canRemove: true, isSelf: false },
  { id: 'r2', username: 'bob', email: 'bo@x', role: 'reader', blockedAt: '2026-09-01T10:00:00Z', canBlock: true, canRemove: true, isSelf: false },
  { id: 'a2', username: 'adm', email: 'ad@x', role: 'admin', blockedAt: null, canBlock: false, canRemove: false, isSelf: false },
];
let view;

const FIREFOX = 'Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0';
const IPHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 Version/17.6 Mobile/15E148 Safari/604.1';
let anaSessions;

beforeEach(async () => {
  vi.clearAllMocks();
  anaSessions = [
    { id: 'x1', userAgent: FIREFOX, createdAt: '2026-10-01T09:00:00Z', lastSeenAt: new Date(Date.now() - 30 * 1000).toISOString() },
    { id: 'x2', userAgent: IPHONE, createdAt: '2026-10-02T09:00:00Z', lastSeenAt: null },
  ];
  api.get.mockImplementation(async (url) => (url === '/users/r1/sessions' ? { data: anaSessions } : { data: { data: accounts } }));
  api.delete.mockImplementation(async (url) => {
    if (url === '/users/r1/sessions') { const n = anaSessions.length; anaSessions = []; return { data: { revoked: n } }; }
    anaSessions = anaSessions.filter((x) => `/users/r1/sessions/${x.id}` !== url);
    return {};
  });
  api.post.mockResolvedValue({});
  view = await mount(<AccountsTab />);
});
afterEach(() => view.unmount());

describe('AccountsTab', () => {
  it('shows each account with its role and whether it is blocked', async () => {
    expect(view.text()).toContain('boss');
    expect(view.text()).toContain('(você)');
    expect(view.text()).toContain('Dono');
    expect(view.text()).toContain('Bloqueada');
  });

  it('marks the accounts that sign in through the directory', async () => {
    expect(view.text()).toContain('Diretório');
    expect([...document.querySelectorAll('span')].filter((s) => s.textContent === 'Diretório')).toHaveLength(1);
  });

  it('offers block or unblock only where the server allows it', async () => {
    const blockButtons = [...document.querySelectorAll('button')].filter((b) => b.textContent === 'Bloquear');
    expect(blockButtons).toHaveLength(1); // only ana: the owner, an admin and yourself are out
    expect(view.button('Desbloquear')).toBeTruthy(); // bob
  });

  it('blocks only after confirming', async () => {
    await view.click(view.button('Bloquear'));
    expect(view.dialog().textContent).toContain('Bloquear ana?');
    expect(api.post).not.toHaveBeenCalled();

    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(api.post).toHaveBeenCalledWith('/users/r1/block');
  });

  it('does not block when the question is cancelled', async () => {
    await view.click(view.button('Bloquear'));
    await view.click(view.button('Cancelar'));
    expect(api.post).not.toHaveBeenCalled();
  });

  it('unblocks with one click, since nothing is lost', async () => {
    await view.click(view.button('Desbloquear'));
    expect(api.post).toHaveBeenCalledWith('/users/r2/unblock');
  });

  it('offers to delete only where the server allows it', async () => {
    const del = [...document.querySelectorAll('button')].filter((b) => b.textContent === 'Excluir');
    expect(del).toHaveLength(2); // ana and bob
  });

  it('deletes only after the username is typed exactly', async () => {
    api.delete.mockResolvedValue({});
    await view.click([...document.querySelectorAll('button')].filter((b) => b.textContent === 'Excluir')[0]);
    expect(view.dialog().textContent).toContain('Excluir ana de vez?');
    const confirm = () => view.button('Excluir conta');
    expect(confirm().disabled).toBe(true);

    await view.type(view.dialog().querySelector('input'), 'an');
    expect(confirm().disabled).toBe(true);
    await view.type(view.dialog().querySelector('input'), 'ana');
    expect(confirm().disabled).toBe(false);

    await view.click(confirm());
    expect(api.delete).toHaveBeenCalledWith('/users/r1', { data: { confirmUsername: 'ana' } });
  });

  it('does not delete when the question is cancelled', async () => {
    await view.click([...document.querySelectorAll('button')].filter((b) => b.textContent === 'Excluir')[0]);
    await view.click(view.button('Cancelar'));
    expect(api.delete).not.toHaveBeenCalled();
  });

  it('shows what the server said when it refuses', async () => {
    api.post.mockRejectedValue({ response: { status: 403, data: 'Forbidden\n' } });
    await view.click(view.button('Desbloquear'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Você não tem permissão para isso.');
  });
});


describe('AccountsTab: the sessions of an account', () => {
  const sessionsButtons = () => [...document.querySelectorAll('button')].filter((b) => b.textContent === 'Sessões');
  const modal = () => document.querySelector('[role="dialog"][aria-label^="Sessões de"]');
  const endButton = (label) => document.querySelector(`button[aria-label="Encerrar a sessão ${label}"]`);

  it('offers the button where the server says the account may be managed, and only there', async () => {
    expect(sessionsButtons()).toHaveLength(2); // ana and bob; not the owner, an admin or yourself
    expect(document.querySelector('button[aria-label="Sessões de ana"]')).toBeTruthy();
    expect(document.querySelector('button[aria-label="Sessões de boss"]')).toBeNull();
    expect(document.querySelector('button[aria-label="Sessões de adm"]')).toBeNull();
  });

  it('lists the sessions of that account with the device and the use, and no address', async () => {
    await view.click(document.querySelector('button[aria-label="Sessões de ana"]'));
    expect(api.get).toHaveBeenCalledWith('/users/r1/sessions');
    const text = modal().textContent;
    expect(text).toContain('Sessões de ana');
    expect(text).toContain('Firefox em Linux');
    expect(text).toContain('Ativa agora');
    expect(text).toContain('Safari em iPhone');
    expect(text).toContain('Último uso não registrado');
    expect(text).toContain('não aparece aqui');
    expect(text).not.toMatch(/Endereço/);
    expect(text).not.toContain('·'); // no empty slot where the address would be
  });

  it('ends one session after asking, saying that it does not block the person', async () => {
    await view.click(document.querySelector('button[aria-label="Sessões de ana"]'));
    await view.click(endButton('Safari em iPhone'));
    const question = [...document.querySelectorAll('[role="dialog"]')].at(-1).textContent;
    expect(question).toContain('não é bloqueada');
    expect(api.delete).not.toHaveBeenCalled();
    await view.click(view.button('Encerrar sessão'));
    expect(api.delete).toHaveBeenCalledWith('/users/r1/sessions/x2');
    expect(modal().textContent).not.toContain('Safari em iPhone');
  });

  it('ends all of them after asking', async () => {
    await view.click(document.querySelector('button[aria-label="Sessões de ana"]'));
    await view.click(view.button('Encerrar todas'));
    expect([...document.querySelectorAll('[role="dialog"]')].at(-1).textContent).toContain('Todos os aparelhos saem');
    await view.click([...document.querySelectorAll('[role="dialog"] button')].filter((b) => b.textContent === 'Encerrar todas').at(-1));
    expect(api.delete).toHaveBeenCalledWith('/users/r1/sessions');
    expect(modal().textContent).toContain('Nenhuma sessão aberta.');
  });

  it('offers "end all" only when there is more than one', async () => {
    anaSessions = anaSessions.slice(0, 1);
    await view.click(document.querySelector('button[aria-label="Sessões de ana"]'));
    expect(view.button('Encerrar todas')).toBeUndefined();
    expect(endButton('Firefox em Linux')).toBeTruthy();
  });

  it('does nothing when the question is cancelled, and closes', async () => {
    await view.click(document.querySelector('button[aria-label="Sessões de ana"]'));
    await view.click(endButton('Safari em iPhone'));
    await view.click(view.button('Cancelar'));
    expect(api.delete).not.toHaveBeenCalled();
    await view.click(view.button('Fechar'));
    expect(modal()).toBeNull();
  });

  it('says it when the server refuses, in Portuguese', async () => {
    await view.click(document.querySelector('button[aria-label="Sessões de ana"]'));
    api.delete.mockRejectedValue(Object.assign(new Error('404'), { response: { status: 404, data: 'Session not found' } }));
    await view.click(endButton('Safari em iPhone'));
    await view.click(view.button('Encerrar sessão'));
    expect(modal().textContent).toContain('Essa sessão não existe mais');
  });

  it('says it could not load', async () => {
    api.get.mockImplementation(async (url) => { if (url === '/users/r1/sessions') throw new Error('down'); return { data: { data: accounts } }; });
    await view.click(document.querySelector('button[aria-label="Sessões de ana"]'));
    expect(modal().textContent).toContain('Não foi possível carregar as sessões.');
  });
});
