import { useState } from 'react';
import { useJobs, useRerunJob, useRerunFailedJobs, useCancelJob, describeError } from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { formatDate } from '../format';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';

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
  backup: 'Backup',
  verify_backup: 'Verificação de backup',
  extract_text: 'Leitura do texto para busca',
  ocr: 'Leitura de páginas escaneadas (OCR)',
};

export function JobsTab() {
  const [state, setState] = useState('');
  const { data, isLoading, isError, error, refetch, isRefetching } = useJobs({ state });
  const rerun = useRerunJob();
  const rerunAll = useRerunFailedJobs();
  const [asking, setAsking] = useState(false);
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
            {value && counts[value] > 0 && <span className="ml-1">({counts[value]})</span>}
          </button>
        ))}
      </div>

      {state === 'failed' && counts.failed > 0 && (
        <div className="mb-4 flex flex-wrap items-center gap-3">
          <Btn onClick={() => setAsking(true)} disabled={rerunAll.isPending}>Tentar todos de novo ({counts.failed})</Btn>
          <span className="text-[12px] text-ink-faint">Depois de uma falha geral (um serviço fora do ar, uma pasta que não estava montada).</span>
        </div>
      )}
      {rerunAll.isSuccess && (
        <p role="status" className="mb-4 text-[13px] text-ink-soft">
          {rerunAll.data.requeued === 0
            ? 'Nenhum trabalho voltou para a fila.'
            : `${rerunAll.data.requeued} ${rerunAll.data.requeued === 1 ? 'trabalho voltou' : 'trabalhos voltaram'} para a fila.`}
          {rerunAll.data.left > 0 && ` ${rerunAll.data.left} ${rerunAll.data.left === 1 ? 'continua' : 'continuam'} com falha: ${rerunAll.data.left === 1 ? 'é um trabalho' : 'são trabalhos'} sem obra, ou de uma obra que já tem o mesmo trabalho na fila, ou uma falha mais antiga do mesmo tipo. Tente-${rerunAll.data.left === 1 ? 'o' : 'os'} um a um.`}
        </p>
      )}

      {isLoading && <Loading />}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar os trabalhos.</LoadError>}
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
                {job.lastError && <p className="mt-1 text-[12px] text-danger">{job.lastError}</p>}
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
      <ErrorNote>{rerun.isError ? describeError(rerun.error) : rerunAll.isError ? describeError(rerunAll.error) : cancel.isError && describeError(cancel.error)}</ErrorNote>

      {asking && (
        <ConfirmDialog
          title="Tentar de novo todos os trabalhos com falha?"
          message={<p>Cada trabalho com falha, de uma obra, volta para a fila com tentativas novas. Os que falharem outra vez ficam aqui, com o motivo.</p>}
          choices={[{ label: 'Tentar todos de novo', value: true }]}
          onChoose={() => { setAsking(false); rerunAll.mutate(); }}
          onCancel={() => setAsking(false)}
        />
      )}
    </Section>
  );
}
