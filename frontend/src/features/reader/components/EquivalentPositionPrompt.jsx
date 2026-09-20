import React, { useState } from 'react';
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

  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm" role="dialog" aria-label="Continuar de onde parou?">
      <div className="w-full max-w-md rounded-xl border border-zinc-800 bg-zinc-900 p-6 shadow-2xl">
        <h2 className="text-lg font-semibold text-zinc-100">Continuar de onde parou na outra versão?</h2>
        <p className="mt-2 text-sm text-zinc-300">{whereYouAre(from)}</p>
        {sourceExcerpt && <blockquote className="mt-2 border-l-2 border-zinc-700 pl-3 text-xs italic text-zinc-500">{sourceExcerpt}</blockquote>}

        {ambiguous ? (
          <div className="mt-4 flex flex-col gap-2">
            <p className="text-xs text-zinc-400">Encontramos mais de um lugar parecido; escolha um, ou abra a sua posição própria.</p>
            {candidates.map((c, i) => (
              <label key={i} className="flex cursor-pointer items-start gap-2 rounded-md border border-zinc-800 bg-zinc-950 p-3 text-sm has-[:checked]:border-blue-600">
                <input type="radio" name="candidate" className="mt-1" checked={chosen === i} onChange={() => setChosen(i)} />
                <span>
                  <span className="block text-xs text-zinc-500">{candidateLabel(c)}{c.section ? ` · ${c.section}` : ''}</span>
                  <span className="text-zinc-300">{c.excerpt}</span>
                </span>
              </label>
            ))}
          </div>
        ) : (
          candidates[0] && (
            <div className="mt-4 rounded-md border border-zinc-800 bg-zinc-950 p-3 text-sm">
              <p className="text-xs text-zinc-500">{candidateLabel(candidates[0])}{candidates[0].section ? ` · ${candidates[0].section}` : ''}</p>
              <p className="text-zinc-300">{candidates[0].excerpt}</p>
            </div>
          )
        )}
        <p className="mt-2 text-xs text-zinc-500">É uma posição aproximada, ligada aqui uma única vez: cada versão continua com o seu próprio progresso.</p>

        <div className="mt-5 flex flex-col gap-2 sm:flex-row-reverse">
          <button
            onClick={() => onAccept(candidates[ambiguous ? chosen : 0])}
            disabled={busy}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-40"
          >
            Continuar daqui
          </button>
          <button
            onClick={onDecline}
            disabled={busy}
            className="rounded-md border border-zinc-700 bg-zinc-900 px-4 py-2 text-sm text-zinc-300 hover:bg-zinc-800 disabled:opacity-40"
          >
            Não, abrir minha posição
          </button>
        </div>
      </div>
    </div>
  );
}
