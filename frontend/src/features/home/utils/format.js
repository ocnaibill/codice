const numberFormatter = new Intl.NumberFormat('pt-BR');

export function formatCount(n) {
  return numberFormatter.format(n ?? 0);
}

const LABELS = {
  livros: ['livro', 'livros'],
  quadrinhos: ['quadrinho', 'quadrinhos'],
  mangas: ['mangá', 'mangás'],
  audio: ['áudio', 'áudios'],
};

/** "2 livros • 1 quadrinho • 1 mangá • 1 áudio" — skips zero categories, singular/plural aware. */
export function formatBreakdown(breakdown) {
  if (!breakdown) return '';
  return Object.entries(breakdown)
    .filter(([, count]) => count > 0)
    .map(([key, count]) => {
      const [singular, plural] = LABELS[key] ?? [key, key];
      return `${count} ${count === 1 ? singular : plural}`;
    })
    .join(' • ');
}

/** Total accumulated reading time as "12h 30min", "45min", or "0min". */
export function formatReadingTime(totalSeconds) {
  const seconds = totalSeconds ?? 0;
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours === 0) return `${minutes}min`;
  if (minutes === 0) return `${hours}h`;
  return `${hours}h ${minutes}min`;
}

/** Coarse relative label for a timestamp, e.g. "há 2h", "ontem", "há 3 dias". */
export function formatRelativeDate(isoString) {
  if (!isoString) return '';
  const date = new Date(isoString);
  if (Number.isNaN(date.getTime())) return '';
  const diffMs = Date.now() - date.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return 'agora mesmo';
  if (diffMin < 60) return `há ${diffMin} min`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `há ${diffHours}h`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays === 1) return 'ontem';
  return `há ${diffDays} dias`;
}

/** The count of a shelf as the menus show it, in brackets: "[05]", "[12]", "[1.420]". Two digits at least, so that
 *  the column of a menu lines up. Nothing for a count that is not known yet. */
export function bracketCount(n) {
  if (n == null) return '';
  return `[${n < 10 ? String(n).padStart(2, '0') : numberFormatter.format(n)}]`;
}

/** What the corner of a cover says: the format when the work has one, "2 formatos" when it has more than one. */
export function formatBadge(item) {
  return item.formatCount > 1 ? `${item.formatCount} formatos` : item.format.toUpperCase();
}


const SINGULAR = { obras: 'obra', itens: 'item', 'volumes no acervo': 'volume no acervo' };

/** The word of a count, in the singular for one: "[ 1 obra ]", "[ 2 obras ]". A word it does not know is left as it was. */
export function countWordFor(n, plural) {
  return n === 1 ? SINGULAR[plural] ?? plural : plural;
}
