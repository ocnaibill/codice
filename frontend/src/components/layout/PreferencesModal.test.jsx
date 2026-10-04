import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}));

import { api } from '../../lib/api';
import { mount } from '../../features/admin/testUtils';
import { PreferencesModal } from './PreferencesModal';

let view;
const radio = (label) => [...document.body.querySelectorAll('label')].find((l) => l.textContent.includes(label))?.querySelector('input');

async function open(prefs = { choice: '', library: 'given_first', effective: 'given_first' }, onClose = vi.fn()) {
  api.get.mockImplementation(async (url) => {
    if (url === '/auth/preferences') return { data: prefs };
    if (url === '/auth/export') return { data: { export: null } };
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
});

