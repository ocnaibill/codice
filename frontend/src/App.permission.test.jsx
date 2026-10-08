import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from './features/admin/testUtils';

const state = vi.hoisted(() => ({ role: 'reader', adminOpen: true, closeBook: vi.fn() }));
vi.mock('./lib/api', () => ({
  api: { get: vi.fn() },
  wsUrl: () => new Promise(() => {}),
  refreshAssetToken: () => Promise.resolve(),
  clearAssetToken: vi.fn(),
  UNAUTHORIZED_EVENT: 'codice:unauthorized',
}));
vi.mock('./store/useGlobalStore', () => {
  const get = () => ({
    activeBookId: null, closeBook: state.closeBook, setSearchQuery: vi.fn(), adminOpen: state.adminOpen, openAdmin: vi.fn(),
    notesOpen: false, searchQuery: '', metadataWorkId: null, metadataTab: 'suggestions', closeMetadata: vi.fn(),
  });
  const useGlobalStore = (selector) => selector(get());
  useGlobalStore.getState = get;
  return { useGlobalStore };
});
vi.mock('./features/notes/NotesPage', () => ({ NotesPage: () => null }));
vi.mock('./features/library/components/EditBookModal', () => ({ EditBookModal: () => null }));
vi.mock('./features/auth/api/useMe', () => ({
  useMe: () => ({ data: { id: 'u1', username: 'ana', role: state.role } }),
  isStaff: (me) => me?.role === 'owner' || me?.role === 'admin',
}));
vi.mock('./features/reader/preferences', () => ({
  setPreferenceOwner: vi.fn(), getEpubSettings: () => ({}), hasSavedEpubSettings: () => false, saveEpubSettings: vi.fn(),
}));
vi.mock('./lib/refreshLibrary', () => ({ refreshLibrary: vi.fn() }));
vi.mock('./lib/routeSync', () => ({ startRouteSync: () => () => {} }));
vi.mock('./components/layout/AppShell', () => ({ AppShell: ({ children }) => <main>{children}</main> }));
vi.mock('./pages/HomePage', () => ({ HomePage: () => <div>Acervo</div> }));
vi.mock('./features/reader/components/Reader', () => ({ Reader: () => null }));
vi.mock('./features/reader/components/WorkSheet', () => ({ WorkSheet: () => null }));
vi.mock('./features/upload/components/UploadModal', () => ({ UploadModal: () => null }));
vi.mock('./features/auth/components/Auth', () => ({ Auth: () => null }));
vi.mock('./features/admin/AdminPage', () => ({ AdminPage: ({ isOwner }) => <div>Administração {isOwner ? 'do dono' : 'da equipe'}</div> }));
vi.mock('./features/auth/components/FirstRunSetup', () => ({ FirstRunSetup: () => null }));
vi.mock('./features/ownership/OwnershipBanner', () => ({ OwnershipBanner: () => null }));
vi.mock('./components/layout/ChangePasswordModal', () => ({ ChangePasswordModal: () => null }));
vi.mock('./features/auth/components/ResetPassword', () => ({ ResetPassword: () => null }));
vi.mock('./features/auth/components/AcceptInvite', () => ({ AcceptInvite: () => null }));

import { api } from './lib/api';
import App from './App';

let view;
beforeEach(() => {
  state.role = 'reader';
  state.adminOpen = true;
  state.closeBook = vi.fn();
  localStorage.setItem('codice_token', 'sessao');
  api.get.mockResolvedValue({ data: { isFirstRun: false } });
  vi.spyOn(console, 'log').mockImplementation(() => {});
});
afterEach(() => {
  view.unmount();
  localStorage.clear();
  vi.restoreAllMocks();
});

describe('the administration, for who may not be in it', () => {
  it('says so to a reader who got there, and gives a way back, instead of showing the library as if nothing had happened', async () => {
    view = await mount(<App />);
    await act(async () => { await Promise.resolve(); });
    expect(view.text()).toContain('A administração é de quem cuida do acervo');
    expect(view.text()).toContain('Sua conta é de leitura.');
    expect(view.text()).not.toContain('Administração da equipe');
    expect(view.text()).not.toContain('Acervo');
    await view.click(view.button('Voltar ao acervo'));
    expect(state.closeBook).toHaveBeenCalledTimes(1);
  });

  it('shows the administration to an admin and to the owner, who is told which it is', async () => {
    state.role = 'admin';
    view = await mount(<App />);
    await act(async () => { await Promise.resolve(); });
    expect(view.text()).toContain('Administração da equipe');
    expect(view.text()).not.toContain('A administração é de quem');
    view.unmount();
    state.role = 'owner';
    view = await mount(<App />);
    await act(async () => { await Promise.resolve(); });
    expect(view.text()).toContain('Administração do dono');
  });

  it('shows the library when the administration is not open', async () => {
    state.adminOpen = false;
    view = await mount(<App />);
    await act(async () => { await Promise.resolve(); });
    expect(view.text()).toContain('Acervo');
    expect(view.text()).not.toContain('A administração é de quem');
  });
});
