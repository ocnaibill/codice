import React from 'react';

/**
 * Said when a work of a series was just finished and another follows it (#187): a line under the header that offers the next
 * one. It asks nothing that is hard to undo: "Agora não" only closes it, and the next stays one tap away in the header.
 */
export function NextInSeriesPrompt({ finished, next, onRead, onDismiss }) {
  return (
    <section role="status" aria-label="Próximo da série" className="shrink-0 border-b border-border-hairline bg-brand/5 px-4 py-3 sm:px-6">
      <p className="text-sm text-ink">
        Você terminou <strong>{finished}</strong>. O próximo da série é <strong>{next}</strong>.
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        <button onClick={onRead} className="min-h-10 rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white hover:bg-brand-light">
          Ler {next} agora
        </button>
        <button onClick={onDismiss} className="min-h-10 rounded-lg bg-surface-alt px-4 py-2 text-xs font-medium text-ink hover:bg-border-hairline">
          Agora não
        </button>
      </div>
    </section>
  );
}
