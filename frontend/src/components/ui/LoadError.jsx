import React from 'react';
import { Notice } from './Notice';

/**
 * What a screen shows when what it was loading did not come: the message, and a button to ask again (a failure of a moment
 * is common, and a message with nothing to do about it leaves the person to reload the whole page). `onRetry` is the
 * `refetch` of the query; `retrying` says it is being asked again.
 */
export function LoadError({ children = 'Não foi possível carregar.', onRetry, retrying = false, className = '' }) {
  return (
    <Notice
      tone="danger"
      className={className}
      action={(
        <button
          type="button"
          onClick={onRetry}
          disabled={retrying}
          className="min-h-9 rounded-lg bg-white px-3 text-[12px] font-semibold text-danger shadow-sm transition-[filter] hover:brightness-95 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {retrying ? 'Tentando…' : 'Tentar de novo'}
        </button>
      )}
    >
      {children}
    </Notice>
  );
}
