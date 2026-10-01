// The most the server puts in one export (the oldest first).
export const EXPORT_CAP = 10000;

const ONLY = { note: 'só notas', highlight: 'só destaques', bookmark: 'só marcadores' };

/** What the filters narrow the notes to, in words. */
export function describeFilters({ q, kind, tag, work }) {
  const parts = [];
  if (work) parts.push(`obra “${work.title}”`);
  if (kind) parts.push(ONLY[kind] ?? `só ${kind}`);
  if (tag) parts.push(`tag #${tag}`);
  if (q?.trim()) parts.push(`texto “${q.trim()}”`);
  return parts.length ? parts.join(', ') : 'todas as suas anotações';
}
