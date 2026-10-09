import { useState } from 'react';
import { useCategories, useCreateCategory, useUpdateCategory, useDeleteCategory } from '../../categories/api/useCategories';
import { inOrder, pathOf, placesFor } from '../../categories/tree';
import { describeError } from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';

const inputClass = 'rounded bg-surface px-3 py-1.5 text-[13px] outline-none';
const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;

/** "No topo" and the places a category can go, each by its path, for a `<select>`. */
function PlaceSelect({ flat, id, value, onChange, label }) {
  return (
    <select aria-label={label} value={value ?? ''} onChange={(event) => onChange(event.target.value === '' ? null : Number(event.target.value))} className={inputClass}>
      <option value="">No topo</option>
      {placesFor(flat, id).map((place) => (
        <option key={place.id} value={place.id}>{pathOf(flat, place.id)}</option>
      ))}
    </select>
  );
}

function Row({ category, flat, busy, onSave, onDelete }) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(category.name);
  const [parentId, setParentId] = useState(category.parentId ?? null);
  const indent = { paddingLeft: `${(category.depth - 1) * 20}px` };

  if (editing) {
    return (
      <li className="py-2" style={indent}>
        <form
          className="flex flex-wrap items-center gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            onSave({ id: category.id, name: name.trim(), parentId }, () => setEditing(false));
          }}
        >
          <input value={name} onChange={(event) => setName(event.target.value)} maxLength={80} aria-label={`Nome de ${category.name}`} className={`${inputClass} min-w-[160px] flex-1`} />
          <PlaceSelect flat={flat} id={category.id} value={parentId} onChange={setParentId} label={`Onde fica ${category.name}`} />
          <Btn tone="primary" type="submit" disabled={busy || !name.trim()}>Salvar</Btn>
          <Btn type="button" onClick={() => { setEditing(false); setName(category.name); setParentId(category.parentId ?? null); }}>Cancelar</Btn>
        </form>
      </li>
    );
  }
  return (
    <li className="flex flex-wrap items-center justify-between gap-2 py-2" style={indent}>
      <div className="min-w-0">
        <span className="text-[14px] text-ink">{category.name}</span>
        <span className="ml-2 text-[12px] text-ink-faint">
          {count(category.works, 'obra', 'obras')}
          {category.own !== category.works && ` (${category.own} direto)`}
        </span>
      </div>
      <div className="flex gap-2">
        <Btn onClick={() => setEditing(true)} disabled={busy} aria-label={`Editar ${category.name}`}>Editar</Btn>
        <Btn
          tone="danger"
          onClick={() => onDelete(category)}
          disabled={busy || category.hasChildren}
          title={category.hasChildren ? 'Mova ou apague as subcategorias primeiro' : undefined}
          aria-label={`Apagar ${category.name}`}
        >
          Apagar
        </Btn>
      </div>
    </li>
  );
}

/**
 * The tree of categories that organises the navigation of the library by theme (DEC-140). It starts empty: owner and admin make the
 * ones that fit their library, under one another up to three levels. A work may be in several, and one in a subcategory also counts
 * in the ones above it. The tags stay apart, for the subjects that cross the categories.
 */
export function CategoriesTab() {
  const { data, isLoading, isError, error, refetch, isRefetching } = useCategories();
  const create = useCreateCategory();
  const update = useUpdateCategory();
  const remove = useDeleteCategory();
  const [name, setName] = useState('');
  const [parentId, setParentId] = useState(null);
  const [deleting, setDeleting] = useState(null);
  const [done, setDone] = useState('');
  const flat = data ?? [];
  const busy = create.isPending || update.isPending || remove.isPending;

  return (
    <div className="flex flex-col gap-5">
      <Section
        title="Categorias"
        hint="Organizam a navegação do acervo por tema, como Ficção científica ou Mangá › Seinen. Uma obra pode estar em várias, e a que está numa subcategoria também conta na de cima. Começa vazio: crie as que fizerem sentido para o seu acervo. As tags continuam à parte."
      >
        <form
          className="mb-4 flex flex-wrap items-center gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            setDone('');
            create.mutate({ name: name.trim(), parentId }, { onSuccess: (response) => { setName(''); setDone(`“${response.data.name}” foi criada.`); } });
          }}
        >
          <input value={name} onChange={(event) => setName(event.target.value)} maxLength={80} placeholder="Nome da categoria" aria-label="Nome da nova categoria" className={`${inputClass} min-w-[200px] flex-1`} />
          <PlaceSelect flat={flat} id={null} value={parentId} onChange={setParentId} label="Onde fica a nova categoria" />
          <Btn tone="primary" type="submit" disabled={busy || !name.trim()}>Criar categoria</Btn>
        </form>
        {isLoading && <Loading />}
        {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar as categorias.</LoadError>}
        {data && flat.length === 0 && <Empty>Nenhuma categoria ainda. Crie a primeira acima.</Empty>}
        {flat.length > 0 && <p className="mb-2 text-[13px] text-ink-soft">{count(flat.length, 'categoria', 'categorias')}.</p>}
        <ul className="divide-y divide-border-hairline" aria-label="Categorias">
          {inOrder(flat).map((category) => (
            <Row
              key={category.id}
              category={category}
              flat={flat}
              busy={busy}
              onSave={(change, close) => {
                setDone('');
                update.mutate(change, { onSuccess: () => { close(); setDone(`“${change.name}” foi salva.`); } });
              }}
              onDelete={setDeleting}
            />
          ))}
        </ul>
        {done && <p role="status" className="mt-3 text-[13px] text-ink-soft">{done}</p>}
        <ErrorNote>{create.isError ? describeError(create.error) : update.isError ? describeError(update.error) : remove.isError && describeError(remove.error)}</ErrorNote>
      </Section>

      {deleting && (
        <ConfirmDialog
          title="Apagar esta categoria?"
          message={
            <p>
              “{deleting.name}” será apagada.{' '}
              {deleting.own > 0
                ? `${count(deleting.own, 'obra deixa', 'obras deixam')} de estar nela, mas nenhuma obra é apagada.`
                : 'Nenhuma obra está nela.'}
            </p>
          }
          choices={[{ label: 'Apagar categoria', value: true, tone: 'danger' }]}
          onChoose={() => {
            const category = deleting;
            setDeleting(null);
            setDone('');
            remove.mutate(category.id, { onSuccess: () => setDone(`“${category.name}” foi apagada.`) });
          }}
          onCancel={() => setDeleting(null)}
        />
      )}
    </div>
  );
}
