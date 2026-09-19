import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { ForgotPassword } from './ForgotPassword';
import { ResetPassword } from './ResetPassword';

let view;
beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('ForgotPassword', () => {
  it('sends the request and says nothing about whether the account exists', async () => {
    api.post.mockResolvedValue({ data: {} });
    view = await mount(<ForgotPassword initialUsername="ana" onBack={() => {}} />);
    expect(view.container.querySelector('input').value).toBe('ana');

    await view.click(view.button('Pedir redefinição'));
    expect(api.post).toHaveBeenCalledWith('/auth/reset-request', { username: 'ana' });
    expect(view.text()).toContain('Se a conta existir');
    expect(view.text()).not.toMatch(/não existe|não encontrada/i);
  });

  it('goes back to the login without sending anything', async () => {
    const back = vi.fn();
    view = await mount(<ForgotPassword onBack={back} />);
    await view.click(view.button('Voltar ao login'));
    expect(back).toHaveBeenCalled();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('tells the person to wait when there were too many requests', async () => {
    api.post.mockRejectedValue({ response: { status: 429 } });
    view = await mount(<ForgotPassword initialUsername="ana" onBack={() => {}} />);
    await view.click(view.button('Pedir redefinição'));
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('Muitas tentativas');
  });
});

describe('ResetPassword', () => {
  const fill = async (a, b) => {
    const [pass, conf] = [...view.container.querySelectorAll('input')];
    await view.type(pass, a);
    await view.type(conf, b);
  };

  it('checks the link, then sets the password and points to the login', async () => {
    api.get.mockResolvedValue({ data: { username: 'ana' } });
    api.post.mockResolvedValue({});
    const done = vi.fn();
    view = await mount(<ResetPassword token="abc" onDone={done} />);
    expect(api.get).toHaveBeenCalledWith('/auth/reset', { params: { token: 'abc' } });
    expect(view.text()).toContain('para ana');

    await fill('nova-senha-1', 'nova-senha-1');
    await view.click(view.button('Alterar senha'));
    expect(api.post).toHaveBeenCalledWith('/auth/reset', { token: 'abc', password: 'nova-senha-1' });
    expect(view.text()).toContain('desconectado de todos os aparelhos');
    await view.click(view.button('Ir para o login'));
    expect(done).toHaveBeenCalled();
  });

  it('says so when the link is not valid and shows no form', async () => {
    api.get.mockRejectedValue({ response: { status: 404 } });
    view = await mount(<ResetPassword token="x" onDone={() => {}} />);
    expect(view.text()).toContain('Link inválido');
    expect(view.container.querySelector('form')).toBeNull();
  });

  it('checks the password before calling the server', async () => {
    api.get.mockResolvedValue({ data: { username: 'ana' } });
    view = await mount(<ResetPassword token="abc" onDone={() => {}} />);
    await fill('curta', 'curta');
    await view.click(view.button('Alterar senha'));
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('pelo menos 8');
    await fill('nova-senha-1', 'outra-coisa-9');
    await view.click(view.button('Alterar senha'));
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('não coincidem');
    expect(api.post).not.toHaveBeenCalled();
  });

  it('explains a link that stopped working', async () => {
    api.get.mockResolvedValue({ data: { username: 'ana' } });
    api.post.mockRejectedValue({ response: { status: 404, data: 'This reset link is not valid\n' } });
    view = await mount(<ResetPassword token="abc" onDone={() => {}} />);
    await fill('nova-senha-1', 'nova-senha-1');
    await view.click(view.button('Alterar senha'));
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('não vale mais');
  });
});

describe('login page', () => {
  it('offers the forgotten-password request and keeps the typed username', async () => {
    const { Auth } = await import('./Auth');
    view = await mount(<Auth onLoginSuccess={() => {}} />);
    await view.type(view.container.querySelector('input'), 'ana');
    await view.click(view.button('Esqueci minha senha'));
    expect(view.text()).toContain('Peça a quem administra o acervo');
    expect(view.container.querySelector('input').value).toBe('ana');
  });
});
