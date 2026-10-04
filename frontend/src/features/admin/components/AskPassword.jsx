import { useEffect, useRef, useState } from 'react';
import { isTopmostDialog } from '../../../lib/topDialog';

/**
 * Asks the owner for the password of the account again, before something that a session alone must not do
 * (DEC-123: making a copy of the whole library, or rehearsing a restore). The password goes with the request
 * and is not kept; a wrong one is said in the dialog, which stays open.
 */
export function AskPassword({ title, message, confirmLabel, busy = false, error = '', onConfirm, onCancel }) {
  const [password, setPassword] = useState('');
  const dialogRef = useRef(null);

  useEffect(() => {
    const onKey = (event) => event.key === 'Escape' && isTopmostDialog(dialogRef.current) && onCancel();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onCancel]);

  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onCancel()}
    >
      <form
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onSubmit={(event) => {
          event.preventDefault();
          if (password && !busy) onConfirm(password);
        }}
        className="w-full max-w-md animate-pop-in rounded-xl bg-white p-6 shadow-2xl"
      >
        <h2 className="font-display text-xl font-semibold text-ink">{title}</h2>
        <div className="mt-3 text-[14px] leading-relaxed text-ink-soft">{message}</div>
        <label className="mt-4 flex flex-col gap-1 text-[12px] text-ink-soft">
          Sua senha, para confirmar
          <input
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            autoFocus
            autoComplete="current-password"
            aria-label="Sua senha"
            className="rounded bg-surface px-3 py-2 text-[14px] text-ink outline-none"
          />
        </label>
        {error && <p role="alert" className="mt-2 text-[13px] text-danger">{error}</p>}
        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <button type="button" onClick={onCancel} className="rounded px-4 py-2 text-[13px] text-ink-soft hover:text-ink">Cancelar</button>
          <button
            type="submit"
            disabled={!password || busy}
            className="rounded bg-brand px-4 py-2 text-[13px] font-medium text-white hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {busy ? 'Enviando…' : confirmLabel}
          </button>
        </div>
      </form>
    </div>
  );
}
