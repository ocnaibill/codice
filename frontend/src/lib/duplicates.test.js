import { describe, it, expect } from 'vitest';
import { REASON, contentLine } from './duplicates';

const pair = (evidence) => ({ a: { title: 'Duna' }, b: { title: 'Dune' }, evidence });

describe('REASON', () => {
  it('says each reason in words, and the content one says it is the text', () => {
    expect(REASON.isbn).toBe('Mesmo ISBN');
    expect(REASON.title_author).toBe('Mesmo título e autor');
    expect(REASON.content).toBe('O mesmo texto, em outro arquivo');
    expect(REASON.manual).toBe('Reunida à mão');
  });
});

describe('contentLine', () => {
  it('says how much of each text is in the other, in whole percent', () => {
    expect(contentLine(pair({ content: { ofA: 0.858, ofB: 0.892 } }))).toBe('86% do texto de “Duna” está em “Dune”, e 89% do texto de “Dune” está em “Duna”.');
    expect(contentLine(pair({ content: { ofA: 1, ofB: 0.7 } }))).toBe('100% do texto de “Duna” está em “Dune”, e 70% do texto de “Dune” está em “Duna”.');
  });

  it('is nothing for a pair that has no evidence of the text, or an evidence it cannot read', () => {
    expect(contentLine(pair(undefined))).toBe('');
    expect(contentLine(pair(null))).toBe('');
    expect(contentLine(pair({}))).toBe('');
    expect(contentLine(pair({ content: {} }))).toBe('');
    expect(contentLine(pair({ content: { ofA: 0.9 } }))).toBe('');
    expect(contentLine(pair({ content: { ofA: '90%', ofB: 0.9 } }))).toBe('');
    expect(contentLine(null)).toBe('');
  });
});
