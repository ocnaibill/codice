import { useEffect, useState } from 'react';

const TONES = {
  neutral: 'bg-surface-alt text-ink hover:brightness-95',
  primary: 'bg-brand text-white hover:brightness-110',
  danger: 'bg-red-700 text-white hover:brightness-110',
};

/**
 * A question that must be answered before anything happens. Every choice is a
 * button of its own, so the person never confirms by pressing "OK" without
 * knowing what it means. Escape and the backdrop cancel.
 *
 * choices: [{ label, value, tone }]; onChoose(value) is called with the one picked.
 */
export function ConfirmDialog({ title, message, choices, onChoose, onCancel, cancelLabel = 'Cancelar', requireText }) {
  const [typed, setTyped] = useState('');
  const locked = requireText !== undefined && typed !== requireText;

  useEffect(() => {
    const onKey = (event) => {
      if (event.key === 'Escape') onCancel();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onCancel]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onCancel()}
    >
      <div role="dialog" aria-modal="true" aria-label={title} className="w-full max-w-md rounded-xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">{title}</h2>
        <div className="mt-3 text-[14px] leading-relaxed text-ink-soft">{message}</div>
        {requireText !== undefined && (
          <label className="mt-4 flex flex-col gap-1 text-[12px] text-ink-soft">
            Para confirmar, digite <strong className="text-ink">{requireText}</strong>
            <input
              value={typed} onChange={(event) => setTyped(event.target.value)} autoFocus autoComplete="off"
              aria-label="Confirmação digitada"
              className="rounded bg-surface px-3 py-2 text-[14px] text-ink outline-none"
            />
          </label>
        )}
        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <button onClick={onCancel} className="rounded px-4 py-2 text-[13px] text-ink-soft hover:text-ink">
            {cancelLabel}
          </button>
          {choices.map((choice) => (
            <button
              key={String(choice.value)}
              onClick={() => onChoose(choice.value)}
              disabled={locked}
              className={`rounded px-4 py-2 text-[13px] font-medium disabled:cursor-not-allowed disabled:opacity-40 ${TONES[choice.tone || 'neutral']}`}
            >
              {choice.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
