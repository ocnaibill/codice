export function EmptyState({ children }) {
  return (
    <div className="flex w-full items-center justify-center rounded-lg border border-dashed border-surface-alt bg-surface/50 px-4 py-8 text-center font-body text-[13px] text-ink-faint">
      {children}
    </div>
  );
}

export function Skeleton({ className = '' }) {
  return <div className={`animate-pulse rounded bg-surface-alt ${className}`} />;
}
