import { describe, it, expect } from 'vitest';
import { LOOKUP_LANGUAGES, isLookupable, lookupLanguage, posLabel, senseFormOf, tagLabel, tagsLine, visibleSenses } from './dictionaryLookup';

describe('isLookupable', () => {
  it('is true for a word, a word with a hyphen and a short expression', () => {
    for (const text of ['casa', 'Ação', 'dar-lhe', "l'amour", 'por favor', 'a priori sempre', '走る', '書', '123']) {
      expect(isLookupable(text), text).toBe(true);
    }
  });

  it('ignores what is around the word', () => {
    expect(isLookupable('  casa  ')).toBe(true);
    expect(isLookupable('«casa»,')).toBe(true);
  });

  it('is false for a sentence, a long passage, or nothing', () => {
    expect(isLookupable('Era uma vez um texto longo')).toBe(false);
    expect(isLookupable('a b c d')).toBe(false);
    expect(isLookupable('x'.repeat(41))).toBe(false);
    expect(isLookupable('')).toBe(false);
    expect(isLookupable('   ')).toBe(false);
    expect(isLookupable(null)).toBe(false);
    expect(isLookupable(undefined)).toBe(false);
  });

  it('is false for what has no letter or number in it', () => {
    for (const text of ['...', '—', '«»', '(!)']) expect(isLookupable(text), text).toBe(false);
  });

  it('counts the limit in characters, not in the halves of one that does not fit in two bytes', () => {
    expect(isLookupable('𠮷'.repeat(40))).toBe(true);
    expect(isLookupable('𠮷'.repeat(41))).toBe(false);
  });

  it('counts a letter of any script, and the limit in letters and not in bytes', () => {
    expect(isLookupable('あ'.repeat(40))).toBe(true);
    expect(isLookupable('あ'.repeat(41))).toBe(false);
    expect(isLookupable('x'.repeat(40))).toBe(true);
  });
});

describe('lookupLanguage', () => {
  it('reads a code of a language or of a region as its language', () => {
    expect(lookupLanguage('pt')).toBe('pt');
    expect(lookupLanguage('pt-BR')).toBe('pt');
    expect(lookupLanguage('pt_BR')).toBe('pt');
    expect(lookupLanguage('EN-us')).toBe('en');
    expect(lookupLanguage('zh-Hant')).toBe('zh');
    expect(lookupLanguage('ja')).toBe('ja');
  });

  it('is nothing for what the dictionary has no words of, or is not a code', () => {
    for (const code of ['ru', 'ar', 'xx', '', null, undefined, 'portuguese', '-']) expect(lookupLanguage(code), String(code)).toBeNull();
  });

  it('offers the languages of the library', () => {
    expect(LOOKUP_LANGUAGES.map(([c]) => c)).toEqual(['pt', 'en', 'es', 'fr', 'de', 'it', 'ja', 'zh']);
    expect(LOOKUP_LANGUAGES.every(([c]) => lookupLanguage(c) === c)).toBe(true);
  });
});

describe('posLabel', () => {
  it('says the class in Portuguese, and as it came when it is not known', () => {
    expect(posLabel('noun')).toBe('substantivo');
    expect(posLabel('verb')).toBe('verbo');
    expect(posLabel('adj')).toBe('adjetivo');
    expect(posLabel('adv')).toBe('advérbio');
    expect(posLabel('xyz')).toBe('xyz');
    expect(posLabel('')).toBe('');
    expect(posLabel(undefined)).toBe('');
  });
});

describe('the tags of a sense', () => {
  it('says them in Portuguese, and as they came when they are not known', () => {
    expect(tagLabel('third-person')).toBe('3ª pessoa');
    expect(tagLabel('pluperfect')).toBe('mais-que-perfeito');
    expect(tagLabel('Brazil')).toBe('Brasil');
    expect(tagLabel('figurative')).toBe('figurado');
    expect(tagLabel('something-new')).toBe('something-new');
  });

  it('leaves out what only repeats that the sense is a form of another word, and what is repeated', () => {
    expect(tagsLine(['form-of', 'plural', 'plural', 'alt-of', 'past'])).toEqual(['plural', 'pretérito']);
    expect(tagsLine(['form-of'])).toEqual([]);
    expect(tagsLine(undefined)).toEqual([]);
    expect(tagsLine([])).toEqual([]);
  });

  it('knows every tag the worker keeps of the forms of a Portuguese verb', () => {
    expect(['singular', 'plural', 'first-person', 'second-person', 'third-person', 'indicative', 'subjunctive', 'imperative', 'present', 'past', 'preterite', 'imperfect', 'pluperfect', 'future', 'conditional', 'infinitive', 'gerund', 'participle'].map(tagLabel)).toEqual([
      'singular', 'plural', '1ª pessoa', '2ª pessoa', '3ª pessoa', 'indicativo', 'subjuntivo', 'imperativo', 'presente', 'pretérito', 'pretérito perfeito',
      'imperfeito', 'mais-que-perfeito', 'futuro', 'condicional', 'infinitivo', 'gerúndio', 'particípio',
    ]);
  });
});

describe('senses', () => {
  it('says what a sense is a form of, or a variant of', () => {
    expect(senseFormOf({ form_of: [{ word: 'correr' }] })).toEqual(['correr']);
    expect(senseFormOf({ alt_of: [{ word: 'Egito' }, {}] })).toEqual(['Egito']);
    expect(senseFormOf({ glosses: ['a'] })).toEqual([]);
    expect(senseFormOf(undefined)).toEqual([]);
  });

  it('shows the first few senses and says how many more there are', () => {
    const data = { senses: Array.from({ length: 7 }, (_, i) => ({ glosses: [`sentido ${i}`] })) };
    const { shown, more, all } = visibleSenses(data);
    expect(shown.map((s) => s.glosses[0])).toEqual(['sentido 0', 'sentido 1', 'sentido 2', 'sentido 3']);
    expect(more).toBe(3);
    expect(all).toHaveLength(7);
    expect(visibleSenses(data, 2).more).toBe(5);
    expect(visibleSenses({ senses: [{ glosses: ['a'] }] }).more).toBe(0);
  });

  it('leaves out a sense that says nothing, and keeps one that only points to its lemma', () => {
    const data = { senses: [{ glosses: [] }, { tags: ['x'] }, { glosses: [], form_of: [{ word: 'y' }] }, { glosses: ['bom'] }] };
    expect(visibleSenses(data).all).toHaveLength(2);
  });

  it('copes with an entry that has no senses', () => {
    expect(visibleSenses({}).all).toEqual([]);
    expect(visibleSenses(undefined).shown).toEqual([]);
  });
});
