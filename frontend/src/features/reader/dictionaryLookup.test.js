import { describe, it, expect } from 'vitest';
import { LOOKUP_LANGUAGES, groupTranslations, scriptLanguage, isLookupable, languageName, lookupLanguage, posLabel, selectedWord, senseFormOf, tagLabel, tagsLine, visibleSenses } from './dictionaryLookup';

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
    for (const code of ['sw', 'ar', 'xx', '', null, undefined, 'portuguese', '-']) expect(lookupLanguage(code), String(code)).toBeNull();
  });

  it('offers the languages of the library', () => {
    expect(LOOKUP_LANGUAGES.map(([c]) => c)).toEqual(['de', 'zh', 'ko', 'ku', 'es', 'fr', 'el', 'nl', 'id', 'en', 'it', 'ja', 'ms', 'pl', 'pt', 'ru', 'th', 'cs', 'tr', 'vi']);
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

describe('languageName', () => {
  it('says the language in Portuguese, and a code it does not know as it came', () => {
    expect(languageName('ja')).toBe('Japonês');
    expect(languageName('xx')).toBe('xx');
  });
});

describe('groupTranslations', () => {
  const t = (lang, word) => ({ lang, word });

  it('groups the words by language, in the order they came, the language that is preferred first', () => {
    expect(groupTranslations([t('en', 'house'), t('fr', 'maison'), t('en', 'home')], 'fr')).toEqual([
      { lang: 'fr', name: 'Francês', words: ['maison'], more: 0 },
      { lang: 'en', name: 'Inglês', words: ['house', 'home'], more: 0 },
    ]);
  });

  it('keeps the order they came in when none is preferred, or the preferred one is not there', () => {
    const list = [t('en', 'a'), t('fr', 'b')];
    expect(groupTranslations(list, 'de').map((g) => g.lang)).toEqual(['en', 'fr']);
    expect(groupTranslations(list, undefined).map((g) => g.lang)).toEqual(['en', 'fr']);
  });

  it('says each word once', () => {
    expect(groupTranslations([t('en', 'house'), t('en', 'house')], 'pt')[0].words).toEqual(['house']);
  });

  it('shows five words of a language and counts the rest, and four languages', () => {
    const many = 'abcdefg'.split('').map((w) => t('en', w));
    expect(groupTranslations(many, 'pt')[0]).toMatchObject({ words: ['a', 'b', 'c', 'd', 'e'], more: 2 });
    const langs = ['en', 'fr', 'de', 'it', 'es'].map((l) => t(l, 'x'));
    expect(groupTranslations(langs, 'es').map((g) => g.lang)).toEqual(['es', 'en', 'fr', 'de']);
  });

  it('leaves out what is not a translation, and gives nothing for nothing', () => {
    expect(groupTranslations([{ lang: 'en' }, { word: 'x' }, null], 'pt')).toEqual([]);
    expect(groupTranslations(undefined, 'pt')).toEqual([]);
  });
});

describe('the tags of the other editions', () => {
  it('says the remote past in Portuguese', () => {
    expect(tagLabel('past-remote')).toBe('pretérito remoto');
  });
});

describe('scriptLanguage', () => {
  it('says the language of a word by its script', () => {
    expect(scriptLanguage('走る')).toBe('ja');
    expect(scriptLanguage('ありがとう')).toBe('ja');
    expect(scriptLanguage('カタカナ')).toBe('ja');
    expect(scriptLanguage('사람')).toBe('ko');
    expect(scriptLanguage('слово')).toBe('ru');
    expect(scriptLanguage('λόγος')).toBe('el');
    expect(scriptLanguage('สวัสดี')).toBe('th');
  });

  it('says nothing for Latin letters, which are the language of the file', () => {
    for (const w of ['casa', 'Ação', "l'amour", 'Straße', 'çà', '123']) expect(scriptLanguage(w), w).toBeNull();
  });

  it('says nothing for Han alone, which is Chinese or Japanese', () => {
    expect(scriptLanguage('猫')).toBeNull();
    expect(scriptLanguage('学习')).toBeNull();
  });

  it('goes by the script that has more letters, and by the file when the Latin ones are as many', () => {
    expect(scriptLanguage('猫です')).toBe('ja'); // Han and kana: kana says it
    expect(scriptLanguage('go 走る')).toBeNull(); // two Latin letters and one of kana: the file
    expect(scriptLanguage('a走る')).toBeNull(); // as many as the Latin ones is not more
    expect(scriptLanguage('a ありがとう')).toBe('ja');
  });

  it('says nothing for nothing', () => {
    for (const w of ['', '   ', null, undefined]) expect(scriptLanguage(w)).toBeNull();
  });
});

describe('selectedWord: a book with a soft hyphen inside every word', () => {
  it('takes out what is not part of the word, wherever it is', () => {
    expect(selectedWord('Le\u00adva')).toBe('Leva');
    expect(selectedWord('For\u00admi\u00add\u00e1\u00advel')).toBe('Formidável');
    expect(selectedWord('a\u200bb\u2060c\ufeffd')).toBe('abcd');
  });

  it('leaves the rest as it is: the hyphen that is part of the word, the accents and the case', () => {
    expect(selectedWord('guarda-chuva')).toBe('guarda-chuva');
    expect(selectedWord('Ação')).toBe('Ação');
    expect(selectedWord('  tempo ')).toBe('  tempo ');
  });

  it('is nothing for nothing', () => {
    expect(selectedWord(null)).toBe('');
    expect(selectedWord(undefined)).toBe('');
  });

  it('makes a word broken by soft hyphens something to look up, and keeps what is nothing but them from being one', () => {
    expect(isLookupable('Le\u00adva')).toBe(true);
    expect(isLookupable('cei\u00adfa\u00addo\u00adra')).toBe(true);
    expect(isLookupable('\u00ad\u200b')).toBe(false);
    // the length is of the word, not of the word with its hints
    const long = 'a\u00ad'.repeat(30);
    expect(isLookupable(long)).toBe(true);
  });
});
