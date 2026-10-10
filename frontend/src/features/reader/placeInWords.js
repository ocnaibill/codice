// Where the person is, in words (DEC-148): the chapter they are in and which unit of how many (the page of a PDF or a comic, the position of
// an EPUB, which has no pages). It goes with the locator, so the card of "continue reading" can say it without opening the file.

const MAX_TITLE = 200;

/**
 * The chapter at a place: of the entries [{ title, at }] (`at` is where each starts, in the unit of the format: the number of the page, the
 * index in the spine), the one that starts last at or before `current`. Two entries that start in the same place are a chapter and a part
 * of it: `tie` says which is meant ('first' when the place inside it is not known, as in an EPUB, where an entry is a chapter's file and
 * maybe an anchor; 'last' when it is, as in a PDF, where an entry that starts on the page has begun).
 */
export function chapterAt(entries, current, tie = 'last') {
  if (!Number.isFinite(current)) return undefined;
  let best;
  for (const entry of entries || []) {
    if (!entry?.title || !Number.isFinite(entry.at) || entry.at > current) continue;
    if (!best || entry.at > best.at || (entry.at === best.at && tie === 'last')) best = entry;
  }
  return best?.title;
}

/** [{ title, at }] for the table of contents of an EPUB (already flat, as `flattenToc` gives), each with the index of its file in the spine. */
export function epubChapterEntries(flatToc, spine) {
  const items = spine?.spineItems || spine?.items || [];
  const indexOf = (href) => {
    const file = String(href || '').split('#')[0];
    if (!file) return undefined;
    const found = spine?.get?.(file);
    if (Number.isInteger(found?.index)) return found.index;
    // A table of contents says the files relative to itself, the spine relative to the package: one may be the tail of the other.
    const item = items.find((i) => i.href && (i.href.endsWith(file) || file.endsWith(i.href)));
    return Number.isInteger(item?.index) ? item.index : undefined;
  };
  return (flatToc || []).map((entry) => ({ title: entry.label, at: indexOf(entry.href) })).filter((e) => e.title && e.at !== undefined);
}

/** { index, total } of a place in the positions of an EPUB (epub.js `locations`), from 1; undefined while the positions are not known. */
export function epubUnit(locations, cfi) {
  const total = locations?.length?.() ?? 0;
  if (!(total > 0) || !cfi) return undefined;
  const at = locations.locationFromCfi?.(cfi);
  if (!Number.isInteger(at) || at < 0) return undefined;
  return { index: Math.min(at + 1, total), total };
}

/** { index, total } for a page of a PDF or a comic (from 1) in a file of `total` pages; undefined when either is not known. */
export function pageUnit(page, total) {
  if (!Number.isInteger(page) || !Number.isInteger(total) || total < 1 || page < 1 || page > total) return undefined;
  return { index: page, total };
}

/** What a viewer adds to the save of a place: the chapter's title, as one line, and the unit; nothing for what it does not know. */
export function whereExtras({ chapter, unit } = {}) {
  const extras = {};
  const title = String(chapter || '').replace(/\s+/g, ' ').trim();
  if (title) extras.chapter = title.slice(0, MAX_TITLE);
  if (unit) {
    extras.unitIndex = unit.index;
    extras.unitTotal = unit.total;
  }
  return extras;
}

export const COMIC_FORMATS = new Set(['cbz', 'cbr']);
const PAGED_FORMATS = new Set(['pdf', ...COMIC_FORMATS]);

/**
 * Where the person is, as a short line: "Página 42 de 310" for a PDF or a comic, "Pos. 3.412 de 5.018" for an EPUB (which has no pages; the
 * word is the one in the maintainer's drawing, and a title says it whole). A position saved before the reader said how many (DEC-148) still
 * gives the page of a PDF or a comic, and says nothing for the other formats: the text of a plain file is a place in characters, an EPUB's a
 * place in its code, and neither is a page.
 */
export function placeLabel(format, last, legacyProgress) {
  const { unitIndex, unitTotal } = last || {};
  if (unitIndex > 0 && unitTotal >= unitIndex) {
    return `${PAGED_FORMATS.has(format) ? 'Página' : 'Pos.'} ${unitIndex.toLocaleString('pt-BR')} de ${unitTotal.toLocaleString('pt-BR')}`;
  }
  if (!PAGED_FORMATS.has(format)) return null;
  const pageNum = Number.parseInt(legacyProgress, 10);
  if (Number.isNaN(pageNum)) return null;
  return `Página ${COMIC_FORMATS.has(format) ? pageNum + 1 : pageNum}`;
}
