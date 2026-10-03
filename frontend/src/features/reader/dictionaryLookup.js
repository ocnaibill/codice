// Looking up the word a reader selected (#109): which selections are a word to look up, what language to look it up in, and
// how an entry of the dictionary reads. Kept apart from the card so that the rules can be tested without drawing it.

/** The languages the dictionary has words of, as the card offers them. */
export const LOOKUP_LANGUAGES = [
  ['pt', 'Português'], ['en', 'Inglês'], ['es', 'Espanhol'], ['fr', 'Francês'],
  ['de', 'Alemão'], ['it', 'Italiano'], ['ja', 'Japonês'], ['zh', 'Chinês'],
];

/** The longest selection that is looked up, and the most words in it: a word, a word with a hyphen, a short expression. */
export const MAX_LOOKUP_LENGTH = 40;
export const MAX_LOOKUP_WORDS = 3;

/** Whether a selection is something to look up in a dictionary: a word or a few, not a sentence. */
export function isLookupable(text) {
  const trimmed = String(text ?? '').trim();
  if (!trimmed || [...trimmed].length > MAX_LOOKUP_LENGTH) return false;
  if (trimmed.split(/\s+/).length > MAX_LOOKUP_WORDS) return false;
  return /[\p{L}\p{N}]/u.test(trimmed);
}

/** "pt-BR" and "pt_BR" are Portuguese; what is not a language code, or is one the dictionary has no words of, is nothing. */
export function lookupLanguage(code) {
  const base = String(code ?? '').toLowerCase().split(/[-_]/)[0];
  return LOOKUP_LANGUAGES.some(([c]) => c === base) ? base : null;
}

const POS = {
  noun: 'substantivo', verb: 'verbo', adj: 'adjetivo', adv: 'advérbio', pron: 'pronome', prep: 'preposição', conj: 'conjunção',
  intj: 'interjeição', num: 'numeral', det: 'determinante', article: 'artigo', phrase: 'locução', prefix: 'prefixo', suffix: 'sufixo',
  abbrev: 'abreviatura', contraction: 'contração', character: 'caractere', name: 'nome próprio', particle: 'partícula', symbol: 'símbolo',
};

/** The class of a word, in Portuguese; one that is not known is shown as the dictionary gave it. */
export const posLabel = (pos) => (pos ? (POS[pos] ?? pos) : '');

// The tags the Wiktionary puts on a sense or a form are in English: said in Portuguese, and the ones that only repeat what the
// sense says (that it is a form of another word) left out.
const TAGS = {
  singular: 'singular', plural: 'plural', masculine: 'masculino', feminine: 'feminino', neuter: 'neutro', 'first-person': '1ª pessoa',
  'second-person': '2ª pessoa', 'third-person': '3ª pessoa', indicative: 'indicativo', subjunctive: 'subjuntivo', imperative: 'imperativo',
  present: 'presente', past: 'pretérito', preterite: 'pretérito perfeito', imperfect: 'imperfeito', pluperfect: 'mais-que-perfeito',
  future: 'futuro', conditional: 'condicional', infinitive: 'infinitivo', gerund: 'gerúndio', participle: 'particípio',
  'past-participle': 'particípio passado', 'present-participle': 'particípio presente', archaic: 'arcaico', colloquial: 'coloquial',
  informal: 'informal', formal: 'formal', figurative: 'figurado', literary: 'literário', slang: 'gíria', vulgar: 'vulgar',
  pejorative: 'pejorativo', rare: 'raro', obsolete: 'obsoleto', dated: 'antiquado', transitive: 'transitivo', intransitive: 'intransitivo',
  pronominal: 'pronominal', reflexive: 'reflexivo', Brazil: 'Brasil', Portugal: 'Portugal', abbreviation: 'abreviatura', diminutive: 'diminutivo',
  augmentative: 'aumentativo', superlative: 'superlativo', comparative: 'comparativo', countable: 'contável', uncountable: 'incontável',
};
const SILENT_TAGS = new Set(['form-of', 'alt-of']);

/** A tag of a sense or a form, in Portuguese; one that is not known is shown as it came. */
export const tagLabel = (tag) => TAGS[tag] ?? tag;

/** The tags worth showing, in Portuguese, with none repeated. */
export const tagsLine = (tags) => [...new Set((tags ?? []).filter((t) => !SILENT_TAGS.has(t)).map(tagLabel))];

/** What a sense is a form of ("correr" in "forma de correr"), when it is. */
export const senseFormOf = (sense) => (sense?.form_of ?? sense?.alt_of ?? []).map((x) => x.word).filter(Boolean);

/** The senses of an entry as the card shows them: the first few, and how many more there are. */
export function visibleSenses(data, shown = 4) {
  const senses = (data?.senses ?? []).filter((s) => (s.glosses?.length ?? 0) > 0 || senseFormOf(s).length > 0);
  return { shown: senses.slice(0, shown), more: Math.max(0, senses.length - shown), all: senses };
}
