import React from 'react';
import { describeError } from '../../admin/api/admin';
import { useDecideIdentity, useForgetIdentity, useWikiIdentity } from '../api/useWiki';
import { kindName } from '../wiki';

const FIELD = 'mt-1 w-full rounded-lg border border-border-hairline bg-white px-3 py-2 text-sm text-ink';

const RULE_TEXT = {
  same: (r) => `Juntas: ${r.a} e ${r.b}`,
  distinct: (r) => `Mantidas distintas: ${r.a} e ${r.b}`,
  mention: (r) => `“${r.b}” não é de ${r.a}`,
};

/**
 * Who is who, for owner and admins (DEC-181): the pairs the generator took for the same without proof, with "Confirmar junção" and
 * "Manter distintas"; a way to join this entity with another and to take a mention away from it; and what was decided before. A decision
 * is real in the wiki now and is followed by every generation after (it is a rule of the work or series).
 */
export function IdentityPanel({ projectId, page, index, onOpen }) {
  const decide = useDecideIdentity(projectId);
  const forget = useForgetIdentity(projectId);
  const rules = useWikiIdentity(projectId).data?.rules ?? [];
  const [other, setOther] = React.useState('');
  const [mention, setMention] = React.useState('');
  const [asking, setAsking] = React.useState(null); // the decision waiting for a second click: joining cannot be undone in this wiki
  const suggestions = page.suggestions ?? [];
  const candidates = (index ?? []).filter((e) => e.slug !== page.slug).sort((a, b) => a.name.localeCompare(b.name, 'pt'));
  const run = (decision) => {
    setAsking(null);
    decide.mutate(decision, {
      onSuccess: (done) => {
        setOther('');
        setMention('');
        if (done?.slug && done.slug !== page.slug) onOpen(done.slug);
      },
    });
  };
  const nameOf = (slug) => index?.find((e) => e.slug === slug)?.name ?? slug;

  return (
    <section aria-labelledby="wiki-identity" className="rounded-xl border border-border-hairline bg-white p-5 shadow-sm">
      <h2 id="wiki-identity" className="font-display text-xl text-ink">Identidade</h2>
      <p className="mt-1 text-[13px] text-ink-faint">Só o staff vê isto. O que você decide vale já e nas próximas gerações.</p>

      {suggestions.length > 0 && (
        <ul className="mt-3 flex flex-col gap-3" aria-label="Sugestões de identidade">
          {suggestions.map((s) => (
            <li key={s.id} className="rounded-lg bg-warning-soft/50 p-3 text-sm">
              <p className="font-semibold text-ink">
                {s.kind === 'merge' ? (
                  <>Pode ser a mesma que <button type="button" onClick={() => onOpen(s.other)} className="text-brand hover:underline">{s.otherName || nameOf(s.other)}</button></>
                ) : 'Uma menção pode ser de outra entidade'}
              </p>
              {s.evidence && <p className="mt-1 text-[13px] text-ink-soft">{s.evidence}</p>}
              {s.kind === 'merge' && (
                <div className="mt-2 flex flex-wrap gap-2">
                  <button type="button" className="library-button" disabled={decide.isPending} onClick={() => setAsking({ action: 'merge', entity: s.entity, other: s.other, suggestion: s.id, label: s.otherName || nameOf(s.other) })}>
                    Confirmar junção
                  </button>
                  <button type="button" className="library-button" disabled={decide.isPending} onClick={() => run({ action: 'distinct', entity: s.entity, other: s.other, suggestion: s.id })}>
                    Manter distintas
                  </button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}

      <div className="mt-4">
        <label className="block text-sm text-ink">
          Juntar com outra entidade
          <select className={FIELD} value={other} onChange={(e) => setOther(e.target.value)}>
            <option value="">Escolha…</option>
            {candidates.map((e) => (
              <option key={e.slug} value={e.slug}>{e.name} ({kindName(e.kind).toLowerCase()})</option>
            ))}
          </select>
        </label>
        <button
          type="button"
          className="library-button mt-2"
          disabled={!other || decide.isPending}
          onClick={() => setAsking({ action: 'merge', entity: page.slug, other, label: nameOf(other) })}
        >
          Juntar
        </button>
      </div>

      {page.aliases.length > 0 && (
        <div className="mt-4">
          <label className="block text-sm text-ink">
            Separar uma menção
            <select className={FIELD} value={mention} onChange={(e) => setMention(e.target.value)}>
              <option value="">Escolha um dos outros nomes…</option>
              {page.aliases.map((a) => (
                <option key={a} value={a}>{a}</option>
              ))}
            </select>
          </label>
          <p className="mt-1 text-xs text-ink-faint">Para um nome que é de outra entidade e foi parar nesta.</p>
          <button type="button" className="library-button mt-2" disabled={!mention || decide.isPending} onClick={() => run({ action: 'mention', entity: page.slug, mention })}>
            Separar
          </button>
        </div>
      )}

      {asking && (
        <div role="alertdialog" aria-label="Confirmar junção" className="mt-4 rounded-lg border border-warning/40 bg-warning-soft p-3 text-sm text-ink">
          <p>
            Juntar <strong>{page.name}</strong> com <strong>{asking.label}</strong>? As duas viram uma entidade, e a decisão vale também para as
            próximas gerações. Para separar de novo é preciso gerar a wiki outra vez.
          </p>
          <div className="mt-2 flex gap-2">
            <button type="button" className="library-button" onClick={() => run(asking)}>Juntar</button>
            <button type="button" className="library-button" onClick={() => setAsking(null)}>Cancelar</button>
          </div>
        </div>
      )}
      {decide.isError && <p role="alert" className="mt-3 text-[13px] text-danger">{describeError(decide.error)}</p>}

      {rules.length > 0 && (
        <details className="mt-4">
          <summary className="cursor-pointer text-sm text-ink-soft">O que já foi decidido ({rules.length})</summary>
          <ul className="mt-2 flex flex-col gap-1.5 text-[13px]">
            {rules.map((r) => (
              <li key={r.id} className="flex items-start justify-between gap-2">
                <span className="text-ink">{RULE_TEXT[r.kind]?.(r) ?? r.kind}</span>
                <button type="button" className="shrink-0 text-brand hover:underline" disabled={forget.isPending} onClick={() => forget.mutate(r.id)}>
                  Esquecer
                </button>
              </li>
            ))}
          </ul>
          <p className="mt-2 text-xs text-ink-faint">Esquecer vale para as próximas gerações; o que já foi juntado fica juntado até gerar de novo.</p>
        </details>
      )}
    </section>
  );
}
