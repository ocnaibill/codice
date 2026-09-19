import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { AcceptInvite } from './AcceptInvite';

let view;
const fields = () => [...view.container.querySelectorAll('input')];

async function open({ valid = { role: 'reader', emailRestricted: false }, onAccepted = () => {}, onCancel = () => {} } = {}) {
  if (valid) api.get.mockResolvedValue({ data: valid });
  else api.get.mockRejectedValue({ response: { status: 404 } });
  view = await mount(<AcceptInvite token="abc" onAccepted={onAccepted} onCancel={onCancel} />);
}
async function fill(username, password, confirm = password) {
  const [user, , pass, conf] = fields();
  await view.type(user, username);
  await view.type(pass, password);
  await view.type(conf, confirm);
}
const submit = () => view.click(view.button('Criar conta'));

beforeEach(() => { vi.clearAllMocks(); localStorage.clear(); });
afterEach(() => view.unmount());

describe('AcceptInvite', () => {
  it('checks the link with the server first', async () => {
    await open();
    expect(api.get).toHaveBeenCalledWith('/auth/invitation', { params: { token: 'abc' } });
    expect(view.text()).toContain('Você foi convidado');
  });

  it('says so, and offers the login, when the link is not valid', async () => {
    const back = vi.fn();
    await open({ valid: false, onCancel: back });
    expect(view.text()).toContain('Convite inválido');
    expect(view.container.querySelector('form')).toBeNull();
    await view.click(view.button('Ir para o login'));
    expect(back).toHaveBeenCalled();
  });

  it('does not ask for an address that the invitation already fixed', async () => {
    await open({ valid: { role: 'reader', emailRestricted: true } });
    expect(view.container.querySelector('input[type="email"]')).toBeNull();
  });

  it('creates the account, keeps the session and moves on', async () => {
    const accepted = vi.fn();
    api.post.mockResolvedValue({ data: { token: 'jwt-1' } });
    await open({ onAccepted: accepted });
    await fill('ana', 'senha-longa');
    await submit();

    expect(api.post).toHaveBeenCalledWith('/auth/redeem', { token: 'abc', username: 'ana', password: 'senha-longa', email: '' });
    expect(localStorage.getItem('codice_token')).toBe('jwt-1');
    expect(accepted).toHaveBeenCalled();
  });

  it('refuses a short or mismatched password without calling the server', async () => {
    await open();
    await fill('ana', 'curta');
    await submit();
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('pelo menos 8');

    await fill('ana', 'senha-longa', 'outra-coisa-x');
    await submit();
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('não coincidem');
    expect(api.post).not.toHaveBeenCalled();
  });

  it('explains a link that stopped working and shows other refusals as they are', async () => {
    api.post.mockRejectedValueOnce({ response: { status: 404, data: 'This invitation is not valid\n' } });
    await open();
    await fill('ana', 'senha-longa');
    await submit();
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('não vale mais');

    api.post.mockRejectedValueOnce({ response: { status: 409, data: 'That username or email is already taken\n' } });
    await submit();
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('That username or email is already taken');
    expect(localStorage.getItem('codice_token')).toBeNull();
  });
});
