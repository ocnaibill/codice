// What the screens of the wiki (RF-050 to RF-052, DEC-183) say about what the server sends: the names of the kinds and of the groups
// of the ficha, how much the cursor keeps back, the choices of the cursor control, and the graph around an entity.

/** The kinds of entity, in the order of the filters, with the name of one and of many. */
export const KINDS = [
  { key: 'person', one: 'Pessoa', many: 'Pessoas' },
  { key: 'place', one: 'Lugar', many: 'Lugares' },
  { key: 'organization', one: 'Organização', many: 'Organizações' },
  { key: 'concept', one: 'Conceito', many: 'Conceitos' },
  { key: 'object', one: 'Objeto', many: 'Objetos' },
  { key: 'event', one: 'Evento', many: 'Eventos' },
  { key: 'other', one: 'Outro', many: 'Outros' },
];
const KIND_BY_KEY = Object.fromEntries(KINDS.map((k) => [k.key, k]));
export const kindName = (key) => KIND_BY_KEY[key]?.one ?? 'Outro';

/** The colour of each kind in the graphs: the brand for people, and quieter tones of the same paper for the rest. */
export const KIND_COLORS = {
  person: '#9c4918',
  place: '#3a6758',
  organization: '#5b5f8f',
  concept: '#8a6d1f',
  object: '#7a4f6d',
  event: '#a33b3b',
  other: '#7d7268',
};

/** The groups of the ficha (DEC-178), in its order. */
export const GROUPS = {
  family: 'Família',
  mentors: 'Mentores',
  romance: 'Romance',
  allies: 'Aliados',
  enemies: 'Inimigos',
  places: 'Lugares',
  things: 'Objetos, conceitos e eventos',
  others: 'Outros vínculos',
};

/** How many of each kind the index has, for the filters: only the kinds that have someone. */
export function kindCounts(index) {
  const counts = new Map();
  for (const e of index ?? []) counts.set(e.kind, (counts.get(e.kind) ?? 0) + 1);
  return KINDS.filter((k) => counts.has(k.key)).map((k) => ({ ...k, count: counts.get(k.key) }));
}

const fold = (text) => String(text ?? '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().trim();

/**
 * The entities of the index a filter keeps: of the kind chosen (or any), whose name or another name has what was typed. Those with a page
 * come first, then the ones that appear the most.
 */
export function filterIndex(index, { kind = '', text = '' } = {}) {
  const wanted = fold(text);
  return (index ?? [])
    .filter((e) => !kind || e.kind === kind)
    .filter((e) => !wanted || [e.name, ...(e.aliases ?? [])].some((n) => fold(n).includes(wanted)))
    .sort((a, b) => Number(b.page) - Number(a.page) || b.appearances - a.appearances || a.name.localeCompare(b.name, 'pt'));
}

/**
 * What the cursor keeps back: the story chapters after it in its book, and the main-line books after that one. Someone who has not
 * started has every book kept back.
 */
export function keptBack(books, cursor) {
  const main = (books ?? []).filter((b) => !b.side);
  if (!cursor || cursor.book < 0) return { chapters: 0, books: main.length };
  const here = main.find((b) => b.position === cursor.book);
  const chapters = here ? here.chapters.filter((c) => c.chapter > cursor.chapter).length : 0;
  return { chapters, books: main.filter((b) => b.position > cursor.book).length };
}

/** The name of a chapter in the cursor control: its number, and its title when the reader may know it. */
export function chapterName(c) {
  const title = String(c.title ?? '').trim();
  if (!title) return `Capítulo ${c.chapter}`;
  return /^\d+$/.test(title) || /^cap/i.test(title) ? title : `${c.chapter}. ${title}`;
}

/** The value of a point in the cursor control ("book:chapter") and back. */
export const pointValue = (book, chapter) => `${book}:${chapter}`;
export function pointOf(value) {
  const [book, chapter] = String(value).split(':').map(Number);
  return Number.isInteger(book) && Number.isInteger(chapter) ? { book, chapter } : null;
}

/** "1 capítulo", "3 capítulos". */
export const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

/**
 * The entity and its closest neighbours in the graph, for the minigraph of its page: at most `limit`, the strongest links first, and the
 * labels that say why they are linked.
 */
export function neighbourhood(graph, slug, limit = 8) {
  if (!graph) return null;
  const nodes = new Map((graph.nodes ?? []).map((n) => [n.slug, n]));
  const center = nodes.get(slug);
  if (!center) return null;
  const links = (graph.edges ?? [])
    .filter((e) => e.a === slug || e.b === slug)
    .map((e) => ({ ...e, other: nodes.get(e.a === slug ? e.b : e.a) }))
    .filter((e) => e.other)
    .sort((x, y) => y.weight - x.weight || x.other.name.localeCompare(y.other.name, 'pt'))
    .slice(0, limit);
  return { center, links };
}

/** The nodes and edges a filter keeps: of the kind chosen (or any), linked by at least `weight` records, and no node left alone. */
export function filterGraph(graph, { kind = '', weight = 1 } = {}) {
  const keep = new Set((graph?.nodes ?? []).filter((n) => !kind || n.kind === kind).map((n) => n.slug));
  const edges = (graph?.edges ?? []).filter((e) => e.weight >= weight && keep.has(e.a) && keep.has(e.b));
  const linked = new Set(edges.flatMap((e) => [e.a, e.b]));
  const nodes = (graph?.nodes ?? []).filter((n) => linked.has(n.slug));
  return { nodes, edges, alone: keep.size - nodes.length };
}

/**
 * Where each node of a graph goes: a force layout (nodes push one another away, edges pull their ends together) run a fixed number of
 * times from places taken from the order of the nodes, so that the same graph always lands the same way. Positions are in [0, 1].
 */
export function layoutGraph(nodes, edges, { iterations = 300 } = {}) {
  const n = nodes.length;
  if (n === 0) return new Map();
  const index = new Map(nodes.map((node, i) => [node.slug, i]));
  // A spiral, by importance (the order the server gives): the most present ones near the middle.
  const x = new Float64Array(n);
  const y = new Float64Array(n);
  for (let i = 0; i < n; i += 1) {
    const angle = i * 2.399963; // the golden angle
    const r = Math.sqrt((i + 0.5) / n) * 0.5;
    x[i] = 0.5 + r * Math.cos(angle);
    y[i] = 0.5 + r * Math.sin(angle);
  }
  const links = edges
    .map((e) => [index.get(e.a), index.get(e.b), e.weight])
    .filter(([a, b]) => a !== undefined && b !== undefined && a !== b);
  const maxWeight = links.reduce((m, [, , w]) => Math.max(m, w), 1);
  const ideal = Math.min(0.18, 0.9 / Math.sqrt(n));
  const dx = new Float64Array(n);
  const dy = new Float64Array(n);
  for (let step = 0; step < iterations; step += 1) {
    const heat = 0.08 * (1 - step / iterations) + 0.002;
    dx.fill(0);
    dy.fill(0);
    for (let i = 0; i < n; i += 1) {
      for (let j = i + 1; j < n; j += 1) {
        let ex = x[i] - x[j];
        let ey = y[i] - y[j];
        let d2 = ex * ex + ey * ey;
        if (d2 < 1e-9) {
          ex = ((i * 7 + j) % 13) / 1300 - 0.005;
          ey = ((i * 3 + j) % 11) / 1100 - 0.005;
          d2 = ex * ex + ey * ey;
        }
        const f = (ideal * ideal) / d2;
        dx[i] += ex * f;
        dy[i] += ey * f;
        dx[j] -= ex * f;
        dy[j] -= ey * f;
      }
    }
    for (const [a, b, w] of links) {
      const ex = x[a] - x[b];
      const ey = y[a] - y[b];
      const d = Math.sqrt(ex * ex + ey * ey) || 1e-6;
      const f = (d / ideal) * (0.4 + 0.6 * (w / maxWeight));
      dx[a] -= ex * f;
      dy[a] -= ey * f;
      dx[b] += ex * f;
      dy[b] += ey * f;
    }
    for (let i = 0; i < n; i += 1) {
      // A pull to the middle keeps the pieces that are not linked from drifting off.
      dx[i] += (0.5 - x[i]) * 0.6;
      dy[i] += (0.5 - y[i]) * 0.6;
      const len = Math.sqrt(dx[i] * dx[i] + dy[i] * dy[i]) || 1;
      const move = Math.min(len, heat);
      x[i] += (dx[i] / len) * move;
      y[i] += (dy[i] / len) * move;
    }
  }
  // Fitted to the square, with a margin.
  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
  for (let i = 0; i < n; i += 1) {
    minX = Math.min(minX, x[i]); maxX = Math.max(maxX, x[i]);
    minY = Math.min(minY, y[i]); maxY = Math.max(maxY, y[i]);
  }
  const span = Math.max(maxX - minX, maxY - minY) || 1;
  const out = new Map();
  nodes.forEach((node, i) => {
    out.set(node.slug, {
      x: 0.05 + 0.9 * ((x[i] - minX) / span + (1 - (maxX - minX) / span) / 2),
      y: 0.05 + 0.9 * ((y[i] - minY) / span + (1 - (maxY - minY) / span) / 2),
    });
  });
  return out;
}

/** "deepseek" → "DeepSeek": the providers the generator knows, by the name people know them by. */
const PROVIDERS = { deepseek: 'DeepSeek', nim: 'NVIDIA NIM', tokenharbor: 'Tokenharbor', ollama: 'Ollama', openai: 'OpenAI', anthropic: 'Anthropic' };
export const providerName = (key) => PROVIDERS[String(key ?? '').toLowerCase()] ?? key;
