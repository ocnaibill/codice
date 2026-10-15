import React from 'react';

/**
 * Said while the reader shows a place opened only to look at it (a reference of the wiki): the reading position has not moved, so the
 * wiki still shows up to where the person was. They may go back to what opened it, or go on reading from here, which moves the
 * position to this place.
 */
export function PeekNotice({ onBack, onReadOn, backLabel = 'Voltar à wiki' }) {
  return (
    <section role="status" aria-label="Consulta de um trecho" className="shrink-0 border-b border-border-hairline bg-info-soft/40 px-4 py-3 sm:px-6">
      <p className="text-sm text-ink">
        <strong className="font-semibold">Você está consultando um trecho.</strong>{' '}
        Sua posição de leitura continua onde estava, e a wiki também.
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        <button type="button" onClick={onBack} className="min-h-10 rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white hover:bg-brand-light">
          {backLabel}
        </button>
        <button type="button" onClick={onReadOn} className="min-h-10 rounded-lg border border-border-hairline bg-white px-4 py-2 text-xs font-medium text-ink hover:bg-surface-alt">
          Continuar lendo daqui
        </button>
      </div>
    </section>
  );
}
