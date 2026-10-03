import { useEffect, useMemo, useRef, useState } from 'react';
import { REFERENCED_PAGE, describeError, useCleanups, useMoveToManaged, useReferenced, useRoots, useScanActivity, useTransfers } from '../api/admin';
import { formatBytes } from '../format';
import { STATE_HINT, STATE_LABEL, canMove, explainCleanupReason, explainTransferError, transferBadge } from '../storageText';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';

// The most files one request moves (the server refuses more).
export const MAX_SELECTED = 500;

const STATES = [['', 'Todos'], ['ok', 'No disco'], ['missing', 'Ausentes'], ['conflict', 'Mudaram']];

const OUTCOME = {
  moved: { label: 'movido', tone: 'text-success' },
  moved_original_kept: { label: 'movido; o original ficou', tone: 'text-warning' },
  failed: { label: 'não foi movido', tone: 'text-danger' },
  cancelled: { label: 'cancelado', tone: 'text-ink-faint' },
  queued: { label: 'na fila', tone: 'text-ink-soft' },
  running: { label: 'movendo…', tone: 'text-ink-soft' },
  not_queued: { label: 'não entrou na fila', tone: 'text-danger' },
};

function useDebounced(value, ms) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), ms);
    return () => clearTimeout(timer);
  }, [value, ms]);
  return debounced;
}

/** What happened to each file asked for: the ones that did not enter the queue, and the ones that did, until each ends. */
function Results({ asked, onClose }) {
  const queued = asked.filter((a) => a.jobId);
  const transfers = useTransfers(queued.map((a) => a.jobId));
  const byJob = new Map((transfers.data?.data || []).map((t) => [t.jobId, t]));
  const rows = asked.map((a) => {
    if (!a.jobId) return { key: `f${a.fileId}`, title: a.title, outcome: 'not_queued', error: a.error };
    const t = byJob.get(a.jobId);
    return t
      ? { key: `j${a.jobId}`, title: t.title || a.title, format: t.format, outcome: t.outcome, error: t.error, origin: t.origin, reason: t.reason }
      : { key: `j${a.jobId}`, title: a.title, outcome: 'queued' };
  });
  const count = (...outcomes) => rows.filter((r) => outcomes.includes(r.outcome)).length;
  const waiting = count('queued', 'running');
  const kept = rows.filter((r) => r.outcome === 'moved_original_kept');

  return (
    <div role="region" aria-label="Resultado da transferência" className="mt-4 rounded-lg border border-border-hairline bg-surface p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-[14px] font-medium text-ink" role="status">
          {waiting > 0
            ? `Movendo… ${rows.length - waiting} de ${rows.length} terminados.`
            : `Terminou: ${count('moved', 'moved_original_kept')} movido(s), ${count('failed', 'not_queued', 'cancelled')} sem mover.`}
        </p>
        <Btn onClick={onClose} disabled={waiting > 0}>Fechar</Btn>
      </div>
      {transfers.isError && <ErrorNote>Não foi possível acompanhar a transferência. O que já foi pedido continua na fila (veja em Trabalhos).</ErrorNote>}
      {kept.length > 0 && (
        <p className="mt-2 text-[13px] text-warning">
          {kept.length === 1 ? 'Um original não pôde ser apagado' : `${kept.length} originais não puderam ser apagados`}: a cópia gerenciada está correta, e eles
          aguardam em “Originais que não puderam ser apagados”, mais abaixo.
        </p>
      )}
      <ul className="mt-3 divide-y divide-border-hairline text-[13px]">
        {rows.map((r) => (
          <li key={r.key} className="py-2">
            <p className="flex flex-wrap items-baseline justify-between gap-2">
              <span className="font-medium text-ink">{r.title}{r.format ? ` · ${r.format.toUpperCase()}` : ''}</span>
              <span className={OUTCOME[r.outcome]?.tone}>{OUTCOME[r.outcome]?.label}</span>
            </p>
            {(r.outcome === 'failed' || r.outcome === 'not_queued') && <p className="text-[12px] text-ink-soft">{explainTransferError(r.error)}</p>}
            {r.outcome === 'moved_original_kept' && (
              <p className="text-[12px] text-ink-soft">
                Original em <code>{r.origin}</code>: {explainCleanupReason(r.reason)}
              </p>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * The files the library only points at (#15, RF-011): they live in a directory the owner authorised and stay there
 * until an administrator moves them into the managed storage. The list says where each is, what it is, and whether
 * it is still there; chosen files are moved after a confirmation that says what happens to the original, and the
 * outcome of each is shown as it ends.
 */
export function ReferencedFiles({ maxSelected = MAX_SELECTED }) {
  const roots = useRoots().data?.roots || [];
  const cleanups = useCleanups().data?.data || [];
  const [rootId, setRootId] = useState('');
  const [state, setState] = useState('');
  const [text, setText] = useState('');
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState(() => new Map()); // fileId -> { title, sizeBytes }
  const [confirming, setConfirming] = useState(false);
  const [asked, setAsked] = useState(null);
  const q = useDebounced(text, 300);

  // A different filter starts from the first page and from nothing chosen: what is chosen is what is in view.
  useEffect(() => {
    setPage(1);
    setSelected(new Map());
  }, [rootId, state, q]);

  // A scan ends after it was asked for: while one is going on the list asks again by itself, and once more when it ends.
  const scanning = useScanActivity().data === true;
  const { data, isLoading, isError, refetch, isFetching } = useReferenced({ rootId, state, q, page }, { live: scanning });
  const wasScanning = useRef(false);
  useEffect(() => {
    if (wasScanning.current && !scanning) refetch();
    wasScanning.current = scanning;
  }, [scanning, refetch]);
  const move = useMoveToManaged();
  const files = useMemo(() => data?.data ?? [], [data]);
  const total = data?.total ?? 0;
  const summary = data?.summary ?? { ok: 0, missing: 0, conflict: 0 };
  const pages = Math.max(1, Math.ceil(total / REFERENCED_PAGE));
  const movable = files.filter(canMove);
  const allOnPage = movable.length > 0 && movable.every((f) => selected.has(f.fileId));
  const chosenBytes = [...selected.values()].reduce((sum, f) => sum + (f.sizeBytes || 0), 0);

  const toggle = (file) =>
    setSelected((prev) => {
      const next = new Map(prev);
      if (next.has(file.fileId)) next.delete(file.fileId);
      else if (next.size < maxSelected) next.set(file.fileId, { title: file.title, sizeBytes: file.sizeBytes });
      return next;
    });
  const togglePage = () =>
    setSelected((prev) => {
      const next = new Map(prev);
      if (allOnPage) movable.forEach((f) => next.delete(f.fileId));
      else movable.forEach((f) => next.size < maxSelected && next.set(f.fileId, { title: f.title, sizeBytes: f.sizeBytes }));
      return next;
    });

  const confirmMove = () => {
    const chosen = [...selected.entries()];
    setConfirming(false);
    move.mutate(chosen.map(([id]) => id), {
      onSuccess: (answer) => {
        const titles = new Map(chosen.map(([id, f]) => [id, f.title]));
        setAsked((answer?.data || []).map((a) => ({ ...a, title: titles.get(a.fileId) || `Arquivo ${a.fileId}` })));
        setSelected(new Map());
      },
    });
  };

  const first = total === 0 ? 0 : (page - 1) * REFERENCED_PAGE + 1;
  const last = Math.min(page * REFERENCED_PAGE, total);
  const filtered = !!(rootId || state || q.trim());

  return (
    <Section
      title="Arquivos referenciados"
      hint="Arquivos que o acervo só aponta: ficam na pasta de origem. Mover para o gerenciado copia o arquivo para o armazenamento do Códice, confere a cópia e só então remove o original da pasta."
      actions={<Btn onClick={() => refetch()} disabled={isFetching}>Atualizar</Btn>}
    >
      {scanning && <p role="status" className="mb-3 text-[13px] text-ink-soft">Varredura em andamento: a lista se atualiza sozinha.</p>}
      <div className="flex flex-wrap items-end gap-3">
        <label className="flex flex-col gap-1 text-[12px] text-ink-soft">
          Pasta
          <select value={rootId} onChange={(e) => setRootId(e.target.value)} className="rounded bg-surface px-3 py-2 text-[13px] text-ink outline-none">
            <option value="">Todas</option>
            {roots.map((r) => <option key={r.id} value={r.id}>{r.path}</option>)}
          </select>
        </label>
        <label className="flex min-w-[200px] flex-1 flex-col gap-1 text-[12px] text-ink-soft">
          Buscar
          <input
            type="search"
            value={text}
            onChange={(e) => setText(e.target.value)}
            maxLength={200}
            placeholder="título ou caminho"
            aria-label="Buscar arquivos referenciados"
            className="rounded bg-surface px-3 py-2 text-[13px] text-ink outline-none"
          />
        </label>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-2" role="group" aria-label="Filtrar por situação">
        {STATES.map(([value, label]) => (
          <button
            key={value || 'all'}
            onClick={() => setState(value)}
            aria-pressed={state === value}
            className={`min-h-9 rounded-full px-3 py-1.5 text-[12px] font-medium ${state === value ? 'bg-brand text-white' : 'bg-surface-alt text-ink-soft hover:text-ink'}`}
          >
            {label}
          </button>
        ))}
      </div>
      <p className="mt-3 text-[12px] text-ink-soft">
        {summary.ok} no disco · {summary.missing} ausentes do disco · {summary.conflict} mudaram depois de catalogados
        {rootId ? ' (nesta pasta)' : ''}
      </p>

      {isLoading && <Loading />}
      {isError && <ErrorNote>Não foi possível carregar os arquivos referenciados.</ErrorNote>}
      {!isLoading && !isError && files.length === 0 && (
        <Empty>
          {filtered
            ? 'Nenhum arquivo referenciado com estes filtros.'
            : roots.length === 0
              ? 'Nenhum arquivo referenciado: autorize uma pasta e varra-a para catalogar os livros dela sem copiá-los.'
              : 'Nenhum arquivo referenciado: o que foi catalogado das pastas autorizadas já está no armazenamento gerenciado, ou ainda não foi varrido.'}
        </Empty>
      )}

      {files.length > 0 && (
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-border-hairline pt-3">
          <label className="flex items-center gap-2 text-[13px] text-ink">
            <input type="checkbox" checked={allOnPage} onChange={togglePage} disabled={movable.length === 0} aria-label="Escolher os arquivos desta página" />
            Escolher os desta página que podem ser movidos ({movable.length})
          </label>
          <p className="text-[12px] text-ink-soft">
            {selected.size === 0 ? 'Nenhum escolhido' : `${selected.size} escolhido(s), ${formatBytes(chosenBytes)}`}
            {selected.size >= maxSelected ? ` (o máximo de uma vez é ${maxSelected})` : ''}
          </p>
        </div>
      )}
      <ul className="divide-y divide-border-hairline">
        {files.map((file) => {
          const badge = transferBadge(file.transfer);
          const can = canMove(file);
          return (
            <li key={file.fileId} className="flex items-start gap-3 py-3">
              <input
                type="checkbox"
                checked={selected.has(file.fileId)}
                disabled={!can || (!selected.has(file.fileId) && selected.size >= maxSelected)}
                onChange={() => toggle(file)}
                aria-label={`Escolher ${file.title}`}
                className="mt-1"
              />
              <div className="min-w-0 flex-1">
                <p className="flex flex-wrap items-baseline gap-x-2 text-[14px] text-ink">
                  <span className="font-medium">{file.title}</span>
                  {file.author && <span className="text-[12px] text-ink-faint">{file.author}</span>}
                </p>
                <p className="break-all font-mono text-[11px] text-ink-faint">{file.root}/{file.path}</p>
                <p className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-ink-soft">
                  {file.format && <span className="rounded bg-surface-alt px-1.5 py-0.5 font-mono uppercase">{file.format}</span>}
                  <span>{file.sizeBytes != null ? formatBytes(file.sizeBytes) : 'tamanho desconhecido'}</span>
                  <span className={file.state === 'ok' ? 'text-success' : 'text-danger'}>{STATE_LABEL[file.state] || file.state}</span>
                </p>
                {STATE_HINT[file.state] && <p className="text-[12px] text-ink-faint">{STATE_HINT[file.state]}</p>}
                {badge && <p className={`text-[12px] ${badge.tone === 'error' ? 'text-danger' : 'text-ink-soft'}`}>{badge.text}</p>}
              </div>
            </li>
          );
        })}
      </ul>

      {total > REFERENCED_PAGE && (
        <nav className="mt-3 flex items-center justify-between gap-3 text-[12px] text-ink-soft" aria-label="Páginas dos arquivos referenciados">
          <Btn onClick={() => setPage((p) => p - 1)} disabled={page <= 1}>Anterior</Btn>
          <span>{first}–{last} de {total}</span>
          <Btn onClick={() => setPage((p) => p + 1)} disabled={page >= pages}>Próxima</Btn>
        </nav>
      )}

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <Btn tone="primary" onClick={() => setConfirming(true)} disabled={selected.size === 0 || move.isPending}>
          Mover para o gerenciado…
        </Btn>
        {cleanups.length > 0 && <span className="text-[12px] text-ink-faint">{cleanups.length} original(is) aguardando para serem apagados (veja mais abaixo).</span>}
      </div>
      <ErrorNote>{move.isError && describeError(move.error)}</ErrorNote>
      {asked && <Results asked={asked} onClose={() => setAsked(null)} />}

      {confirming && (
        <ConfirmDialog
          title={`Mover ${selected.size === 1 ? '1 arquivo' : `${selected.size} arquivos`} para o armazenamento gerenciado?`}
          message={
            <div className="space-y-2">
              <p>Cada arquivo é copiado para o armazenamento do Códice ({formatBytes(chosenBytes)} no total), a cópia é conferida byte a byte e só então ela passa a ser a do acervo.</p>
              <p>
                <strong>Depois disso o original é apagado da pasta de origem</strong>, se continuar sendo exatamente o que foi copiado. Se não puder ser apagado, ele fica numa lista de limpeza pendente, com o motivo.
              </p>
              <p>Nada que seja diferente é sobrescrito, e um arquivo que não puder ser movido continua onde está.</p>
            </div>
          }
          choices={[{ label: 'Mover', value: true, tone: 'primary' }]}
          onChoose={confirmMove}
          onCancel={() => setConfirming(false)}
        />
      )}
    </Section>
  );
}
