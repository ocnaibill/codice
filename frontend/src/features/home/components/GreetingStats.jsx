import iconCheck from '../../../assets/icons/stat-check.svg';
import iconBook from '../../../assets/icons/stat-book.svg';
import iconMini from '../../../assets/icons/stat-mini-icon.svg';
import iconBookmark from '../../../assets/icons/stat-bookmark.svg';
import { Skeleton } from '../../../components/ui/EmptyState';
import { formatCount, formatBreakdown, formatReadingTime } from '../utils/format';

function greetingPeriod(hour) {
  if (hour < 12) return 'Bom Dia';
  if (hour < 18) return 'Boa Tarde';
  return 'Boa Noite';
}

export function GreetingStats({ userName, stats, isLoading }) {
  const now = new Date();
  const dateLabel = now.toLocaleString('pt-BR', {
    day: '2-digit',
    month: 'long',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZoneName: 'short',
  });

  return (
    <div className="flex flex-col gap-6 lg:flex-row lg:items-center lg:gap-10">
      <div className="shrink-0">
        <p className="font-display text-[10px] text-brand">{dateLabel}</p>
        <p className="font-display text-xl text-ink">
          {greetingPeriod(now.getHours())}{userName && <>, <span className="font-medium text-brand">{userName} :)</span></>}
        </p>
        <p className="font-display text-xl italic text-ink">O que queremos hoje?</p>
      </div>

      {isLoading || !stats ? (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3 lg:flex lg:flex-1">
          <Skeleton className="h-[116px] w-full lg:w-[229px]" />
          <Skeleton className="h-[116px] w-full lg:w-[229px]" />
          <Skeleton className="h-[116px] w-full lg:w-[348px]" />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3 lg:flex lg:flex-1">
          <div className="flex w-full items-start justify-between rounded bg-surface p-4 shadow-[0px_1px_1px_rgba(0,0,0,0.05)] lg:w-[229px]">
            <div>
              <p className="font-body text-[11px] tracking-[0.55px] uppercase text-ink-soft">Obras Preservadas</p>
              <p className="font-display text-[40px] leading-none tracking-[-0.6px] text-ink">{formatCount(stats.worksTotal)}</p>
              <p className="mt-2 flex items-center gap-1 font-body text-[11px] tracking-[0.44px] text-success">
                <img src={iconCheck} alt="" className="size-3" /> {stats.catalogedPercent}% catalogadas
              </p>
            </div>
            <div className="flex size-10 shrink-0 items-center justify-center rounded-sm bg-surface-alt">
              <img src={iconBook} alt="" className="h-[18px] w-[20px]" />
            </div>
          </div>

          <div className="flex w-full items-start justify-between rounded bg-surface p-4 shadow-[0px_1px_1px_rgba(0,0,0,0.05)] lg:w-[229px]">
            <div>
              <p className="font-body text-[11px] tracking-[0.55px] uppercase text-ink-soft">Em Andamento</p>
              <p className="font-display text-[40px] leading-none tracking-[-0.6px] text-brand">{stats.inProgressCount}</p>
              <p className="mt-2 flex items-center gap-1 font-body text-[11px] tracking-[0.44px] text-ink-soft">
                <img src={iconMini} alt="" className="size-[10px]" />
                {formatBreakdown(stats.inProgressBreakdown) || 'Nada em andamento'}
              </p>
            </div>
            <div className="flex size-10 shrink-0 items-center justify-center rounded-sm bg-brand/20">
              <img src={iconBookmark} alt="" className="h-[20px] w-[17px]" />
            </div>
          </div>

          <div className="flex w-full items-center gap-7 rounded bg-surface p-4 shadow-[0px_1px_1px_rgba(0,0,0,0.05)] lg:w-auto">
            <div>
              <p className="font-body text-[11px] tracking-[0.55px] uppercase text-ink-soft">Sua Atividade</p>
              <p className="font-display text-[40px] leading-none tracking-[-0.6px] text-brand">{stats.completedThisMonth}</p>
              <p className="font-body text-[11px] tracking-[0.55px] uppercase text-ink-soft">Obras completas neste mês.</p>
              <p className="mt-2 flex items-center gap-1 font-body text-[11px] tracking-[0.44px] text-ink-soft">
                <img src={iconMini} alt="" className="size-[10px]" />
                {formatBreakdown(stats.completedBreakdown) || 'Nenhuma ainda'}
              </p>
            </div>
            <span className="h-[100px] w-px shrink-0 bg-[rgba(219,193,182,0.6)]" />
            <div>
              <p className="font-display text-[40px] leading-none tracking-[-0.6px] text-brand">{formatReadingTime(stats.totalReadingSeconds)}</p>
              <p className="font-body text-[11px] tracking-[0.55px] uppercase text-ink-soft">de tempo para tudo isso.</p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
