import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../lib/api';
import { mount, flush } from '../../features/admin/testUtils';
import { SessionsModal } from './SessionsModal';

let view;
const HOUR = 3600 * 1000;
const ago = (ms) => new Date(Date.now() - ms).toISOString();
const FIREFOX = 'Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0';
const IPHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 Version/17.6 Mobile/15E148 Safari/604.1';

const HERE = { id: 's1', userAgent: FIREFOX, ip: '203.0.113.7', createdAt: ago(48 * HOUR), lastSeenAt: ago(30 * 1000), current: true };
const PHONE = { id: 's2', userAgent: IPHONE, ip: '198.51.100.20', createdAt: ago(72 * HOUR), lastSeenAt: ago(5 * HOUR), current: false };
const OLD = { id: 's3', userAgent: 'curl/8.4.0', ip: null, createdAt: ago(200 * HOUR), lastSeenAt: null, current: false };
const KOREADER = { id: 't1', name: 'KOReader', createdAt: '2026-09-01T10:00:00Z', lastUsedAt: '2026-09-20T18:30:00Z' };

async function open({ sessions = [HERE, PHONE], apps = [], onOpenApps, sessionsError } = {}) {
  let live = sessions;
  let tokens = apps;
  api.get.mockImplementation(async (url) => {
    if (url === '/auth/sessions') {
      if (sessionsError) throw sessionsError;
      return { data: live };
    }
    if (url === '/auth/app-tokens') return { data: tokens };
    throw new Error(`unexpected GET ${url}`);
  });
  api.delete.mockImplementation(async (url) => {
    if (url.startsWith('/auth/sessions/')) live = live.filter((s) => `/auth/sessions/${s.id}` !== url);
    if (url.startsWith('/auth/app-tokens/')) tokens = tokens.filter((t) => `/auth/app-tokens/${t.id}` !== url);
    return {};
  });
  api.post.mockImplementation(async (url) => {
    if (url === '/auth/sessions/revoke-others') { const n = live.filter((s) => !s.current).length; live = live.filter((s) => s.current); return { data: { revoked: n } }; }
    throw new Error(`unexpected POST ${url}`);
  });
  const onClose = vi.fn();
  view = await mount(<SessionsModal onClose={onClose} onOpenApps={onOpenApps} />);
  return onClose;
}
const text = () => view.dialog().textContent;
const dialogs = () => [...document.body.querySelectorAll('[role="dialog"]')];
const endButton = (label) => view.container.ownerDocument.querySelector(`button[aria-label="Encerrar a sessão ${label}"]`);

beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('SessionsModal', () => {
  it('lists the sessions with the device, the address and the use, the one in use marked and not offered', async () => {
    await open();
    expect(text()).toContain('Firefox em Linux');
    expect(text()).toContain('Esta sessão');
    expect(text()).toContain('Endereço 203.0.113.7');
    expect(text()).toContain('Ativa agora');
    expect(text()).toContain('Safari em iPhone');
    expect(text()).toContain('Endereço 198.51.100.20');
    expect(text()).toContain('Último uso há 5 horas');
    expect(endButton('Firefox em Linux')).toBeNull();
    expect(endButton('Safari em iPhone')).toBeTruthy();
  });

  it('says what it does not know instead of inventing it', async () => {
    await open({ sessions: [HERE, OLD] });
    expect(text()).toContain('Dispositivo desconhecido');
    expect(text()).toContain('Endereço não registrado');
    expect(text()).toContain('Último uso não registrado');
  });

  it('says there is no other session when there is only this one, and offers no "end all"', async () => {
    await open({ sessions: [HERE] });
    expect(text()).toContain('Não há outra sessão aberta além desta.');
    expect(view.button('Encerrar todas as outras')).toBeUndefined();
  });

  it('asks before ending one session, and ends it when confirmed', async () => {
    await open();
    await view.click(endButton('Safari em iPhone'));
    expect(dialogs().length).toBe(2);
    expect(dialogs()[1].textContent).toContain('precisa da sua senha');
    expect(api.delete).not.toHaveBeenCalled();
    await view.click(view.button('Encerrar sessão'));
    expect(api.delete).toHaveBeenCalledWith('/auth/sessions/s2');
    await flush();
    expect(text()).not.toContain('Safari em iPhone');
    expect(text()).toContain('Não há outra sessão aberta além desta.');
  });

  it('does not end anything when the question is cancelled', async () => {
    await open();
    await view.click(endButton('Safari em iPhone'));
    await view.click(view.button('Cancelar'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(text()).toContain('Safari em iPhone');
  });

  it('ends every other session at once, after asking', async () => {
    await open({ sessions: [HERE, PHONE, OLD] });
    await view.click(view.button('Encerrar todas as outras'));
    expect(dialogs()[1].textContent).toContain('Esta sessão continua');
    expect(api.post).not.toHaveBeenCalled();
    await view.click(view.button('Encerrar todas'));
    expect(api.post).toHaveBeenCalledWith('/auth/sessions/revoke-others');
    await flush();
    expect(text()).toContain('Firefox em Linux');
    expect(text()).not.toContain('Safari em iPhone');
  });

  it('says it when ending fails, in Portuguese', async () => {
    await open();
    api.delete.mockRejectedValue(Object.assign(new Error('404'), { response: { status: 404, data: 'Session not found' } }));
    await view.click(endButton('Safari em iPhone'));
    await view.click(view.button('Encerrar sessão'));
    await flush();
    expect(text()).toContain('Essa sessão não existe mais');
  });

  it('says it could not load, and tries again', async () => {
    await open({ sessionsError: new Error('down') });
    expect(text()).toContain('Não foi possível carregar as sessões.');
    api.get.mockImplementation(async (url) => (url === '/auth/sessions' ? { data: [HERE] } : { data: [] }));
    await view.click(view.button('Tentar de novo'));
    expect(text()).toContain('Firefox em Linux');
  });

  it('says there is no permission for a 403, without offering to try again', async () => {
    await open({ sessionsError: Object.assign(new Error('403'), { response: { status: 403 } }) });
    expect(text()).toContain('Você não tem permissão para ver isto');
    expect(view.button('Tentar de novo')).toBeUndefined();
  });

  describe('the apps', () => {
    it('lists the apps that were given access, and says when there is none', async () => {
      await open({ apps: [KOREADER] });
      expect(text()).toContain('Aplicativos conectados');
      expect(text()).toContain('KOReader');
      expect(text()).toContain('usado em');
      view.unmount();
      await open({ apps: [] });
      expect(text()).toContain('Nenhum aplicativo conectado.');
    });

    it('removes one after asking', async () => {
      await open({ apps: [KOREADER] });
      await view.click(view.container.ownerDocument.querySelector('button[aria-label="Remover o acesso KOReader"]'));
      expect(api.delete).not.toHaveBeenCalled();
      await view.click(view.button('Remover acesso'));
      expect(api.delete).toHaveBeenCalledWith('/auth/app-tokens/t1');
      await flush();
      expect(text()).toContain('Nenhum aplicativo conectado.');
    });

    it('opens the screen that connects a new one', async () => {
      const onOpenApps = vi.fn();
      await open({ onOpenApps });
      await view.click(view.button('Conectar um aplicativo'));
      expect(onOpenApps).toHaveBeenCalledTimes(1);
    });

    it('has no such button when nothing handles it', async () => {
      await open();
      expect(view.button('Conectar um aplicativo')).toBeUndefined();
    });
  });

  it('closes with the button, with Escape and by clicking outside', async () => {
    const onClose = await open();
    await view.click(view.button('Fechar'));
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await view.click(view.dialog().parentElement);
    expect(onClose).toHaveBeenCalledTimes(3);
  });
});
