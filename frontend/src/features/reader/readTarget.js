/**
 * What the "Read" button of a card does (DEC-081):
 * - a work in progress opens the version that counts, the one opened last, straight away;
 * - a work with more than one file to choose from, and a work that has been finished before (so
 *   there is a count and "read again" to offer), opens its sheet, where the person chooses;
 * - a work with a single file and no history opens that file: there is nothing to choose.
 */
export function readTarget(work) {
  if (work.inProgress && work.continue?.fileId) return { kind: 'file', fileId: work.continue.fileId };
  if ((work.fileCount ?? 1) > 1 || work.continue?.completed) return { kind: 'sheet' };
  return { kind: 'file', fileId: null }; // the primary file
}

/** The label a person reads for the button, from the same rule. */
export function readLabel(work) {
  if (work.inProgress) return 'Continuar de onde parou';
  return readTarget(work).kind === 'sheet' ? 'Escolher versão para ler' : 'Ler';
}
