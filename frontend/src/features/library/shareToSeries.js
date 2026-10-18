// What the editor of a work can hand to the other works of its series (DEC-203). The server decides what changes; this is what the page offers
// and how it says what the preview found.

export const SHARE_FIELDS = [
  ['publisher', 'Editora'],
  ['language', 'Idioma'],
  ['comic_kind', 'Tipo (mangá ou quadrinho)'],
  ['original_year', 'Ano original'],
  ['description', 'Sinopse'],
  ['authors', 'Autores'],
  ['translators', 'Tradutores'],
  ['illustrators', 'Desenhistas'],
  ['categories', 'Categorias'],
  ['tags', 'Etiquetas'],
];

export const SHARE_WHERE = [
  ['all', 'Todas as obras', []],
  ['chapter', 'Só capítulos', ['chapter']],
  ['volume', 'Só volumes', ['volume']],
];

export const SHARE_MODES = [
  ['fill', 'Preencher só onde está em branco'],
  ['replace', 'Substituir em todas'],
];

const NAMES = Object.fromEntries(SHARE_FIELDS);

/** The body of the request: what to give, to whom and how. `source` is the work being edited. */
export function shareRequest({ source, fields, where, mode, preview }) {
  const units = SHARE_WHERE.find(([id]) => id === where)?.[2] ?? [];
  return { sourceWorkId: source, fields, units, mode, preview };
}

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

/** What the preview of the server says, in words: how many works change, how many keep their own, and what the work has none of. */
export function previewLines(answer, mode) {
  const lines = [];
  const changed = answer.worksChanged ?? 0;
  lines.push(
    changed === 0
      ? `Nenhuma das ${plural(answer.works, 'outra obra', 'outras obras')} da série muda.`
      : `${plural(changed, 'obra muda', 'obras mudam')}, de ${plural(answer.works, 'outra da série', 'outras da série')}.`
  );
  const kept = Object.entries(answer.kept ?? {}).filter(([, n]) => n > 0);
  if (kept.length && mode === 'fill') {
    lines.push(`Ficam como estão, por já terem outro valor: ${kept.map(([field, n]) => `${NAMES[field] ?? field} (${n})`).join(', ')}.`);
  }
  const none = answer.sourceHasNone ?? [];
  if (none.length) lines.push(`Esta obra não tem ${none.map((field) => NAMES[field] ?? field).join(', ')}: não há o que levar.`);
  return lines;
}
