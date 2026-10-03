import { describe, it, expect } from 'vitest';
import { formatTime, spoken } from './audioTime';

describe('formatTime', () => {
  it('shows minutes and seconds for a short audio', () => {
    expect(formatTime(0)).toBe('0:00');
    expect(formatTime(5)).toBe('0:05');
    expect(formatTime(65)).toBe('1:05');
    expect(formatTime(150)).toBe('2:30');
    expect(formatTime(3599)).toBe('59:59');
  });

  it('shows hours from the hour on, as an audiobook is long', () => {
    expect(formatTime(3600)).toBe('1:00:00');
    expect(formatTime(3725)).toBe('1:02:05');
    expect(formatTime(36000)).toBe('10:00:00');
  });

  it('drops the fraction, and takes what is not a time as the start', () => {
    expect(formatTime(59.9)).toBe('0:59');
    expect(formatTime(-4)).toBe('0:00');
    expect(formatTime(NaN)).toBe('0:00');
    expect(formatTime(Infinity)).toBe('0:00');
    expect(formatTime(undefined)).toBe('0:00');
  });
});

describe('spoken', () => {
  it('says the time as a person would, with the plural right', () => {
    expect(spoken(0)).toBe('0 segundos');
    expect(spoken(1)).toBe('1 segundo');
    expect(spoken(45)).toBe('45 segundos');
    expect(spoken(60)).toBe('1 minuto');
    expect(spoken(61)).toBe('1 minuto e 1 segundo');
    expect(spoken(754)).toBe('12 minutos e 34 segundos');
    expect(spoken(3600)).toBe('1 hora');
    expect(spoken(3725)).toBe('1 hora, 2 minutos e 5 segundos');
    expect(spoken(7260)).toBe('2 horas e 1 minuto');
  });

  it('takes what is not a time as the start', () => {
    expect(spoken(NaN)).toBe('0 segundos');
    expect(spoken(-1)).toBe('0 segundos');
  });
});
