import React from 'react';
import { useCandidates, useDecideCandidate } from '../api/useCandidates';
import { reasonOf } from '../api/useVersions';
import { keyLabel } from '../../../lib/authority';
import { languageName } from '../files';
import { localizeRoles, suggestionText } from '../suggestionText';
import { FIELD_LABELS, FIELD_ORDER } from '../../../lib/suggestionFields';

function Suggestion({ candidate, onDecide, busy }) {
  let current = candidate.current;
  if (candidate.field === 'contributors') current = localizeRoles(current || '');
  if (candidate.field === 'language') current = languageName(current) || current;
  const keys = candidate.keys || [];
  return (
    <li className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0 space-y-1">
        <p className="font-mono text-[11px] uppercase tracking-wide text-ink-faint">
          {FIELD_LABELS[candidate.field] || candidate.field} · {candidate.source}
        </p>
        <p className={`break-words text-sm text-ink ${candidate.field === 'description' ? 'max-h-40 overflow-y-auto whitespace-pre-line' : ''}`}>
          {suggestionText(candidate)}
        </p>
        {current ? (
          <p className={`break-words text-xs text-ink-faint ${candidate.field === 'description' ? 'line-clamp-3' : ''}`}>Hoje: {current}</p>
        ) : (
          <p className="text-xs text-ink-faint">Hoje: em branco</p>
        )}
        {keys.length > 0 && (
          <p className="text-xs text-ink-soft">
            Ao aceitar, guarda a chave de {keys.map((k) => `${k.name}: ${keyLabel(`${k.scheme}:${k.value}`)}`).join('; ')}.
          </p>
        )}
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">
        <button
          disabled={busy}
          onClick={() => onDecide(candidate.id, 'accept')}
          className="min-h-10 rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40"
        >
          Aceitar
        </button>
        <button
          disabled={busy}
          onClick={() => onDecide(candidate.id, 'reject')}
          className="min-h-10 rounded-lg border border-border-hairline bg-surface px-4 py-2 text-xs text-ink hover:bg-surface-alt disabled:opacity-40"
        >
          Recusar
        </button>
      </div>
    </li>
  );
}

/**
 * What the providers suggested for this work (owner and admin; #70). Nothing changes until someone accepts:
 * accepting a field applies it as a confirmed value and locks it, rejecting remembers it so it does not come
 * back. With `emptyText` the list says so when there is nothing to decide; without it, it is not there.
 */
export function WorkSuggestions({ workId, emptyText }) {
  const { data } = useCandidates(workId);
  const decide = useDecideCandidate(workId);
  const [error, setError] = React.useState(null);
  const candidates = [...(data || [])].sort(
    (a, b) => (FIELD_ORDER.indexOf(a.field) + 1 || 99) - (FIELD_ORDER.indexOf(b.field) + 1 || 99) || a.id - b.id
  );
  if (candidates.length === 0) return emptyText ? <p className="text-sm text-ink-faint">{emptyText}</p> : null;

  const onDecide = (id, verb) => {
    setError(null);
    decide.mutate(
      { id, verb },
      { onError: (e) => setError(reasonOf(e, verb === 'accept' ? 'Não foi possível aceitar a sugestão.' : 'Não foi possível recusar a sugestão.')) }
    );
  };

  return (
    <section className="flex flex-col gap-3" aria-label="Sugestões dos provedores">
      <p className="text-sm text-ink-soft">
        Nada muda até você aceitar. Aceitar um campo o trava contra a extração automática; etiquetas e pessoas só são acrescentadas.
      </p>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <ul className="flex flex-col gap-3">
        {candidates.map((candidate) => (
          <Suggestion key={candidate.id} candidate={candidate} onDecide={onDecide} busy={decide.isPending} />
        ))}
      </ul>
    </section>
  );
}
