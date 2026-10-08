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
  useClassifyCollection,
  useCollection,
  useOrderCollection,
  useRemoveFromCollection,
  useRenameCollection,
  useRestoreCollection,
  useRetireCollection,
} from '../api/useCollections';
import { collectionLine, COMIC_KINDS, groupByUnit, numberText, stepText, UNITS, unitLabel, wordsOf } from '../text';
import { CollectionFavoriteButton } from './CollectionFavoriteButton';

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

function WorkRow({ work, label, index, count, staff, confirming, busy, words, onOpen, onMove, onAskRemove, onRemove, onCancel }) {
  const gone = !work.available;
  return (
    <li className="flex flex-col gap-2 rounded-xl border border-border-hairline bg-white p-3 shadow-sm">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="w-14 shrink-0 text-center font-mono text-sm text-ink-soft" title={work.position == null ? 'Sem número' : `Número ${numberText(work.position)}`}>
          {label}
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

const SHOWN = 50;

/** One group of the works of a collection (the volumes, the chapters…), of which the first are shown and the rest come when asked. */
function WorkGroup({ group, heading, words, official, rowProps, onMove }) {
  const [shown, setShown] = React.useState(SHOWN);
  const visible = group.works.slice(0, shown);
  return (
    <section aria-label={heading ?? undefined} className="flex flex-col gap-2">
      {heading && (
        <h3 className="font-mono text-[11px] font-semibold uppercase tracking-widest text-ink-faint">
          {heading} <span className="font-normal">({group.works.length})</span>
        </h3>
      )}
      <ol aria-label={heading ? `${heading} da ${words.thing}` : `Obras da ${words.thing}`} className="flex flex-col gap-2">
        {visible.map((work, index) => (
          <WorkRow
            key={work.entryId}
            work={work}
            label={official ? unitLabel(work.unit, work.position) : numberText(work.position)}
            index={index}
            count={group.works.length}
            onMove={(i, delta) => onMove(group, i, delta)}
            {...rowProps(work)}
          />
        ))}
      </ol>
      {group.works.length > shown && (
        <button onClick={() => setShown(shown + SHOWN)} className={BUTTON}>
          Mostrar mais {Math.min(SHOWN, group.works.length - shown)} de {group.works.length - shown}
        </button>
      )}
    </section>
  );
}

const SELECT = 'min-h-11 rounded-lg border border-border-hairline bg-surface px-3 text-sm text-ink';

/**
 * Says what the works of a collection are, all at once (#187, DEC-134): the unit (volume, chapter, one-shot) and whether it is a manga
 * or a comic. What is left as it is stays; with the box ticked only the works that have no value yet are changed.
 */
function ClassifyPanel({ collection, onDone }) {
  const classify = useClassifyCollection();
  const [unit, setUnit] = React.useState('');
  const [kind, setKind] = React.useState('');
  const [onlyUnset, setOnlyUnset] = React.useState(true);
  const [message, setMessage] = React.useState('');
  // '' leaves the field as it is, and "none" clears it.
  const value = (v) => (v === '' ? undefined : v === 'none' ? '' : v);
  const submit = (event) => {
    event.preventDefault();
    if (unit === '' && kind === '') return;
    setMessage('');
    classify.mutate(
      { id: collection.id, unit: value(unit), comicKind: value(kind), onlyUnset },
      {
        onSuccess: (done) => {
          const parts = [];
          if ('unit' in done.changed) parts.push(`a unidade de ${done.changed.unit}`);
          if ('comic_kind' in done.changed) parts.push(`o tipo de ${done.changed.comic_kind}`);
          setMessage(`Mudei ${parts.join(' e ')} ${parts.length === 1 && (done.changed.unit ?? done.changed.comic_kind) === 1 ? 'obra' : 'obras'}.`);
        },
        onError: (error) => setMessage(collectionReason(error, 'Não foi possível classificar as obras.')),
      }
    );
  };
  return (
    <form onSubmit={submit} aria-label="Classificar as obras" className="flex flex-wrap items-end gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <label className="flex flex-col gap-1 text-sm text-ink-soft">
        Quadrinho ou mangá
        <select className={SELECT} value={kind} onChange={(e) => setKind(e.target.value)} disabled={classify.isPending}>
          <option value="">Deixar como está</option>
          {COMIC_KINDS.map((k) => <option key={k.key} value={k.key}>{k.one}</option>)}
          <option value="none">Tirar o tipo</option>
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm text-ink-soft">
        Unidade
        <select className={SELECT} value={unit} onChange={(e) => setUnit(e.target.value)} disabled={classify.isPending}>
          <option value="">Deixar como está</option>
          {UNITS.map((u) => <option key={u.key} value={u.key}>{u.one}</option>)}
          <option value="none">Tirar a unidade</option>
        </select>
      </label>
      <label className="flex min-h-11 items-center gap-2 text-sm text-ink-soft">
        <input type="checkbox" checked={onlyUnset} onChange={(e) => setOnlyUnset(e.target.checked)} disabled={classify.isPending} />
        Só as que ainda não têm
      </label>
      <button type="submit" disabled={classify.isPending || (unit === '' && kind === '')} className={PRIMARY}>Aplicar</button>
      <button type="button" onClick={onDone} className={BUTTON}>Fechar</button>
      <p className="basis-full text-xs text-ink-faint">Vale para as obras da coleção que não estão na lixeira. Cada obra se corrige à parte na edição dela.</p>
      {message && <p role="status" className="basis-full text-sm text-ink">{message}</p>}
    </form>
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
  const openBook = useGlobalStore((state) => state.openBook);
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
  const goOn = data?.continue ?? null; // where to go on in a series: only an official collection says it (#187)
  const busy = order.isPending || remove.isPending || retire.isPending || restore.isPending;
  const fail = (fallback) => (err) => setMessage(collectionReason(err, fallback));
  // An official collection shows its works in groups by unit (#187); a list of the person is one row of places.
  const official = kind === 'official';
  const { groups, headings } = official ? groupByUnit(works) : { groups: [{ key: '', heading: '', works }], headings: false };
  const move = (group, index, delta) => {
    const items = [...group.works];
    [items[index], items[index + delta]] = [items[index + delta], items[index]];
    setMessage('');
    order.mutate({ id, items, kind, unit: headings ? group.key : undefined }, { onError: fail('Não foi possível mudar a ordem.') });
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
          <div className="flex shrink-0 items-center gap-1">
            {collection && !collection.retired && (
              <CollectionFavoriteButton
                collection={collection}
                className="library-favorite flex min-h-11 min-w-11 items-center justify-center rounded-lg text-ink-soft hover:bg-surface-alt hover:text-brand"
              />
            )}
            <button ref={closeRef} onClick={close} className="flex min-h-11 min-w-11 items-center justify-center rounded-lg text-xl text-ink-soft hover:bg-surface-alt hover:text-brand" aria-label="Fechar">✕</button>
          </div>
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
                    {official && <button onClick={() => setMode(mode === 'classify' ? null : 'classify')} aria-pressed={mode === 'classify'} className={BUTTON}>Classificar obras</button>}
                    <button onClick={() => setMode(mode === 'retire' ? null : 'retire')} aria-pressed={mode === 'retire'} className={BUTTON}>Aposentar</button>
                  </div>
                )}
                {staff && collection.retired && (
                  <button onClick={() => restore.mutate({ id, kind }, { onError: fail('Não foi possível restaurar.') })} disabled={busy} className={PRIMARY}>Restaurar a {words.thing}</button>
                )}
              </div>

              {goOn && !collection.retired && (
                <div>
                  <button onClick={() => openBook(goOn.id)} className={PRIMARY} title={goOn.title}>
                    {goOn.started ? 'Continuar' : goOn.begun ? 'Próximo' : 'Começar'}: {stepText(goOn)}
                  </button>
                </div>
              )}

              {staff && mode === 'rename' && <RenameForm collection={collection} onDone={() => setMode(null)} />}
              {staff && mode === 'add' && <AddWorkPanel collection={collection} members={works} onDone={() => setMode(null)} />}
              {staff && official && mode === 'classify' && <ClassifyPanel collection={collection} onDone={() => setMode(null)} />}
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
                <div className="flex flex-col gap-5">
                  {groups.map((group) => (
                    <WorkGroup
                      key={group.key || 'all'}
                      group={group}
                      heading={headings ? group.heading : null}
                      words={words}
                      official={official}
                      onMove={move}
                      rowProps={(work) => ({
                        staff: staff && !collection.retired,
                        confirming: removing === work.entryId,
                        busy,
                        words,
                        onOpen: (w) => openWork(w.id),
                        onAskRemove: setRemoving,
                        onRemove: take,
                        onCancel: () => setRemoving(null),
                      })}
                    />
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
