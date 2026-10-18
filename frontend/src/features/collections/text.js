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

/**
 * The works of a series in the order of its story (DEC-202). `story` is what the server says: the works in order, the loose chapters nested under the
 * volume that contains them, the works that have no place in it and the chapters no file contains. A nested chapter stays in the list, folded under
 * its volume (`after` has it, with a line for the numbers that are missing right after the work that precedes them); with `flat` (a filter is
 * on) every placed work is a row of its own, so that the search finds it. Works with no place end the view, and the complementary ones after them.
 */
export function groupByStory(works, story, { flat = false, mark = 'none' } = {}) {
  const byId = new Map(works.map((w) => [w.id, w]));
  const pick = (ids) => (ids ?? []).map((id) => byId.get(id)).filter(Boolean);
  const endOf = (w) => w.coversTo ?? w.position ?? 0;
  const top = pick(story.order);
  const after = {};
  const ordered = [];
  const aside = (id) => (after[id] ??= { nested: [], gaps: [] });
  for (const w of top) {
    ordered.push(w);
    const nested = pick(story.nested?.[w.id]);
    if (flat) ordered.push(...nested);
    else if (nested.length > 0) aside(w.id).nested = nested;
  }
  // A stretch no file contains is said right after the last work that ends before it.
  if (!flat) {
    for (const gap of story.gaps ?? []) {
      const before = [...top].reverse().find((w) => endOf(w) < gap.from);
      if (before) aside(before.id).gaps.push(gap);
    }
  }
  const dividers = flat ? {} : dividersOf(top, works, mark);
  const groups = [{ key: 'story', heading: '', works: flat ? ordered : top, after, dividers }];
  const unplaced = pick(story.unplaced);
  if (unplaced.length > 0) {
    groups.push({
      key: 'unplaced',
      heading: 'Sem lugar na história',
      note: 'Não dizem quais capítulos reúnem: diga a faixa na edição da obra e elas entram na ordem.',
      works: unplaced,
    });
  }
  return groups;
}

/** What a work is marked with in the list of the story (DEC-202): the volume it is in (a volume file is its own) or its arc; null when it has none. */
export function markOf(work, mark) {
  if (work.unit === 'extra') return null;
  if (mark === 'volume') return (work.unit === 'volume' ? work.position : work.volumeNumber) ?? null;
  if (mark === 'arc') return work.storyArc || null;
  return null;
}

/** The marks the works of a series carry, to offer "Marcar por": the volume when some work says it, then the arc. */
export function marksOf(works) {
  const sequence = works.filter((w) => w.unit !== 'extra');
  return [
    ...(sequence.some((w) => w.volumeNumber != null) ? [{ key: 'volume', label: 'Volume' }] : []),
    ...(sequence.some((w) => w.storyArc) ? [{ key: 'arc', label: 'Arco' }] : []),
  ];
}

/**
 * The dividers of the list of the story (DEC-202): where the volume (or the arc) changes from one work to the next a divider says which it is, how
 * many chapters it holds and from which to which. It is calculated from the works that carry the mark, and it is not a file: it has no progress, and
 * it does not count as coverage. Works with no mark after a marked stretch go under "Sem volume" or "Sem arco"; a series nothing marks has no divider.
 * `top` are the rows of the list, in order; `all` every work of the series, nested ones too. Keyed by the work that comes right after the divider.
 */
export function dividersOf(top, all, mark) {
  if (mark !== 'volume' && mark !== 'arc') return {};
  const out = {};
  let previous;
  for (const work of top) {
    const key = markOf(work, mark);
    if (key !== previous && (key != null || previous != null)) {
      if (key == null) {
        out[work.id] = { heading: mark === 'volume' ? 'Sem volume' : 'Sem arco', note: null };
      } else {
        const members = all.filter((w) => markOf(w, mark) === key);
        const numbers = members.flatMap((w) => (w.unit === 'chapter' && w.position != null ? [w.position] : [w.coversFrom, w.coversTo].filter((n) => n != null)));
        const chapters = members.filter((w) => w.unit === 'chapter').length;
        const span = numbers.length === 0 ? '' : Math.min(...numbers) === Math.max(...numbers) ? `cap. ${numberText(numbers[0])}` : `caps. ${numberText(Math.min(...numbers))} a ${numberText(Math.max(...numbers))}`;
        const count = chapters > 0 ? `${chapters} ${chapters === 1 ? 'capítulo' : 'capítulos'}` : `${members.length} ${members.length === 1 ? 'obra' : 'obras'}`;
        out[work.id] = { heading: mark === 'volume' ? `Volume ${numberText(key)}` : key, note: [span, count].filter(Boolean).join(' · ') };
      }
    }
    previous = key;
  }
  return out;
}

/** "faltam os caps 10 a 11" for the stretch of a gap. */
export function gapText({ from, to }) {
  return from === to ? `falta o cap. ${numberText(from)}` : `faltam os caps. ${numberText(from)} a ${numberText(to)}`;
}

/**
 * The views the page of a series offers (DEC-169, DEC-202): the ones the works have data for. When some work says what it contains the series
 * reads "Na ordem da história" first, and the groups by unit are "Por unidade"; otherwise "Todas" is the way it was and is always there.
 */
export function viewsOf(works, story = null) {
  const sequence = works.filter((w) => w.unit !== 'extra');
  const ranged = story?.ranged === true;
  // The story also reads a series whose chapters say their volume or arc: the dividers are made of them.
  const marked = story != null && marksOf(works).length > 0;
  return [
    ...(ranged || marked ? [{ key: 'story', label: 'Na ordem da história' }] : []),
    { key: 'units', label: ranged || marked ? 'Por unidade' : 'Todas' },
    ...(sequence.some((w) => w.volumeNumber != null) ? [{ key: 'volumes', label: 'Por volumes' }] : []),
    ...(sequence.some((w) => w.storyArc) ? [{ key: 'arcs', label: 'Por arcos' }] : []),
  ];
}
