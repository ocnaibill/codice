import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount, flush } from '../testUtils';
import { LoginsTab } from './LoginsTab';

let view;
const HOUR = 3600 * 1000;
const ago = (ms) => new Date(Date.now() - ms).toISOString();
const FIREFOX = 'Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0';

const entry = (extra) => ({ id: 1, at: ago(2 * HOUR), lastAt: ago(2 * HOUR), count: 1, result: 'success', method: 'local', username: 'ana', ip: '203.0.113.7', userAgent: FIREFOX, ...extra });
const page = (entries, extra = {}) => ({ entries, more: false, retentionDays: 90, sameAddress: { warn: false, address: '' }, ...extra });

let pages;
async function open({ isOwner = true, first = page([]), error } = {}) {
  pages = [first];
  api.get.mockImplementation(async (url, options) => {
    if (url !== '/admin/logins') throw new Error(`unexpected GET ${url}`);
    if (error) throw error;
    // The test server answers the filters it is asked for with nothing, and the rest with the pages it was given.
    if (options?.params?.result) return { data: page([]) };
    const before = options?.params?.before;
    const index = before ? pages.findIndex((p) => p.entries.some((e) => e.id === before)) + 1 : 0;
    return { data: pages[index] ?? page([]) };
  });
  api.put.mockImplementation(async (url, body) => ({ data: { retentionDays: body.retentionDays } }));
  view = await mount(<LoginsTab isOwner={isOwner} />);
}
const text = () => view.text();
const select = async (label, value) => {
  const el = document.querySelector(`select[aria-label="${label}"]`);
  const { act } = await import('react');
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, value);
    el.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await flush();
};
const lastParams = () => api.get.mock.calls.at(-1)[1].params;

beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('LoginsTab', () => {
  it('says there is nothing yet', async () => {
    await open();
    expect(text()).toContain('Ainda não há entradas registradas.');
  });

  it('lists who, how it went, by which way, from where and from what device', async () => {
    await open({ first: page([
      entry({ id: 4 }),
      entry({ id: 3, result: 'bad_password', username: 'bia', method: 'ldap', ip: '198.51.100.9', userAgent: undefined }),
      entry({ id: 2, result: 'unknown_user', username: '', typed: 'root', count: 7, at: ago(5 * HOUR), lastAt: ago(1 * HOUR) }),
      entry({ id: 1, result: 'rate_limited', username: '', userAgent: undefined }),
    ]) });
    const t = text();
    expect(t).toContain('ana');
    expect(t).toContain('Entrou');
    expect(t).toContain('por senha da conta · endereço 203.0.113.7 · Firefox em Linux');
    expect(t).toContain('Senha errada');
    expect(t).toContain('por diretório (LDAP) · endereço 198.51.100.9');
    expect(t).toContain('“root”');
    expect(t).toContain('Usuário que não existe');
    expect(t).toContain('7 vezes, de ');
    expect(t).toContain('Barrado pelo limite de tentativas');
    expect(t).toContain('sem conta');
  });

  it('shows an administrator that someone tried a name, but not which', async () => {
    await open({ isOwner: false, first: page([entry({ id: 2, result: 'unknown_user', username: '', typed: undefined })]) });
    expect(text()).toContain('um nome que não existe');
    expect(text()).toContain('Os nomes digitados por quem não tem conta só o dono vê.');
    view.unmount();
    await open({ isOwner: true, first: page([entry({ id: 2, result: 'unknown_user', username: '', typed: 'root' })]) });
    expect(text()).not.toContain('Os nomes digitados por quem não tem conta só o dono vê.');
  });

  it('warns when every entry comes from one address of the private network', async () => {
    await open({ first: page([entry()], { sameAddress: { warn: true, address: '172.30.77.1' } }) });
    expect(text()).toContain('Todas as entradas vêm do mesmo endereço');
    expect(text()).toContain('172.30.77.1');
    expect(text()).toContain('CODICE_TRUSTED_PROXIES');
    view.unmount();
    await open({ first: page([entry()]) });
    expect(text()).not.toContain('Todas as entradas vêm do mesmo endereço');
  });

  describe('filters', () => {
    it('asks for the result chosen and says nothing matched', async () => {
      await open({ first: page([entry()]) });
      await select('Filtrar pelo resultado', 'bad_password');
      expect(lastParams()).toMatchObject({ result: 'bad_password', limit: 30 });
      expect(text()).toContain('Nenhuma entrada com esses filtros.');
      expect(text()).not.toContain('Ainda não há entradas registradas.');
    });

    it('offers every result by its name', async () => {
      await open();
      const labels = [...document.querySelectorAll('select[aria-label="Filtrar pelo resultado"] option')].map((o) => o.textContent);
      expect(labels).toContain('Tudo');
      expect(labels).toContain('Entrou');
      expect(labels).toContain('Senha errada');
      expect(labels).toContain('Aplicativo com senha errada');
      expect(labels).toContain('Barrado pelo limite de tentativas');
    });

    it('asks for the period as a date', async () => {
      await open();
      await select('Filtrar pelo período', '7');
      const from = new Date(lastParams().from).getTime();
      expect(Math.abs(Date.now() - 7 * 24 * HOUR - from)).toBeLessThan(5000);
      await select('Filtrar pelo período', '');
      expect(lastParams().from).toBeUndefined();
    });

    it('asks for the account typed, trimmed, only when searched', async () => {
      await open();
      const input = document.querySelector('input[aria-label="Filtrar pela conta"]');
      await view.type(input, '  ana ');
      expect(api.get.mock.calls.every(([, o]) => !o.params.username)).toBe(true);
      await view.click(view.button('Buscar'));
      expect(lastParams().username).toBe('ana');
    });

    it('clears the filters', async () => {
      await open();
      expect(view.button('Limpar filtros')).toBeUndefined();
      await select('Filtrar pelo resultado', 'blocked');
      await view.click(view.button('Limpar filtros'));
      expect(lastParams().result).toBeUndefined();
      expect(view.button('Limpar filtros')).toBeUndefined();
    });
  });

  describe('pages', () => {
    it('shows more when there is more, and asks from the last one it has', async () => {
      await open({ first: page([entry({ id: 5, ip: '10.0.0.5' }), entry({ id: 4, ip: '10.0.0.4' })], { more: true }) });
      pages.push(page([entry({ id: 3, ip: '10.0.0.3' })]));
      await view.click(view.button('Mostrar mais'));
      expect(lastParams().before).toBe(4);
      expect(text()).toContain('10.0.0.3');
      expect(text()).toContain('10.0.0.5');
      expect(view.button('Mostrar mais')).toBeUndefined();
    });

    it('offers no more when there is none', async () => {
      await open({ first: page([entry()]) });
      expect(view.button('Mostrar mais')).toBeUndefined();
    });
  });

  describe('how long it is kept', () => {
    it('tells an administrator, and lets only the owner change it', async () => {
      await open({ isOwner: false, first: page([entry()], { retentionDays: 45 }) });
      expect(text()).toContain('guardado por 45 dias');
      expect(view.button('Mudar')).toBeUndefined();
    });

    it('lets the owner change it, and sends the days', async () => {
      await open({ first: page([entry()]) });
      expect(text()).toContain('90 dias');
      await view.click(view.button('Mudar'));
      const input = document.querySelector('input[aria-label="Dias de guarda"]');
      expect(input.value).toBe('90');
      await view.type(input, '30');
      await view.click(view.button('Guardar'));
      expect(api.put).toHaveBeenCalledWith('/admin/logins/settings', { retentionDays: 30 });
      expect(text()).not.toContain('Guardar por quantos dias');
    });

    it('does not send a number outside the limits, and says so', async () => {
      await open({ first: page([entry()]) });
      await view.click(view.button('Mudar'));
      const input = document.querySelector('input[aria-label="Dias de guarda"]');
      for (const bad of ['6', '3651', '0', '-5', '12.5']) {
        await view.type(input, bad);
        expect(view.button('Guardar').disabled, bad).toBe(true);
        expect(text(), bad).toContain('Use um número inteiro de 7 a 3650');
      }
      await view.type(input, '');
      expect(view.button('Guardar').disabled, 'empty').toBe(true);
      for (const good of ['7', '3650']) {
        await view.type(input, good);
        expect(view.button('Guardar').disabled, good).toBe(false);
      }
      expect(api.put).not.toHaveBeenCalled();
    });

    it('sends nothing when the form is submitted with a number that is not valid (Enter does not go around the button)', async () => {
      await open({ first: page([entry()]) });
      await view.click(view.button('Mudar'));
      const input = document.querySelector('input[aria-label="Dias de guarda"]');
      await view.type(input, '3');
      const { act } = await import('react');
      await act(async () => { input.closest('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
      expect(api.put).not.toHaveBeenCalled();
    });

    it('asks the server again after it is saved, so the new number shows', async () => {
      await open({ first: page([entry()]) });
      const before = api.get.mock.calls.length;
      await view.click(view.button('Mudar'));
      await view.type(document.querySelector('input[aria-label="Dias de guarda"]'), '30');
      await view.click(view.button('Guardar'));
      expect(api.get.mock.calls.length).toBeGreaterThan(before);
    });

    it('cancels without sending', async () => {
      await open({ first: page([entry()]) });
      await view.click(view.button('Mudar'));
      await view.click(view.button('Cancelar'));
      expect(api.put).not.toHaveBeenCalled();
      expect(text()).toContain('90 dias');
    });

    it('says when the server refuses', async () => {
      await open({ first: page([entry()]) });
      api.put.mockRejectedValue(Object.assign(new Error('400'), { response: { status: 400, data: 'retentionDays must be between 7 and 3650' } }));
      await view.click(view.button('Mudar'));
      await view.type(document.querySelector('input[aria-label="Dias de guarda"]'), '30');
      await view.click(view.button('Guardar'));
      expect(text()).toContain('Guardar por quantos dias');
      expect(document.querySelector('[role="alert"]')).not.toBeNull();
    });
  });

  it('says it could not load, and tries again', async () => {
    await open({ error: new Error('down') });
    expect(text()).toContain('Não foi possível carregar as entradas.');
    expect(text()).not.toContain('Ainda não há entradas registradas.');
    expect(view.button('Tentar de novo')).toBeTruthy();
  });

  it('says there is no permission for a 403, without offering to try again', async () => {
    await open({ error: Object.assign(new Error('403'), { response: { status: 403 } }) });
    expect(text()).toContain('Você não tem permissão para ver isto');
    expect(view.button('Tentar de novo')).toBeUndefined();
  });
});
