import { numberText } from './text';

// The ruler of a series (DEC-202): the chapters of its story in a line, with what the library has of each, what the person has read and what no
// file contains. It is drawn from what the page already knows: the works and the story the server answered.

/** The most chapter numbers a ruler is drawn for; a series longer than that is a list. */
export const MAX_RULER = 1500;

/**
 * { first, last, cells, volumes, loose, gaps, read } for the ruler, or null when there is nothing to draw (no work says what it contains, or the
 * story is too long). `cells` are the whole numbers from the first to the last chapter, each "read", "have" or "gap". `volumes` are the works
 * that contain more than one chapter and `loose` the chapters that are a file of their own, each with where it begins and ends.
 */
export function rulerOf(works, story) {
  if (!story?.ranged || !story.chapters) return null;
  const placed = works.filter((w) => w.unit !== 'extra' && (w.coversFrom != null || (w.unit === 'chapter' && w.position != null)));
  const spans = placed.map((w) => (w.coversFrom != null ? [w.coversFrom, w.coversTo] : [w.position, w.position]));
  if (spans.length === 0) return null;
  const first = Math.min(...spans.map(([a]) => a));
  const last = Math.max(...spans.map(([, b]) => b));
  const from = Math.ceil(first);
  const to = Math.floor(last);
  if (to < from || to - from + 1 > MAX_RULER) return null;

  const gapAt = new Set();
  for (const g of story.gaps ?? []) for (let n = g.from; n <= g.to; n += 1) gapAt.add(n);
  const readAt = new Set();
  for (const r of story.readSpans ?? []) for (let n = Math.ceil(r.from); n <= r.to; n += 1) readAt.add(n);
  const cells = [];
  for (let n = from; n <= to; n += 1) cells.push({ n, state: gapAt.has(n) ? 'gap' : readAt.has(n) ? 'read' : 'have' });

  const volumes = [];
  const loose = [];
  placed.forEach((w, i) => {
    const [a, b] = spans[i];
    const item = { id: w.id, title: w.title, from: a, to: b, done: !!w.completed, unit: w.unit, position: w.position };
    if (b > a) volumes.push(item);
    else if (w.unit === 'chapter') loose.push(item);
  });
  volumes.sort((x, y) => x.from - y.from);
  loose.sort((x, y) => x.from - y.from);
  return { first: from, last: to, cells, volumes, loose, gaps: story.gaps ?? [], read: story.readSpans ?? [], chapters: story.chapters, readCount: story.read };
}

/** "caps. 27 a 30", "cap. 5": a stretch in words. */
export function spanText({ from, to }) {
  return from === to ? `cap. ${numberText(from)}` : `caps. ${numberText(from)} a ${numberText(to)}`;
}

/** The chips over the ruler: what was read, what the library has of the whole, and what no file contains. */
export function rulerChips(ruler) {
  const total = ruler.cells.length;
  const missing = ruler.cells.filter((c) => c.state === 'gap').length;
  const chips = [];
  if (ruler.read.length > 0) chips.push({ tone: 'ok', text: `Lido: ${ruler.read.map(spanText).join(' · ')}` });
  chips.push({ tone: 'plain', text: `No acervo: ${total - missing} de ${total} capítulos` });
  if (ruler.gaps.length > 0) chips.push({ tone: 'warn', text: `Sem arquivo: ${ruler.gaps.map(spanText).join(' · ')}` });
  if (ruler.volumes.length > 0 || ruler.loose.length > 0) {
    chips.push({ tone: 'plain', text: `${ruler.volumes.length} ${ruler.volumes.length === 1 ? 'arquivo que reúne capítulos' : 'arquivos que reúnem capítulos'} · ${ruler.loose.length} ${ruler.loose.length === 1 ? 'capítulo avulso' : 'capítulos avulsos'}` });
  }
  return chips;
}

/** Where the person stopped, as a place on the ruler: after the last chapter of the first stretch read from the beginning; null when nothing is read. */
export function readThrough(ruler) {
  const first = ruler.read[0];
  return first && first.from <= ruler.first ? first.to : null;
}

/** The label of a work on the ruler: "Vol. 3" for a volume (its number), "Cap. 12" for a chapter, the title otherwise. */
export function rulerLabel(item) {
  if (item.unit === 'volume' && item.position != null) return `Vol. ${numberText(item.position)}`;
  if (item.unit === 'chapter' && item.position != null) return `Cap. ${numberText(item.position)}`;
  return item.title;
}
