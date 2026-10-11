import React, { useEffect, useRef, useState } from 'react';
import { authenticatedUrl } from '../../../lib/api';
import { useDialog } from '../../../lib/useDialog';
import { useAskCoverSearch, useCoverSearch } from '../api/useWorkCover';

const STATES = {
  off: 'Nenhuma fonte externa está ligada. Quem é dono do acervo as liga em Administração › Provedores.',
  failed: 'Não foi possível buscar as capas desta obra.',
};

/**
 * The covers the providers that are on have for a work (DEC-174), to choose from. The picture is not put on the work: the one chosen is
 * handed over as a file, and the page cuts it to 2:3 like one from the person's own computer (DEC-173).
 *
 * `onChoose(file)` is called with the picture chosen; `onClose` closes without choosing.
 */
export function CoverSearchDialog({ work, onChoose, onClose }) {
  const dialogRef = useRef(null);
  const queryRef = useRef(null);
  useDialog(dialogRef, { onEscape: onClose, initialFocus: queryRef });

  const ask = useAskCoverSearch(work.id);
  // Read only once the request is made: an answer read before it would be the one of an earlier search, whose pictures may be gone.
  const search = useCoverSearch(work.id, ask.isSuccess);
  const [query, setQuery] = useState('');
  const [problem, setProblem] = useState('');
  const [opening, setOpening] = useState('');
  const asked = useRef(false);

  // Opening the window is asking: what the work says (title, author, ISBN) is what is searched, until the person types something else.
  useEffect(() => {
    if (asked.current) return;
    asked.current = true;
    ask.mutate('', { onError: () => setProblem('Não foi possível pedir a busca.') });
  }, [ask]);

  const submit = (event) => {
    event.preventDefault();
    setProblem('');
    ask.mutate(query.trim(), { onError: () => setProblem('Não foi possível pedir a busca.') });
  };

  const result = search.data;
  const waiting = ask.isPending || !result || result.state === 'pending';

  const choose = async (candidate) => {
    setProblem('');
    setOpening(candidate.file);
    try {
      const response = await fetch(authenticatedUrl(`/works/${work.id}/cover/candidates/${candidate.file}`));
      if (!response.ok) throw new Error(String(response.status));
      const blob = await response.blob();
      onChoose(new File([blob], candidate.file, { type: blob.type || 'image/jpeg' }));
    } catch {
      setProblem('Essa capa não está mais disponível. Busque de novo.');
      setOpening('');
    }
  };

  return (
    <div
      className="fixed inset-0 z-[60] flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-label="Buscar capas" className="max-h-full w-full max-w-2xl animate-pop-in overflow-y-auto rounded-xl bg-white p-5 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Buscar capas</h2>
        <p className="mt-2 text-[13px] leading-relaxed text-ink-soft">
          Capas que as fontes ligadas têm para esta obra. Escolha uma para recortar: nada muda até você confirmar o recorte.
        </p>

        <form onSubmit={submit} className="mt-4 flex flex-wrap items-end gap-2">
          <label className="flex min-w-[200px] flex-1 flex-col gap-1 text-xs font-medium text-ink-soft">
            Buscar por
            <input
              ref={queryRef}
              value={query}
              maxLength={200}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={work.title || 'Título da obra'}
              className="w-full rounded-lg border border-border-hairline bg-white px-3 py-2 text-sm text-ink outline-none focus:border-brand"
            />
          </label>
          <button type="submit" disabled={waiting} className="min-h-10 rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40">
            Buscar
          </button>
        </form>

        {waiting && <p role="status" className="mt-4 text-sm text-ink-soft">Buscando nas fontes ligadas…</p>}
        {!waiting && STATES[result.state] && <p role="status" className="mt-4 text-sm text-ink">{STATES[result.state]}</p>}
        {!waiting && result.state === 'done' && result.results.length === 0 && (
          <p role="status" className="mt-4 text-sm text-ink">Nenhuma capa encontrada. Tente buscar por outro texto.</p>
        )}
        {problem && <p role="alert" className="mt-4 text-sm text-danger">{problem}</p>}

        {!waiting && result.state === 'done' && result.results.length > 0 && (
          <ul className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
            {result.results.map((candidate) => (
              <li key={candidate.file}>
                <button
                  type="button"
                  onClick={() => choose(candidate)}
                  disabled={!!opening}
                  className="flex w-full flex-col gap-2 rounded-lg border border-border-hairline bg-white p-2 text-left hover:border-brand disabled:opacity-60"
                >
                  <span className="block w-full overflow-hidden rounded bg-surface-alt" style={{ aspectRatio: '2 / 3' }}>
                    <img
                      src={authenticatedUrl(`/works/${work.id}/cover/candidates/${candidate.file}`)}
                      alt=""
                      loading="lazy"
                      className="h-full w-full object-contain"
                    />
                  </span>
                  <span className="min-w-0 text-xs text-ink">
                    <span className="block truncate font-medium">{candidate.title || 'Sem título'}</span>
                    <span className="block truncate text-ink-soft">{[candidate.author, candidate.source].filter(Boolean).join(' · ')}</span>
                  </span>
                  <span className="text-xs font-medium text-brand">{opening === candidate.file ? 'Abrindo…' : 'Usar esta capa'}</span>
                </button>
              </li>
            ))}
          </ul>
        )}

        <div className="mt-6 flex justify-end">
          <button type="button" onClick={onClose} className="min-h-10 rounded px-4 py-2 text-[13px] text-ink-soft hover:text-ink">
            Fechar
          </button>
        </div>
      </div>
    </div>
  );
}
