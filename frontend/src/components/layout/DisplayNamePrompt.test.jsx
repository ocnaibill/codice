import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';

vi.mock('../../lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }));

import { api } from '../../lib/api';
import { mount } from '../../features/admin/testUtils';
import { AskDisplayName } from './DisplayNamePrompt';

let view;
const input = () => view.dialog().querySelector('input');
const ana = { id: 'u1', username: 'ana', displayName: '', displayNameAsked: false };
const open = async (...args) => { view = await mount(<AskDisplayName me={args.length ? args[0] : ana} />); }; // `open(undefined)` is a me that is not there

beforeEach(() => {
  vi.clearAllMocks();
  api.put.mockResolvedValue({ data: {} });
});
afterEach(() => view?.unmount());

describe('the question of how to be called, asked once', () => {
  it('is asked of an account that was not asked, and says what the name is for and that the user name stays', async () => {
    await open();
    expect(view.dialog().getAttribute('aria-label')).toBe('Como você quer ser chamado?');
    expect(view.dialog().textContent).toContain('saudação');
    expect(view.dialog().textContent).toContain('ana');
    expect(view.dialog().textContent).toContain('continua sendo o que você digita para entrar');
    expect(input().placeholder).toBe('ana');
  });

  it('is not asked of one that was, of nobody, or when the server does not say', async () => {
    for (const me of [{ ...ana, displayNameAsked: true }, null, undefined, { id: 'u1', username: 'ana' }]) {
      await open(me);
      expect(view.dialog()).toBeNull();
      view.unmount();
    }
    view = { unmount() {} };
  });

  it('saves what is typed, cleaned by the server, and nothing before there is something to save', async () => {
    await open();
    expect(view.button('Salvar').disabled).toBe(true);
    await view.type(input(), '   ');
    expect(view.button('Salvar').disabled).toBe(true);
    await view.type(input(), 'Aninha');
    expect(view.button('Salvar').disabled).toBe(false);
    await view.click(view.button('Salvar'));
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { displayName: 'Aninha' });
  });

  it('answers with the user name when the button says so', async () => {
    await open();
    await view.type(input(), 'Aninha');
    await view.click(view.button('Usar meu usuário'));
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { displayName: '' });
  });

  it('takes closing it with Escape as an answer too: the user name stays and it is not asked again', async () => {
    await open();
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { displayName: '' });
  });

  it('limits what can be typed to what the server keeps', async () => {
    await open();
    expect(input().maxLength).toBe(60);
  });

  it('says so when it could not be saved, and keeps the question', async () => {
    api.put.mockRejectedValue({ response: { status: 500 } });
    await open();
    await view.type(input(), 'Ana');
    await view.click(view.button('Salvar'));
    expect(view.dialog().textContent).toContain('Não foi possível salvar agora.');
  });
});
