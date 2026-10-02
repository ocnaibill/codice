import { describe, it, expect } from 'vitest';
import { bracketCount } from './format';

describe('bracketCount, the count of a shelf as a menu shows it', () => {
  it('has two digits at least, so that a column lines up', () => {
    expect(bracketCount(0)).toBe('[00]');
    expect(bracketCount(5)).toBe('[05]');
    expect(bracketCount(9)).toBe('[09]');
    expect(bracketCount(10)).toBe('[10]');
    expect(bracketCount(99)).toBe('[99]');
  });
  it('groups the thousands the Brazilian way', () => {
    expect(bracketCount(100)).toBe('[100]');
    expect(bracketCount(1420)).toBe('[1.420]');
  });
  it('says nothing for a count that is not known yet', () => {
    expect(bracketCount(undefined)).toBe('');
    expect(bracketCount(null)).toBe('');
  });
});
