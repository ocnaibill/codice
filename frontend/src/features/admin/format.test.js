import { describe, it, expect } from 'vitest';
import { formatAge, formatLeft } from './format';

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

describe('formatLeft', () => {
  it('says under an hour, then hours, then days', () => {
    expect(formatLeft(0)).toBe('menos de 1 hora');
    expect(formatLeft(-5)).toBe('menos de 1 hora');
    expect(formatLeft(undefined)).toBe('menos de 1 hora');
    expect(formatLeft(59 * MIN)).toBe('menos de 1 hora');
    expect(formatLeft(HOUR)).toBe('1 hora');
    expect(formatLeft(5 * HOUR + 40 * MIN)).toBe('5 horas');
    expect(formatLeft(23 * HOUR + 59 * MIN)).toBe('23 horas');
    expect(formatLeft(24 * HOUR)).toBe('1 dia');
    expect(formatLeft(49 * HOUR)).toBe('2 dias');
  });
});
