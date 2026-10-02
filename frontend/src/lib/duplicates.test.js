import { describe, it, expect } from 'vitest';
import { REASON, contentLine, translationLine } from './duplicates';

const pair = (evidence) => ({ a: { title: 'Duna' }, b: { title: 'Dune' }, evidence });

describe('REASON', () => {
  it('says each reason in words, and the content one says it is the text', () => {
    expect(REASON.isbn).toBe('Mesmo ISBN');
    expect(REASON.title_author).toBe('Mesmo título e autor');
    expect(REASON.content).toBe('O mesmo texto, em outro arquivo');
    expect(REASON.translation).toBe('Parece a mesma obra em outra língua');
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

describe('translationLine', () => {
  const read = (over = {}) => pair({ translation: { hits: 14, samples: 60, order: 0.983, languageA: 'pt', languageB: 'en', ...over } });

  it('says how many passages were found, how many in order and the languages', () => {
    expect(translationLine(read())).toBe('Lidas uma contra a outra, 14 de 60 passagens de uma foram achadas na outra, 98% delas na ordem do livro. Idiomas: português e inglês.');
  });

  it('leaves the languages out when they are not known or are the same one', () => {
    expect(translationLine(read({ languageA: '', languageB: 'en' }))).not.toContain('Idiomas');
    expect(translationLine(read({ languageA: 'pt', languageB: undefined }))).not.toContain('Idiomas');
    expect(translationLine(read({ languageA: 'en', languageB: 'en-GB' }))).not.toContain('Idiomas');
  });

  it('names a language by its two-letter code, in whatever case or with a region, and keeps one it does not know', () => {
    expect(translationLine(read({ languageA: 'PT-BR', languageB: 'fr' }))).toContain('Idiomas: português e francês.');
    expect(translationLine(read({ languageA: 'xx', languageB: 'es' }))).toContain('Idiomas: xx e espanhol.');
    expect(translationLine(read({ languageA: 'pt_BR', languageB: 'en' }))).toContain('Idiomas: português e inglês.');
    expect(translationLine(read({ languageA: 'pt_BR', languageB: 'pt-PT' }))).not.toContain('Idiomas');
  });

  it('leaves the order out when it was not told', () => {
    expect(translationLine(read({ order: -1 }))).toBe('Lidas uma contra a outra, 14 de 60 passagens de uma foram achadas na outra. Idiomas: português e inglês.');
    expect(translationLine(read({ order: undefined }))).not.toContain('ordem');
  });

  it('is nothing for a pair with no such evidence or one it cannot read', () => {
    expect(translationLine(pair(undefined))).toBe('');
    expect(translationLine(pair({}))).toBe('');
    expect(translationLine(pair({ content: { ofA: 1, ofB: 1 } }))).toBe('');
    expect(translationLine(read({ hits: '14' }))).toBe('');
    expect(translationLine(read({ samples: 0 }))).toBe('');
    expect(translationLine(read({ samples: undefined }))).toBe('');
    expect(translationLine(null)).toBe('');
  });
});
