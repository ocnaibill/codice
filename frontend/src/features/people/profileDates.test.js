import { describe, it, expect } from 'vitest';
import { formatProfileDate } from './profileDates';

describe('formatProfileDate: a date of a profile, in words', () => {
  it('says a day, a month or only a year, as far as it is known', () => {
    expect(formatProfileDate('1920-10-08')).toBe('8 de outubro de 1920');
    expect(formatProfileDate('1986-02-11')).toBe('11 de fevereiro de 1986');
    expect(formatProfileDate('1899-03-01')).toBe('1 de março de 1899');
    expect(formatProfileDate('1920-10')).toBe('outubro de 1920');
    expect(formatProfileDate('1920')).toBe('1920');
    expect(formatProfileDate('0899')).toBe('899');
  });

  it('says a year before the common era with its sign', () => {
    expect(formatProfileDate('-0384')).toBe('384 a.C.');
    expect(formatProfileDate('-0044-03-15')).toBe('44 a.C.');
  });

  it('has every month', () => {
    const months = ['janeiro', 'fevereiro', 'março', 'abril', 'maio', 'junho', 'julho', 'agosto', 'setembro', 'outubro', 'novembro', 'dezembro'];
    months.forEach((name, i) => expect(formatProfileDate(`2000-${String(i + 1).padStart(2, '0')}`)).toBe(`${name} de 2000`));
  });

  it('says what is known of a date that is not whole, and nothing of what is not a date', () => {
    expect(formatProfileDate('1920-13')).toBe('1920');          // there is no thirteenth month
    expect(formatProfileDate('1920-00')).toBe('1920');
    expect(formatProfileDate('1920-10-00')).toBe('outubro de 1920');   // a day of zero is no day
    expect(formatProfileDate('1920-10-32')).toBe('outubro de 1920');
    expect(formatProfileDate('1920-10-31')).toBe('31 de outubro de 1920');
    expect(formatProfileDate('1920-10-01')).toBe('1 de outubro de 1920');
    for (const text of ['', null, undefined, 'x', '1920-1', '10/08/1920', '+1920', '1920-10-08T00:00:00Z']) expect(formatProfileDate(text)).toBe('');
  });
});
