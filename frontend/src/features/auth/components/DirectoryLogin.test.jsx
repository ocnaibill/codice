import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { Auth } from './Auth';
import { LinkAccount } from './LinkAccount';

let view;
beforeEach(() => { vi.clearAllMocks(); localStorage.clear(); });
afterEach(() => view.unmount());

const signIn = async (user, pass) => {
  const [u, p] = [...view.container.querySelectorAll('input')];
  await view.type(u, user);
  await view.type(p, pass);
  await view.click(view.button('Entrar'));
};

describe('login with a directory', () => {
  it('moves to the link step when the server asks for the local password, without opening a session', async () => {
    api.post.mockResolvedValueOnce({ status: 202, data: { linkRequired: true, ticket: 't-1', username: 'ana' } });
    const done = vi.fn();
    view = await mount(<Auth onLoginSuccess={done} />);
    await signIn('ana', 'senha-do-diretorio');

    expect(view.text()).toContain('Juntar as contas');
    expect(view.text()).toContain('conta local chamada ana');
    expect(done).not.toHaveBeenCalled();
    expect(localStorage.getItem('codice_token')).toBeNull();
  });

  it('a normal login still stores the token and continues', async () => {
    api.post.mockResolvedValueOnce({ status: 200, data: { token: 'jwt-1' } });
    const done = vi.fn();
    view = await mount(<Auth onLoginSuccess={done} />);
    await signIn('ana', 'x');
    expect(localStorage.getItem('codice_token')).toBe('jwt-1');
    expect(done).toHaveBeenCalled();
  });

  it('says the directory is not answering, apart from a wrong password', async () => {
    api.post.mockRejectedValueOnce({ response: { status: 503, data: 'The sign-in directory is not answering.' } });
    view = await mount(<Auth onLoginSuccess={() => {}} />);
    await signIn('ana', 'x');
    expect(view.container.querySelector('.text-red-700').textContent).toContain('diretório) não respondeu');
  });

  it('goes back to the login from the link step', async () => {
    api.post.mockResolvedValueOnce({ status: 202, data: { linkRequired: true, ticket: 't-1', username: 'ana' } });
    view = await mount(<Auth onLoginSuccess={() => {}} />);
    await signIn('ana', 'x');
    await view.click(view.button('Voltar ao login'));
    expect(view.text()).toContain('Bem-vindo de volta');
  });
});

describe('LinkAccount', () => {
  const fill = async (pw) => view.type(view.container.querySelector('input'), pw);

  it('sends the ticket with the local password and signs in', async () => {
    api.post.mockResolvedValue({ data: { token: 'jwt-2' } });
    const linked = vi.fn();
    view = await mount(<LinkAccount ticket="t-1" username="ana" onLinked={linked} onBack={() => {}} />);
    await fill('senha-local');
    await view.click(view.button('Juntar as contas'));
    expect(api.post).toHaveBeenCalledWith('/auth/link', { ticket: 't-1', password: 'senha-local' });
    expect(localStorage.getItem('codice_token')).toBe('jwt-2');
    expect(linked).toHaveBeenCalled();
  });

  it('keeps the person out and says why when the local password is wrong', async () => {
    api.post.mockRejectedValue({ response: { status: 401 } });
    const linked = vi.fn();
    view = await mount(<LinkAccount ticket="t-1" username="ana" onLinked={linked} onBack={() => {}} />);
    await fill('errada');
    await view.click(view.button('Juntar as contas'));
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('Senha local incorreta');
    expect(linked).not.toHaveBeenCalled();
    expect(localStorage.getItem('codice_token')).toBeNull();
  });
});
