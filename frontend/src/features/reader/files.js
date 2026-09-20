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

// The plain-text position the viewers open from, given a locator (the same forms the server
// keeps in sync: a CFI, a page number from 1, an image index from 0, seconds, a character offset).
export function positionFromLocator(locator) {
  switch (locator?.type) {
    case 'epub':
      return locator.cfi || locator.href || undefined;
    case 'pdf':
      return String(locator.page + 1);
    case 'image':
      return String(locator.index);
    case 'audio':
      return String(locator.ms / 1000);
    case 'text':
      return String(locator.offset);
    default:
      return undefined;
  }
}

// Where a locator points, in words, for a person.
export function placeLabel(locator) {
  switch (locator?.type) {
    case 'epub':
      return locator.href ? `EPUB, capítulo ${locator.href}` : 'EPUB, posição salva';
    case 'pdf':
      return locator.label ? `PDF, página ${locator.label}` : `PDF, página ${locator.page + 1}`;
    case 'image':
      return `Imagem ${locator.index + 1}`;
    case 'audio': {
      const s = Math.floor(locator.ms / 1000);
      return `Áudio, faixa ${locator.track + 1}, ${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
    }
    case 'text':
      return `Texto, caractere ${locator.offset}`;
    default:
      return null;
  }
}

// "a, b ,, c" -> ['a', 'b', 'c']; the server cleans them again, this only splits what was typed.
export function parseTags(text) {
  return (text || '').split(',').map((t) => t.trim()).filter(Boolean);
}
