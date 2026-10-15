import React from 'react';
import { kindName, plural } from '../wiki';
import { useWikiReview, useWikiReviewActions } from '../api/useWiki';

/**
 * What the last update changed, for owner and admins (DEC-180, RF-052): a book came into the series and the wiki was written again for
 * it. The pages that are new and what the others gained, each with "Desfazer" (hides what it gained, for everyone), and "Marcar como
 * revisada" to close it. The wiki is already what readers get: the review is after the fact, not a gate.
 */
export function ReviewBanner({ projectId, onOpen }) {
  const { data } = useWikiReview(projectId);
  const { approve, undo } = useWikiReviewActions(projectId);
  const [open, setOpen] = React.useState(false);
  const changes = data?.changes ?? [];
  const fresh = changes.filter((c) => c.new);
  const grew = changes.filter((c) => !c.new);
  return (
    <section aria-label="Revisão da atualização" className="rounded-xl border border-warning/40 bg-warning-soft/60 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-semibold text-warning">Esta wiki foi atualizada com um livro novo e falta revisar.</p>
          <p className="mt-0.5 text-[13px] text-ink-soft">
            {changes.length === 0
              ? 'Nada novo para mostrar: só conferir se ficou bom.'
              : `${plural(fresh.length, 'página nova', 'páginas novas')} e ${plural(grew.length, 'página que ganhou seções', 'páginas que ganharam seções')}. `}
            Os leitores já veem o que foi escrito; aqui você desfaz o que não ficou bom.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {changes.length > 0 && (
            <button type="button" className="library-button" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
              {open ? 'Esconder o que mudou' : 'Ver o que mudou'}
            </button>
          )}
          <button type="button" className="library-button" disabled={approve.isPending} onClick={() => approve.mutate()}>
            Marcar como revisada
          </button>
        </div>
      </div>
      {open && (
        <ul className="mt-3 divide-y divide-border-hairline rounded-lg bg-white" aria-label="O que mudou">
          {changes.map((c) => (
            <li key={c.slug} className="flex flex-wrap items-start justify-between gap-2 px-3 py-2">
              <div className="min-w-0">
                <button type="button" onClick={() => onOpen(c.slug)} className="text-left font-semibold text-brand hover:underline">{c.name}</button>
                <span className="ml-2 font-mono text-[11px] text-ink-faint">{kindName(c.kind)}{c.new ? ' · nova' : ''}</span>
                {c.labels?.length > 0 && <p className="text-[13px] text-ink-soft">Ganhou: {c.labels.join(' · ')}</p>}
                {c.new && c.labels?.length === 0 && <p className="text-[13px] text-ink-soft">Página nova</p>}
              </div>
              {c.undone ? (
                <span className="font-mono text-[11px] text-ink-faint">desfeito</span>
              ) : (
                <button type="button" className="library-button" disabled={undo.isPending} onClick={() => undo.mutate(c.slug)}>
                  Desfazer
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
