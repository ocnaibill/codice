// How the reading of scanned pages by OCR is said to a person (#24), in the administration, in the sheet of a work and in
// the search. It lives apart so that the three say the same thing the same way.

const LANGUAGE_NAMES = {
  por: 'Português', eng: 'Inglês', spa: 'Espanhol', fra: 'Francês', ita: 'Italiano', cat: 'Catalão', ron: 'Romeno',
  deu: 'Alemão', nld: 'Holandês', lat: 'Latim',
};

export const languageName = (code) => LANGUAGE_NAMES[code] || code;

/** "por+eng" -> "Português e Inglês". */
export function languagesLabel(codes) {
  // The list is of sets as they were told to the engine ("por+eng", "por"): each is taken apart, and none is said twice.
  const parts = (Array.isArray(codes) ? codes : [codes]).flatMap((set) => String(set || '').split('+')).filter(Boolean);
  const names = [...new Set(parts)].map(languageName);
  if (names.length <= 1) return names[0] || '';
  return `${names.slice(0, -1).join(', ')} e ${names[names.length - 1]}`;
}

const pages = (n) => (n === 1 ? '1 página' : `${n} páginas`);

/**
 * What became of the pages of a scan, for the list of the administration: { text, tone } with the tone one of "ok",
 * "warn" or "plain". `item` is a row of GET /admin/ocr and `enabled` says whether the owner turned OCR on.
 */
export function fileProgress(item, enabled) {
  const total = item.pagesWithoutText?.length ?? 0;
  const read = item.read ?? 0;
  const failed = item.failed ?? 0;
  if (item.state === 'reading') return { text: `Lendo: ${read} de ${total} páginas`, tone: 'plain' };
  if (item.state === 'queued') return { text: 'Na fila para ser lido', tone: 'plain' };
  const where = item.languages?.length ? ` (${languagesLabel(item.languages)})` : '';
  if (failed > 0) {
    return { text: `${read} de ${total} páginas lidas${where}; ${failed === 1 ? '1 não pôde' : `${failed} não puderam`} ser lida${failed === 1 ? '' : 's'}`, tone: 'warn' };
  }
  if (total > 0 && read >= total) return { text: `${total === 1 ? 'A página foi lida' : `As ${total} páginas foram lidas`}${where}`, tone: 'ok' };
  if (!enabled) return { text: `${pages(total)} sem texto, esperando o OCR ser ligado`, tone: 'plain' };
  return { text: read > 0 ? `${read} de ${total} páginas lidas; esperando a vez` : `${pages(total)} esperando a vez`, tone: 'plain' };
}

/** The same for the sheet of a work, from `file.ocr` of the work detail; { text, tone, title } or null when the file has none. */
export function sheetNote(file) {
  if (!file?.needsOcr) return null;
  const ocr = file.ocr;
  if (!ocr) return { text: 'Páginas sem texto', tone: 'warn', title: 'Estas páginas são imagens: não dá para buscar o texto nelas.' };
  if (ocr.state === 'reading') return { text: `Lendo as páginas sem texto (${ocr.read} de ${ocr.pages})`, tone: 'plain', title: 'O OCR está lendo as imagens em segundo plano.' };
  if (ocr.state === 'queued') return { text: 'Páginas sem texto na fila para leitura', tone: 'plain', title: 'O OCR vai ler estas imagens em segundo plano.' };
  if (ocr.failed > 0) {
    return { text: `Texto reconhecido por OCR em ${ocr.read} de ${ocr.pages} páginas; ${ocr.failed} falharam`, tone: 'warn', title: 'O texto reconhecido por OCR pode ter erros. A equipe pode pedir para tentar de novo as páginas que falharam.' };
  }
  if (ocr.pages > 0 && ocr.read >= ocr.pages) return { text: 'Texto reconhecido por OCR', tone: 'ok', title: 'O texto destas páginas foi reconhecido a partir das imagens e pode ter erros.' };
  return { text: 'Páginas sem texto', tone: 'warn', title: 'Estas páginas são imagens: o dono do acervo pode ligar o OCR para torná-las pesquisáveis.' };
}
