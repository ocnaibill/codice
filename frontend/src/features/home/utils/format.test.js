import { describe, it, expect } from 'vitest';
import { bracketCount, countWordFor, formatBreakdown } from './format';

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

describe('formatBreakdown, what is read or finished said by shelf', () => {
  it('names the comics and the mangas apart, singular or plural, and skips the empty shelves (#187)', () => {
    expect(formatBreakdown({ livros: 2, quadrinhos: 1, mangas: 3, audio: 0 })).toBe('2 livros • 1 quadrinho • 3 mangás');
    expect(formatBreakdown({ livros: 0, quadrinhos: 4, mangas: 1, audio: 1 })).toBe('4 quadrinhos • 1 mangá • 1 áudio');
    expect(formatBreakdown(undefined)).toBe('');
  });
});

describe('countWordFor', () => {
  it('is the singular for one, and what it was told for the rest', () => {
    expect(countWordFor(1, 'obras')).toBe('obra');
    expect(countWordFor(1, 'itens')).toBe('item');
    expect(countWordFor(1, 'volumes no acervo')).toBe('volume no acervo');
    expect(countWordFor(0, 'obras')).toBe('obras');
    expect(countWordFor(2, 'obras')).toBe('obras');
    expect(countWordFor(1, 'coisas')).toBe('coisas');
  });
});
