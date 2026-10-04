import { describe, it, expect } from 'vitest';
import { sessionLines } from './sessionText';

const NOW = new Date('2026-10-04T12:00:00Z').getTime();
const FIREFOX = 'Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0';
const session = (extra = {}) => ({ userAgent: FIREFOX, ip: '203.0.113.7', createdAt: '2026-10-01T09:00:00Z', lastSeenAt: '2026-10-04T11:59:00Z', ...extra });

describe('sessionLines', () => {
  it('names the device and says where and when it entered', () => {
    const lines = sessionLines(session(), NOW);
    expect(lines.title).toBe('Firefox em Linux');
    expect(lines.kind).toBe('Computador');
    expect(lines.address).toBe('Endereço 203.0.113.7');
    expect(lines.entered).toMatch(/^Entrou em 0?1\/10\/2026/);
  });

  it('says a session used in the last two minutes is active now, and not one just past that', () => {
    expect(sessionLines(session({ lastSeenAt: '2026-10-04T11:58:30Z' }), NOW).use).toBe('Ativa agora');
    expect(sessionLines(session({ lastSeenAt: '2026-10-04T11:57:00Z' }), NOW).use).toBe('Último uso há 3 minutos');
  });

  it('says how long ago it was used, in hours and days', () => {
    expect(sessionLines(session({ lastSeenAt: '2026-10-04T09:00:00Z' }), NOW).use).toBe('Último uso há 3 horas');
    expect(sessionLines(session({ lastSeenAt: '2026-10-01T12:00:00Z' }), NOW).use).toBe('Último uso há 3 dias');
  });

  it('says so when the use was never written (a session from before the screen)', () => {
    expect(sessionLines(session({ lastSeenAt: null }), NOW).use).toBe('Último uso não registrado');
    expect(sessionLines({ ...session(), lastSeenAt: undefined }, NOW).use).toBe('Último uso não registrado');
  });

  it('says the address was not recorded when the server has none, and says nothing when it is not shown at all', () => {
    expect(sessionLines(session({ ip: '' }), NOW).address).toBe('Endereço não registrado');
    expect(sessionLines(session({ ip: null }), NOW).address).toBe('Endereço não registrado');
    const { ip, ...another } = session();
    expect(ip).toBeTruthy();
    expect(sessionLines(another, NOW).address).toBeNull();
  });

  it('calls a phone a phone', () => {
    const phone = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 Version/17.6 Mobile/15E148 Safari/604.1';
    expect(sessionLines(session({ userAgent: phone }), NOW)).toMatchObject({ title: 'Safari em iPhone', kind: 'Celular' });
  });
});
