import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { WorkCover } from '../../../components/ui/WorkCover';
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
import { collectionLine, COMIC_KINDS, groupByUnit, numberText, goOnText, UNITS, unitLabel, wordsOf, worksText } from '../text';
import { CollectionFavoriteButton } from './CollectionFavoriteButton';
import { ProgressBar } from '../../../components/ui/ProgressBar';
import { formatReadingTime } from '../../home/utils/format';
import { placeLabel } from '../../reader/placeInWords';
import { WorkHighlights } from '../../library/components/WorkHighlights';
import { useCollectionNotes } from '../api/useCollectionNotes';

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

const DESCRIPTION_MAX = 2000;

/** What the collection is, in the person's words (DEC-163): a few lines under the name. Saving it empty takes it away. */
function DescribeForm({ collection, onDone }) {
  const [text, setText] = React.useState(collection.description ?? '');
  const [message, setMessage] = React.useState('');
  const save = useRenameCollection();
  const submit = (event) => {
    event.preventDefault();
    setMessage('');
    save.mutate(
      { id: collection.id, description: text, kind: collection.kind },
      { onSuccess: onDone, onError: (error) => setMessage(collectionReason(error, 'Não foi possível salvar a descrição.')) }
    );
  };
  return (
    <form onSubmit={submit} aria-label="Descrição da coleção" className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <label className="flex flex-col gap-1 text-sm text-ink-soft">
        Descrição
        <textarea
          autoFocus
          rows={4}
          value={text}
          maxLength={DESCRIPTION_MAX}
          onChange={(event) => setText(event.target.value)}
          className="rounded-lg border border-border-hairline bg-surface px-3 py-2 text-ink"
        />
      </label>
      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={save.isPending} className={PRIMARY}>Salvar</button>
        <button type="button" onClick={onDone} className={BUTTON}>Cancelar</button>
        <span className="font-mono text-[11px] text-ink-faint">{text.length} de {DESCRIPTION_MAX}</span>
      </div>
      {message && <p role="alert" className="text-sm text-danger">{message}</p>}
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

const STARS = (n) => '★'.repeat(n) + '☆'.repeat(5 - n);
const monthYear = new Intl.DateTimeFormat('pt-BR', { month: 'long', year: 'numeric' });

/** What the caller has done of a work of the collection, in a word: finished, in progress with how far, the next one, or nothing. */
function statusOf(work, nextId) {
  if (work.completed) return { text: 'Lida', tone: 'text-success bg-success/10' };
  if (work.started) return { text: `Em leitura ${Math.round(work.percent)}%`, tone: 'text-brand bg-brand/10' };
  if (nextId != null && work.id === nextId) return { text: 'Próxima da fila', tone: 'text-ink-soft bg-surface-alt' };
  return null;
}

function WorkRow({ work, label, index, count, staff, confirming, busy, words, rich, nextId, onOpen, onRead, onMove, onAskRemove, onRemove, onCancel }) {
  const gone = !work.available;
  const status = gone ? null : statusOf(work, nextId);
  const place = work.started ? placeLabel(work.readFormat, { unitIndex: work.unitIndex, unitTotal: work.unitTotal }) : null;
  return (
    <li className="flex flex-col gap-2 rounded-xl border border-border-hairline bg-white p-3 shadow-sm">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="w-14 shrink-0 text-center font-mono text-sm text-ink-soft" title={work.position == null ? 'Sem número' : `Número ${numberText(work.position)}`}>
          {label}
        </span>
        <button onClick={() => onOpen(work)} disabled={gone} className="flex min-w-0 flex-1 basis-[180px] items-center gap-3 text-left disabled:cursor-default" aria-label={gone ? undefined : `Abrir a obra ${work.title}`}>
          <WorkCover item={work} className={`${rich ? 'h-24 w-16' : 'h-16 w-11'} shrink-0 rounded-sm object-cover`} />
          <span className="min-w-0">
            <span className="line-clamp-2 block font-display text-lg leading-snug text-ink">{work.title}</span>
            <span className="block truncate text-xs text-ink-soft">{work.author}</span>
            {rich && !gone && (
              <span className="mt-1 flex flex-wrap items-center gap-x-2 font-mono text-[10px] text-ink-faint">
                {work.originalYear != null && <span>Publicado em {work.originalYear}</span>}
                {work.formats?.length > 0 && <span>{work.formats.map((f) => f.toUpperCase()).join(' · ')}</span>}
                {work.rating > 0 && <span aria-label={`${work.rating} de 5 estrelas`} title={`${work.rating} de 5 estrelas`}>{STARS(work.rating)}</span>}
                {work.completedAt && <span>Lida em {monthYear.format(new Date(work.completedAt))}</span>}
                {work.notes > 0 && <span>{work.notes} {work.notes === 1 ? 'anotação' : 'anotações'}</span>}
              </span>
            )}
          </span>
        </button>
        {gone && <span className="shrink-0 rounded-full bg-surface-alt px-2 py-0.5 font-mono text-[10px] text-ink-soft" title="A obra saiu do acervo: a lista guarda o que ela era">Fora do acervo</span>}
        {status && <span className={`shrink-0 rounded-full px-2 py-0.5 font-mono text-[10px] ${status.tone}`}>{status.text}</span>}
        {staff && (
          <div className="flex shrink-0 basis-full items-center justify-end gap-1 sm:basis-auto">
            <button onClick={() => onMove(index, -1)} disabled={busy || index === 0} className={BUTTON} aria-label={`Subir “${work.title}”`}>↑</button>
            <button onClick={() => onMove(index, 1)} disabled={busy || index === count - 1} className={BUTTON} aria-label={`Descer “${work.title}”`}>↓</button>
            <button onClick={() => onAskRemove(work.entryId)} disabled={busy} className={BUTTON} aria-label={`Tirar “${work.title}” da ${words.thing}`}>Tirar</button>
          </div>
        )}
      </div>
      {rich && !gone && work.synopsis && <p className="line-clamp-2 pl-[68px] text-sm text-ink-soft">{work.synopsis}</p>}
      {rich && !gone && work.started && (
        <div className="flex flex-wrap items-center gap-3 pl-[68px]">
          <div className="min-w-[160px] flex-1">
            <ProgressBar percent={work.percent} color="brand" />
            <p className="mt-1 font-mono text-[11px] text-ink-soft">
              {work.chapter ? `${work.chapter}` : ''}
              {work.chapter && place ? ' · ' : ''}
              {place ?? ''}
            </p>
          </div>
          <button onClick={() => onRead(work)} className={PRIMARY}>Retomar</button>
        </div>
      )}
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
export function CollectionPage() {
  const id = useGlobalStore((state) => state.collectionSheetId);
  const close = useGlobalStore((state) => state.closeCollection);
  const openWork = useGlobalStore((state) => state.openWork);
  const openBook = useGlobalStore((state) => state.openBook);
  const staffMember = isStaff(useMe().data);
  const openPerson = useGlobalStore((state) => state.openPerson);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const headingRef = React.useRef(null);
  const pageRef = React.useRef(null);
  const [rich, setRich] = React.useState(true); // "Cartões ricos" or "Lista compacta"
  const { data, isLoading, isError, error, refetch, isRefetching } = useCollection(id);
  // The notes are asked for only when the collection says there are some.
  const notes = useCollectionNotes(id, (data?.summary?.notes ?? 0) > 0).data?.data;
  const order = useOrderCollection();
  const remove = useRemoveFromCollection();
  const retire = useRetireCollection();
  const restore = useRestoreCollection();
  const [mode, setMode] = React.useState(null); // 'rename' | 'describe' | 'add' | 'classify' | 'retire'
  const [removing, setRemoving] = React.useState(null); // the place waiting for a yes
  const [message, setMessage] = React.useState('');
  const notesRef = React.useRef(null);

  React.useEffect(() => {
    setMode(null);
    setRemoving(null);
    setMessage('');
  }, [id]);

  // A page that opens starts at its top, with the focus on what it is about.
  React.useEffect(() => {
    if (!id) return;
    pageRef.current?.scrollIntoView?.({ block: 'start' });
    headingRef.current?.focus?.({ preventScroll: true });
  }, [id, !!data]);

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

  const summary = data?.summary ?? null;
  const openNote = (note) => openBook(note.workId, note.fileId, { locator: note.locator, context: { kind: 'note', quote: note.quote } });
  const goOnWork = goOn ? works.find((w) => w.id === goOn.id) : null;
  const years = summary?.yearFrom != null ? (summary.yearTo !== summary.yearFrom ? `${summary.yearFrom}–${summary.yearTo}` : String(summary.yearFrom)) : null;
  const missing = (summary?.missing ?? []).map((m) => unitLabel(m.unit, m.number));
  const read = (work) => openBook(work.id);

  return (
    <div ref={pageRef} className="library-dashboard" role="region" aria-label={words.dialog}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <nav aria-label="Onde você está" className="flex min-w-0 flex-wrap items-center gap-2 text-[13px] text-ink-soft">
          <button className="library-button" onClick={close}>← Voltar</button>
          <button className="library-text-link" style={{ marginLeft: 0 }} onClick={() => setView('all')}>Biblioteca</button>
          <span aria-hidden="true">›</span>
          <span className="text-ink-faint">{official ? 'Coleção' : 'Lista'}</span>
          <span aria-hidden="true">›</span>
          <span aria-current="page" className="min-w-0 truncate text-ink">{collection?.name ?? 'Carregando…'}</span>
        </nav>
        {collection && !collection.retired && (
          <CollectionFavoriteButton
            collection={collection}
            className="library-favorite flex min-h-11 min-w-11 items-center justify-center rounded-lg text-ink-soft hover:bg-surface-alt hover:text-brand"
          />
        )}
      </div>

      {isLoading && <p className="animate-pulse py-6 text-sm text-ink-faint">Carregando a coleção…</p>}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível abrir esta coleção.</LoadError>}
      {collection && (
        <div className="mt-4 flex flex-col gap-6">
          <section className="relative overflow-hidden rounded-2xl bg-surface-alt p-4 shadow-sm sm:p-6" aria-label="Sobre a coleção">
            <div className="relative grid gap-5 md:grid-cols-[minmax(140px,200px)_minmax(0,1fr)] md:gap-8">
              <div className="mx-auto w-32 md:mx-0 md:w-full">
                <div className="relative">
                  <WorkCover item={{ title: collection.name, coverUrl: collection.coverUrl }} className="aspect-[2/3] w-full rounded-lg object-cover shadow-lg" />
                  <span className="absolute left-1.5 top-1.5 rounded-sm bg-ink/90 px-1.5 py-1 font-mono text-[9px] uppercase text-white">{official ? 'Coleção' : 'Lista'}</span>
                </div>
              </div>
              <div className="min-w-0 space-y-4">
                <div>
                  <p className="font-mono text-[11px] uppercase tracking-widest text-brand">
                    {[years, collection.workCount > 0 ? worksText(collection.workCount) : null].filter(Boolean).join(' · ') || words.eyebrow}
                  </p>
                  <h1 ref={headingRef} tabIndex={-1} className="font-display text-4xl leading-tight text-ink outline-none sm:text-5xl">{collection.name}</h1>
                  {(summary?.authors?.length > 0 || summary?.translators?.length > 0) && (
                    <p className="mt-1 text-sm text-ink-soft">
                      {summary.authors.map((p, i) => (
                        <React.Fragment key={p.id}>
                          {i > 0 && ', '}
                          <button type="button" onClick={() => openPerson(p.id)} className="font-semibold text-brand hover:underline">{p.name}</button>
                        </React.Fragment>
                      ))}
                      {summary.translators.length > 0 && (
                        <>
                          {summary.authors.length > 0 && ' · '}Trad.:{' '}
                          {summary.translators.map((p, i) => (
                            <React.Fragment key={p.id}>
                              {i > 0 && ', '}
                              <button type="button" onClick={() => openPerson(p.id)} className="hover:underline">{p.name}</button>
                            </React.Fragment>
                          ))}
                        </>
                      )}
                    </p>
                  )}
                </div>
                {collection.description && <p className="max-w-2xl whitespace-pre-line text-sm leading-relaxed text-ink-soft">{collection.description}</p>}
                {summary?.tags?.length > 0 && (
                  <ul aria-label="Etiquetas" className="flex flex-wrap gap-x-3 gap-y-1">
                    {summary.tags.map((t) => <li key={t.name} className="font-mono text-[11px] text-ink-soft">#{t.name}</li>)}
                  </ul>
                )}
                {summary && summary.works > 0 && (
                  <div className="max-w-xl rounded-xl bg-white p-4 shadow-sm" aria-label="Seu progresso">
                    <div className="flex items-baseline justify-between gap-3">
                      <span className="font-mono text-[11px] uppercase tracking-widest text-ink-faint">{official ? 'Progresso na série' : 'Seu progresso'}</span>
                      <span className="font-display text-xl text-brand">{Math.round(summary.percent)}%</span>
                    </div>
                    <ProgressBar percent={summary.percent} color="brand" />
                    <p className="mt-2 font-mono text-[11px] text-ink-soft">
                      {summary.finished} de {summary.works} {summary.works === 1 ? 'lida' : 'lidas'}
                      {summary.inProgress > 0 ? ` · ${summary.inProgress} em andamento` : ''}
                      {summary.readingSeconds > 0 ? ` · ${formatReadingTime(summary.readingSeconds)} lidas` : ''}
                    </p>
                    {summary.notes > 0 && (
                      <p className="mt-1 font-mono text-[11px] text-ink-soft">
                        {summary.notes} {summary.notes === 1 ? 'anotação sua' : 'anotações suas'}
                        {' · '}
                        <button type="button" onClick={() => notesRef.current?.scrollIntoView?.({ block: 'start', behavior: 'smooth' })} className="text-brand underline decoration-dotted underline-offset-4 hover:text-ink">
                          Ver notas
                        </button>
                      </p>
                    )}
                  </div>
                )}
                {goOn && !collection.retired && (
                  <div>
                    <button onClick={() => openBook(goOn.id)} className={PRIMARY} title={goOn.title}>
                      {goOnText(goOn)}
                      {goOnWork?.started ? ` (${Math.round(goOnWork.percent)}%)` : ''}
                    </button>
                    {goOnWork?.started && goOnWork.chapter && <p className="mt-1 text-xs text-ink-soft">Parou em {goOnWork.chapter}</p>}
                  </div>
                )}
              </div>
            </div>
          </section>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-ink-soft">
              {collection.retired ? `${words.retired}. ` : ''}
              {collectionLine(collection)}
              {collection.system ? ' É uma lista do Códice: ela guarda o que você deixou para ler depois.' : ''}
            </p>
            {staff && !collection.retired && (
              <div className="flex flex-wrap items-center gap-2">
                {!collection.system && <button onClick={() => setMode(mode === 'rename' ? null : 'rename')} aria-pressed={mode === 'rename'} className={BUTTON}>Renomear</button>}
                {!collection.system && (
                  <button onClick={() => setMode(mode === 'describe' ? null : 'describe')} aria-pressed={mode === 'describe'} className={BUTTON}>
                    {collection.description ? 'Editar descrição' : 'Escrever descrição'}
                  </button>
                )}
                <button onClick={() => setMode(mode === 'add' ? null : 'add')} aria-pressed={mode === 'add'} className={BUTTON}>Acrescentar obra</button>
                {official && <button onClick={() => setMode(mode === 'classify' ? null : 'classify')} aria-pressed={mode === 'classify'} className={BUTTON}>Classificar obras</button>}
                {!collection.system && <button onClick={() => setMode(mode === 'retire' ? null : 'retire')} aria-pressed={mode === 'retire'} className={BUTTON}>Aposentar</button>}
              </div>
            )}
            {staff && collection.retired && (
              <button onClick={() => restore.mutate({ id, kind }, { onError: fail('Não foi possível restaurar.') })} disabled={busy} className={PRIMARY}>Restaurar a {words.thing}</button>
            )}
          </div>

          {missing.length > 0 && !collection.retired && (
            <p role="status" className="rounded-lg border border-border-hairline bg-white px-4 py-2 text-sm text-ink-soft">
              {missing.length === 1 ? `Falta no acervo: ${missing[0]}.` : `Faltam no acervo: ${missing.join(', ')}.`}
            </p>
          )}

          {staff && mode === 'rename' && <RenameForm collection={collection} onDone={() => setMode(null)} />}
          {staff && mode === 'describe' && <DescribeForm collection={collection} onDone={() => setMode(null)} />}
          {staff && mode === 'add' && <AddWorkPanel collection={collection} members={works} onDone={() => setMode(null)} />}
          {staff && official && mode === 'classify' && <ClassifyPanel collection={collection} onDone={() => setMode(null)} />}
          {staff && mode === 'retire' && (
            <div role="alertdialog" aria-label={`Aposentar a ${words.thing}`} className="flex flex-wrap items-center gap-3 rounded-xl border border-border-hairline bg-white p-4 text-sm text-ink-soft">
              <span className="min-w-[220px] flex-1">
                Aposentar “{collection.name}”? {words.retireNote}
              </span>
              <button onClick={retireIt} disabled={busy} className={PRIMARY}>Aposentar</button>
              <button onClick={() => setMode(null)} className={BUTTON}>Cancelar</button>
            </div>
          )}
          {message && <p role="alert" className="text-sm text-danger">{message}</p>}

          {works.length > 0 && (
            <div role="group" aria-label="Como ver as obras" className="flex items-center gap-2 text-xs text-ink-soft">
              Exibir:
              <button type="button" aria-pressed={rich} onClick={() => setRich(true)} className={BUTTON}>Cartões ricos</button>
              <button type="button" aria-pressed={!rich} onClick={() => setRich(false)} className={BUTTON}>Lista compacta</button>
            </div>
          )}

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
                    rich,
                    nextId: goOn?.id ?? null,
                    onOpen: (w) => openWork(w.id),
                    onRead: read,
                    onAskRemove: setRemoving,
                    onRemove: take,
                    onCancel: () => setRemoving(null),
                  })}
                />
              ))}
            </div>
          )}

          {summary?.notes > 0 && (
            <div ref={notesRef} className="scroll-mt-4">
              <WorkHighlights notes={notes} onOpen={openNote} title={`Suas anotações ${official ? 'nesta coleção' : 'nesta lista'}`} showWork />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
