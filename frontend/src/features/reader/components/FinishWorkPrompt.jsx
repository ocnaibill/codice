import React from 'react';
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
  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm" role="dialog" aria-label="Obra finalizada?">
      <div className="w-full max-w-md rounded-xl border border-zinc-800 bg-zinc-900 p-6 shadow-2xl">
        <h2 className="text-lg font-semibold text-zinc-100">Você terminou esta versão</h2>
        <p className="mt-2 text-sm text-zinc-300">{finished}</p>
        <p className="mt-3 text-sm text-zinc-400">
          Você ainda está em {others.map(label).join(' e ')}. Quer marcar a obra toda como finalizada? Ela sai de "Continuar
          lendo", e a outra versão não é dada como lida.
        </p>
        <div className="mt-5 flex flex-col gap-2 sm:flex-row-reverse">
          <button
            onClick={onFinish}
            disabled={busy}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-40"
          >
            Sim, a obra está finalizada
          </button>
          <button
            onClick={onKeep}
            disabled={busy}
            className="rounded-md border border-zinc-700 bg-zinc-900 px-4 py-2 text-sm text-zinc-300 hover:bg-zinc-800 disabled:opacity-40"
          >
            Não, continuar a outra versão depois
          </button>
        </div>
      </div>
    </div>
  );
}
