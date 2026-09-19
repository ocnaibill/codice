import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { LdapTab } from './LdapTab';

let view;
const state = (over = {}) => ({ configured: true, host: 'ldap://ldap.example', baseDN: 'dc=example', linkedAccounts: 3,
  policy: { allowCreate: false, revalidateHours: 24 }, ...over });

async function open(s) {
  api.get.mockResolvedValue({ data: s });
  api.put.mockResolvedValue({ data: {} });
  view = await mount(<LdapTab />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('LdapTab', () => {
  it('shows where the directory is, never a secret', async () => {
    await open(state());
    expect(view.text()).toContain('ldap://ldap.example');
    expect(view.text()).toContain('3 conta(s) ligada(s)');
    expect(view.text()).toContain('Configurado');
  });

  it('says how to turn it on when the server has no LDAP, and does not offer to create accounts', async () => {
    await open(state({ configured: false, host: '', baseDN: '' }));
    expect(view.text()).toContain('LDAP_URL');
    expect(view.container.querySelector('input[type="checkbox"]').disabled).toBe(true);
  });

  it('saves the two decisions that are the owner’s', async () => {
    await open(state());
    await view.click(view.container.querySelector('input[type="checkbox"]'));
    await view.type(view.container.querySelector('input[type="number"]'), '48');
    await view.click(view.button('Salvar política'));
    expect(api.put).toHaveBeenCalledWith('/admin/ldap/policy', { allowCreate: true, revalidateHours: 48 });
  });

  it('starts with creation off, as the server default', async () => {
    await open(state());
    expect(view.container.querySelector('input[type="checkbox"]').checked).toBe(false);
  });

  it('tests the connection and says what went wrong in words', async () => {
    await open(state());
    api.post.mockResolvedValueOnce({ data: { ok: true, reason: '' } });
    await view.click(view.button('Testar conexão'));
    expect(api.post).toHaveBeenCalledWith('/admin/ldap/check');
    expect(view.text()).toContain('funcionando');

    api.post.mockResolvedValueOnce({ data: { ok: false, reason: 'unavailable' } });
    await view.click(view.button('Testar conexão'));
    expect(view.text()).toContain('conta de serviço foi recusada');
  });

  it('shows what the server said when saving is refused', async () => {
    await open(state());
    api.put.mockRejectedValue({ response: { status: 400, data: 'revalidateHours must be between 1 and 336\n' } });
    await view.click(view.button('Salvar política'));
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('between 1 and 336');
  });
});
