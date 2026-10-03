// What a person is told when a work is only partly processed (RN-018, #77): a work can be readable and not yet searchable, a
// scan has no text until OCR reads it, and the search sees only what has been read. The sheet of a work and the search say it the
// same way, from here.

// The formats whose text is read for the search; a comic or an audiobook says nothing of "text waiting".
const TEXT_FORMATS = ['pdf', 'epub', 'txt', 'md', 'mobi'];

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

/**
 * The state of the text of a file for the search, as the sheet of a work shows it: { text, tone, title }, with the tone "ok",
 * "warn" or "plain"; or null when there is nothing to say (a kind of file that has no text, such as a comic or an audiobook).
 * `ocr` is what lib/ocr.js says of the pages of a scan, when there is one.
 */
export function fileTextState(file, ocr = null) {
  switch (file?.textStatus) {
    case 'ready':
      return file.textSegments > 0
        ? { text: 'texto indexado', tone: 'ok', title: 'O texto deste arquivo está indexado para a busca' }
        : { text: 'nenhum texto pesquisável', tone: 'warn', title: 'O arquivo foi lido, mas não tem texto que a busca possa achar' };
    case 'failed':
      return { text: 'texto não lido', tone: 'warn', title: 'O arquivo abre, mas o texto não pôde ser lido para a busca' };
    case 'empty':
      // A scan that OCR has not been given leaves this to the note of the OCR, which says what is to be done.
      return ocr ? null : { text: 'sem texto pesquisável', tone: 'warn', title: 'O arquivo não tem texto que a busca possa achar' };
    case 'unsupported':
      return null;
    default:
      if (file?.format && !TEXT_FORMATS.includes(String(file.format).toLowerCase())) return null;
      return { text: 'texto na fila', tone: 'plain', title: 'O texto deste arquivo ainda não foi lido para a busca: ele já pode ser aberto' };
  }
}

/**
 * What the search of the text cannot see yet, for a sentence under the passages: `coverage` is what the server says
 * ({ reading, failed, noText }). Null when it sees everything, or does not say.
 */
export function coverageNote(coverage) {
  const reading = coverage?.reading ?? 0;
  const failed = coverage?.failed ?? 0;
  const noText = coverage?.noText ?? 0;
  const parts = [];
  if (reading > 0) parts.push(`o texto de ${plural(reading, 'arquivo', 'arquivos')} ainda está sendo lido`);
  if (failed > 0) parts.push(`o texto de ${plural(failed, 'arquivo', 'arquivos')} não pôde ser lido`);
  if (noText > 0) parts.push(`${plural(noText, 'arquivo não tem', 'arquivos não têm')} texto (digitalizações, que o OCR pode ler)`);
  if (parts.length === 0) return null;
  const list = parts.length === 1 ? parts[0] : `${parts.slice(0, -1).join(', ')} e ${parts[parts.length - 1]}`;
  return `A busca ainda não vê tudo: ${list}. Uma passagem que não aparece pode estar num desses arquivos.`;
}
