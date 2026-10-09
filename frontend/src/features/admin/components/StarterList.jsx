import { useState } from 'react';
import { useCreateFromStarter, useStarterList } from '../../categories/api/useCategories';
import { describeError } from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, ErrorNote } from './ui';

const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;
const termsOf = (group) => group.terms.length + (group.children ?? []).reduce((sum, child) => sum + termsOf(child), 0);
const namesOf = (group) => 1 + (group.children ?? []).reduce((sum, child) => sum + namesOf(child), 0);

/**
 * The list of categories offered to start from (DEC-140), for whoever does not want to type them: it is only read when asked for, shows
 * what each would bring, and makes only the ones that stay checked, with their subcategories and the terms of their rules. It puts no
 * work in any category: that is for the rules, applied after their preview. A category that is already there is used, not made twice.
 */
export function StarterList({ hasCategories }) {
  const [open, setOpen] = useState(false);
  const [unchecked, setUnchecked] = useState(() => new Set());
  const [done, setDone] = useState('');
  const list = useStarterList({ enabled: open });
  const make = useCreateFromStarter();
  const groups = list.data ?? [];
  const chosen = groups.filter((group) => !unchecked.has(group.name));
  const toggle = (name) => setUnchecked((current) => {
    const next = new Set(current);
    if (!next.delete(name)) next.add(name);
    return next;
  });

  return (
    <>
      <Btn onClick={() => { setDone(''); setOpen(true); }} disabled={make.isPending}>
        {hasCategories ? 'Adicionar da lista sugerida' : 'Começar de uma lista sugerida'}
      </Btn>
      {done && <p role="status" className="basis-full text-[13px] text-ink-soft">{done}</p>}
      <ErrorNote>{make.isError && describeError(make.error)}</ErrorNote>
      {open && (
        <ConfirmDialog
          title="Lista sugerida de categorias"
          message={
            <div>
              <p>
                Marque as que quer criar. Cada uma vem com as subcategorias e com termos para as regras, que você vê e ajusta depois. Nada é posto em nenhuma obra por
                aqui: isso é das regras, quando você as aplicar.
              </p>
              {list.isLoading && <p className="mt-3 text-[13px] text-ink-faint">Carregando a lista…</p>}
              {list.isError && <p role="alert" className="mt-3 text-[13px] text-danger">Não foi possível carregar a lista.</p>}
              <ul className="mt-3 max-h-[40vh] overflow-y-auto" aria-label="Categorias da lista">
                {groups.map((group) => (
                  <li key={group.name}>
                    <label className="flex min-h-10 items-start gap-3 py-1 text-[13px] text-ink">
                      <input type="checkbox" checked={!unchecked.has(group.name)} onChange={() => toggle(group.name)} className="mt-0.5 h-4 w-4" />
                      <span>
                        {group.name} <span className="text-ink-faint">· {count(termsOf(group), 'termo', 'termos')}</span>
                        {group.children?.length > 0 && <span className="block text-[12px] text-ink-soft">{group.children.map((child) => child.name).join(', ')}</span>}
                      </span>
                    </label>
                  </li>
                ))}
              </ul>
            </div>
          }
          choices={[{
            label: chosen.length === 0 ? 'Criar' : `Criar ${count(chosen.reduce((sum, group) => sum + namesOf(group), 0), 'categoria', 'categorias')}`,
            value: true,
            tone: 'primary',
            disabled: chosen.length === 0,
          }]}
          onChoose={() => {
            setOpen(false);
            make.mutate(chosen.map((group) => group.name), {
              onSuccess: (made) => setDone(`${count(made.categories, 'categoria criada', 'categorias criadas')} e ${count(made.rules, 'termo', 'termos')} nas regras.`),
            });
          }}
          onCancel={() => setOpen(false)}
        />
      )}
    </>
  );
}
