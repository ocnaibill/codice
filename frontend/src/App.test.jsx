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
const store = vi.hoisted(() => ({ metadataWorkId: null, metadataTab: 'suggestions', closeMetadata: vi.fn(), notesOpen: false, sheetWorkId: null, personSheetId: null, collectionSheetId: null, searchQuery: '', openAccountDialog: vi.fn(), closeAccountDialog: vi.fn() }));
vi.mock('./store/useGlobalStore', () => ({
  useGlobalStore: (selector) => selector({
    activeBookId: null,
    closeBook: vi.fn(),
    setSearchQuery: vi.fn(),
    adminOpen: false,
    openAdmin: vi.fn(),
    accountDialogs: [],
    ...store,
  }),
}));
vi.mock('./features/notes/NotesPage', () => ({ NotesPage: () => <div>Todas as anotações</div> }));
vi.mock('./features/library/components/EditBookModal', () => ({
  EditBookModal: ({ workId, tab, onClose }) => <button onClick={onClose}>Metadados {workId} {tab}</button>,
}));
vi.mock('./features/auth/api/useMe', () => ({ useMe: () => ({ data: null }), isStaff: () => false }));
vi.mock('./features/reader/preferences', () => ({ setPreferenceOwner: vi.fn() }));
vi.mock('./lib/refreshLibrary', () => ({ refreshLibrary: vi.fn() }));
// The addresses of the screens are tested apart (lib/routeSync.test.js): here, only that the app starts them.
const routeSync = vi.hoisted(() => ({ stop: vi.fn(), start: vi.fn() }));
vi.mock('./lib/routeSync', () => ({ startRouteSync: (...args) => { routeSync.start(...args); return routeSync.stop; } }));
vi.mock('./components/layout/AppShell', () => ({
  AppShell: ({ children, onChangePassword, onOpenPreferences, onOpenApps, onOpenAbout, onOpenSessions }) => (
    <main>
      <button onClick={onChangePassword}>menu senha</button>
      <button onClick={onOpenPreferences}>menu preferências</button>
      <button onClick={onOpenApps}>menu aplicativos</button>
      <button onClick={onOpenAbout}>menu sobre</button>
      <button onClick={onOpenSessions}>menu sessões</button>
      {children}
    </main>
  ),
}));
vi.mock('./pages/HomePage', () => ({ HomePage: () => <div>Acervo</div> }));
vi.mock('./features/reader/components/Reader', () => ({ Reader: () => null }));
vi.mock('./features/library/components/WorkPage', () => ({ WorkPage: () => <div>Página da obra montada</div> }));
vi.mock('./features/collections/components/CollectionPage', () => ({ CollectionPage: () => <div>Página da coleção montada</div> }));
vi.mock('./features/people/components/PersonPage', () => ({ PersonPage: () => <div>Página da pessoa montada</div> }));
vi.mock('./features/upload/components/UploadModal', () => ({ UploadModal: () => null }));
vi.mock('./features/auth/components/Auth', () => ({ Auth: () => <div>Login</div> }));
vi.mock('./features/admin/AdminPage', () => ({ AdminPage: () => null }));
vi.mock('./features/auth/components/FirstRunSetup', () => ({ FirstRunSetup: () => null }));
vi.mock('./features/ownership/OwnershipBanner', () => ({ OwnershipBanner: () => null }));
vi.mock('./components/layout/ChangePasswordModal', () => ({ ChangePasswordModal: ({ onClose }) => <button onClick={onClose}>diálogo senha</button> }));
vi.mock('./components/layout/PreferencesModal', () => ({ PreferencesModal: ({ onClose }) => <button onClick={onClose}>diálogo preferências</button> }));
vi.mock('./components/layout/AppsModal', () => ({ AppsModal: ({ onClose }) => <button onClick={onClose}>diálogo aplicativos</button> }));
vi.mock('./components/layout/AboutModal', () => ({ AboutModal: ({ onClose }) => <button onClick={onClose}>diálogo sobre</button> }));
vi.mock('./components/layout/SessionsModal', () => ({
  SessionsModal: ({ onClose, onOpenApps }) => (
    <div>
      <button onClick={onClose}>diálogo sessões</button>
      <button onClick={onOpenApps}>sessões abrem aplicativos</button>
    </div>
  ),
}));
vi.mock('./features/auth/components/ResetPassword', () => ({ ResetPassword: ({ onDone }) => <button onClick={onDone}>Ir para o login</button> }));
vi.mock('./features/auth/components/AcceptInvite', () => ({ AcceptInvite: () => null }));

import { api } from './lib/api';
import App from './App';

let view;
beforeEach(() => {
  localStorage.clear();
  window.history.replaceState(null, '', '/');
  store.metadataWorkId = null;
  store.notesOpen = false;
  store.sheetWorkId = null;
  store.personSheetId = null;
  store.collectionSheetId = null;
  store.searchQuery = '';
  store.accountDialogs = [];
  api.get.mockResolvedValue({ data: { isFirstRun: false } });
});
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
  localStorage.clear();
  window.history.replaceState(null, '', '/');
});

describe('App: the dialogs of the account are in the store (#182)', () => {
  beforeEach(() => localStorage.setItem('codice_token', 'current-session'));

  it('opens each dialog by its name from the menu of the account, and over the sessions the one of the apps', async () => {
    view = await mount(<App />);
    for (const [button, name] of [
      ['menu senha', 'senha'], ['menu preferências', 'preferencias'], ['menu aplicativos', 'aplicativos'], ['menu sobre', 'sobre'], ['menu sessões', 'sessoes'],
    ]) {
      store.openAccountDialog.mockClear();
      await view.click(view.button(button));
      expect(store.openAccountDialog, button).toHaveBeenCalledTimes(1);
      expect(store.openAccountDialog, button).toHaveBeenCalledWith(name);
    }
    store.accountDialogs = ['sessoes'];
    view.unmount();
    view = await mount(<App />);
    store.openAccountDialog.mockClear();
    await view.click(view.button('sessões abrem aplicativos'));
    expect(store.openAccountDialog).toHaveBeenCalledWith('aplicativos');
  });

  it('shows the dialogs the store says are open, and each one closes itself in the store', async () => {
    for (const [name, text] of [['senha', 'diálogo senha'], ['preferencias', 'diálogo preferências'], ['aplicativos', 'diálogo aplicativos'], ['sobre', 'diálogo sobre'], ['sessoes', 'diálogo sessões']]) {
      store.accountDialogs = [name];
      store.closeAccountDialog.mockClear();
      view = await mount(<App />);
      for (const other of ['senha', 'preferências', 'aplicativos', 'sobre', 'sessões']) {
        if (`diálogo ${other}` !== text) expect(view.text(), `${name} / ${other}`).not.toContain(`diálogo ${other}`);
      }
      await view.click(view.button(text));
      expect(store.closeAccountDialog, name).toHaveBeenCalledWith(name);
      view.unmount();
      view = null;
    }
  });

  it('shows two dialogs together when the store says so, the last over the first', async () => {
    store.accountDialogs = ['sessoes', 'aplicativos'];
    view = await mount(<App />);
    expect(view.text()).toContain('diálogo sessões');
    expect(view.text()).toContain('diálogo aplicativos');
  });

  it('shows none when the store says none', async () => {
    view = await mount(<App />);
    expect(view.text()).not.toContain('diálogo');
  });
});

describe('App startup', () => {
  it('starts the addresses of the screens with the session, and not before (#182)', async () => {
    view = await mount(<App />);
    expect(routeSync.start).not.toHaveBeenCalled();
    expect(view.text()).toContain('Login');
    view.unmount();
    localStorage.setItem('codice_token', 'current-session');
    view = await mount(<App />);
    expect(routeSync.start).toHaveBeenCalledTimes(1);
    expect(routeSync.stop).not.toHaveBeenCalled();
    view.unmount();
    view = null;
    expect(routeSync.stop).toHaveBeenCalledTimes(1);
  });

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

  it('keeps the session when the question "is it set up?" fails (the limit on it, a moment without network)', async () => {
    localStorage.setItem('codice_token', 'current-session');
    api.get.mockRejectedValue(Object.assign(new Error('429'), { response: { status: 429 } }));
    vi.spyOn(console, 'error').mockImplementation(() => {});

    view = await mount(<App />);

    expect(view.text()).toContain('Acervo');
    expect(view.text()).not.toContain('Login');
    expect(localStorage.getItem('codice_token')).toBe('current-session');
  });

  it('shows the login when the question fails and there is no session to keep', async () => {
    api.get.mockRejectedValue(Object.assign(new Error('429'), { response: { status: 429 } }));
    vi.spyOn(console, 'error').mockImplementation(() => {});
    view = await mount(<App />);
    expect(view.text()).toContain('Login');
    expect(view.text()).not.toContain('Acervo');
  });

  it('still opens a reset link when the question fails and a session is stored', async () => {
    window.history.replaceState(null, '', '/?reset=example');
    localStorage.setItem('codice_token', 'old-session');
    api.get.mockRejectedValue(Object.assign(new Error('429'), { response: { status: 429 } }));
    vi.spyOn(console, 'error').mockImplementation(() => {});
    view = await mount(<App />);
    expect(view.text()).toContain('Ir para o login');
    expect(view.text()).not.toContain('Acervo');
  });

  it('opens the metadata of a work over everything when asked to, and closes it through the store', async () => {
    localStorage.setItem('codice_token', 'current-session');
    store.metadataWorkId = 7;
    store.metadataTab = 'edit';

    view = await mount(<App />);

    expect(view.text()).toContain('Metadados 7 edit');
    await view.click(view.buttonMatching(/^Metadados 7/));
    expect(store.closeMetadata).toHaveBeenCalled();
  });

  it('has no page of a collection, of a person or of a work while none is open', async () => {
    localStorage.setItem('codice_token', 'current-session');
    view = await mount(<App />);
    expect(view.text()).not.toContain('Página da coleção montada');
    expect(view.text()).not.toContain('Página da pessoa montada');
    expect(view.text()).not.toContain('Página da obra montada');
  });

  it('shows the page of a collection in the place of the library, and the pages of a person and of a work over it', async () => {
    localStorage.setItem('codice_token', 'current-session');
    store.collectionSheetId = 5;
    view = await mount(<App />);
    expect(view.text()).toContain('Página da coleção montada');
    expect(view.text()).not.toContain('Acervo');
    view.unmount();
    store.personSheetId = 4;
    view = await mount(<App />);
    expect(view.text()).toContain('Página da pessoa montada');
    expect(view.text()).not.toContain('Página da coleção montada');
    view.unmount();
    store.sheetWorkId = 7;
    view = await mount(<App />);
    expect(view.text()).toContain('Página da obra montada');
    expect(view.text()).not.toContain('Página da pessoa montada');
  });

  it('shows the page of a person in the place of the library, and the page of a work over it', async () => {
    localStorage.setItem('codice_token', 'current-session');
    store.personSheetId = 4;
    view = await mount(<App />);
    expect(view.text()).toContain('Página da pessoa montada');
    expect(view.text()).not.toContain('Acervo');
    view.unmount();
    store.sheetWorkId = 7;
    view = await mount(<App />);
    expect(view.text()).toContain('Página da obra montada');
    expect(view.text()).not.toContain('Página da pessoa montada');
  });

  it('shows the page of a work in the place of the library, with the pages of a collection and of a person out of the way', async () => {
    localStorage.setItem('codice_token', 'current-session');
    store.sheetWorkId = 7;
    view = await mount(<App />);
    expect(view.text()).toContain('Página da obra montada');
    expect(view.text()).not.toContain('Acervo');
    expect(view.text()).not.toContain('Página da coleção montada');
  });

  it('shows the page of a work over the notes, and over the administration', async () => {
    localStorage.setItem('codice_token', 'current-session');
    store.notesOpen = true;
    store.sheetWorkId = 7;
    view = await mount(<App />);
    expect(view.text()).toContain('Página da obra montada');
    expect(view.text()).not.toContain('Todas as anotações');
  });

  it('has no metadata open unless a work asked for it', async () => {
    localStorage.setItem('codice_token', 'current-session');
    view = await mount(<App />);
    expect(view.text()).not.toContain('Metadados');
  });

  it('shows the screen of every note instead of the library when it is open', async () => {
    localStorage.setItem('codice_token', 'current-session');
    store.notesOpen = true;
    view = await mount(<App />);
    expect(view.text()).toContain('Todas as anotações');
    expect(view.text()).not.toContain('Acervo');
  });

  it('shows the search instead of the notes when something is searched for', async () => {
    localStorage.setItem('codice_token', 'current-session');
    store.notesOpen = true;
    store.searchQuery = 'duna';
    view = await mount(<App />);
    expect(view.text()).toContain('Acervo');
    expect(view.text()).not.toContain('Todas as anotações');
  });

  it('shows the library when the notes are not open', async () => {
    localStorage.setItem('codice_token', 'current-session');
    view = await mount(<App />);
    expect(view.text()).toContain('Acervo');
    expect(view.text()).not.toContain('Todas as anotações');
  });
});
