import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { Auth } from './Auth';
import { FirstRunSetup } from './FirstRunSetup';

let view;
beforeEach(() => { vi.clearAllMocks(); localStorage.clear(); });
afterEach(() => view.unmount());

const signIn = async () => {
  const [u, p] = [...view.container.querySelectorAll('input')];
  await view.type(u, 'ana');
  await view.type(p, 'errada');
  await view.click(view.button('Entrar'));
};
const alert = () => view.container.querySelector('[role="alert"]');

describe('what the login says when it is refused', () => {
  it('says in Portuguese that the user or the password is wrong, and not what the server said in English', async () => {
    api.post.mockRejectedValueOnce({ response: { status: 401, data: 'Invalid username or password\n' } });
    view = await mount(<Auth onLoginSuccess={vi.fn()} />);
    await signIn();
    expect(alert().textContent).toBe('Usuário ou senha incorretos.');
  });

  it('says in Portuguese that registering is closed', async () => {
    api.post.mockRejectedValueOnce({ response: { status: 403, data: 'Registration is disabled' } });
    view = await mount(<Auth onLoginSuccess={vi.fn()} />);
    await signIn();
    expect(alert().textContent).toContain('O cadastro está desativado');
  });

  it('uses its own sentence for what the server said in English and nobody translated', async () => {
    api.post.mockRejectedValueOnce({ response: { status: 400, data: 'something is not allowed here' } });
    view = await mount(<Auth onLoginSuccess={vi.fn()} />);
    await signIn();
    expect(alert().textContent).toBe('Falha na autenticação. Verifique suas credenciais.');
  });

  it('uses its own sentence when the server itself failed, and when it did not answer', async () => {
    api.post.mockRejectedValueOnce({ response: { status: 500, data: 'Error querying database' } });
    view = await mount(<Auth onLoginSuccess={vi.fn()} />);
    await signIn();
    expect(alert().textContent).toBe('Falha na autenticação. Verifique suas credenciais.');
    view.unmount();
    api.post.mockRejectedValueOnce(new Error('Network Error'));
    view = await mount(<Auth onLoginSuccess={vi.fn()} />);
    await signIn();
    expect(alert().textContent).toBe('Falha na autenticação. Verifique suas credenciais.');
  });

  it('is a danger notice of the system, which a screen reader announces', async () => {
    api.post.mockRejectedValueOnce({ response: { status: 401, data: 'Invalid username or password' } });
    view = await mount(<Auth onLoginSuccess={vi.fn()} />);
    await signIn();
    expect(alert().className).toContain('border-danger/30');
  });
});

describe('what the first setup says when it is refused', () => {
  it('says in Portuguese that it was already done', async () => {
    api.post.mockRejectedValueOnce({ response: { status: 403, data: 'First-time setup has already been completed' } });
    view = await mount(<FirstRunSetup onSetupComplete={vi.fn()} />);
    const inputs = [...view.container.querySelectorAll('input')];
    for (const [i, value] of ['dono', 'dono@exemplo.test', 'senha-longa-1', 'senha-longa-1'].entries()) {
      if (inputs[i]) await view.type(inputs[i], value);
    }
    await view.click(view.container.querySelector('button[type="submit"]'));
    expect(alert().textContent).toContain('A configuração inicial já foi feita');
  });
});
