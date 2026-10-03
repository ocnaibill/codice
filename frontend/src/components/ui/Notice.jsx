import React from 'react';

// The roles of color a notice can take (#77), the same as the toast's: the soft color for the ground and the strong one for the
// icon and the title, which are the ones that have the contrast (the text is the ink of the app).
const TONES = {
  danger: { box: 'border-danger/30 bg-danger-soft/40', icon: 'bg-danger-soft text-danger', title: 'text-danger', role: 'alert', path: 'M7 7l10 10M17 7L7 17' },
  warning: { box: 'border-warning/30 bg-warning-soft/50', icon: 'bg-warning-soft text-warning', title: 'text-warning', role: 'status', path: 'M12 8v5M12 16v.01' },
  success: { box: 'border-success/30 bg-success-soft/40', icon: 'bg-success-soft text-success', title: 'text-success', role: 'status', path: 'M5 13l4 4L19 7' },
  info: { box: 'border-border-hairline bg-surface-alt/70', icon: 'bg-info-soft text-brand', title: 'text-brand', role: 'status', path: 'M12 8v.01M12 11v5' },
};

/**
 * A notice that stays on the page, in the flow of it (a toast is for what passes): what happened, in a tone, with what to do
 * about it when there is something (`action`, a button). An error is an alert for a screen reader; the others are a status.
 */
export function Notice({ tone = 'info', title, children, action, className = '' }) {
  const t = TONES[tone] ?? TONES.info;
  return (
    <div role={t.role} className={`flex items-start gap-3 rounded-xl border px-3.5 py-3 text-ink ${t.box} ${className}`}>
      <span aria-hidden="true" className={`mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full ${t.icon}`}>
        <svg viewBox="0 0 24 24" className="size-3.5" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
          <path d={t.path} />
        </svg>
      </span>
      <div className="min-w-0 flex-1 text-[13px] leading-snug">
        {title && <p className={`font-semibold ${t.title}`}>{title}</p>}
        {children && <div className={title ? 'mt-0.5 text-ink-soft' : ''}>{children}</div>}
      </div>
      {action && <div className="shrink-0 self-center">{action}</div>}
    </div>
  );
}
