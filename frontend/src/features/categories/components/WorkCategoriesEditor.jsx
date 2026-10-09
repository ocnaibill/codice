import { useState } from 'react';
import { serverMessage } from '../../../lib/serverMessage';
import { useCategories, useSetWorkCategories } from '../api/useCategories';
import { inOrder } from '../tree';

/**
 * The categories of one work, for owner and admin (DEC-140): the tree, with the ones the work is in checked. Putting a work in a
 * subcategory already puts it in the ones above, so there is no need to check both.
 */
export function WorkCategoriesEditor({ work }) {
  const { data, isLoading, isError } = useCategories();
  const save = useSetWorkCategories(work.id);
  const [checked, setChecked] = useState(() => new Set((work.metadata?.categories ?? []).map((c) => c.id)));
  const [saved, setSaved] = useState(false);
  const tree = inOrder(data ?? []);
  const toggle = (id) => {
    setSaved(false);
    setChecked((current) => {
      const next = new Set(current);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  };

  if (isLoading) return <p className="animate-pulse text-sm text-ink-faint">Carregando as categorias…</p>;
  if (isError) return <p role="alert" className="text-sm text-danger">Não foi possível carregar as categorias.</p>;
  if (tree.length === 0) {
    return <p className="text-sm text-ink-soft">Ainda não há categorias. Crie as primeiras em Administração → Categorias.</p>;
  }
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault();
        setSaved(false);
        save.mutate([...checked], { onSuccess: () => setSaved(true) });
      }}
    >
      <p className="text-sm text-ink-soft">Marque as categorias desta obra. Marcar uma subcategoria já a põe nas de cima.</p>
      <ul className="flex flex-col gap-1" aria-label="Categorias">
        {tree.map((category) => (
          <li key={category.id} style={{ paddingLeft: `${(category.depth - 1) * 20}px` }}>
            <label className="flex min-h-10 items-center gap-3 text-sm text-ink">
              <input type="checkbox" checked={checked.has(category.id)} onChange={() => toggle(category.id)} disabled={save.isPending} className="h-4 w-4" />
              {category.name}
            </label>
          </li>
        ))}
      </ul>
      <div className="flex items-center gap-3">
        <button type="submit" disabled={save.isPending} className="min-h-10 rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40">
          {save.isPending ? 'Salvando…' : 'Salvar categorias'}
        </button>
        {saved && <p role="status" className="text-sm text-ink-soft">Categorias salvas.</p>}
      </div>
      {save.isError && <p role="alert" className="text-sm text-danger">{serverMessage(save.error, 'Não foi possível salvar as categorias.')}</p>}
    </form>
  );
}
