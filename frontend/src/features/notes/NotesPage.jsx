import React, { useEffect, useState } from 'react';
import { LoadError } from '../../components/ui/LoadError';
import { useGlobalStore } from '../../store/useGlobalStore';
import { NoteItem } from '../reader/components/NoteItem';
import { ExportNotes } from './components/ExportNotes';
import { NotesFacets } from './components/NotesFacets';
import { Skeleton } from '../../components/ui/Skeleton';
import { useNotesFacets } from './api/useNotesFacets';
import { PAGE_SIZE, useNotesList } from './api/useNotesList';

/**
 * Every note, highlight and bookmark of the person, from every work (#13, RF-016, RF-018, UI-05): newest first,
 * narrowed by text, kind, tag and work, with the reference of each and a way back to the place in the file. A
 * panel beside the list says how many notes each kind and each tag has, under the other filters.
 * Notes of a work that left the library stay, with their text and the source marked unavailable. Nothing here
 * is shown to anyone else.
 */
export function NotesPage() {
  const openBook = useGlobalStore((state) => state.openBook);
  const [text, setText] = useState('');
  const [kind, setKind] = useState('');
  const [tag, setTag] = useState('');
  const [work, setWork] = useState(null); // { id, title }
  const [page, setPage] = useState(1);
  const [exporting, setExporting] = useState(false);
  // What is searched follows what is typed after a pause, but is cleared at once.
  const [q, setQ] = useState('');
  useEffect(() => {
    const timer = setTimeout(() => setQ(text), 300);
    return () => clearTimeout(timer);
  }, [text]);

  // A different filter starts again from the first page.
  useEffect(() => setPage(1), [q, kind, tag, work?.id]);

  const filters = { q, kind, tag, workId: work?.id };
  const { data, isLoading, isError, isFetching, refetch, isRefetching } = useNotesList(filters, page);
  const { data: facets } = useNotesFacets(filters);
  const notes = data?.data ?? [];
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  // Deleting the last note of a page leaves nothing to show there: go back to the last page that has some.
  useEffect(() => {
    if (data && notes.length === 0 && total > 0 && page > pages) setPage(pages);
  }, [data, notes.length, total, page, pages]);

  const filtered = !!(q.trim() || kind || tag || work);
  const clear = () => {
    setText('');
    setQ('');
    setKind('');
    setTag('');
    setWork(null);
  };
  const first = total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
  const last = Math.min(page * PAGE_SIZE, total);

  return (
    <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6" aria-busy={isFetching}>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-brand">À margem da leitura</p>
          <h1 className="font-display text-3xl text-ink">Suas anotações</h1>
          <p className="text-xs text-ink-soft">
            {isLoading ? 'Carregando…' : total === 1 ? '1 registro privado' : `${total} registros privados`}
            {filtered && !isLoading ? ' com estes filtros' : ''}
          </p>
        </div>
        <button
          onClick={() => setExporting(true)}
          disabled={isLoading || total === 0}
          className="min-h-10 rounded-lg border border-border-hairline bg-white px-4 py-2 text-xs font-medium text-ink hover:bg-surface-alt disabled:opacity-40"
        >
          Exportar…
        </button>
      </div>

      <div className="mt-4 flex flex-col gap-3">
        <input
          type="search"
          value={text}
          onChange={(event) => setText(event.target.value)}
          placeholder="Buscar no trecho, no seu texto, no título e nas tags"
          aria-label="Buscar nas anotações"
          maxLength={200}
          className="w-full rounded-lg border border-border-hairline bg-white px-3 py-2.5 font-body text-sm text-ink placeholder:text-ink-faint outline-none focus:border-brand focus:ring-2 focus:ring-brand/15"
        />
        {(work || tag) && (
          <div className="flex flex-wrap items-center gap-2 text-xs" aria-label="Filtros ativos">
            {work && (
              <button onClick={() => setWork(null)} className="rounded-full bg-brand/10 px-3 py-1.5 text-brand hover:bg-brand/20" aria-label={`Tirar o filtro da obra ${work.title}`}>
                Obra: {work.title} ✕
              </button>
            )}
            {tag && (
              <button onClick={() => setTag('')} className="rounded-full bg-brand/10 px-3 py-1.5 text-brand hover:bg-brand/20" aria-label={`Tirar o filtro da tag ${tag}`}>
                Tag: #{tag} ✕
              </button>
            )}
          </div>
        )}
        {filtered && (
          <button onClick={clear} className="self-start text-xs text-ink-soft underline hover:text-brand">
            Limpar filtros
          </button>
        )}
      </div>

      <div className="mt-5 grid gap-6 lg:grid-cols-[14rem_minmax(0,1fr)]">
        <aside aria-label="Filtros das anotações" className="lg:sticky lg:top-4 lg:self-start">
          <NotesFacets facets={facets} kind={kind} onKind={setKind} tag={tag} onTag={setTag} />
        </aside>
        <div>
        {isLoading && (
          <div className="flex flex-col gap-3" aria-label="Carregando as anotações">
            <Skeleton className="h-28" />
            <Skeleton className="h-28" />
            <Skeleton className="h-28" />
          </div>
        )}
        {isError && <LoadError onRetry={refetch} retrying={isRefetching}>Não foi possível carregar as anotações.</LoadError>}
        {!isLoading && !isError && notes.length === 0 && (
          <p className="rounded-xl border border-dashed border-border-hairline bg-white p-4 text-sm text-ink-soft">
            {filtered
              ? 'Nenhuma anotação com estes filtros.'
              : 'Nenhuma anotação ainda. Use o botão “Notas” no leitor para guardar trechos, destaques e marcadores. Só você os vê.'}
          </p>
        )}
        <ul className="flex flex-col gap-3">
          {notes.map((note) => (
            <NoteItem
              key={note.id}
              note={note}
              showSource
              onFilterWork={(n) => setWork({ id: n.workId, title: n.workTitle })}
              onFilterTag={setTag}
              onOpenAt={(n) => openBook(n.workId, n.fileId, { locator: n.locator, context: { kind: 'note', quote: n.quote } })}
            />
          ))}
        </ul>

        {total > PAGE_SIZE && (
          <nav className="mt-5 flex items-center justify-between gap-3 text-xs text-ink-soft" aria-label="Páginas de anotações">
            <button onClick={() => setPage((p) => p - 1)} disabled={page <= 1} className="min-h-10 rounded-lg border border-border-hairline bg-white px-4 py-2 hover:bg-surface-alt disabled:opacity-40">
              Anterior
            </button>
            <span>{first}–{last} de {total}</span>
            <button onClick={() => setPage((p) => p + 1)} disabled={page >= pages} className="min-h-10 rounded-lg border border-border-hairline bg-white px-4 py-2 hover:bg-surface-alt disabled:opacity-40">
              Próxima
            </button>
          </nav>
        )}
        </div>
      </div>

      {exporting && <ExportNotes filters={{ q, kind, tag, work }} total={total} onClose={() => setExporting(false)} />}
    </div>
  );
}
