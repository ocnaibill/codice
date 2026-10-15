import React from 'react';
import { filterIndex, kindName, plural } from '../wiki';

/**
 * The index of the wiki (DEC-183): every entity that appeared up to the cursor, of the kind chosen above, with a search by name or by
 * another name. Those with a page of their own come first; the others open the page too, with what is known of them (the ficha, the
 * situation, the timeline).
 */
export function WikiIndex({ index, kind, onOpen }) {
  const [text, setText] = React.useState('');
  const shown = filterIndex(index, { kind, text });

  if (index.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-border-hairline bg-surface/50 px-4 py-8 text-center text-sm text-ink-faint">
        Ninguém apareceu ainda até este ponto. Comece a ler, ou escolha ir além no controle acima.
      </p>
    );
  }

  return (
    <section aria-label="Índice da wiki" className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm text-ink-soft sm:max-w-sm">
        Procurar pelo nome
        <input
          type="search"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Nome ou apelido"
          className="min-h-10 rounded-lg border border-border-hairline bg-white px-3 text-sm text-ink placeholder:text-ink-faint"
        />
      </label>
      {shown.length === 0 ? (
        <p className="text-sm text-ink-faint">Nada com esse nome até este ponto.</p>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {shown.map((e) => (
            <li key={e.slug}>
              <button
                type="button"
                onClick={() => onOpen(e.slug)}
                className="flex h-full w-full flex-col gap-2 rounded-xl border border-border-hairline bg-white p-4 text-left shadow-sm transition-colors hover:border-brand/40 hover:bg-[var(--library-paper)]"
              >
                <span className="flex flex-wrap items-center gap-2">
                  <span className="rounded bg-surface-alt px-2 py-0.5 font-mono text-[11px] text-ink-soft">{kindName(e.kind)}</span>
                  {e.page && <span className="font-mono text-[11px] text-brand">página completa</span>}
                </span>
                <span className="font-display text-xl leading-tight text-ink">{e.name}</span>
                {e.aliases?.length > 0 && <span className="line-clamp-2 text-[13px] text-ink-soft">Também: {e.aliases.slice(0, 4).join(' · ')}</span>}
                <span className="mt-auto font-mono text-[11px] text-ink-faint">em {plural(e.appearances, 'capítulo', 'capítulos')}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
