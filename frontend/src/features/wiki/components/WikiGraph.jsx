import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { KINDS, KIND_COLORS, filterGraph, kindName, layoutGraph, plural } from '../wiki';

const W = 1000;
const H = 720;
// Names written on the graph without hovering: the most present ones, so that it can be read and does not become a blot.
const LABELED = 30;
// Past this many links, the graph opens showing only those seen at least twice.
const BUSY = 300;

/**
 * The whole graph of the wiki up to the cursor (DEC-182): every kind of entity, so that the places, events and objects of the story are
 * in it too. A node is chosen with a click (or the keyboard) and its links are listed beside it, with why they are linked and the way to
 * its page. Distinct from the personal graph of notes (DEC-109).
 */
export function WikiGraph({ query, kind, focus, onOpen }) {
  const { data: graph, isLoading, isError, error, refetch, isRefetching } = query;
  // A big graph starts without the links seen only once, which make it a blot; the person may ask for them.
  const [chosenWeight, setWeight] = React.useState(null);
  const weight = chosenWeight ?? ((graph?.edges?.length ?? 0) > BUSY ? 2 : 1);
  const [chosen, setChosen] = React.useState(focus ?? null);
  const { nodes, edges, alone } = React.useMemo(() => filterGraph(graph, { kind, weight }), [graph, kind, weight]);
  const places = React.useMemo(() => layoutGraph(nodes, edges), [nodes, edges]);
  const maxWeight = edges.reduce((m, e) => Math.max(m, e.weight), 1);
  const maxSeen = nodes.reduce((m, n) => Math.max(m, n.appearances), 1);

  const byslug = React.useMemo(() => new Map((graph?.nodes ?? []).map((n) => [n.slug, n])), [graph]);
  const current = chosen && byslug.get(chosen);
  const links = React.useMemo(
    () =>
      current
        ? (graph.edges ?? [])
            .filter((e) => e.a === chosen || e.b === chosen)
            .map((e) => ({ ...e, other: byslug.get(e.a === chosen ? e.b : e.a) }))
            .filter((e) => e.other)
            .sort((a, b) => b.weight - a.weight)
        : [],
    [graph, chosen, current, byslug]
  );
  const near = new Set(links.map((l) => l.other.slug));
  const labeled = React.useMemo(() => {
    // Names are written from the node chosen, its links (strongest first) and then the most present ones, and one that would fall on
    // a name already written is left for the hover and the list beside.
    const order = [
      ...(chosen ? [chosen] : []),
      ...links.map((l) => l.other.slug),
      ...[...nodes].sort((a, b) => b.appearances - a.appearances).map((n) => n.slug),
    ];
    const names = new Map(nodes.map((n) => [n.slug, n.name]));
    const taken = [];
    const out = new Set();
    for (const slug of order) {
      if (out.has(slug) || !names.has(slug)) continue;
      if (out.size >= LABELED + (chosen ? links.length + 1 : 0)) break;
      const p = places.get(slug);
      if (!p) continue;
      const w = names.get(slug).length * 7.6 + 8;
      const box = { l: p.x * W - w / 2, r: p.x * W + w / 2, t: p.y * H + 2, b: p.y * H + 26 };
      if (taken.some((o) => box.l < o.r && box.r > o.l && box.t < o.b && box.b > o.t)) continue;
      taken.push(box);
      out.add(slug);
    }
    return out;
  }, [nodes, places, chosen, links]);

  if (isLoading) return <p className="animate-pulse py-6 text-sm text-ink-faint">Carregando o grafo…</p>;
  if (isError) return <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível abrir o grafo.</LoadError>;

  const radius = (n) => 4 + 10 * Math.sqrt(n.appearances / maxSeen);

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
      <section aria-label="Grafo da wiki" className="min-w-0 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm text-ink-soft">
            {plural(nodes.length, 'entidade', 'entidades')} e {plural(edges.length, 'vínculo', 'vínculos')}
            {alone > 0 && <span className="text-ink-faint"> · {alone} sem vínculo neste filtro</span>}
          </p>
          <label className="flex items-center gap-2 text-sm text-ink-soft">
            Vínculos com ao menos
            <select value={weight} onChange={(e) => setWeight(Number(e.target.value))} className="min-h-10 rounded-lg border border-border-hairline bg-white px-2 text-sm text-ink">
              {[1, 2, 3, 5, 8].map((w) => (
                <option key={w} value={w}>{plural(w, 'registro', 'registros')}</option>
              ))}
            </select>
          </label>
        </div>
        {nodes.length === 0 ? (
          <p className="py-10 text-center text-sm text-ink-faint">Nenhum vínculo neste filtro até aqui.</p>
        ) : (
          <svg viewBox={`0 0 ${W} ${H}`} role="group" aria-label="Entidades e vínculos" className="mt-3 h-auto w-full touch-pan-y bg-[var(--library-paper)]">
            {edges.map((e) => {
              const a = places.get(e.a);
              const b = places.get(e.b);
              if (!a || !b) return null;
              const lit = chosen && (e.a === chosen || e.b === chosen);
              return (
                <line
                  key={`${e.a}|${e.b}`}
                  x1={a.x * W} y1={a.y * H} x2={b.x * W} y2={b.y * H}
                  stroke={lit ? 'var(--color-brand)' : 'var(--color-border-hairline)'}
                  strokeOpacity={chosen && !lit ? 0.35 : 1}
                  strokeWidth={0.6 + 3 * (e.weight / maxWeight)}
                />
              );
            })}
            {nodes.map((n) => {
              const p = places.get(n.slug);
              if (!p) return null;
              const dim = chosen && n.slug !== chosen && !near.has(n.slug);
              const r = radius(n);
              return (
                <g
                  key={n.slug}
                  role="button"
                  tabIndex={0}
                  aria-pressed={n.slug === chosen}
                  aria-label={`${n.name} (${kindName(n.kind)})`}
                  onClick={() => setChosen(n.slug === chosen ? null : n.slug)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      setChosen(n.slug === chosen ? null : n.slug);
                    }
                  }}
                  opacity={dim ? 0.3 : 1}
                  className="cursor-pointer outline-none [&:focus-visible>circle]:stroke-ink"
                >
                  <title>{n.name}</title>
                  <circle cx={p.x * W} cy={p.y * H} r={r} fill={KIND_COLORS[n.kind] ?? KIND_COLORS.other} stroke={n.slug === chosen ? 'var(--color-ink)' : 'white'} strokeWidth={n.slug === chosen ? 3 : 1.5} />
                  {labeled.has(n.slug) && (
                    <text x={p.x * W} y={p.y * H + r + 13} textAnchor="middle" className="pointer-events-none fill-ink text-[13px]" paintOrder="stroke" stroke="var(--library-paper)" strokeWidth={4}>
                      {n.name}
                    </text>
                  )}
                </g>
              );
            })}
          </svg>
        )}
        <ul aria-label="Legenda" className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-ink-soft">
          {KINDS.filter((k) => nodes.some((n) => n.kind === k.key)).map((k) => (
            <li key={k.key} className="flex items-center gap-1.5">
              <span aria-hidden="true" className="inline-block h-2.5 w-2.5 rounded-full" style={{ background: KIND_COLORS[k.key] }} />
              {k.many}
            </li>
          ))}
        </ul>
      </section>

      <aside aria-label="Entidade escolhida" className="rounded-xl border border-border-hairline bg-white p-5 shadow-sm lg:self-start">
        {!current ? (
          <p className="text-sm text-ink-faint">Escolha uma entidade no grafo para ver os vínculos dela.</p>
        ) : (
          <>
            <p className="library-eyebrow">{kindName(current.kind)}</p>
            <h2 className="mt-1 font-display text-2xl leading-tight text-ink">{current.name}</h2>
            <button type="button" onClick={() => onOpen(current.slug)} className="library-button mt-3">Abrir a página</button>
            <p className="library-eyebrow mt-5 mb-2">{plural(links.length, 'vínculo', 'vínculos')}</p>
            <ul className="flex max-h-[60vh] flex-col gap-3 overflow-y-auto pr-1">
              {links.map((l) => (
                <li key={l.other.slug} className="text-sm">
                  <button type="button" onClick={() => setChosen(l.other.slug)} className="font-semibold text-brand hover:underline">{l.other.name}</button>
                  <span className="ml-2 font-mono text-[11px] text-ink-faint">{plural(l.weight, 'registro', 'registros')}</span>
                  {l.labels?.length > 0 && (
                    <ul className="mt-1 flex flex-col gap-0.5 text-[13px] text-ink-soft">
                      {l.labels.slice(0, 3).map((x, i) => (
                        <li key={i}>{x.label}</li>
                      ))}
                    </ul>
                  )}
                </li>
              ))}
            </ul>
          </>
        )}
      </aside>
    </div>
  );
}
