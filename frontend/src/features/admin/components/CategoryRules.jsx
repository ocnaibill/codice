import { useState } from 'react';
import { useAddRule, useApplyRules, useCategoryRules, useRemoveRule, useRulesPreview } from '../../categories/api/useCategories';
import { inOrder, pathOf } from '../../categories/tree';
import { describeError } from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, ErrorNote, Section } from './ui';

const inputClass = 'rounded bg-surface px-3 py-1.5 text-[13px] outline-none';
const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;

/**
 * The rules that put works in categories (DEC-140): a term for a category, and a work with a tag that is that term (no regard to case or
 * accents, and each part of "Ficção / Ficção científica / Geral") goes in it. The rules only add: they are applied when someone says so,
 * after a preview of what they would do, and they never take a work out of a category nor put one back that a person took out.
 */
export function CategoryRules({ categories }) {
  const rules = useCategoryRules();
  const add = useAddRule();
  const remove = useRemoveRule();
  const preview = useRulesPreview();
  const apply = useApplyRules();
  const [categoryId, setCategoryId] = useState(null);
  const [term, setTerm] = useState('');
  const [confirming, setConfirming] = useState(false);
  const [done, setDone] = useState('');
  const tree = inOrder(categories);
  const chosen = categoryId ?? tree[0]?.id ?? '';
  const all = rules.data ?? [];
  const withRules = tree.filter((category) => all.some((rule) => rule.categoryId === category.id));
  const result = preview.data;
  // The categories the rules reach, in the order of the tree; the ones they do not reach are only counted.
  const order = new Map(tree.map((category, index) => [category.id, index]));
  const matching = (result?.categories ?? []).filter((category) => category.matched > 0).sort((a, b) => (order.get(a.id) ?? 0) - (order.get(b.id) ?? 0));
  const silent = (result?.categories.length ?? 0) - matching.length;
  // What was looked at is no longer what the rules say once they change.
  const changed = () => {
    preview.reset();
    setDone('');
  };

  return (
    <Section
      title="Regras"
      hint="Uma regra liga um termo a uma categoria: uma obra que tem uma tag igual ao termo (sem contar acento nem maiúscula, e cada parte de “Ficção / Ficção científica / Geral”) entra nela. As regras só acrescentam: valem quando você as aplica, depois de ver o que fariam, e nunca tiram uma obra de uma categoria."
    >
      <form
        className="mb-4 flex flex-wrap items-center gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          add.mutate({ categoryId: chosen, term: term.trim() }, { onSuccess: () => { setTerm(''); changed(); } });
        }}
      >
        <select aria-label="Categoria do novo termo" value={chosen} onChange={(event) => setCategoryId(Number(event.target.value))} className={inputClass}>
          {tree.map((category) => (
            <option key={category.id} value={category.id}>{pathOf(categories, category.id)}</option>
          ))}
        </select>
        <input value={term} onChange={(event) => setTerm(event.target.value)} maxLength={100} placeholder="Termo, como “science fiction”" aria-label="Novo termo" className={`${inputClass} min-w-[200px] flex-1`} />
        <Btn tone="primary" type="submit" disabled={add.isPending || !term.trim()}>Adicionar termo</Btn>
      </form>

      {rules.isError && <p role="alert" className="text-[13px] text-danger">Não foi possível carregar as regras.</p>}
      {rules.data && all.length === 0 && <p className="py-2 text-[13px] text-ink-faint">Nenhuma regra ainda. Adicione termos acima, ou comece da lista sugerida.</p>}
      <ul className="divide-y divide-border-hairline" aria-label="Regras por categoria">
        {withRules.map((category) => (
          <li key={category.id} className="py-3">
            <p className="text-[14px] text-ink">{pathOf(categories, category.id)}</p>
            <ul className="mt-2 flex flex-wrap gap-2" aria-label={`Termos de ${category.name}`}>
              {all.filter((rule) => rule.categoryId === category.id).map((rule) => (
                <li key={rule.id} className="flex items-center gap-1 rounded-full bg-surface-alt py-1 pl-3 pr-1 text-[13px] text-ink">
                  {rule.term}
                  <button
                    type="button"
                    aria-label={`Tirar o termo ${rule.term} de ${category.name}`}
                    disabled={remove.isPending}
                    onClick={() => remove.mutate(rule.id, { onSuccess: changed })}
                    className="flex h-6 w-6 items-center justify-center rounded-full text-ink-soft hover:bg-white hover:text-danger disabled:opacity-40"
                  >
                    ×
                  </button>
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ul>

      {all.length > 0 && (
        <div className="mt-4 border-t border-border-hairline pt-4">
          <Btn onClick={() => { setDone(''); preview.mutate(); }} disabled={preview.isPending || apply.isPending}>Ver o que as regras fariam</Btn>
          {result && (
            <div className="mt-3" aria-label="Prévia das regras">
              <p className="text-[13px] text-ink">
                {result.links === 0
                  ? 'As regras não têm nada novo a fazer.'
                  : `${count(result.links, 'novo lugar', 'novos lugares')} em ${count(result.works, 'obra', 'obras')}.`}
              </p>
              <p className="text-[13px] text-ink-soft">
                Sem categoria: {count(result.withoutNow, 'obra', 'obras')} agora, {result.withoutAfter} depois.
              </p>
              <ul className="mt-2 divide-y divide-border-hairline text-[13px]" aria-label="O que cada categoria receberia">
                {matching.map((category) => (
                  <li key={category.id} className="flex flex-wrap justify-between gap-2 py-1.5">
                    <span className="text-ink">{pathOf(categories, category.id) || category.name}</span>
                    <span className="text-ink-soft">{count(category.matched, 'obra casa', 'obras casam')}, {count(category.fresh, 'nova', 'novas')}</span>
                  </li>
                ))}
              </ul>
              {silent > 0 && (
                <p className="mt-1 text-[12px] text-ink-faint">
                  {silent === 1 ? 'Mais 1 categoria com regras não casa com nenhuma obra.' : `Mais ${silent} categorias com regras não casam com nenhuma obra.`}
                </p>
              )}
              <div className="mt-3 flex gap-2">
                <Btn tone="primary" onClick={() => setConfirming(true)} disabled={result.links === 0 || apply.isPending}>Aplicar as regras</Btn>
                <Btn onClick={() => preview.reset()}>Fechar a prévia</Btn>
              </div>
            </div>
          )}
        </div>
      )}
      {done && <p role="status" className="mt-3 text-[13px] text-ink-soft">{done}</p>}
      <ErrorNote>
        {add.isError ? describeError(add.error) : remove.isError ? describeError(remove.error) : preview.isError ? describeError(preview.error) : apply.isError && describeError(apply.error)}
      </ErrorNote>

      {confirming && (
        <ConfirmDialog
          title="Aplicar as regras?"
          message={
            <p>
              {count(result.works, 'obra entra', 'obras entram')} em categorias, com {count(result.links, 'novo lugar', 'novos lugares')} no total. Nenhuma obra sai de
              nenhuma categoria, e o que alguém já tirou de uma categoria não volta. Dá para ajustar cada obra depois, na edição de metadados.
            </p>
          }
          choices={[{ label: 'Aplicar as regras', value: true, tone: 'primary' }]}
          onChoose={() => {
            const links = result.links;
            setConfirming(false);
            apply.mutate(links, {
              onSuccess: (applied) => { preview.reset(); setDone(`${count(applied.links, 'novo lugar foi criado', 'novos lugares foram criados')}, em ${count(applied.works, 'obra', 'obras')}.`); },
              onError: () => preview.reset(),
            });
          }}
          onCancel={() => setConfirming(false)}
        />
      )}
    </Section>
  );
}
