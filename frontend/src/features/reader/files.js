// Which file of a work is being read, from the detail of the work (GET /works/{id}).
// The file asked for wins; without one, the work's primary file (the card's `fileId`).
export function findFile(work, fileId) {
  if (!work) return null;
  const wanted = fileId ?? work.fileId ?? null;
  for (const edition of work.editions ?? []) {
    for (const file of edition.files ?? []) {
      if (file.id === wanted) return { ...file, edition };
    }
  }
  if (work.fileUrl && (fileId == null || fileId === work.fileId)) {
    return { id: work.fileId, format: work.format, url: work.fileUrl, availability: 'available', edition: null };
  }
  return null;
}

const LANGUAGE_NAMES = { pt: 'Português', 'pt-BR': 'Português (Brasil)', en: 'Inglês', es: 'Espanhol', fr: 'Francês', de: 'Alemão', it: 'Italiano', ja: 'Japonês' };

export function languageName(code) {
  if (!code) return null;
  return LANGUAGE_NAMES[code] ?? LANGUAGE_NAMES[code.split('-')[0]] ?? code;
}

export function formatSize(bytes) {
  if (bytes == null) return '';
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
