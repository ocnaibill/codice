import React from 'react';

const LOCK = (
  <svg viewBox="0 0 24 24" className="size-3.5" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <rect x="5" y="11" width="14" height="9" rx="2" />
    <path d="M8 11V8a4 4 0 0 1 8 0v3" />
  </svg>
);

/**
 * What is not given to this person, said where it would be: a section they can see and cannot change says so, with who can
 * (#77: "permissão insuficiente"). It is a note and not an error: nothing failed, it is how the account is. Always "o dono do
 * acervo" or "quem administra": never "owner".
 */
export function PermissionNote({ children, className = '' }) {
  return (
    <p role="note" className={`flex items-start gap-2 rounded-xl bg-surface-alt/70 px-3 py-2 text-[13px] leading-snug text-ink-soft ${className}`}>
      <span className="mt-0.5 shrink-0 text-ink-faint">{LOCK}</span>
      <span>{children}</span>
    </p>
  );
}

/**
 * A whole screen the person is not allowed in: it says so and gives a way out. It is not a failure to load (that one asks
 * to try again) but a "no", which asking again does not change.
 */
export function NoPermission({ title = 'Você não tem permissão para ver isto', children = 'Fale com quem cuida do acervo se você deveria ter.', onBack, backLabel = 'Voltar ao acervo' }) {
  return (
    <div role="alert" className="flex min-h-[40dvh] flex-col items-center justify-center gap-3 p-8 text-center">
      <span aria-hidden="true" className="flex size-12 items-center justify-center rounded-full bg-surface-alt text-ink-soft">
        <svg viewBox="0 0 24 24" className="size-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <rect x="5" y="11" width="14" height="9" rx="2" />
          <path d="M8 11V8a4 4 0 0 1 8 0v3" />
        </svg>
      </span>
      <div className="max-w-sm">
        <h2 className="font-display text-xl font-semibold text-ink">{title}</h2>
        <p className="mt-1 text-[14px] leading-snug text-ink-soft">{children}</p>
      </div>
      {onBack && (
        <button onClick={onBack} className="min-h-11 rounded-lg bg-surface-alt px-5 text-sm font-semibold text-ink transition-[filter] hover:brightness-95">
          {backLabel}
        </button>
      )}
    </div>
  );
}
