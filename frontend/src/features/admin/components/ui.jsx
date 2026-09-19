// Small building blocks shared by the admin screens.

export function Section({ title, hint, actions, children }) {
  return (
    <section className="rounded-xl bg-white p-5 shadow-[0px_1px_8px_0px_rgba(0,0,0,0.05)]">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-display text-lg font-semibold text-ink">{title}</h2>
          {hint && <p className="mt-1 max-w-2xl text-[13px] text-ink-soft">{hint}</p>}
        </div>
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
      </div>
      {children}
    </section>
  );
}

export function Btn({ tone = 'neutral', className = '', ...props }) {
  const tones = {
    neutral: 'bg-surface-alt text-ink hover:brightness-95',
    primary: 'bg-brand text-white hover:brightness-110',
    danger: 'bg-red-700 text-white hover:brightness-110',
  };
  return (
    <button
      {...props}
      className={`rounded px-3 py-1.5 text-[12px] font-medium disabled:cursor-not-allowed disabled:opacity-50 ${tones[tone]} ${className}`}
    />
  );
}

export function Empty({ children }) {
  return <p className="py-4 text-[13px] text-ink-faint">{children}</p>;
}

export function ErrorNote({ children }) {
  return children ? <p role="alert" className="mt-2 text-[13px] text-red-700">{children}</p> : null;
}

export function Loading() {
  return <p className="py-4 text-[13px] text-ink-faint">Carregando…</p>;
}
