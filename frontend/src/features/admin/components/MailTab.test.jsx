import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { MailTab } from './MailTab';

let view;
const state = (over = {}) => ({ enabled: true, host: 'smtp.casa.lan', port: 587, security: 'starttls', username: 'codice', from: 'codice@casa.lan',
  insecureTLS: false, maxMB: 25, hasPassword: true, masterKey: true, ...over });

async function open(s) {
  api.get.mockResolvedValue({ data: s });
  api.put.mockResolvedValue({ data: s });
  api.post.mockResolvedValue({ data: { sent: true } });
  view = await mount(<MailTab />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('MailTab', () => {
  it('shows the server and whether a password is kept, never the password', async () => {
    await open(state());
    expect(view.container.querySelector('input[placeholder="smtp.exemplo.com"]').value).toBe('smtp.casa.lan');
    expect(view.container.querySelector('input[type="password"]').placeholder).toContain('mantida');
    expect(view.container.querySelector('input[type="password"]').value).toBe('');
  });

  it('keeps the password when nothing is typed, and sends a new one when it is', async () => {
    await open(state());
    await view.click(view.button('Salvar'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/mail', expect.not.objectContaining({ password: expect.anything() }));
    await view.type(view.container.querySelector('input[type="password"]'), 'nova-senha');
    await view.click(view.button('Salvar'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/mail', expect.objectContaining({ password: 'nova-senha', host: 'smtp.casa.lan', port: 587, maxMB: 25 }));
  });

  it('takes the password away when asked', async () => {
    await open(state());
    const drop = [...view.container.querySelectorAll('label')].find((l) => l.textContent.includes('Tirar a senha')).querySelector('input');
    await view.click(drop);
    await view.click(view.button('Salvar'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/mail', expect.objectContaining({ password: '' }));
  });

  it('says a password cannot be kept without the master key of the server', async () => {
    await open(state({ masterKey: false, hasPassword: false }));
    expect(view.text()).toContain('CODICE_SECRET_KEY');
    expect(view.container.querySelector('input[type="password"]').disabled).toBe(true);
  });

  it('tests the server, and says what the server answered when it refuses', async () => {
    await open(state());
    await view.type(view.container.querySelector('input[type="email"]'), 'eu@casa.lan');
    await view.click(view.button('Enviar teste'));
    expect(api.post).toHaveBeenCalledWith('/admin/mail/test', { to: 'eu@casa.lan' });
    expect(view.text()).toContain('Mensagem enviada');

    api.post.mockRejectedValue({ response: { status: 502, data: 'Não foi possível enviar: o servidor recusou o usuário ou a senha: 535 falhou' } });
    await view.click(view.button('Enviar teste'));
    expect(view.text()).toContain('o servidor recusou o usuário ou a senha');
  });

  it('does not offer the test while sending is off', async () => {
    await open(state({ enabled: false }));
    expect(view.button('Enviar teste').disabled).toBe(true);
  });
});
