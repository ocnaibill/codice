// How a possible duplicate is explained to the person who decides (#38).

export const REASON = {
  isbn: 'Mesmo ISBN',
  title_author: 'Mesmo título e autor',
  content: 'O mesmo texto, em outro arquivo',
  translation: 'Parece a mesma obra em outra língua',
  manual: 'Reunida à mão',
};

const percent = (share) => `${Math.round(share * 100)}%`;

/**
 * What the words say, for a pair that has evidence of the text in common: "86% do texto de “A” está em “B” e 89% do de
 * “B” está em “A”". `pair` is a row of GET /admin/duplicates; '' when the pair has no such evidence.
 */
export function contentLine(pair) {
  const content = pair?.evidence?.content;
  if (!content || typeof content.ofA !== 'number' || typeof content.ofB !== 'number') return '';
  return `${percent(content.ofA)} do texto de “${pair.a.title}” está em “${pair.b.title}”, e ${percent(content.ofB)} do texto de “${pair.b.title}” está em “${pair.a.title}”.`;
}

const LANGUAGES = { pt: 'português', en: 'inglês', es: 'espanhol', fr: 'francês', it: 'italiano', de: 'alemão', ca: 'catalão', ro: 'romeno', nl: 'holandês', la: 'latim' };
const primary = (code) => String(code || '').toLowerCase().split(/[-_]/)[0];
const languageName = (code) => LANGUAGES[primary(code)] || code;

/**
 * What reading one book against the other found, for a pair that has evidence of a translation: "Lidas uma contra a
 * outra, 14 de 60 passagens de uma foram achadas na outra, 100% na ordem do livro. Idiomas: português e inglês."
 * '' when the pair has no such evidence.
 */
export function translationLine(pair) {
  const read = pair?.evidence?.translation;
  if (!read || typeof read.hits !== 'number' || typeof read.samples !== 'number' || !read.samples) return '';
  const order = typeof read.order === 'number' && read.order >= 0 ? `, ${percent(read.order)} delas na ordem do livro` : '';
  const languages = read.languageA && read.languageB && primary(read.languageA) !== primary(read.languageB)
    ? ` Idiomas: ${languageName(read.languageA)} e ${languageName(read.languageB)}.` : '';
  return `Lidas uma contra a outra, ${read.hits} de ${read.samples} passagens de uma foram achadas na outra${order}.${languages}`;
}
