import React from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { isActive, useMetadataRefreshStatus, useRefreshMetadata } from '../api/useMetadataRefresh';
import { reasonOf } from '../api/useVersions';

const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;

/** What the last search came to, in words. */
export function outcomeText(job) {
  if (job.state === 'failed') return `A busca falhou${job.error ? ` (${job.error})` : ''}. Tente de novo.`;
  const result = job.result || {};
  if (!result.found) return 'Nenhum provedor ligado reconheceu esta obra.';
  const found = `Achou “${result.title}” (${result.source})`;
  if (!result.new) return `${found}, mas não tem nada novo: o que traz já está na obra, já espera decisão ou foi recusado antes.`;
  return `${found}: ${count(result.new, 'sugestão nova', 'sugestões novas')} abaixo.`;
}

/**
 * The button that searches the providers again for a work (owner and admin; DEC-143). It asks with what the work says now (title, author, series and
 * the ISBN of its edition, which someone may have corrected), and what comes back is listed as suggestions below, to keep what is there, add
 * what is missing or change it. Nothing changes until someone accepts.
 */
export function RefreshMetadata({ workId }) {
  const queryClient = useQueryClient();
  const status = useMetadataRefreshStatus(workId);
  const refresh = useRefreshMetadata(workId);
  const job = status.data;
  const [watching, setWatching] = React.useState(false); // this page asked for a search, or found one under way: what it came to is told
  const active = isActive(job) || refresh.isPending;
  const state = job?.state;

  // The suggestions are asked for again when the search that was running ends.
  const wasActive = React.useRef(false);
  React.useEffect(() => {
    if (isActive(job)) setWatching(true);
    if (wasActive.current && job && !isActive(job)) {
      queryClient.invalidateQueries({ queryKey: ['candidates', workId] });
      queryClient.invalidateQueries({ queryKey: ['admin'] });
    }
    wasActive.current = isActive(job);
  }, [job, state, queryClient, workId]);

  const providersOff = refresh.data?.providersOff;
  let message = null;
  if (refresh.isError) message = reasonOf(refresh.error, 'Não foi possível pedir a busca.');
  else if (providersOff) message = 'Nenhum provedor está ligado. O owner liga em Administração → Provedores.';
  else if (active) message = 'Buscando nos provedores ligados… as sugestões aparecem abaixo.';
  else if (job && watching) message = outcomeText(job);

  return (
    <section className="mb-4 flex flex-col gap-2" aria-label="Buscar metadados de novo">
      <div className="flex flex-wrap items-center gap-3">
        <button
          type="button"
          disabled={active}
          onClick={() => {
            setWatching(true);
            refresh.mutate();
          }}
          className="min-h-10 rounded-lg border border-border-hairline bg-surface px-4 py-2 text-sm text-ink hover:bg-surface-alt disabled:opacity-40"
        >
          {active ? 'Buscando…' : 'Buscar metadados de novo'}
        </button>
        <p className="text-xs text-ink-faint">
          Pergunta de novo aos provedores ligados, com o que a obra diz hoje (título, autor, série, ISBN).
        </p>
      </div>
      {message && (
        <p role={refresh.isError || state === 'failed' ? 'alert' : 'status'} className={`text-sm ${refresh.isError || state === 'failed' ? 'text-danger' : 'text-ink-soft'}`}>
          {message}
        </p>
      )}
    </section>
  );
}
