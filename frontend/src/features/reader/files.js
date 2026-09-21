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

// "Terminada 2 vezes: 1 em EPUB, 1 em PDF" (DEC-080), or null when it never was.
export function completionText(completions) {
  const total = completions?.total ?? 0;
  if (!total) return null;
  const parts = Object.entries(completions.byFormat ?? {})
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([format, n]) => `${n} em ${format.toUpperCase()}`);
  return `Terminada ${total} ${total === 1 ? 'vez' : 'vezes'}: ${parts.join(', ')}`;
}

// "Você está em 42% no PDF (Português)": where the person is, in the version that counts.
export function whereYouAre(cont) {
  if (!cont) return null;
  const percent = Math.round(cont.percentComplete || 0);
  const language = cont.language ? ` (${languageName(cont.language)})` : '';
  const format = (cont.format || '').toUpperCase();
  return percent > 0 ? `Você está em ${percent}% no ${format}${language}` : `Você está lendo o ${format}${language}`;
}

// How a candidate position was found, in words a person did not ask to learn the internals of.
const METHOD_LABEL = {
  text: 'mesmo trecho',
  anchors: 'mesmos nomes e números',
  structure: 'mesmo capítulo',
};
const CONFIDENCE_LABEL = { high: 'alta confiança', medium: 'confiança média', low: 'confiança baixa' };

export function candidateLabel(candidate) {
  // An estimate inside the right chapter says so: it is not the passage, and it is not the chapter's start.
  const method = candidate?.precision === 'approximate' ? 'posição aproximada no mesmo capítulo' : (METHOD_LABEL[candidate?.method] ?? 'correspondência aproximada');
  const confidence = CONFIDENCE_LABEL[candidate?.confidence] ?? '';
  return confidence ? `${method}, ${confidence}` : method;
}
