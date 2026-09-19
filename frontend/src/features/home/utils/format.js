const numberFormatter = new Intl.NumberFormat('pt-BR');

export function formatCount(n) {
  return numberFormatter.format(n ?? 0);
}

const LABELS = {
  livros: ['livro', 'livros'],
  mangas: ['mangá', 'mangás'],
  audio: ['áudio', 'áudios'],
};

/** "2 livros • 1 mangá • 1 áudio" — skips zero categories, singular/plural aware. */
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
