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
  // Belongs to the collection but not to its sequence (DEC-164): a companion book, a guide, an art book. It has no number of its own.
  { key: 'extra', heading: 'Complementares', short: 'Compl.', one: 'Complementar' },
];

/** The units that are a work by itself, with no number in a sequence: "Único" and "Compl.". */
const UNNUMBERED = ['oneshot', 'extra'];

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

/** What a work is called in its series: "Vol. 3", "Cap. 27,5", "Único", "Compl."; with no unit, the number alone. */
export function unitLabel(unit, position) {
  const u = UNITS.find((x) => x.key === unit);
  if (!u) return numberText(position);
  return UNNUMBERED.includes(unit) ? u.short : `${u.short} ${numberText(position)}`;
}

/**
 * What a step of a series is called on a button (#187): "Vol. 3" or "Cap. 27,5" when the work has a unit and a number, "Único" for
 * a one-shot, and the title when it has neither.
 */
export function stepText(step) {
  const numbered = UNITS.some((u) => u.key === step.unit) && (UNNUMBERED.includes(step.unit) || step.position != null);
  return numbered ? unitLabel(step.unit, step.position) : step.title;
}

/** What a button says to go on with a series (#187): "Continuar: Cap. 27" (begun), "Próximo: Cap. 28" (read some) or "Começar: Vol. 1". */
export function goOnText(step) {
  return `${step.started ? 'Continuar' : step.begun ? 'Próximo' : 'Começar'}: ${stepText(step)}`;
}

/**
 * What the works of the sequence of a series are, in the one word of their unit ("capítulos", "volumes", "únicos"), when all of them are of
 * the same unit; null when they are mixed or have none (DEC-165). The complementary works and those gone from the library are not the
 * sequence. `lidos` is the participle that goes with the word.
 */
export function sequenceWord(works, count = works.length) {
  const sequence = works.filter((w) => w.available !== false && w.unit !== 'extra');
  const units = new Set(sequence.map((w) => w.unit || ''));
  if (units.size !== 1) return null;
  const words = { volume: ['volume', 'volumes'], chapter: ['capítulo', 'capítulos'], oneshot: ['único', 'únicos'] }[[...units][0]];
  return words ? { noun: words[count === 1 ? 0 : 1], read: count === 1 ? 'lido' : 'lidos' } : null;
}

/** The new ones of a series, as the badge of its card says it (#187): "1 novo", "3 novos". */
export const newText = (n) => `${n} ${n === 1 ? 'novo' : 'novos'}`;

/** What a series has, by unit: "30 volumes · 121 capítulos"; a unit with none is left out. */
export function seriesCounts({ volumes = 0, chapters = 0, oneShots = 0 }) {
  return [
    volumes > 0 && `${volumes} ${volumes === 1 ? 'volume' : 'volumes'}`,
    chapters > 0 && `${chapters} ${chapters === 1 ? 'capítulo' : 'capítulos'}`,
    oneShots > 0 && `${oneShots} ${oneShots === 1 ? 'único' : 'únicos'}`,
  ]
    .filter(Boolean)
    .join(' · ');
}

/**
 * The works of a collection in groups by unit (#187): volumes, chapters, one-shots, those with no unit, and the complementary ones last
 * (DEC-164). Each keeps its order. A group with no work is left out, and the headings are for when there is more than the works with no
 * unit.
 */
export function groupByUnit(works) {
  const sequence = UNITS.filter((u) => u.key !== 'extra').map((u) => ({ key: u.key, heading: u.heading }));
  const extra = UNITS.find((u) => u.key === 'extra');
  const groups = [...sequence, { key: '', heading: 'Sem unidade' }, { key: extra.key, heading: extra.heading }]
    .map((g) => ({ ...g, works: works.filter((w) => (w.unit ?? '') === g.key) }))
    .filter((g) => g.works.length > 0);
  return { groups, headings: groups.some((g) => g.key !== '') };
}

/**
 * The works of a collection by volume (DEC-169): a bound volume holds the chapters that were collected in it, with the volume file of that number
 * first when the library has one; the works that no volume says are in a group of their own, at the end. Volumes go in numeric order.
 */
export function groupByVolume(works) {
  const sequence = works.filter((w) => w.unit !== 'extra');
  const byNumber = new Map();
  const loose = [];
  for (const w of sequence) {
    const n = w.unit === 'volume' ? w.position : w.volumeNumber;
    if (n == null) loose.push(w);
    else (byNumber.get(n) ?? byNumber.set(n, []).get(n)).push(w);
  }
  const groups = [...byNumber.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([n, list]) => {
      // The volume file first, then the chapters in the order of the collection.
      const ordered = [...list.filter((w) => w.unit === 'volume'), ...list.filter((w) => w.unit !== 'volume')];
      const chapters = list.filter((w) => w.unit === 'chapter').length;
      return {
        key: `volume:${n}`,
        heading: `Volume ${numberText(n)}`,
        note: chapters > 0 ? `${chapters} ${chapters === 1 ? 'capítulo' : 'capítulos'}` : null,
        works: ordered,
      };
    });
  if (loose.length > 0) groups.push({ key: 'volume:none', heading: 'Sem volume', note: null, works: loose });
  return groups;
}

/**
 * The works of a collection by story arc (DEC-169), in the order of the first chapter of each arc, with the numbers it spans; the works with no
 * arc are in a group of their own, at the end.
 */
export function groupByArc(works) {
  const sequence = works.filter((w) => w.unit !== 'extra');
  const byArc = new Map();
  const loose = [];
  for (const w of sequence) {
    if (!w.storyArc) loose.push(w);
    else (byArc.get(w.storyArc) ?? byArc.set(w.storyArc, []).get(w.storyArc)).push(w);
  }
  const first = (list) => Math.min(...list.map((w) => (w.position == null ? Infinity : w.position)));
  const groups = [...byArc.entries()]
    .sort((a, b) => first(a[1]) - first(b[1]) || a[0].localeCompare(b[0], 'pt-BR'))
    .map(([arc, list]) => {
      const numbers = list.filter((w) => w.position != null).map((w) => w.position);
      const span = numbers.length === 0 ? '' : Math.min(...numbers) === Math.max(...numbers) ? ` ${numberText(numbers[0])}` : `s ${numberText(Math.min(...numbers))} a ${numberText(Math.max(...numbers))}`;
      const kind = list.every((w) => w.unit === 'volume') ? 'Volume' : list.every((w) => w.unit === 'chapter') ? 'Capítulo' : 'Obra';
      const total = `${list.length} ${list.length === 1 ? 'obra' : 'obras'}`;
      return { key: `arc:${arc}`, heading: arc, note: `${span ? `${kind}${span} · ` : ''}${total}`, works: list };
    });
  if (loose.length > 0) groups.push({ key: 'arc:none', heading: 'Sem arco', note: null, works: loose });
  return groups;
}

/** The complementary works, as the group that ends every view (DEC-164). */
export function extraGroup(works) {
  const extras = works.filter((w) => w.unit === 'extra');
  const unit = UNITS.find((u) => u.key === 'extra');
  return extras.length > 0 ? [{ key: 'extra', heading: unit.heading, works: extras }] : [];
}

/** The views the page of a series offers (DEC-169): the ones the works have data for. "Capítulos" is the way it was and is always there. */
export function viewsOf(works) {
  const sequence = works.filter((w) => w.unit !== 'extra');
  return [
    { key: 'units', label: 'Todas' },
    ...(sequence.some((w) => w.volumeNumber != null) ? [{ key: 'volumes', label: 'Por volumes' }] : []),
    ...(sequence.some((w) => w.storyArc) ? [{ key: 'arcs', label: 'Por arcos' }] : []),
  ];
}
