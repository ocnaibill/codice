/** "1 obra", "12 obras". */
export const worksText = (n) => `${n} ${n === 1 ? 'obra' : 'obras'}`;

/** What a card of a collection says of what is in it and of how much the person read: "12 obras · Leu 3 de 12". */
export function collectionLine({ workCount = 0, completedCount = 0 }) {
  if (workCount === 0) return 'Nenhuma obra ainda';
  return completedCount > 0 ? `${worksText(workCount)} · Leu ${completedCount} de ${workCount}` : worksText(workCount);
}

/**
 * The words that change with what is on screen: an official collection of the library, or a list of the person (#207). The
 * management of one is the staff's and of the other is the person's, and what happens to the works is not the same.
 */
export function wordsOf(kind) {
  if (kind === 'personal') {
    return {
      thing: 'lista',
      eyebrow: 'Biblioteca / Lista',
      dialog: 'Lista',
      nowhere: 'Esta lista ainda não tem obras.',
      retired: 'Lista aposentada',
      retiredOnes: 'Listas aposentadas',
      renameNote: 'Só o nome da lista muda: as obras continuam como estão.',
      retireNote: 'As obras continuam no acervo: a lista só sai do menu, e dá para restaurá-la na lista das aposentadas.',
      removeNote: 'A obra continua no acervo: só sai da lista. Dá para acrescentá-la de novo.',
      added: 'foi para o fim da lista.',
      restoreHint: 'As obras voltam quando a lista for restaurada.',
    };
  }
  return {
    thing: 'coleção',
    eyebrow: 'Biblioteca / Coleção',
    dialog: 'Coleção',
    nowhere: 'Esta coleção ainda não tem obras.',
    retired: 'Coleção aposentada',
    retiredOnes: 'Coleções aposentadas',
    renameNote: 'As obras da coleção passam a ter esse nome como série. O nome antigo continua levando a ela.',
    retireNote: 'As obras não são apagadas nem mudam de série: a coleção só sai do acervo, e dá para restaurá-la na lista das aposentadas.',
    removeNote: 'A série da obra é limpa e travada, para a análise do arquivo não recolocá-la. Dá para acrescentá-la de novo.',
    added: 'foi para o fim da coleção.',
    restoreHint: 'As obras voltam quando a coleção for restaurada.',
  };
}

// The units a work of a series can be (#187, DEC-134), in the order they are listed, with how each is said: the heading of the group,
// the short name that goes before the number, and the name for one.
export const UNITS = [
  { key: 'volume', heading: 'Volumes', short: 'Vol.', one: 'Volume' },
  { key: 'chapter', heading: 'Capítulos', short: 'Cap.', one: 'Capítulo' },
  { key: 'oneshot', heading: 'Únicos', short: 'Único', one: 'Único' },
];

/** What a comic is, in the words of the screens. */
export const COMIC_KINDS = [
  { key: 'manga', one: 'Mangá' },
  { key: 'comic', one: 'Quadrinho' },
];

/** A number of a series as it is written: 3 is "3", 27.5 is "27,5", and none is a dash. */
export function numberText(position) {
  if (position == null) return '—';
  return String(position).replace('.', ',');
}

/** What a work is called in its series: "Vol. 3", "Cap. 27,5", "Único"; with no unit, the number alone. */
export function unitLabel(unit, position) {
  const u = UNITS.find((x) => x.key === unit);
  if (!u) return numberText(position);
  return unit === 'oneshot' ? u.short : `${u.short} ${numberText(position)}`;
}

/**
 * The works of a collection in groups by unit (#187): volumes, chapters, one-shots, and those with no unit last. Each keeps its order. A
 * group with no work is left out, and the headings are for when there is more than the works with no unit.
 */
export function groupByUnit(works) {
  const groups = [...UNITS.map((u) => ({ key: u.key, heading: u.heading })), { key: '', heading: 'Sem unidade' }]
    .map((g) => ({ ...g, works: works.filter((w) => (w.unit ?? '') === g.key) }))
    .filter((g) => g.works.length > 0);
  return { groups, headings: groups.some((g) => g.key !== '') };
}
