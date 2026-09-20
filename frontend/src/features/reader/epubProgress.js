import { completionFor } from './progressRules';

const clamp01 = (n) => Math.min(1, Math.max(0, n));

/**
 * What to save for an EPUB position (the "relocated" location of epub.js): the exact place (the
 * CFI), the chapter (its href), how far into that chapter, and, once the book's positions have been
 * computed, how far through the whole book. `percentageFromCfi` answers 0..1, or null while it does
 * not know yet: no percentage is invented, and it never says the book is finished before it is.
 */
export function buildEpubProgress(location, percentageFromCfi) {
  const start = location?.start;
  if (!start?.cfi) return null;

  const locator = { type: 'epub', cfi: start.cfi };
  if (start.href) locator.href = start.href;
  const { page, total } = start.displayed ?? {};
  if (Number.isFinite(page) && Number.isFinite(total) && total > 0) {
    locator.progression = Math.round(clamp01((page - 1) / total) * 1000) / 1000;
  }

  let percent;
  const fraction = percentageFromCfi ? percentageFromCfi(start.cfi) : null;
  if (typeof fraction === 'number' && Number.isFinite(fraction)) {
    percent = Math.round(clamp01(fraction) * 1000) / 10;
  }
  // The end of the book is 100%, even when there is nothing to measure against (a book of one
  // chapter starts and ends in the same place).
  if (location.atEnd) percent = 100;
  const extras = { percent, completed: location.atEnd ? true : completionFor(percent) };
  return { locator, extras };
}
