import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from './features/admin/testUtils';

const sockets = vi.hoisted(() => []);
vi.mock('./lib/api', () => ({
  api: { get: vi.fn() },
  wsUrl: () => Promise.resolve('ws://teste/ws'),
  refreshAssetToken: () => Promise.resolve(),
  clearAssetToken: vi.fn(),
  UNAUTHORIZED_EVENT: 'codice:unauthorized',
}));
const store = vi.hoisted(() => ({ openWork: vi.fn() }));
vi.mock('./store/useGlobalStore', () => {
  const state = () => ({
    activeBookId: null, closeBook: vi.fn(), setSearchQuery: vi.fn(), adminOpen: false, openAdmin: vi.fn(),
    notesOpen: false, searchQuery: '', metadataWorkId: null, metadataTab: 'suggestions', closeMetadata: vi.fn(), ...store,
  });
  const useGlobalStore = (selector) => selector(state());
  useGlobalStore.getState = state;
  return { useGlobalStore };
});
vi.mock('./features/notes/NotesPage', () => ({ NotesPage: () => null }));
vi.mock('./features/library/components/EditBookModal', () => ({ EditBookModal: () => null }));
vi.mock('./features/auth/api/useMe', () => ({ useMe: () => ({ data: null }), isStaff: () => false }));
vi.mock('./features/reader/preferences', () => ({ setPreferenceOwner: vi.fn() }));
vi.mock('./lib/refreshLibrary', () => ({ refreshLibrary: vi.fn() }));
vi.mock('./components/layout/AppShell', () => ({ AppShell: ({ children }) => <main>{children}</main> }));
vi.mock('./pages/HomePage', () => ({ HomePage: () => <div>Acervo</div> }));
vi.mock('./features/reader/components/Reader', () => ({ Reader: () => null }));
vi.mock('./features/reader/components/WorkSheet', () => ({ WorkSheet: () => null }));
vi.mock('./features/upload/components/UploadModal', () => ({ UploadModal: () => null }));
vi.mock('./features/auth/components/Auth', () => ({ Auth: () => null }));
vi.mock('./features/admin/AdminPage', () => ({ AdminPage: () => null }));
vi.mock('./features/auth/components/FirstRunSetup', () => ({ FirstRunSetup: () => null }));
vi.mock('./features/ownership/OwnershipBanner', () => ({ OwnershipBanner: () => null }));
vi.mock('./components/layout/ChangePasswordModal', () => ({ ChangePasswordModal: () => null }));
vi.mock('./features/auth/components/ResetPassword', () => ({ ResetPassword: () => null }));
vi.mock('./features/auth/components/AcceptInvite', () => ({ AcceptInvite: () => null }));

import { api } from './lib/api';
import { refreshLibrary } from './lib/refreshLibrary';
import { useToasts } from './components/ui/toast';
import App from './App';

class FakeSocket {
  constructor(url) { this.url = url; sockets.push(this); }
  close() {}
}
let view;
const say = (event) => act(async () => { sockets.at(-1).onmessage({ data: JSON.stringify(event) }); });
beforeEach(async () => {
  sockets.length = 0;
  globalThis.WebSocket = FakeSocket;
  localStorage.setItem('codice_token', 'sessao');
  api.get.mockResolvedValue({ data: { isFirstRun: false } });
  useToasts.getState().clear();
  vi.spyOn(console, 'log').mockImplementation(() => {});
  view = await mount(<App />);
  await act(async () => { await Promise.resolve(); });
});
afterEach(() => {
  view.unmount();
  localStorage.clear();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe('what the server says of a work, on screen', () => {
  it('opens the channel once the person is in', () => {
    expect(sockets).toHaveLength(1);
    expect(sockets[0].url).toBe('ws://teste/ws');
  });

  it('shows "metadados atualizados" with the title and a way to open the work, and refreshes the library', async () => {
    await say({ type: 'WORK_READY', work_id: 7, title: 'Duna' });
    expect(view.text()).toContain('Metadados atualizados');
    expect(view.text()).toContain('Duna');
    expect(refreshLibrary).toHaveBeenCalled();
    await act(async () => { view.button('Ver obra').click(); });
    expect(store.openWork).toHaveBeenCalledWith(7);
  });

  it('shows in Portuguese that a work could not be processed, as an alert', async () => {
    await say({ type: 'WORK_ERROR', work_id: 3, error: 'invalid epub' });
    expect(view.text()).toContain('Não foi possível processar a obra');
    expect(view.text()).toContain('invalid epub');
    expect(document.body.querySelector('[role="alert"]')).not.toBeNull();
    expect(view.text()).not.toContain('Failed to process');
  });

  it('refreshes the library and says nothing while a work is only being analysed', async () => {
    await say({ type: 'WORK_ANALYZING', work_id: 1 });
    expect(refreshLibrary).toHaveBeenCalled();
    expect(document.body.querySelector('[aria-label="Avisos"]').children).toHaveLength(0);
  });

  it('shows one notice per work: a failure takes the place of the ready that came before', async () => {
    await say({ type: 'WORK_READY', work_id: 7, title: 'Duna' });
    await say({ type: 'WORK_ERROR', work_id: 7, error: 'x' });
    expect(document.body.querySelector('[aria-label="Avisos"]').children).toHaveLength(1);
    expect(view.text()).toContain('Não foi possível');
  });

  it('survives a message that is not JSON', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    await act(async () => { sockets.at(-1).onmessage({ data: 'not json' }); });
    expect(document.body.querySelector('[aria-label="Avisos"]').children).toHaveLength(0);
  });
});
