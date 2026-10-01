import React from 'react';

const TITLES = {
  note: 'Não foi possível abrir o ponto da anotação',
  search: 'Não foi possível abrir o trecho encontrado',
  equivalent: 'Não foi possível abrir a posição equivalente',
  saved: 'Sua posição salva não existe mais neste arquivo',
};
const QUOTE_CHARS = 280;

/**
 * Said when the place a person asked for cannot be opened in the file (RF-014): where it pointed, why it cannot be
 * opened, the passage that was kept, and that the note is still safe. The person decides: stay where the reader is
 * now (the saved position) or open from the start. Nothing here deletes anything.
 */
export function PlaceNotice({ notice, onStay, onFromStart }) {
  const { kind, label, reason, quote, also } = notice;
  const fromAsk = kind !== 'saved';
  return (
    <section role="alert" aria-label="Ponto não encontrado" className="shrink-0 border-b border-amber-300 bg-amber-50 px-4 py-3 sm:px-6">
      <p className="text-sm font-semibold text-amber-950">{TITLES[kind] ?? TITLES.note}</p>
      <p className="mt-1 text-sm text-amber-950">
        {label ? <>Ele apontava para <strong>{label}</strong>. </> : null}
        {reason}
        {also ? ` ${also}` : ''}
      </p>
      {quote && (
        <blockquote className="mt-2 border-l-2 border-amber-400 pl-3 font-display text-sm italic text-amber-950">
          {quote.length > QUOTE_CHARS ? `${quote.slice(0, QUOTE_CHARS)}…` : quote}
        </blockquote>
      )}
      {kind === 'note' && <p className="mt-2 text-xs text-amber-900">A anotação continua guardada, com o trecho e o ponto.</p>}
      <div className="mt-3 flex flex-wrap gap-2">
        <button onClick={onStay} className="min-h-10 rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white hover:bg-brand-light">
          {fromAsk ? 'Ficar onde estou' : 'Entendi'}
        </button>
        {fromAsk && (
          <button onClick={onFromStart} className="min-h-10 rounded-lg border border-amber-400 bg-white px-4 py-2 text-xs font-medium text-amber-950 hover:bg-amber-100">
            Abrir do começo
          </button>
        )}
      </div>
    </section>
  );
}
