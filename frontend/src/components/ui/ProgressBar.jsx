export function ProgressBar({ percent, color = 'brand' }) {
  const barColor = color === 'success' ? 'bg-success' : 'bg-brand';
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-surface-alt">
      <div className={`h-full rounded-full ${barColor}`} style={{ width: `${percent}%` }} />
    </div>
  );
}
