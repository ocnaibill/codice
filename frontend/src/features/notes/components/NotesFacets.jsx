import React, { useState } from 'react';

const KINDS = [
  ['note', 'Notas'],
  ['highlight', 'Destaques'],
  ['bookmark', 'Marcadores'],
];

// How many tags show before "mostrar todas": a person's tags are few, but a screen of them is not a filter.
export const TAGS_SHOWN = 12;

const row = (on) =>
  `flex min-h-9 items-center justify-between gap-2 rounded-lg px-3 py-1.5 text-left text-xs font-medium transition-colors ${on ? 'bg-brand text-white' : 'bg-surface-alt text-ink-soft hover:bg-border-hairline hover:text-ink'}`;
const count = (on) => `font-mono text-[11px] ${on ? 'text-white/80' : 'text-ink-faint'}`;

/**
 * The panel that narrows the notes (UI-05): by kind, with how many each has, and by the tags in use, with how many
 * notes carry each. A click on the one that is on takes it away. Counts follow the other filters, so a choice that
 * would leave nothing shows 0 and the person does not have to try it.
 */
export function NotesFacets({ facets, kind, onKind, tag, onTag }) {
  const [all, setAll] = useState(false);
  const kinds = facets?.kinds ?? {};
  const total = KINDS.reduce((sum, [key]) => sum + (kinds[key] ?? 0), 0);
  const tags = facets?.tags ?? [];
  const shown = all ? tags : tags.slice(0, TAGS_SHOWN);
  // The tag chosen stays in view even if it is not among the first ones.
  const chosen = tag && !shown.some((t) => t.tag.toLowerCase() === tag.toLowerCase()) ? tags.find((t) => t.tag.toLowerCase() === tag.toLowerCase()) : null;
  const visible = chosen ? [chosen, ...shown] : shown;

  return (
    <div className="flex flex-col gap-5">
      <div>
        <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.18em] text-ink-soft">Tipo</p>
        <div className="flex flex-wrap gap-2 lg:flex-col lg:gap-1.5" role="group" aria-label="Filtrar por tipo">
          <button onClick={() => onKind('')} aria-pressed={kind === ''} className={row(kind === '')}>
            <span>Todas</span>
            <span className={count(kind === '')}>{total}</span>
          </button>
          {KINDS.map(([key, label]) => (
            <button key={key} onClick={() => onKind(kind === key ? '' : key)} aria-pressed={kind === key} className={row(kind === key)}>
              <span>{label}</span>
              <span className={count(kind === key)}>{kinds[key] ?? 0}</span>
            </button>
          ))}
        </div>
      </div>

      {tags.length > 0 && (
        <div>
          <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.18em] text-ink-soft">
            Tags <span className="text-ink-faint">{tags.length}</span>
          </p>
          <div className="flex flex-wrap gap-2 lg:flex-col lg:gap-1.5" role="group" aria-label="Filtrar por tag">
            {visible.map(({ tag: name, count: n }) => {
              const on = tag.toLowerCase() === name.toLowerCase();
              return (
                <button key={name} onClick={() => onTag(on ? '' : name)} aria-pressed={on} className={row(on)}>
                  <span className="truncate">#{name}</span>
                  <span className={count(on)}>{n}</span>
                </button>
              );
            })}
          </div>
          {tags.length > TAGS_SHOWN && (
            <button onClick={() => setAll((v) => !v)} className="mt-2 text-xs text-ink-soft underline hover:text-brand">
              {all ? 'Mostrar menos' : `Mostrar todas as ${tags.length}`}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
