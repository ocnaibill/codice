// The names of a work, and which one it goes by now (#185, DEC-131). A work has a main title; its editions may have a title of their own
// that owner or admin wrote (`titleSet`), and it may have other names kept for it. The name a person sees is the one of the edition
// they are reading, when someone wrote one for it, and the main title otherwise; the other names go under it.

// The title as compared: without accents, case, punctuation or a trailing note in parentheses ("Duna (edição de bolso)" is "Duna").
const key = (title) =>
  String(title ?? '')
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/\s*\([^)]*\)\s*$/, '')
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim();

/** The name the work goes by while `edition` is the one being read: the title written for that edition, or else the main title. */
export function titleInUse(work, edition) {
  const written = edition?.titleSet ? String(edition.title ?? '').trim() : '';
  return written || work?.title || '';
}

/** What a card says: the title written for the edition the person is reading (the version that counts), or else the main title. */
export const cardTitle = (item) => item.continue?.title?.trim() || item.title;

/**
 * The other names of the work, under the one it goes by: the main title if the work goes by another now, and the names kept for it
 * (the ones of other editions that were written included), each once, in that order.
 */
export function otherTitles(work, inUse) {
  const seen = new Set([key(inUse)]);
  const out = [];
  for (const candidate of [{ title: work?.title, language: '' }, ...(work?.metadata?.alternativeTitles ?? [])]) {
    const k = key(candidate.title);
    if (!k || seen.has(k)) continue;
    seen.add(k);
    out.push({ title: candidate.title, language: candidate.language || '' });
  }
  return out;
}
