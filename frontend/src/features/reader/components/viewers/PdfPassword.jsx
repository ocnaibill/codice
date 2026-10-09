import React, { useRef, useState } from 'react';
import { useDialog } from '../../../../lib/useDialog';

/**
 * Asked when a PDF wants a password to be opened (#89): in the place of the box that the browser would show, which asks again at each
 * wrong answer and cannot say it was wrong. The password is kept only in the field, and goes to the reader of the PDF: it is not sent
 * to the server nor stored. Escape and "Cancelar" give up, and the book is closed.
 */
export function PdfPassword({ wrong, onSubmit, onCancel }) {
  const dialogRef = useRef(null);
  const fieldRef = useRef(null);
  const [value, setValue] = useState('');
  useDialog(dialogRef, { onEscape: onCancel, initialFocus: fieldRef });

  const submit = (event) => {
    event.preventDefault();
    if (value) onSubmit(value);
  };

  return (
    <div ref={dialogRef} className="fixed inset-0 z-[70] flex animate-fade-in items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true" aria-label="Este PDF tem senha">
      <form onSubmit={submit} className="w-full max-w-md animate-pop-in rounded-2xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Este PDF tem senha</h2>
        <p className="mt-2 text-sm text-ink-soft">Digite a senha para abrir. Ela não é guardada: vale só para esta leitura.</p>
        <label className="mt-4 flex flex-col gap-1 text-sm font-medium text-ink-soft">
          Senha do PDF
          <input
            ref={fieldRef}
            type="password"
            autoComplete="off"
            value={value}
            onChange={(event) => setValue(event.target.value)}
            aria-invalid={wrong ? 'true' : undefined}
            aria-describedby={wrong ? 'pdf-password-wrong' : undefined}
            className="min-h-11 rounded-lg border border-border-hairline bg-surface px-3 text-base text-ink"
          />
        </label>
        {wrong && (
          <p id="pdf-password-wrong" role="alert" className="mt-2 text-sm text-danger">
            Senha incorreta. Tente de novo.
          </p>
        )}
        <div className="mt-5 flex flex-col gap-2 sm:flex-row-reverse">
          <button type="submit" disabled={!value} className="min-h-11 rounded-lg bg-brand px-4 text-sm font-semibold text-white transition-[filter] hover:brightness-110 disabled:opacity-40">
            Abrir
          </button>
          <button type="button" onClick={onCancel} className="min-h-11 rounded-lg bg-surface-alt px-4 text-sm font-medium text-ink transition-[filter] hover:brightness-95">
            Cancelar
          </button>
        </div>
      </form>
    </div>
  );
}
