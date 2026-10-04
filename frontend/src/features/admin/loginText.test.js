import { describe, it, expect } from 'vitest';
import { loginDetail, loginDevice, loginWhen, loginWho, METHODS, RESULTS, RESULT_FILTERS } from './loginText';

const NOW = new Date('2026-10-04T12:00:00Z').getTime();
const FIREFOX = 'Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0';

describe('loginText', () => {
  it('names every result the server can say, and offers each in the filter', () => {
    const serverSays = ['success', 'bad_password', 'unknown_user', 'blocked', 'directory_unavailable', 'rate_limited', 'link_offered', 'bad_app_token'];
    expect(Object.keys(RESULTS).sort()).toEqual([...serverSays].sort());
    expect([...RESULT_FILTERS].sort()).toEqual([...serverSays].sort());
    for (const r of Object.values(RESULTS)) expect(['ok', 'bad', 'warn', 'neutral']).toContain(r.tone);
  });

  it('shows each result in the tone it deserves: what got in is calm, what was refused is not, what is in doubt is a warning', () => {
    const tones = Object.fromEntries(Object.entries(RESULTS).map(([key, value]) => [key, value.tone]));
    expect(tones).toEqual({
      success: 'ok', bad_password: 'bad', unknown_user: 'bad', blocked: 'bad', bad_app_token: 'bad',
      directory_unavailable: 'warn', rate_limited: 'warn', link_offered: 'neutral',
    });
  });

  it('names every way in', () => {
    expect(Object.keys(METHODS).sort()).toEqual(['app', 'invite', 'ldap', 'local', 'setup']);
  });

  it('says who it was about', () => {
    expect(loginWho({ result: 'bad_password', username: 'ana' })).toBe('ana');
    expect(loginWho({ result: 'unknown_user', typed: 'root' })).toBe('“root”');
    expect(loginWho({ result: 'unknown_user' })).toBe('um nome que não existe');
    expect(loginWho({ result: 'rate_limited' })).toBe('sem conta');
  });

  it('says when, once or as a run', () => {
    const once = loginWhen({ at: '2026-10-04T09:00:00Z', lastAt: '2026-10-04T09:00:00Z', count: 1 }, NOW);
    expect(once.text).toBe('há 3 horas');
    expect(once.title).toMatch(/04\/10\/2026/);
    const run = loginWhen({ at: '2026-10-04T08:00:00Z', lastAt: '2026-10-04T09:30:00Z', count: 12 }, NOW);
    expect(run.text).toContain('12 vezes, de ');
    expect(run.text).toContain(' a ');
    expect(run.title).toBe('há 2 horas');
    expect(loginWhen({ at: '2026-10-04T09:00:00Z', count: 1 }, NOW).text).toBe('há 3 horas');
  });

  it('says the device only when there is one', () => {
    expect(loginDevice({ userAgent: FIREFOX })).toBe('Firefox em Linux');
    expect(loginDevice({})).toBe('');
    expect(loginDevice({ userAgent: '' })).toBe('');
  });

  it('puts the way, the address and the device on one line, whichever there are', () => {
    expect(loginDetail({ method: 'local', ip: '203.0.113.7', userAgent: FIREFOX })).toBe('por senha da conta · endereço 203.0.113.7 · Firefox em Linux');
    expect(loginDetail({ method: 'app', ip: '10.0.0.1' })).toBe('por aplicativo (OPDS) · endereço 10.0.0.1');
    expect(loginDetail({ method: 'ldap' })).toBe('por diretório (LDAP)');
    expect(loginDetail({ method: 'novo' })).toBe('por novo');
  });
});
