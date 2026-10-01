// When a place a person asked for (a note, a search hit, a saved position) cannot be opened in the file, the reader
// does not open the start in silence: it says where the place pointed and why it cannot be opened (RF-014). These
// are the checks, one per kind of file; each answers null when the place can be opened, or what is wrong, in words.

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

/** A PDF page, 1-based as asked, against the pages the file has. `asked` is what the position said. */
export function pdfPlaceProblem(asked, numPages) {
  if (asked == null || asked === '') return null;
  const page = Number(asked);
  if (!Number.isInteger(page) || page < 1) return 'A página pedida não é válida.';
  if (numPages && page > numPages) return `A página ${page} não existe: o arquivo tem ${plural(numPages, 'página', 'páginas')}.`;
  return null;
}

/** An image of a comic, from 0, against the images it has. */
export function imagePlaceProblem(index, total) {
  if (index == null || index === '') return null;
  const n = Number(index);
  if (!Number.isInteger(n) || n < 0) return 'A imagem pedida não é válida.';
  if (total && n >= total) return `A imagem ${n + 1} não existe: o arquivo tem ${plural(total, 'imagem', 'imagens')}.`;
  return null;
}

/** A time in an audio file, in seconds, against how long it is. */
export function audioPlaceProblem(seconds, duration) {
  if (seconds == null || seconds === '') return null;
  const s = Number(seconds);
  if (!Number.isFinite(s) || s < 0) return 'O ponto pedido não é válido.';
  if (Number.isFinite(duration) && duration > 0 && s > duration) return 'O ponto passa do fim do áudio.';
  return null;
}

const sameChapter = (spineHref, wanted) => {
  const a = (spineHref || '').split('#')[0];
  const b = (wanted || '').split('#')[0];
  return !!a && !!b && (a === b || a.endsWith(`/${b}`) || b.endsWith(`/${a}`));
};

/**
 * An EPUB place: its CFI against the book. `section` is the chapter the CFI points into (null when the book has no
 * such chapter) and `locator.href` the chapter the place was saved in: a different chapter at that position means
 * the book changed since.
 */
export function epubPlaceProblem({ section, locator }) {
  if (!section) return 'O capítulo onde o ponto estava não existe mais neste EPUB: o arquivo pode ter mudado.';
  if (locator?.href && !sameChapter(section.href, locator.href)) {
    return 'O capítulo neste ponto não é mais o mesmo de quando ele foi guardado: o arquivo pode ter mudado.';
  }
  return null;
}
