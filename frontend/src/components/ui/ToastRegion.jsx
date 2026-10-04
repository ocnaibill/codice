import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useToasts } from './toast';

// How long the exit takes before the notice leaves the page: the length of the toast-out animation, not an event of
// it, so that nothing depends on an animation running (with reduced motion there is none).
export const EXIT_MS = 170;

const TONES = {
  success: { box: 'border-success/30', icon: 'bg-success-soft text-success', path: 'M5 13l4 4L19 7' },
  info: { box: 'border-border-hairline', icon: 'bg-info-soft text-brand', path: 'M12 8v.01M12 11v5' },
  warning: { box: 'border-warning/30', icon: 'bg-warning-soft text-warning', path: 'M12 8v5M12 16v.01' },
  error: { box: 'border-danger/30', icon: 'bg-danger-soft text-danger', path: 'M7 7l10 10M17 7L7 17' },
};

function Toast({ item }) {
  const dismiss = useToasts((state) => state.dismiss);
  const [leaving, setLeaving] = useState(false);
  const paused = useRef(false);
  const remaining = useRef(item.duration);
  const startedAt = useRef(0);
  const timer = useRef(null);
  const tone = TONES[item.tone] ?? TONES.info;

  const leave = useCallback(() => {
    setLeaving(true);
    setTimeout(() => dismiss(item.id), EXIT_MS);
  }, [dismiss, item.id]);

  const start = useCallback(() => {
    clearTimeout(timer.current);
    startedAt.current = Date.now();
    timer.current = setTimeout(leave, remaining.current);
  }, [leave]);
  // The time is held while the pointer is on the notice or focus is in it (a notice that goes while it is being read
  // or aimed at is a rude one), and goes on from where it was.
  const hold = () => {
    if (paused.current) return;
    paused.current = true;
    clearTimeout(timer.current);
    remaining.current = Math.max(1000, remaining.current - (Date.now() - startedAt.current));
  };
  const release = () => {
    if (!paused.current) return;
    paused.current = false;
    start();
  };

  useEffect(() => {
    start();
    return () => clearTimeout(timer.current);
  }, [start]);

  return (
    <div
      role={item.tone === 'error' ? 'alert' : 'status'}
      onMouseEnter={hold}
      onMouseLeave={release}
      onFocus={hold}
      onBlur={release}
      className={`pointer-events-auto flex w-full items-start gap-3 rounded-xl border bg-white px-3.5 py-3 text-ink shadow-lg ${tone.box} ${leaving ? 'animate-toast-out' : 'animate-rise-in'}`}
    >
      <span className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full ${tone.icon}`} aria-hidden="true">
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
          <path d={tone.path} />
        </svg>
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-[13px] font-semibold leading-snug">{item.title}</p>
        {item.message && <p className="mt-0.5 break-words text-[12px] leading-snug text-ink-soft">{item.message}</p>}
        {item.action && (
          <button
            onClick={() => {
              item.action.onClick();
              leave();
            }}
            className="mt-1.5 min-h-8 rounded-md text-[12px] font-semibold text-brand underline-offset-2 hover:underline"
          >
            {item.action.label}
          </button>
        )}
      </div>
      <button
        onClick={leave}
        aria-label="Fechar aviso"
        className="-mr-1 flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-ink-faint transition-colors hover:bg-surface-alt hover:text-ink"
      >
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden="true">
          <path d="M6 6l12 12M18 6L6 18" />
        </svg>
      </button>
    </div>
  );
}

/**
 * Where the notices appear: at the bottom, above the bar of the phone, and at the bottom right on a larger screen.
 * One per `key` and at most three; each leaves by itself, stays while it is being read, and can be closed.
 */
export function ToastRegion() {
  const items = useToasts((state) => state.items);
  return (
    <div
      role="region"
      aria-label="Avisos"
      className="pointer-events-none fixed inset-x-3 bottom-[calc(env(safe-area-inset-bottom)+5rem)] z-[70] flex flex-col items-center gap-2 sm:inset-x-auto sm:bottom-6 sm:right-6 sm:w-96 sm:items-stretch lg:bottom-6"
    >
      {items.map((item) => (
        <Toast key={item.id} item={item} />
      ))}
    </div>
  );
}
