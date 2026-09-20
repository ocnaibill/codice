import { useState } from 'react';
import { useJobs, useRerunJob, useCancelJob, describeError } from '../api/admin';
import { formatDate } from '../format';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';

const STATES = [
  ['', 'Todos'],
  ['pending', 'Na fila'],
  ['running', 'Em andamento'],
  ['failed', 'Com falha'],
  ['succeeded', 'Concluídos'],
  ['cancelled', 'Cancelados'],
];
const STATE_LABEL = Object.fromEntries(STATES);
const TYPE_LABEL = {
  ingest: 'Leitura do arquivo',
  organize: 'Organização em disco',
  scan: 'Varredura de pasta',
  transfer: 'Mover para o acervo',
  dedupe: 'Busca de duplicatas',
  extract_text: 'Leitura do texto para busca',
};

export function JobsTab() {
  const [state, setState] = useState('');
  const { data, isLoading, isError } = useJobs({ state });
  const rerun = useRerunJob();
  const cancel = useCancelJob();
  const jobs = data?.data || [];
  const counts = data?.counts || {};

  return (
    <Section title="Trabalhos" hint="O que o sistema está fazendo em segundo plano. Os que falharam podem ser tentados de novo.">
      <div className="mb-4 flex flex-wrap gap-2">
        {STATES.map(([value, label]) => (
          <button
            key={value}
            onClick={() => setState(value)}
            aria-pressed={state === value}
            className={`rounded-full px-3 py-1 text-[12px] ${
              state === value ? 'bg-brand text-white' : 'bg-surface-alt text-ink-soft hover:brightness-95'
            }`}
          >
            {label}
            {value && counts[value] > 0 && <span className="ml-1 opacity-80">({counts[value]})</span>}
          </button>
        ))}
      </div>

      {isLoading && <Loading />}
      {isError && <ErrorNote>Não foi possível carregar os trabalhos.</ErrorNote>}
      {!isLoading && !isError && jobs.length === 0 && <Empty>Nenhum trabalho neste filtro.</Empty>}

      {jobs.length > 0 && (
        <ul className="divide-y divide-border-hairline">
          {jobs.map((job) => (
            <li key={job.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
              <div className="min-w-0">
                <p className="text-[14px] text-ink">
                  {TYPE_LABEL[job.type] || job.type}
                  {job.workTitle && <span className="text-ink-soft"> · {job.workTitle}</span>}
                </p>
                <p className="text-[12px] text-ink-faint">
                  {STATE_LABEL[job.state] || job.state} · tentativa {job.attempts}/{job.maxAttempts} · {formatDate(job.createdAt)}
                </p>
                {job.lastError && <p className="mt-1 text-[12px] text-red-700">{job.lastError}</p>}
              </div>
              <div className="flex gap-2">
                {(job.state === 'failed' || job.state === 'cancelled') && (
                  <Btn onClick={() => rerun.mutate(job.id)} disabled={rerun.isPending}>Tentar de novo</Btn>
                )}
                {(job.state === 'pending' || job.state === 'running') && !job.cancelRequested && (
                  <Btn onClick={() => cancel.mutate(job.id)} disabled={cancel.isPending}>Cancelar</Btn>
                )}
                {job.cancelRequested && <span className="text-[12px] text-ink-faint">Cancelando…</span>}
              </div>
            </li>
          ))}
        </ul>
      )}
      <ErrorNote>{rerun.isError ? describeError(rerun.error) : cancel.isError && describeError(cancel.error)}</ErrorNote>
    </Section>
  );
}
