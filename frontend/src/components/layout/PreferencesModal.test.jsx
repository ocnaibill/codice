import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}));

import { api } from '../../lib/api';
import { flush, mount } from '../../features/admin/testUtils';
import { PreferencesModal } from './PreferencesModal';
import { getKeepScreenOn, saveKeepScreenOn, setPreferenceOwner } from '../../features/reader/preferences';

let view;
const radio = (label) => [...document.body.querySelectorAll('label')].find((l) => l.textContent.includes(label))?.querySelector('input');

async function open(prefs = { choice: '', library: 'given_first', effective: 'given_first' }, onClose = vi.fn()) {
  api.get.mockImplementation(async (url) => {
    if (url === '/auth/preferences') return { data: prefs };
    if (url === '/auth/export') return { data: { export: null } };
    if (url === '/auth/me') return { data: { id: 'u1', username: 'ana', displayNameAsked: true } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: prefs });
  view = await mount(<PreferencesModal onClose={onClose} />);
  return onClose;
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('PreferencesModal', () => {
  it('has the section of the person\'s own data, below the preferences', async () => {
    await open();
    const text = view.dialog().textContent;
    expect(text).toContain('Meus dados');
    expect(text.indexOf('Como mostrar o nome dos autores')).toBeLessThan(text.indexOf('Meus dados'));
    expect(view.button('Preparar o meu arquivo')).toBeTruthy();
  });

  it('offers both orders with an example and the library default, and says what the default is today', async () => {
    await open({ choice: '', library: 'family_first', effective: 'family_first' });
    const text = view.dialog().textContent;
    expect(text).toContain('Nome Sobrenome');
    expect(text).toContain('Frank Herbert');
    expect(text).toContain('Sobrenome, Nome');
    expect(text).toContain('Herbert, Frank');
    expect(text).toContain('O padrão da biblioteca');
    expect(text).toContain('hoje: Sobrenome, Nome');
    expect(text).toContain('nada do que está guardado é alterado');
  });

  it('has selected what the account chose, or the default when it has not chosen', async () => {
    await open({ choice: 'family_first', library: 'given_first', effective: 'family_first' });
    expect(radio('Sobrenome, Nome').checked).toBe(true);
    expect(radio('O padrão da biblioteca').checked).toBe(false);
    view.unmount();
    await open({ choice: '', library: 'given_first', effective: 'given_first' });
    expect(radio('O padrão da biblioteca').checked).toBe(true);
    expect(radio('Nome Sobrenome').checked).toBe(false);
  });

  it('saves the choice at once', async () => {
    await open();
    await view.click(radio('Sobrenome, Nome'));
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { nameOrder: 'family_first' });
  });

  it('saves an empty choice to go back to the default', async () => {
    await open({ choice: 'family_first', library: 'given_first', effective: 'family_first' });
    await view.click(radio('O padrão da biblioteca'));
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { nameOrder: '' });
  });

  it('says so when it cannot save, and when it cannot load', async () => {
    await open();
    api.put.mockRejectedValue({ response: { status: 500 } });
    await view.click(radio('Sobrenome, Nome'));
    expect(view.text()).toContain('Não foi possível salvar');
    view.unmount();
    api.get.mockRejectedValue(new Error('boom'));
    view = await mount(<PreferencesModal onClose={() => {}} />);
    expect(view.text()).toContain('Não foi possível carregar as preferências');
  });

  it('closes with the button and with Escape', async () => {
    const onClose = await open();
    await view.click(view.button('Fechar'));
    expect(onClose).toHaveBeenCalledTimes(1);
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});

describe('PreferencesModal, how it appears', () => {
  it('fades the backdrop in and lets the dialog rise, with the motion of the system', async () => {
    await open();
    expect(view.dialog().className).toContain('animate-pop-in');
    expect(view.dialog().parentElement.className).toContain('animate-fade-in');
  });

  describe('how the person wants to be called (#179)', () => {
    const field = () => view.dialog().querySelector('input[autocomplete="given-name"]');

    it('shows what is saved, and the user name as the placeholder', async () => {
      await open({ choice: '', library: 'given_first', effective: 'given_first', displayName: 'Aninha', displayNameAsked: true });
      await flush(); // the account is read once the form is there
      expect(field().value).toBe('Aninha');
      expect(field().placeholder).toBe('ana');
      expect(view.dialog().textContent).toContain('Vazio, vale o seu usuário (ana)');
    });

    it('saves only when there is something changed, and goes back to the user name when it is emptied', async () => {
      await open({ choice: '', library: 'given_first', effective: 'given_first', displayName: 'Aninha', displayNameAsked: true });
      const save = () => [...view.dialog().querySelectorAll('button')].find((b) => b.textContent === 'Salvar');
      expect(save().disabled).toBe(true);
      await view.type(field(), '  Aninha ');
      expect(save().disabled).toBe(true); // the same name with spaces around it is not a change
      await view.type(field(), 'Ana Maria');
      expect(save().disabled).toBe(false);
      await view.click(save());
      expect(api.put).toHaveBeenCalledWith('/auth/preferences', { displayName: 'Ana Maria' });
      await view.type(field(), '');
      await view.click(save());
      expect(api.put).toHaveBeenLastCalledWith('/auth/preferences', { displayName: '' });
    });

    it('limits what can be typed to what the server keeps', async () => {
      await open();
      expect(field().maxLength).toBe(60);
    });

    it('says so when it could not be saved', async () => {
      await open();
      api.put.mockRejectedValue({ response: { status: 500 } });
      await view.type(field(), 'Ana');
      await view.click([...view.dialog().querySelectorAll('button')].find((b) => b.textContent === 'Salvar'));
      expect(view.dialog().textContent).toContain('Não foi possível salvar.');
    });
  });

  describe('keeping the screen on while reading (#180)', () => {
    const box = () => [...view.dialog().querySelectorAll('input[type="checkbox"]')][0];
    afterEach(() => { delete navigator.wakeLock; localStorage.clear(); });
    beforeEach(() => { localStorage.clear(); setPreferenceOwner('ana'); });

    it('is on, and turning it off is remembered on this device', async () => {
      Object.defineProperty(navigator, 'wakeLock', { value: { request: vi.fn() }, configurable: true });
      await open();
      expect(box().checked).toBe(true);
      expect(view.dialog().textContent).toContain('Só neste aparelho');
      await view.click(box());
      expect(getKeepScreenOn()).toBe(false);
      expect(box().checked).toBe(false);
      await view.click(box());
      expect(getKeepScreenOn()).toBe(true);
    });

    it('shows what was chosen before', async () => {
      Object.defineProperty(navigator, 'wakeLock', { value: { request: vi.fn() }, configurable: true });
      saveKeepScreenOn(false);
      await open();
      expect(box().checked).toBe(false);
    });

    it('says why it cannot be chosen where the browser has no such thing, and does not pretend it is on', async () => {
      await open();
      expect(box().disabled).toBe(true);
      expect(box().checked).toBe(false);
      expect(view.dialog().textContent).toContain('sem HTTPS');
    });
  });
});
