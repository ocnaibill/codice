// How a possible duplicate is explained to the person who decides (#38).

export const REASON = {
  isbn: 'Mesmo ISBN',
  title_author: 'Mesmo título e autor',
  content: 'O mesmo texto, em outro arquivo',
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
