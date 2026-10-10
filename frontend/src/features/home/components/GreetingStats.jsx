import { useEffect, useState } from 'react';
import { Skeleton } from '../../../components/ui/EmptyState';
import { LibraryIcon } from '../../../components/ui/LibraryIcon';
import { formatCount, formatBreakdown, formatReadingTime } from '../utils/format';

function greetingPeriod(hour) {
  if (hour < 12) return 'Bom dia';
  if (hour < 18) return 'Boa tarde';
  return 'Boa noite';
}

/** The time as the person's computer tells it, with the time zone ("10:43 BRT" or "10:43 GMT-3"). */
export function timeLabel(date) {
  return date.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit', timeZoneName: 'short' });
}

/** The current moment, asked again every half minute, so the time on the page is the time. */
function useNow(everyMs = 30_000) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), everyMs);
    return () => clearInterval(id);
  }, [everyMs]);
  return now;
}

/**
 * The top of the home: who is greeted and when, and three cards beside it: how many works are kept, how many are being read, and what
 * was done this month (works finished, and the time spent reading, in all).
 */
export function GreetingStats({ userName, stats, isLoading, error, onRetry }) {
  const now = useNow();
  const dateLabel = now.toLocaleDateString('pt-BR', { day: '2-digit', month: 'long', year: 'numeric' });
  const waiting = isLoading || !stats;
  return (
    <section className="home-top" aria-labelledby="library-heading">
      <div className="home-greeting">
        <p className="library-eyebrow">
          {dateLabel}. <time dateTime={now.toISOString()}>{timeLabel(now)}</time>
        </p>
        <h1 id="library-heading">
          {greetingPeriod(now.getHours())}
          {userName && (
            <>
              , <span>{userName}</span> :)
            </>
          )}
        </h1>
        <p className="home-greeting-question">O que queremos hoje?</p>
      </div>
      {error ? (
        <div role="alert" className="library-error home-metrics-error">
          Não foi possível carregar as estatísticas.
          <button className="library-button" onClick={onRetry}>
            Tentar novamente
          </button>
        </div>
      ) : waiting ? (
        <div className="home-metrics" aria-label="Carregando estatísticas">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-[132px] w-full" />
          ))}
        </div>
      ) : (
        <dl className="home-metrics">
          <div className="library-stat">
            <LibraryIcon name="library" />
            <dt>Obras preservadas</dt>
            <dd>{formatCount(stats.worksTotal)}</dd>
            <dd className="library-stat-detail">{stats.catalogedPercent ?? 0}% catalogadas</dd>
          </div>
          <div className="library-stat">
            <LibraryIcon name="bookmark" />
            <dt>Em andamento</dt>
            <dd>{formatCount(stats.inProgressCount)}</dd>
            <dd className="library-stat-detail">{formatBreakdown(stats.inProgressBreakdown) || 'Uma nova leitura espera'}</dd>
          </div>
          <div className="library-stat home-activity">
            <LibraryIcon name="check" />
            <dt>Sua atividade</dt>
            <dd className="home-activity-pair">
              <span>
                <strong>{formatCount(stats.completedThisMonth)}</strong>
                <small>obras completas neste mês</small>
              </span>
              <span>
                <strong>{formatReadingTime(stats.totalReadingSeconds)}</strong>
                <small>de leitura, no total</small>
              </span>
            </dd>
            <dd className="library-stat-detail">{formatBreakdown(stats.completedBreakdown) || 'Cada página conta'}</dd>
          </div>
        </dl>
      )}
    </section>
  );
}
