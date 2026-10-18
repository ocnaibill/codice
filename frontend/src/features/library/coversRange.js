// The part of the story a work contains (DEC-202): the first and the last chapter. The server keeps the two numbers; the form lets a person
// give the quantity instead, and suggests the range from the chapters that already say they are in this volume.

const toNumber = (text) => {
  const n = Number(String(text ?? '').trim().replace(',', '.'));
  return Number.isFinite(n) && n > 0 ? n : null;
};

/** How many chapters a range holds, when it is a whole range; null when it is not (one end missing, or the end before the start). */
export function rangeCount(from, to) {
  const a = toNumber(from);
  const b = toNumber(to);
  if (a == null || b == null || b < a) return null;
  return Math.floor(b - a) + 1;
}

/** The last chapter of a range that begins at `from` and holds `count` chapters, as text; '' when either is not usable. */
export function endFromCount(from, count) {
  const a = toNumber(from);
  const n = Math.floor(Number(String(count ?? '').trim()));
  if (a == null || !Number.isFinite(n) || n < 1) return '';
  return String(a + n - 1);
}

/**
 * What the chapters that say "volume N" suggest a volume contains: from the first to the last of their numbers. `works` are the works of the
 * series as the page of the collection gives them. null when no chapter says it is in that volume.
 */
export function suggestRange(works, volumeNumber) {
  if (volumeNumber == null) return null;
  const numbers = (works ?? [])
    .filter((w) => w.unit === 'chapter' && w.volumeNumber === volumeNumber && w.position != null)
    .map((w) => w.position);
  if (numbers.length === 0) return null;
  return { from: Math.min(...numbers), to: Math.max(...numbers), chapters: numbers.length };
}
