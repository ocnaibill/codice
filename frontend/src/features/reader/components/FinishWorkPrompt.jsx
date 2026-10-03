import React, { useEffect } from 'react';
import { languageName } from '../files';

const label = ({ file, edition }) => {
  const percent = Math.round(file.percentComplete || 0);
  return `${(file.format || '').toUpperCase()}${edition.language ? ` (${languageName(edition.language)})` : ''}${percent > 0 ? `, ${percent}%` : ''}`;
};

/**
 * Asked when a version was just finished and another is still in progress (DEC-080). "Yes" marks
 * the whole work as finished, which takes every version out of Continue Reading without saying the
 * other was read to the end. "No" leaves the other version where it is.
 */
export function FinishWorkPrompt({ finished, others, busy, onFinish, onKeep }) {
  // Escape is the careful answer: the other version is left where it is, and nothing is marked.
  useEffect(() => {
    const onKey = (event) => {
      if (event.key === 'Escape') onKeep();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onKeep]);

  return (
    <div className="fixed inset-0 z-[70] flex animate-fade-in items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true" aria-label="Obra finalizada?">
      <div className="w-full max-w-md animate-pop-in rounded-2xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Você terminou esta versão</h2>
        <p className="mt-2 text-sm text-ink-soft">{finished}</p>
        <p className="mt-3 text-sm text-ink-soft">
          Você ainda está em {others.map(label).join(' e ')}. Quer marcar a obra toda como finalizada? Ela sai de "Continuar
          lendo", e a outra versão não é dada como lida.
        </p>
        <div className="mt-5 flex flex-col gap-2 sm:flex-row-reverse">
          <button
            onClick={onFinish}
            disabled={busy}
            className="min-h-11 rounded-lg bg-brand px-4 text-sm font-semibold text-white transition-[filter] hover:brightness-110 disabled:opacity-40"
          >
            Sim, a obra está finalizada
          </button>
          <button
            onClick={onKeep}
            disabled={busy}
            className="min-h-11 rounded-lg bg-surface-alt px-4 text-sm font-medium text-ink transition-[filter] hover:brightness-95 disabled:opacity-40"
          >
            Não, continuar a outra versão depois
          </button>
        </div>
      </div>
    </div>
  );
}
