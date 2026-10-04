import React, { useRef, useState } from 'react';
import { useDialog } from '../../../lib/useDialog';
import { candidateLabel, whereYouAre } from '../files';

/**
 * Offers to open this file where the person's other, in-progress version of the book is (RF-042,
 * DEC-030): a one-time jump, never a sync. Every candidate points at content really found in this
 * file; with one it opens on "yes", with more than one the person picks. Declining, or there
 * being nothing to offer, changes nothing: the file's own saved position is what opens.
 */
export function EquivalentPositionPrompt({ from, sourceExcerpt, status, candidates, busy, onAccept, onDecline }) {
  const [chosen, setChosen] = useState(0);
  const ambiguous = status === 'ambiguous';

  // Escape is declining: the file opens where its own saved position is. The focus starts there too.
  const dialogRef = useRef(null);
  const declineRef = useRef(null);
  useDialog(dialogRef, { onEscape: onDecline, initialFocus: declineRef });

  return (
    <div ref={dialogRef} className="fixed inset-0 z-[70] flex animate-fade-in items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true" aria-label="Continuar de onde parou?">
      <div className="max-h-[90dvh] w-full max-w-md animate-pop-in overflow-y-auto rounded-2xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Continuar de onde parou na outra versão?</h2>
        <p className="mt-2 text-sm text-ink-soft">{whereYouAre(from)}</p>
        {sourceExcerpt && <blockquote className="mt-2 border-l-2 border-brand pl-3 font-display text-sm italic text-ink-soft">{sourceExcerpt}</blockquote>}

        {ambiguous ? (
          <div className="mt-4 flex flex-col gap-2">
            <p className="text-xs text-ink-soft">Encontramos mais de um lugar parecido; escolha um, ou abra a sua posição própria.</p>
            {candidates.map((c, i) => (
              <label key={i} className="flex min-h-11 cursor-pointer items-start gap-2 rounded-xl border border-border-hairline bg-surface p-3 text-sm has-[:checked]:border-brand has-[:checked]:bg-info-soft/40">
                <input type="radio" name="candidate" className="mt-1 accent-brand" checked={chosen === i} onChange={() => setChosen(i)} />
                <span>
                  <span className="block text-xs text-ink-faint">{candidateLabel(c)}{c.section ? ` · ${c.section}` : ''}</span>
                  <span className="text-ink">{c.excerpt}</span>
                </span>
              </label>
            ))}
          </div>
        ) : (
          candidates[0] && (
            <div className="mt-4 rounded-xl border border-border-hairline bg-surface p-3 text-sm">
              <p className="text-xs text-ink-faint">{candidateLabel(candidates[0])}{candidates[0].section ? ` · ${candidates[0].section}` : ''}</p>
              <p className="text-ink">{candidates[0].excerpt}</p>
            </div>
          )
        )}
        <p className="mt-2 text-xs text-ink-faint">É uma posição aproximada, ligada aqui uma única vez: cada versão continua com o seu próprio progresso.</p>

        <div className="mt-5 flex flex-col gap-2 sm:flex-row-reverse">
          <button
            onClick={() => onAccept(candidates[ambiguous ? chosen : 0])}
            disabled={busy}
            className="min-h-11 rounded-lg bg-brand px-4 text-sm font-semibold text-white transition-[filter] hover:brightness-110 disabled:opacity-40"
          >
            Continuar daqui
          </button>
          <button
            ref={declineRef}
            onClick={onDecline}
            disabled={busy}
            className="min-h-11 rounded-lg bg-surface-alt px-4 text-sm font-medium text-ink transition-[filter] hover:brightness-95 disabled:opacity-40"
          >
            Não, abrir minha posição
          </button>
        </div>
      </div>
    </div>
  );
}
