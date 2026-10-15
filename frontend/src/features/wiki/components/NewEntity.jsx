import React from 'react';
import { describeError } from '../../admin/api/admin';
import { useCreateManual } from '../api/useWiki';
import { KINDS, filterIndex, kindName, newEntityItems } from '../wiki';
import { PointSelect, startingPoint } from './ManualPanel';

const FIELD = 'mt-1 w-full rounded-lg border border-border-hairline bg-white px-3 py-2 text-sm text-ink';

/**
 * A new entity written by hand (DEC-186): a person, a place or an event the generator did not find. It appears from the chapter
 * chosen, optionally with a text for its page and ties to the ones that took part, and it is kept through new generations.
 */
export function NewEntity({ projectId, books, index, shown, onCreated, onCancel }) {
  const create = useCreateManual(projectId);
  const [form, setForm] = React.useState({
    name: '', kind: 'event', aliases: '', point: startingPoint(books, shown), text: '', label: 'participou', tied: [],
  });
  const [find, setFind] = React.useState('');
  const set = (patch) => setForm((f) => ({ ...f, ...patch }));
  const items = newEntityItems(form);
  const shownList = filterIndex(index, { text: find }).slice(0, 12);
  const toggle = (slug) => set({ tied: form.tied.includes(slug) ? form.tied.filter((s) => s !== slug) : [...form.tied, slug] });
  const nameOf = (slug) => index.find((e) => e.slug === slug)?.name ?? slug;

  return (
    <form
      aria-label="Nova entidade"
      className="rounded-xl border border-border-hairline bg-white p-5 shadow-sm"
      onSubmit={(e) => {
        e.preventDefault();
        if (items) create.mutate(items, { onSuccess: (done) => onCreated(done.slugs?.[0] ?? '') });
      }}
    >
      <h2 className="font-display text-xl text-ink">Nova entidade</h2>
      <p className="mt-1 text-[13px] text-ink-faint">
        Para quem ou o que o gerador não achou: uma pessoa, um lugar, um acontecimento. Só aparece a partir do capítulo escolhido.
      </p>
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <label className="block text-sm text-ink">
          Nome
          <input className={FIELD} value={form.name} onChange={(e) => set({ name: e.target.value })} maxLength={160} autoFocus />
        </label>
        <label className="block text-sm text-ink">
          Tipo
          <select className={FIELD} value={form.kind} onChange={(e) => set({ kind: e.target.value })}>
            {KINDS.map((k) => (
              <option key={k.key} value={k.key}>{k.one}</option>
            ))}
          </select>
        </label>
        <label className="block text-sm text-ink sm:col-span-2">
          Outros nomes (separados por vírgula)
          <input className={FIELD} value={form.aliases} onChange={(e) => set({ aliases: e.target.value })} />
        </label>
        <div className="sm:col-span-2">
          <PointSelect books={books} value={form.point} onChange={(point) => set({ point })} label="Aparece a partir de" />
        </div>
        <label className="block text-sm text-ink sm:col-span-2">
          Texto da página (opcional)
          <textarea className={`${FIELD} min-h-24`} value={form.text} onChange={(e) => set({ text: e.target.value })} />
        </label>
      </div>

      <fieldset className="mt-3">
        <legend className="text-sm text-ink">Ligada a (opcional)</legend>
        <input
          type="search"
          className={FIELD}
          placeholder="Procurar quem ou o quê"
          aria-label="Procurar entidade para ligar"
          value={find}
          onChange={(e) => setFind(e.target.value)}
        />
        <ul className="mt-2 flex max-h-40 flex-col gap-1 overflow-y-auto">
          {shownList.map((e) => (
            <li key={e.slug}>
              <label className="flex items-center gap-2 text-sm text-ink">
                <input type="checkbox" checked={form.tied.includes(e.slug)} onChange={() => toggle(e.slug)} />
                {e.name} <span className="text-ink-faint">({kindName(e.kind).toLowerCase()})</span>
              </label>
            </li>
          ))}
        </ul>
        {form.tied.length > 0 && (
          <>
            <p className="mt-2 text-[13px] text-ink-soft">Ligada a: {form.tied.map(nameOf).join(', ')}</p>
            <label className="mt-2 block text-sm text-ink">
              O que as liga
              <input className={FIELD} value={form.label} onChange={(e) => set({ label: e.target.value })} maxLength={300} />
            </label>
          </>
        )}
      </fieldset>

      {create.isError && <p role="alert" className="mt-3 text-[13px] text-danger">{describeError(create.error)}</p>}
      <div className="mt-4 flex flex-wrap gap-2">
        <button type="submit" className="library-button" disabled={!items || create.isPending}>Criar</button>
        <button type="button" className="library-button" onClick={onCancel}>Cancelar</button>
      </div>
    </form>
  );
}
