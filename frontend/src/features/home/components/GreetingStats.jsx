import { Skeleton } from '../../../components/ui/EmptyState';
import { LibraryIcon } from '../../../components/ui/LibraryIcon';
import {
  formatCount,
  formatBreakdown,
  formatReadingTime,
} from '../utils/format';

function greetingPeriod(hour) {
  if (hour < 12) return 'Bom dia';
  if (hour < 18) return 'Boa tarde';
  return 'Boa noite';
}

export function GreetingStats({ userName, stats, isLoading, error, onRetry }) {
  const now = new Date();
  const dateLabel = now.toLocaleDateString('pt-BR', {
    day: '2-digit',
    month: 'long',
    year: 'numeric',
  });
  const metrics = [
    {
      label: 'Obras preservadas',
      value: formatCount(stats?.worksTotal),
      detail: `${stats?.catalogedPercent ?? 0}% catalogadas`,
      icon: 'library',
    },
    {
      label: 'Em andamento',
      value: formatCount(stats?.inProgressCount),
      detail:
        formatBreakdown(stats?.inProgressBreakdown) ||
        'Uma nova leitura espera',
      icon: 'bookmark',
    },
    {
      label: 'Concluídas no mês',
      value: formatCount(stats?.completedThisMonth),
      detail: formatBreakdown(stats?.completedBreakdown) || 'Cada página conta',
      icon: 'check',
    },
    {
      label: 'Tempo de leitura',
      value: formatReadingTime(stats?.totalReadingSeconds),
      detail: 'Seu tempo acumulado',
      icon: 'clock',
    },
  ];
  return (
    <section className="library-intro" aria-labelledby="library-heading">
      <div>
        <p className="library-eyebrow">{dateLabel}</p>
        <h1 id="library-heading">Biblioteca & repositório</h1>
        <p className="library-intro-subtitle">
          {greetingPeriod(now.getHours())}
          {userName && (
            <>
              , <span>{userName} :)</span>
            </>
          )}{' '}
          — qual será a próxima leitura?
        </p>
      </div>
      {error ? (
        <div role="alert" className="library-error">
          Não foi possível carregar as estatísticas.
          <button className="library-button" onClick={onRetry}>
            Tentar novamente
          </button>
        </div>
      ) : isLoading || !stats ? (
        <div className="library-stat-grid" aria-label="Carregando estatísticas">
          {metrics.map((metric) => (
            <Skeleton key={metric.label} className="h-[132px] w-full" />
          ))}
        </div>
      ) : (
        <dl className="library-stat-grid">
          {metrics.map((metric) => (
            <div key={metric.label} className="library-stat">
              <LibraryIcon name={metric.icon} />
              <dt>{metric.label}</dt>
              <dd>{metric.value}</dd>
              <p>{metric.detail}</p>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}
