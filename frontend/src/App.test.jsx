import React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from './features/admin/testUtils';

vi.mock('./lib/api', () => ({
  api: { get: vi.fn() },
  wsUrl: () => new Promise(() => {}),
  refreshAssetToken: () => Promise.resolve(),
  clearAssetToken: vi.fn(),
  UNAUTHORIZED_EVENT: 'codice:unauthorized',
}));
vi.mock('./store/useGlobalStore', () => ({
  useGlobalStore: (selector) => selector({
    activeBookId: null,
    closeBook: vi.fn(),
    searchQuery: '',
    setSearchQuery: vi.fn(),
    adminOpen: false,
    openAdmin: vi.fn(),
  }),
}));
vi.mock('./features/auth/api/useMe', () => ({ useMe: () => ({ data: null }), isStaff: () => false }));
vi.mock('./features/reader/preferences', () => ({ setPreferenceOwner: vi.fn() }));
vi.mock('./lib/refreshLibrary', () => ({ refreshLibrary: vi.fn() }));
vi.mock('./components/layout/AppShell', () => ({ AppShell: ({ children }) => <main>{children}</main> }));
vi.mock('./pages/HomePage', () => ({ HomePage: () => <div>Acervo</div> }));
vi.mock('./features/reader/components/Reader', () => ({ Reader: () => null }));
vi.mock('./features/reader/components/WorkSheet', () => ({ WorkSheet: () => null }));
vi.mock('./features/upload/components/UploadModal', () => ({ UploadModal: () => null }));
vi.mock('./features/auth/components/Auth', () => ({ Auth: () => <div>Login</div> }));
vi.mock('./features/admin/AdminPage', () => ({ AdminPage: () => null }));
vi.mock('./features/auth/components/FirstRunSetup', () => ({ FirstRunSetup: () => null }));
vi.mock('./features/ownership/OwnershipBanner', () => ({ OwnershipBanner: () => null }));
vi.mock('./components/layout/ChangePasswordModal', () => ({ ChangePasswordModal: () => null }));
vi.mock('./features/auth/components/ResetPassword', () => ({ ResetPassword: ({ onDone }) => <button onClick={onDone}>Ir para o login</button> }));
vi.mock('./features/auth/components/AcceptInvite', () => ({ AcceptInvite: () => null }));

import { api } from './lib/api';
import App from './App';

let view;
beforeEach(() => {
  localStorage.clear();
  window.history.replaceState(null, '', '/');
  api.get.mockResolvedValue({ data: { isFirstRun: false } });
});
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
  localStorage.clear();
  window.history.replaceState(null, '', '/');
});

describe('App startup', () => {
  it('opens a reset link despite a stored session and clears the token on return to login', async () => {
    window.history.replaceState(null, '', '/?reset=example');
    localStorage.setItem('codice_token', 'old-session');

    view = await mount(<App />);

    expect(view.text()).toContain('Ir para o login');
    expect(view.text()).not.toContain('Acervo');
    expect(localStorage.getItem('codice_token')).toBe('old-session');

    await view.click(view.button('Ir para o login'));

    expect(view.text()).toContain('Login');
    expect(localStorage.getItem('codice_token')).toBeNull();
    expect(window.location.search).toBe('');
    expect(api.get).toHaveBeenCalledWith('/auth/setup-status');
  });

  it('keeps the existing session when there is no reset link', async () => {
    localStorage.setItem('codice_token', 'current-session');

    view = await mount(<App />);

    expect(view.text()).toContain('Acervo');
    expect(localStorage.getItem('codice_token')).toBe('current-session');
  });
});
