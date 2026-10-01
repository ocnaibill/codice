// The outline of a PDF (its own table of contents), as a flat list a person can walk: where each entry goes, in
// pages from 1, and how deep it sits. Entries that go nowhere (a link to a site, a destination the file does not
// have) are left out rather than shown as something that does nothing.

export const MAX_OUTLINE = 2000;
const MAX_DEPTH = 6;

async function pageOf(pdf, dest) {
  try {
    let explicit = dest;
    if (typeof dest === 'string') explicit = await pdf.getDestination(dest);
    if (!Array.isArray(explicit) || explicit.length === 0) return null;
    const target = explicit[0];
    if (Number.isInteger(target)) return target + 1; // some files give the page index itself
    if (target && typeof target === 'object') return (await pdf.getPageIndex(target)) + 1;
  } catch {
    // a destination that cannot be resolved goes nowhere
  }
  return null;
}

/** [{ title, page, depth }] in the order of the outline; [] when the PDF has none. */
export async function loadOutline(pdf) {
  if (typeof pdf?.getOutline !== 'function') return [];
  let outline;
  try {
    outline = await pdf.getOutline();
  } catch {
    return [];
  }
  const out = [];
  const walk = async (items, depth) => {
    for (const item of items || []) {
      if (out.length >= MAX_OUTLINE) return;
      const title = (item?.title || '').replace(/\s+/g, ' ').trim();
      const page = title && item?.dest ? await pageOf(pdf, item.dest) : null;
      if (title && page && page >= 1 && (!pdf.numPages || page <= pdf.numPages)) out.push({ title, page, depth });
      if (depth < MAX_DEPTH) await walk(item?.items, depth + 1);
    }
  };
  await walk(outline, 0);
  return out;
}
