import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { WorkCover } from '../../../components/ui/WorkCover';
import { useDialog } from '../../../lib/useDialog';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { isStaff, useMe } from '../../auth/api/useMe';
import { useWorkSearch } from '../../reader/api/useVersions';
import {
  collectionReason,
  useAddToCollection,
  useCollection,
  useOrderCollection,
  useRemoveFromCollection,
  useRenameCollection,
  useRestoreCollection,
  useRetireCollection,
} from '../api/useCollections';
import { collectionLine, wordsOf } from '../text';

const BUTTON = 'min-h-10 rounded-lg border border-border-hairline bg-surface px-3 text-xs text-ink hover:bg-surface-alt disabled:opacity-40';
const PRIMARY = 'min-h-10 rounded-lg bg-brand px-4 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40';

function RenameForm({ collection, onDone }) {
  const words = wordsOf(collection.kind);
  const [name, setName] = React.useState(collection.name);
  const [message, setMessage] = React.useState('');
  const rename = useRenameCollection();
  const submit = (event) => {
    event.preventDefault();
    if (!name.trim()) return;
    setMessage('');
    rename.mutate(
      { id: collection.id, name, kind: collection.kind },
      { onSuccess: onDone, onError: (error) => setMessage(collectionReason(error, 'Não foi possível renomear.')) }
    );
  };
  return (
    <form onSubmit={submit} className="flex flex-wrap items-end gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <label className="flex min-w-[220px] flex-1 flex-col gap-1 text-sm text-ink-soft">
        Novo nome
        <input autoFocus value={name} maxLength={512} onChange={(event) => setName(event.target.value)} className="min-h-11 rounded-lg border border-border-hairline bg-surface px-3 text-ink" />
      </label>
      <button type="submit" disabled={!name.trim() || rename.isPending} className={PRIMARY}>Salvar</button>
      <button type="button" onClick={onDone} className={BUTTON}>Cancelar</button>
      <p className="basis-full text-xs text-ink-faint">{words.renameNote}</p>
      {message && <p role="alert" className="basis-full text-sm text-danger">{message}</p>}
    </form>
  );
}

function AddWorkPanel({ collection, members, onDone }) {
  const words = wordsOf(collection.kind);
  const [term, setTerm] = React.useState('');
  const [message, setMessage] = React.useState('');
  const search = useWorkSearch(term);
  const add = useAddToCollection();
  const here = new Set(members.filter((work) => work.available).map((work) => work.id));
  const results = (search.data?.data ?? []).filter((work) => !here.has(work.id));
  const put = (work) => {
    setMessage('');
    add.mutate(
      { id: collection.id, workId: work.id, kind: collection.kind },
      {
        onSuccess: () => setMessage(`“${work.title}” ${words.added}`),
        onError: (error) => setMessage(collectionReason(error, 'Não foi possível acrescentar a obra.')),
      }
    );
  };
  return (
    <section aria-label="Acrescentar obras" className="rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <div className="flex flex-wrap items-end gap-3">
        <label className="flex min-w-[220px] flex-1 flex-col gap-1 text-sm text-ink-soft">
          Procurar obra para acrescentar
          <input autoFocus value={term} onChange={(event) => { setTerm(event.target.value); setMessage(''); }} placeholder="Título ou autor" className="min-h-11 rounded-lg border border-border-hairline bg-surface px-3 text-ink" />
        </label>
        <button onClick={onDone} className={BUTTON}>Fechar</button>
      </div>
      {term.trim().length >= 2 && search.isSuccess && results.length === 0 && !message && (
        <p className="mt-3 text-sm text-ink-soft">Nenhuma obra para acrescentar com esse nome.</p>
      )}
      {results.length > 0 && (
        <ul className="mt-3 flex flex-col gap-2">
          {results.map((work) => {
            // Only an official collection takes the work out of the series it was in; a list takes nothing from anywhere.
            const leaves = collection.kind !== 'personal' && work.series && work.series.trim().toLowerCase() !== collection.name.trim().toLowerCase();
            return (
              <li key={work.id} className="flex items-center gap-3 rounded-lg bg-surface px-3 py-2">
                <WorkCover item={work} className="h-12 w-8 shrink-0 rounded-sm object-cover" />
                <div className="min-w-0 flex-1 text-sm">
                  <p className="truncate font-semibold text-ink">{work.title}</p>
                  <p className="line-clamp-2 text-xs text-ink-soft">
                    {work.author}
                    {leaves ? ` · sairá de “${work.series}”` : ''}
                  </p>
                </div>
                <button onClick={() => put(work)} disabled={add.isPending} className={PRIMARY} aria-label={`Acrescentar “${work.title}”`}>
                  Acrescentar
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {message && <p role="status" className="mt-3 text-sm text-ink">{message}</p>}
    </section>
  );
}

function WorkRow({ work, index, count, staff, confirming, busy, words, onOpen, onMove, onAskRemove, onRemove, onCancel }) {
  const gone = !work.available;
  return (
    <li className="flex flex-col gap-2 rounded-xl border border-border-hairline bg-white p-3 shadow-sm">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="w-8 shrink-0 text-center font-mono text-sm text-ink-soft" title={work.position == null ? 'Sem número' : `Número ${work.position}`}>
          {work.position == null ? '—' : work.position}
        </span>
        <button onClick={() => onOpen(work)} disabled={gone} className="flex min-w-0 flex-1 basis-[180px] items-center gap-3 text-left disabled:cursor-default" aria-label={gone ? undefined : `Abrir a obra ${work.title}`}>
          <WorkCover item={work} className="h-16 w-11 shrink-0 rounded-sm object-cover" />
          <span className="min-w-0">
            <span className="line-clamp-2 block font-display text-lg leading-snug text-ink">{work.title}</span>
            <span className="block truncate text-xs text-ink-soft">{work.author}</span>
          </span>
        </button>
        {gone && <span className="shrink-0 rounded-full bg-surface-alt px-2 py-0.5 font-mono text-[10px] text-ink-soft" title="A obra saiu do acervo: a lista guarda o que ela era">Fora do acervo</span>}
        {work.completed && <span className="shrink-0 rounded-full bg-success/10 px-2 py-0.5 font-mono text-[10px] text-success">Lida</span>}
        {staff && (
          <div className="flex shrink-0 basis-full items-center justify-end gap-1 sm:basis-auto">
            <button onClick={() => onMove(index, -1)} disabled={busy || index === 0} className={BUTTON} aria-label={`Subir “${work.title}”`}>↑</button>
            <button onClick={() => onMove(index, 1)} disabled={busy || index === count - 1} className={BUTTON} aria-label={`Descer “${work.title}”`}>↓</button>
            <button onClick={() => onAskRemove(work.entryId)} disabled={busy} className={BUTTON} aria-label={`Tirar “${work.title}” da ${words.thing}`}>Tirar</button>
          </div>
        )}
      </div>
      {confirming && (
        <div role="alertdialog" aria-label={`Tirar “${work.title}” da ${words.thing}`} className="flex flex-wrap items-center gap-3 rounded-lg bg-surface px-3 py-2 text-xs text-ink-soft">
          <span className="min-w-[200px] flex-1">{words.removeNote}</span>
          <button onClick={() => onRemove(work)} disabled={busy} className={PRIMARY}>Tirar da {words.thing}</button>
          <button onClick={onCancel} className={BUTTON}>Cancelar</button>
        </div>
      )}
    </li>
  );
}

/**
 * The page of a collection (#184, DEC-130): its works, in order, each opening its own sheet. Owner and admin also rename it,
 * put works in and take them out, change the order and retire it. Everything is reversible: nothing is deleted, and a
 * retired collection is restored from the list of the retired ones.
 */
export function CollectionSheet() {
  const id = useGlobalStore((state) => state.collectionSheetId);
  const close = useGlobalStore((state) => state.closeCollection);
  const openWork = useGlobalStore((state) => state.openWork);
  const staffMember = isStaff(useMe().data);
  const dialogRef = React.useRef(null);
  const closeRef = React.useRef(null);
  const { data, isLoading, isError, error, refetch, isRefetching } = useCollection(id);
  const order = useOrderCollection();
  const remove = useRemoveFromCollection();
  const retire = useRetireCollection();
  const restore = useRestoreCollection();
  const [mode, setMode] = React.useState(null); // 'rename' | 'add' | 'retire'
  const [removing, setRemoving] = React.useState(null); // the place waiting for a yes
  const [message, setMessage] = React.useState('');

  React.useEffect(() => {
    setMode(null);
    setRemoving(null);
    setMessage('');
  }, [id]);

  useDialog(dialogRef, { active: !!id, initialFocus: closeRef, onEscape: close });

  if (!id) return null;

  const collection = data?.collection;
  const kind = collection?.kind ?? 'official';
  const words = wordsOf(kind);
  // The staff manages the official collections; a person manages their own lists, and no one else's.
  const staff = kind === 'personal' || staffMember;
  const works = data?.works ?? [];
  const busy = order.isPending || remove.isPending || retire.isPending || restore.isPending;
  const fail = (fallback) => (err) => setMessage(collectionReason(err, fallback));
  const move = (index, delta) => {
    const items = [...works];
    [items[index], items[index + delta]] = [items[index + delta], items[index]];
    setMessage('');
    order.mutate({ id, items, kind }, { onError: fail('Não foi possível mudar a ordem.') });
  };
  const take = (work) => {
    setMessage('');
    remove.mutate({ id, work, kind }, { onSuccess: () => setRemoving(null), onError: fail('Não foi possível tirar a obra.') });
  };
  const retireIt = () => {
    setMessage('');
    retire.mutate({ id, kind }, { onSuccess: close, onError: fail('Não foi possível aposentar a coleção.') });
  };

  return (
    <div ref={dialogRef} className="fixed inset-0 z-50 flex items-center justify-center bg-ink/60 backdrop-blur-sm sm:p-4" role="dialog" aria-modal="true" aria-label={words.dialog}>
      <div className="flex h-full w-full flex-col overflow-hidden bg-[#faf8f4] shadow-2xl sm:max-h-[92vh] sm:h-auto sm:max-w-4xl sm:rounded-2xl">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline bg-[#faf8f4] px-4 py-3 sm:px-6">
          <div className="min-w-0">
            <p className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">{words.eyebrow}</p>
            <h2 className="truncate font-display text-2xl text-ink sm:text-3xl">{collection?.name ?? 'Carregando…'}</h2>
          </div>
          <button ref={closeRef} onClick={close} className="flex min-h-11 min-w-11 items-center justify-center rounded-lg text-xl text-ink-soft hover:bg-surface-alt hover:text-brand" aria-label="Fechar">✕</button>
        </div>

        <div className="min-h-0 overflow-y-auto px-4 py-5 sm:px-6 sm:py-6">
          {isLoading && <p className="animate-pulse text-sm text-ink-faint">Carregando a coleção…</p>}
          {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível abrir esta coleção.</LoadError>}
          {collection && (
            <div className="flex flex-col gap-5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="text-sm text-ink-soft">
                  {collection.retired ? `${words.retired}. ` : ''}
                  {collectionLine(collection)}
                </p>
                {staff && !collection.retired && (
                  <div className="flex flex-wrap items-center gap-2">
                    <button onClick={() => setMode(mode === 'rename' ? null : 'rename')} aria-pressed={mode === 'rename'} className={BUTTON}>Renomear</button>
                    <button onClick={() => setMode(mode === 'add' ? null : 'add')} aria-pressed={mode === 'add'} className={BUTTON}>Acrescentar obra</button>
                    <button onClick={() => setMode(mode === 'retire' ? null : 'retire')} aria-pressed={mode === 'retire'} className={BUTTON}>Aposentar</button>
                  </div>
                )}
                {staff && collection.retired && (
                  <button onClick={() => restore.mutate({ id, kind }, { onError: fail('Não foi possível restaurar.') })} disabled={busy} className={PRIMARY}>Restaurar a {words.thing}</button>
                )}
              </div>

              {staff && mode === 'rename' && <RenameForm collection={collection} onDone={() => setMode(null)} />}
              {staff && mode === 'add' && <AddWorkPanel collection={collection} members={works} onDone={() => setMode(null)} />}
              {staff && mode === 'retire' && (
                <div role="alertdialog" aria-label={`Aposentar a ${words.thing}`} className="flex flex-wrap items-center gap-3 rounded-xl border border-border-hairline bg-white p-4 text-sm text-ink-soft shadow-sm">
                  <span className="min-w-[220px] flex-1">
                    Aposentar “{collection.name}”? {words.retireNote}
                  </span>
                  <button onClick={retireIt} disabled={busy} className={PRIMARY}>Aposentar</button>
                  <button onClick={() => setMode(null)} className={BUTTON}>Cancelar</button>
                </div>
              )}
              {message && <p role="alert" className="text-sm text-danger">{message}</p>}

              {works.length === 0 ? (
                <p className="rounded-lg border border-dashed border-surface-alt bg-surface/50 px-4 py-8 text-center text-sm text-ink-faint">
                  {collection.retired ? words.restoreHint : words.nowhere}
                </p>
              ) : (
                <ol aria-label={`Obras da ${words.thing}`} className="flex flex-col gap-2">
                  {works.map((work, index) => (
                    <WorkRow
                      key={work.entryId}
                      work={work}
                      index={index}
                      count={works.length}
                      staff={staff && !collection.retired}
                      confirming={removing === work.entryId}
                      busy={busy}
                      words={words}
                      onOpen={(w) => openWork(w.id)}
                      onMove={move}
                      onAskRemove={setRemoving}
                      onRemove={take}
                      onCancel={() => setRemoving(null)}
                    />
                  ))}
                </ol>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
