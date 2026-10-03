import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { WorkCover } from '../../../components/ui/WorkCover';
import { reasonOf, useJoinWork, useNotTheSame, useWorkSearch } from '../api/useVersions';

const filesText = (n) => (n === 1 ? '1 arquivo' : `${n ?? 0} arquivos`);

/**
 * "This work is another version of…" (#37, DEC-090): the person looks for the work the files belong
 * under and confirms. Nothing is deleted: the files stay, each with its own position, and the work
 * that is left is retired, so it can be undone by separating the edition. "It is not the same work"
 * is the other answer, and the system will not propose the pair again.
 */
export function JoinVersionsDialog({ work, onClose, onJoined }) {
  const [term, setTerm] = React.useState('');
  const [chosen, setChosen] = React.useState(null);
  const [message, setMessage] = React.useState('');
  const inputRef = React.useRef(null);
  const search = useWorkSearch(term);
  const join = useJoinWork();
  const notSame = useNotTheSame();
  const busy = join.isPending || notSame.isPending;
  const results = (search.data?.data ?? []).filter((w) => w.id !== work.id);

  React.useEffect(() => {
    inputRef.current?.focus();
    const onKeyDown = (event) => {
      if (event.key === 'Escape') {
        event.stopImmediatePropagation(); // the sheet under it must not close too
        onClose();
      }
    };
    window.addEventListener('keydown', onKeyDown, true);
    return () => window.removeEventListener('keydown', onKeyDown, true);
  }, [onClose]);

  const confirmJoin = () =>
    join.mutate(
      { workId: work.id, into: chosen.id },
      {
        onSuccess: () => onJoined(chosen.id),
        onError: (error) => setMessage(reasonOf(error, 'Não foi possível juntar as obras.')),
      }
    );
  const confirmNotSame = () =>
    notSame.mutate(
      { workId: work.id, otherId: chosen.id },
      {
        onSuccess: () => {
          setMessage(`Registrado: “${chosen.title}” não é a mesma obra. O Códice não vai sugerir essa junção de novo.`);
          setChosen(null);
        },
        onError: (error) => setMessage(reasonOf(error, 'Não foi possível registrar.')),
      }
    );

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-ink/60 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label="Juntar com outra obra">
      <div className="flex max-h-[90vh] w-full max-w-lg flex-col overflow-hidden rounded-2xl bg-[#faf8f4] shadow-2xl">
        <div className="flex items-start justify-between gap-3 border-b border-border-hairline px-5 py-4">
          <div className="min-w-0">
            <h2 className="font-display text-2xl text-ink">Esta obra é outra versão de…</h2>
            <p className="mt-1 text-sm text-ink-soft">
              Procure a obra que deve guardar os arquivos de “{work.title}”. Nada é apagado.
            </p>
          </div>
          <button onClick={onClose} className="flex min-h-11 min-w-11 items-center justify-center rounded-lg text-xl text-ink-soft hover:bg-surface-alt hover:text-brand" aria-label="Fechar">
            ✕
          </button>
        </div>

        <div className="min-h-0 space-y-4 overflow-y-auto px-5 py-4">
          {!chosen && (
            <>
              <label className="block text-sm text-ink-soft">
                <span className="font-mono text-[11px] font-semibold uppercase tracking-widest text-ink-faint">Título ou autor</span>
                <input
                  ref={inputRef}
                  value={term}
                  onChange={(e) => setTerm(e.target.value)}
                  className="mt-1 min-h-11 w-full rounded-lg border border-border-hairline bg-white px-3 text-ink focus:border-brand focus:outline-none"
                  placeholder="Por exemplo: Dune"
                />
              </label>
              {search.isFetching && <p className="animate-pulse text-sm text-ink-faint">Procurando…</p>}
              {search.isError && <LoadError onRetry={() => search.refetch()} retrying={search.isRefetching}>Não foi possível procurar.</LoadError>}
              {term.trim().length >= 2 && !search.isFetching && !search.isError && results.length === 0 && (
                <p className="text-sm text-ink-faint">Nenhuma outra obra encontrada.</p>
              )}
              <ul className="flex flex-col gap-2">
                {results.map((w) => (
                  <li key={w.id}>
                    <button
                      onClick={() => { setChosen(w); setMessage(''); }}
                      className="flex w-full items-center gap-3 rounded-xl border border-border-hairline bg-white p-3 text-left hover:border-brand"
                    >
                      <WorkCover item={w} className="h-16 w-11 shrink-0 rounded object-cover" />
                      <span className="min-w-0">
                        <span className="block truncate text-sm font-semibold text-ink">{w.title}</span>
                        <span className="block truncate text-xs text-ink-soft">{w.author}</span>
                        <span className="block font-mono text-[11px] text-ink-faint">
                          {[w.format?.toUpperCase(), filesText(w.fileCount)].filter(Boolean).join(' · ')}
                        </span>
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            </>
          )}

          {chosen && (
            <div className="space-y-4" aria-label="Confirmar">
              <div className="rounded-xl border-l-4 border-brand bg-surface-alt p-4 text-sm text-ink">
                <p>
                  “{work.title}” ({filesText(work.fileCount ?? work.editions?.reduce((n, e) => n + e.files.length, 0))}) passa a ser
                  uma versão de <strong>“{chosen.title}”</strong>, de {chosen.author}.
                </p>
                <ul className="mt-3 list-disc space-y-1 pl-5 text-ink-soft">
                  <li>Todos os arquivos continuam guardados, cada um com a sua posição de leitura e as suas notas.</li>
                  <li>Você escolhe o idioma e o formato na ficha da obra, e a retomada entre versões passa a funcionar.</li>
                  <li>Os dados de “{chosen.title}” (capa, título, edição principal) prevalecem.</li>
                  <li>Dá para separar a edição depois.</li>
                </ul>
              </div>
              <div className="flex flex-col gap-2 sm:flex-row-reverse">
                <button onClick={confirmJoin} disabled={busy} className="min-h-11 rounded-lg bg-brand px-5 py-2 text-sm font-semibold text-white hover:bg-brand-light disabled:opacity-40">
                  Juntar
                </button>
                <button onClick={confirmNotSame} disabled={busy} className="min-h-11 rounded-lg border border-border-hairline bg-white px-4 py-2 text-sm text-ink hover:bg-surface-alt disabled:opacity-40">
                  Não é a mesma obra
                </button>
                <button onClick={() => setChosen(null)} disabled={busy} className="min-h-11 px-4 py-2 text-sm text-ink-soft hover:text-brand disabled:opacity-40">
                  Voltar
                </button>
              </div>
            </div>
          )}

          {message && <p role="status" className="text-sm text-ink-soft">{message}</p>}
        </div>
      </div>
    </div>
  );
}
