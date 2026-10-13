const MONTHS = ['janeiro', 'fevereiro', 'março', 'abril', 'maio', 'junho', 'julho', 'agosto', 'setembro', 'outubro', 'novembro', 'dezembro'];

/**
 * A date of the profile of a person as Wikidata knows it, in words: "1920-10-08" -> "8 de outubro de 1920", "1920-10" -> "outubro de 1920",
 * "1920" -> "1920", and a year before the common era ("-0384") -> "384 a.C.". Whatever is not one of those is told as it came, or not at all.
 */
export function formatProfileDate(text) {
  const match = /^(-?)(\d{1,12})(?:-(\d{2}))?(?:-(\d{2}))?$/.exec(String(text ?? '').trim());
  if (!match) return '';
  const [, minus, yearText, month, day] = match;
  const year = String(Number(yearText));
  if (minus) return `${year} a.C.`;
  const monthName = MONTHS[Number(month) - 1];
  if (!month || !monthName) return year;
  if (!day || Number(day) < 1 || Number(day) > 31) return `${monthName} de ${year}`;
  return `${Number(day)} de ${monthName} de ${year}`;
}

/** The year of a date of the profile, as it is told in a life span: "1920-10-08" -> "1920", "-0384" -> "384 a.C."; "" when it is not a date. */
export function formatProfileYear(text) {
  const match = /^(-?)(\d{1,12})(?:-\d{2})?(?:-\d{2})?$/.exec(String(text ?? '').trim());
  if (!match) return '';
  const year = String(Number(match[2]));
  return match[1] ? `${year} a.C.` : year;
}

/** "1920 — 1986", "1920 —" for someone who has not died or whose death is not known, "— 1986" when only that is known; "" with neither. */
export function lifeSpan(born, died) {
  const from = formatProfileYear(born);
  const to = formatProfileYear(died);
  if (!from && !to) return '';
  return `${from} — ${to}`.trim();
}
