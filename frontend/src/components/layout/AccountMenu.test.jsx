import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
}));

import { api } from '../../lib/api';
import { mount } from '../../features/admin/testUtils';
import { Header } from './Header';
import { ChangePasswordModal } from './ChangePasswordModal';

let view;
beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('account menu', () => {
  it('opens from the avatar with the password change and sign-out', async () => {
    const logout = vi.fn();
    const change = vi.fn();
    view = await mount(<Header onLogout={logout} onChangePassword={change} />);
    expect(view.container.querySelector('[role="menu"]')).toBeNull();

    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    await view.click(view.button('Alterar senha'));
    expect(change).toHaveBeenCalledTimes(1);
    expect(logout).not.toHaveBeenCalled();

    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    await view.click(view.button('Sair'));
    expect(logout).toHaveBeenCalledTimes(1);
  });

  it('opens the preferences from the menu', async () => {
    const preferences = vi.fn();
    view = await mount(<Header onOpenPreferences={preferences} />);
    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    await view.click(view.button('Preferências'));
    expect(preferences).toHaveBeenCalledTimes(1);
  });

  it('opens the apps from the menu', async () => {
    const apps = vi.fn();
    view = await mount(<Header onOpenApps={apps} />);
    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    await view.click(view.button('Aplicativos'));
    expect(apps).toHaveBeenCalledTimes(1);
  });

  it('opens the about screen from the menu', async () => {
    const about = vi.fn();
    view = await mount(<Header onOpenAbout={about} />);
    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    await view.click(view.button('Sobre'));
    expect(about).toHaveBeenCalledTimes(1);
  });

  it('opens the sessions and devices from the menu', async () => {
    const sessions = vi.fn();
    view = await mount(<Header onOpenSessions={sessions} />);
    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    await view.click(view.button('Sessões e dispositivos'));
    expect(sessions).toHaveBeenCalledTimes(1);
  });

  it('has no sessions item when nothing handles it', async () => {
    view = await mount(<Header />);
    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    expect(view.button('Sessões e dispositivos')).toBeUndefined();
  });

  it('has no about item when nothing handles it', async () => {
    view = await mount(<Header />);
    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    expect(view.button('Sobre')).toBeUndefined();
  });

  it('has no apps item when nothing handles it', async () => {
    view = await mount(<Header />);
    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    expect(view.button('Aplicativos')).toBeUndefined();
  });

  it('has no preferences item when nothing handles it', async () => {
    view = await mount(<Header />);
    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    expect(view.button('Preferências')).toBeUndefined();
  });

  it('does not sign out just for opening the menu', async () => {
    const logout = vi.fn();
    view = await mount(<Header onLogout={logout} />);
    await view.click(view.container.querySelector('button[aria-haspopup="menu"]'));
    expect(logout).not.toHaveBeenCalled();
  });
});

describe('ChangePasswordModal', () => {
  const fill = async (values) => {
    const inputs = [...view.container.querySelectorAll('input')].concat([...document.querySelectorAll('input')]);
    const unique = [...new Set(inputs)];
    for (let i = 0; i < values.length; i += 1) await view.type(unique[i], values[i]);
  };
  const submit = () => view.click(view.buttonMatching(/^Alterar senha$/));

  it('sends the current and the new password, then says other devices were signed out', async () => {
    api.post.mockResolvedValue({});
    view = await mount(<ChangePasswordModal onClose={() => {}} />);
    await fill(['antiga-123', 'nova-senha-1', 'nova-senha-1']);
    await submit();

    expect(api.post).toHaveBeenCalledWith('/auth/password', { current: 'antiga-123', new: 'nova-senha-1' });
    expect(view.text()).toContain('outros aparelhos');
  });

  it('checks the new password before calling the server', async () => {
    view = await mount(<ChangePasswordModal onClose={() => {}} />);
    await fill(['antiga-123', 'curta', 'curta']);
    await submit();
    expect(document.querySelector('[role="alert"]').textContent).toContain('pelo menos 8');

    await fill(['antiga-123', 'nova-senha-1', 'outra-coisa-9']);
    await submit();
    expect(document.querySelector('[role="alert"]').textContent).toContain('não coincidem');
    expect(api.post).not.toHaveBeenCalled();
  });

  it('shows the reason when the current password is wrong', async () => {
    api.post.mockRejectedValue({ response: { status: 403, data: 'The current password is not correct\n' } });
    view = await mount(<ChangePasswordModal onClose={() => {}} />);
    await fill(['errada-000', 'nova-senha-1', 'nova-senha-1']);
    await submit();
    expect(document.querySelector('[role="alert"]').textContent).toBe('A senha atual não está correta.');
  });
});
