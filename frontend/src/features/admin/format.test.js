import { describe, it, expect } from 'vitest';
import { formatAge } from './format';

const MIN = 60 * 1000;
const HOUR = 60 * MIN;

describe('formatAge', () => {
  it('says "agora há pouco" under a minute, and for nonsense', () => {
    expect(formatAge(0)).toBe('agora há pouco');
    expect(formatAge(59 * 1000)).toBe('agora há pouco');
    expect(formatAge(-5000)).toBe('agora há pouco');
    expect(formatAge(undefined)).toBe('agora há pouco');
  });

  it('counts minutes, with the singular for one', () => {
    expect(formatAge(60 * 1000)).toBe('há 1 minuto');
    expect(formatAge(90 * 1000)).toBe('há 1 minuto');
    expect(formatAge(59 * MIN)).toBe('há 59 minutos');
  });

  it('counts hours up to two days, with the singular for one', () => {
    expect(formatAge(HOUR)).toBe('há 1 hora');
    expect(formatAge(9 * HOUR + 40 * MIN)).toBe('há 9 horas');
    expect(formatAge(47 * HOUR)).toBe('há 47 horas');
  });

  it('counts days from two days on', () => {
    expect(formatAge(48 * HOUR)).toBe('há 2 dias');
    expect(formatAge(50 * HOUR)).toBe('há 2 dias');
    expect(formatAge(10 * 24 * HOUR)).toBe('há 10 dias');
  });
});
