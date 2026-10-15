import React from 'react';
import { KIND_COLORS, kindName, neighbourhood } from '../wiki';

const W = 300;
const H = 220;

/** A name short enough for under a node of the minigraph. */
const short = (name) => (name.length > 16 ? `${name.split(' ')[0].slice(0, 15)}…` : name);

/**
 * The entity and its strongest links up to the cursor (DEC-182, DEC-183), around it in a circle; a node opens its page, and
 * "Expandir grafo" opens the whole graph. Nothing is drawn of what lies beyond the cursor: the server does not send it.
 */
export function MiniGraph({ graph, slug, onOpen, onExpand }) {
  const near = neighbourhood(graph, slug);
  const placed = (near?.links ?? []).map((link, i, all) => {
    const angle = (i / all.length) * Math.PI * 2 - Math.PI / 2;
    return { ...link, x: W / 2 + Math.cos(angle) * 105, y: H / 2 + Math.sin(angle) * 72 };
  });
  const max = placed[0]?.weight || 1;
  return (
    <section aria-labelledby="wiki-minigraph" className="rounded-xl border border-border-hairline bg-white p-5 shadow-sm">
      <h2 id="wiki-minigraph" className="font-display text-xl text-ink">Vínculos no grafo</h2>
      {!near ? (
        <p className="mt-2 text-sm text-ink-faint">{graph ? 'Sem vínculos no grafo até aqui.' : 'Carregando o grafo…'}</p>
      ) : near.links.length === 0 ? (
        <p className="mt-2 text-sm text-ink-faint">Sem vínculos no grafo até aqui.</p>
      ) : (
        <svg viewBox={`0 0 ${W} ${H}`} role="group" aria-label={`${near.center.name} e seus vínculos mais fortes`} className="mt-3 w-full">
          {placed.map((link) => (
            <line key={`l-${link.other.slug}`} x1={W / 2} y1={H / 2} x2={link.x} y2={link.y} stroke="var(--color-border-hairline)" strokeWidth={1 + 2.5 * (link.weight / max)} />
          ))}
          <circle cx={W / 2} cy={H / 2} r={20} fill={KIND_COLORS[near.center.kind] ?? KIND_COLORS.other} />
          <text x={W / 2} y={H / 2 + 34} textAnchor="middle" className="fill-ink font-display text-[12px] font-semibold">{short(near.center.name)}</text>
          {placed.map((link) => {
            const why = link.labels?.map((l) => l.label).filter(Boolean).slice(0, 3).join('; ');
            return (
              <g
                key={link.other.slug}
                role="button"
                tabIndex={0}
                aria-label={`${link.other.name} (${kindName(link.other.kind)})${why ? `: ${why}` : ''}`}
                onClick={() => onOpen(link.other.slug)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    onOpen(link.other.slug);
                  }
                }}
                className="cursor-pointer outline-none [&:focus-visible>circle]:stroke-ink"
              >
                <title>{`${link.other.name}${why ? ` — ${why}` : ''}`}</title>
                <circle cx={link.x} cy={link.y} r={11} fill={KIND_COLORS[link.other.kind] ?? KIND_COLORS.other} stroke="white" strokeWidth={2} />
                <text x={link.x} y={link.y + 24} textAnchor="middle" className="fill-ink-soft text-[10px]">{short(link.other.name)}</text>
              </g>
            );
          })}
        </svg>
      )}
      <button type="button" onClick={onExpand} className="library-button mt-3 w-full justify-center">
        Expandir grafo
      </button>
    </section>
  );
}
