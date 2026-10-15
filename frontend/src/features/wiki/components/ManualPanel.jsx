import React from 'react';
import { describeError } from '../../admin/api/admin';
import { useCreateManual, useDeleteManual, useUpdateManual, useWikiManual } from '../api/useWiki';
import { GROUPS, MANUAL_KINDS, SECTION_KINDS, describeManual, kindName, manualItem, pointOptions, pointValue } from '../wiki';

const FIELD = 'mt-1 w-full rounded-lg border border-border-hairline bg-white px-3 py-2 text-sm text-ink';

/** The choice of a point (book and chapter) of the wiki, for what is written to appear from there on (DEC-186). */
export function PointSelect({ books, value, onChange, label = 'A partir de que ponto aparece' }) {
  const options = React.useMemo(() => pointOptions(books), [books]);
  return (
    <label className="block text-sm text-ink">
      {label}
      <select className={FIELD} value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>{o.label}</option>
        ))}
      </select>
    </label>
  );
}

/** The point the form starts at: the one the wiki is showing, else the first chapter. */
export function startingPoint(books, shown) {
  const options = pointOptions(books);
  const own = shown && shown.book >= 0 ? pointValue(shown.book, shown.chapter) : '';
  return options.find((o) => o.value === own)?.value ?? options[0]?.value ?? '';
}

/**
 * The fields of one thing written by hand: a text of the page, a moment, a tie of the ficha or an edge of the graph. Used to add and to
 * correct; `initial` fills it for the second.
 */
function ItemForm({ kind, books, index, slug, initial, busy, error, onSubmit, onCancel, submitLabel }) {
  const [form, setForm] = React.useState(initial);
  const set = (patch) => setForm((f) => ({ ...f, ...patch }));
  const others = (index ?? []).filter((e) => e.slug !== slug).sort((a, b) => a.name.localeCompare(b.name, 'pt'));
  const item = manualItem(kind, form, slug);
  const editing = !!initial.id;

  return (
    <form
      className="mt-3 flex flex-col gap-3 rounded-lg border border-border-hairline bg-surface/50 p-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (item) onSubmit(item);
      }}
    >
      <PointSelect books={books} value={form.point} onChange={(point) => set({ point })} />

      {kind === 'section' && (
        <>
          {!editing && (
            <label className="block text-sm text-ink">
              Em que parte da página
              <select className={FIELD} value={form.section} onChange={(e) => set({ section: e.target.value })}>
                {Object.entries(SECTION_KINDS).map(([key, name]) => (
                  <option key={key} value={key}>{name}</option>
                ))}
              </select>
            </label>
          )}
          {form.section === 'bio' && (
            <label className="block text-sm text-ink">
              Nome da parte (opcional)
              <input className={FIELD} value={form.part} onChange={(e) => set({ part: e.target.value })} maxLength={200} />
            </label>
          )}
          <label className="block text-sm text-ink">
            Texto
            <textarea className={`${FIELD} min-h-32`} value={form.text} onChange={(e) => set({ text: e.target.value })} />
          </label>
          <p className="-mt-2 text-xs text-ink-faint">Deixe uma linha em branco entre os parágrafos. Cada parágrafo mostra “escrito à mão por você”.</p>
        </>
      )}

      {kind === 'state' && (
        <label className="block text-sm text-ink">
          O que está acontecendo com {editing ? 'ela' : 'esta entidade'} neste capítulo
          <textarea className={`${FIELD} min-h-20`} value={form.text} onChange={(e) => set({ text: e.target.value })} maxLength={500} />
        </label>
      )}

      {kind === 'relation' && (
        <>
          {!editing && (
            <label className="block text-sm text-ink">
              Com quem ou com o quê
              <select className={FIELD} value={form.other} onChange={(e) => set({ other: e.target.value })}>
                <option value="">Algo que não está na wiki…</option>
                {others.map((e) => (
                  <option key={e.slug} value={e.slug}>{e.name} ({kindName(e.kind).toLowerCase()})</option>
                ))}
              </select>
            </label>
          )}
          {!editing && !form.other && (
            <label className="block text-sm text-ink">
              Nome
              <input className={FIELD} value={form.otherName} onChange={(e) => set({ otherName: e.target.value })} maxLength={160} />
            </label>
          )}
          <label className="block text-sm text-ink">
            Grupo da ficha
            <select className={FIELD} value={form.group} onChange={(e) => set({ group: e.target.value })}>
              {Object.entries(GROUPS).map(([key, name]) => (
                <option key={key} value={key}>{name}</option>
              ))}
            </select>
          </label>
          <label className="block text-sm text-ink">
            Papel (opcional)
            <input className={FIELD} value={form.role} onChange={(e) => set({ role: e.target.value })} maxLength={200} />
          </label>
        </>
      )}

      {kind === 'edge' && (
        <>
          {!editing && (
            <label className="block text-sm text-ink">
              Ligar a
              <select className={FIELD} value={form.other} onChange={(e) => set({ other: e.target.value })}>
                <option value="">Escolha…</option>
                {others.map((e) => (
                  <option key={e.slug} value={e.slug}>{e.name} ({kindName(e.kind).toLowerCase()})</option>
                ))}
              </select>
            </label>
          )}
          <label className="block text-sm text-ink">
            O que as liga
            <input className={FIELD} value={form.label} onChange={(e) => set({ label: e.target.value })} maxLength={300} placeholder="participou de, mora em, rival de…" />
          </label>
        </>
      )}

      {error && <p role="alert" className="text-[13px] text-danger">{describeError(error)}</p>}
      <div className="flex flex-wrap gap-2">
        <button type="submit" className="library-button" disabled={!item || busy}>{submitLabel}</button>
        <button type="button" className="library-button" onClick={onCancel}>Cancelar</button>
      </div>
    </form>
  );
}

const EMPTY = { text: '', section: 'who', part: '', group: 'others', role: '', other: '', otherName: '', label: '' };

/** The form values of an item already written, to correct it. */
function valuesOf(item) {
  const d = item.data ?? {};
  return {
    ...EMPTY,
    id: item.id,
    point: pointValue(item.book, item.chapter),
    text: item.kind === 'section' ? (d.paragraphs ?? []).join('\n\n') : d.text ?? '',
    section: d.kind ?? 'who',
    part: d.part ?? '',
    group: d.group ?? 'others',
    role: d.role ?? '',
    label: d.label ?? '',
    other: 'x', // not changed: the other end stays
  };
}

/**
 * What the staff write by hand on an entity (DEC-186): the text of its page (who it is, biography, personality), its moments by
 * chapter, its ties on the ficha and its edges in the graph, each from a point of the book on. What was written is kept through new
 * generations and appears with the mark of who wrote it.
 */
export function ManualPanel({ projectId, page, books, index, shown, onGone }) {
  const list = useWikiManual(projectId, page.slug);
  const create = useCreateManual(projectId);
  const update = useUpdateManual(projectId);
  const remove = useDeleteManual(projectId);
  const [adding, setAdding] = React.useState(null); // the kind being added
  const [editing, setEditing] = React.useState(null); // the item being corrected
  const [asking, setAsking] = React.useState(null); // the item waiting for a second click to be taken away
  const items = list.data?.items ?? [];
  const nameOf = (value) => index?.find((e) => e.slug === value)?.name ?? value;
  const start = startingPoint(books, shown);

  return (
    <section aria-labelledby="wiki-manual" className="rounded-xl border border-border-hairline bg-white p-5 shadow-sm">
      <h2 id="wiki-manual" className="font-display text-xl text-ink">Escrever à mão</h2>
      <p className="mt-1 text-[13px] text-ink-faint">
        Só o staff vê isto. O que você escreve fica ligado a um ponto do livro (ninguém o vê antes dele) e continua nas próximas gerações.
      </p>

      <div className="mt-3 flex flex-wrap gap-2">
        {[
          ['section', page.page ? 'Escrever um texto' : 'Escrever a página'],
          ['state', 'Registrar um momento'],
          ['relation', 'Adicionar um vínculo'],
          ['edge', 'Ligar a outra entidade'],
        ].map(([kind, label]) => (
          <button
            key={kind}
            type="button"
            className="library-button"
            aria-pressed={adding === kind}
            onClick={() => {
              setEditing(null);
              setAdding(adding === kind ? null : kind);
            }}
          >
            {label}
          </button>
        ))}
      </div>

      {adding && (
        <ItemForm
          key={adding}
          kind={adding}
          books={books}
          index={index}
          slug={page.slug}
          initial={{ ...EMPTY, point: start, section: page.who ? 'bio' : 'who' }}
          busy={create.isPending}
          error={create.error}
          submitLabel="Salvar"
          onCancel={() => setAdding(null)}
          onSubmit={(item) => create.mutate([item], { onSuccess: () => setAdding(null) })}
        />
      )}

      {items.length > 0 && (
        <ul className="mt-4 flex flex-col gap-3" aria-label="Escrito à mão">
          {items.map((item) => (
            <li key={item.id} className="rounded-lg bg-surface/60 p-3 text-sm">
              <p className="font-mono text-[11px] text-ink-faint">
                {MANUAL_KINDS[item.kind] ?? item.kind} · cap. {item.chapter} · por {item.author}
              </p>
              <p className="mt-1 line-clamp-4 whitespace-pre-line text-ink">{describeManual(item, nameOf)}</p>
              {editing === item.id ? (
                <ItemForm
                  kind={item.kind}
                  books={books}
                  index={index}
                  slug={page.slug}
                  initial={valuesOf(item)}
                  busy={update.isPending}
                  error={update.error}
                  submitLabel="Salvar a correção"
                  onCancel={() => setEditing(null)}
                  onSubmit={(next) =>
                    update.mutate({ id: item.id, book: next.book, chapter: next.chapter, data: next.data }, { onSuccess: () => setEditing(null) })
                  }
                />
              ) : (
                <div className="mt-2 flex flex-wrap gap-3 text-[13px]">
                  {item.kind !== 'entity' && (
                    <button type="button" className="text-brand hover:underline" onClick={() => { setAdding(null); setEditing(item.id); }}>
                      Corrigir
                    </button>
                  )}
                  <button type="button" className="text-danger hover:underline" disabled={remove.isPending} onClick={() => setAsking(item)}>
                    {item.kind === 'entity' ? 'Apagar entidade' : 'Apagar'}
                  </button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}

      {asking && (
        <div role="alertdialog" aria-label="Confirmar exclusão" className="mt-4 rounded-lg border border-warning/40 bg-warning-soft p-3 text-sm text-ink">
          <p>
            {asking.kind === 'entity'
              ? 'Apagar esta entidade, com tudo o que foi escrito sobre ela?'
              : 'Apagar o que foi escrito? Some da wiki e das próximas gerações.'}
          </p>
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              className="library-button"
              onClick={() =>
                remove.mutate(asking.id, {
                  onSuccess: () => {
                    setAsking(null);
                    if (asking.kind === 'entity') onGone?.();
                  },
                })
              }
            >
              Apagar
            </button>
            <button type="button" className="library-button" onClick={() => setAsking(null)}>Cancelar</button>
          </div>
        </div>
      )}
      {remove.isError && <p role="alert" className="mt-3 text-[13px] text-danger">{describeError(remove.error)}</p>}
    </section>
  );
}
