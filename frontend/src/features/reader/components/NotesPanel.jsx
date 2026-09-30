import React, { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import { downloadFile } from '../../../lib/download';
import { useCreateNote, useDeleteNote, useUpdateNote, useWorkNotes } from '../api/useWorkNotes';
import { parseTags, placeLabel } from '../files';

const KIND_LABEL = { note: 'Nota', highlight: 'Destaque', bookmark: 'Marcador' };

// What the server said, when it said something a person can act on ("a tag has at most 40
// characters"); a generic line otherwise.
function reason(error, fallback) {
  const text = error?.response?.data;
  return typeof text === 'string' && text.trim() && error.response.status < 500 ? text.trim() : fallback;
}

const fieldClass =
  'w-full rounded-lg border border-border-hairline bg-white px-3 py-2.5 font-body text-sm text-ink placeholder:text-ink-faint outline-none focus:border-brand focus:ring-2 focus:ring-brand/15';

/** The fields of a note: the passage kept from the book, the person's own words, tags. */
function NoteFields({ quote, body, tags, onChange }) {
  return (
    <div className="flex flex-col gap-2">
      <textarea
        value={quote}
        onChange={(e) => onChange({ quote: e.target.value })}
        placeholder="Trecho do livro (opcional)"
        aria-label="Trecho do livro"
        rows={2}
        className={fieldClass}
      />
      <textarea
        value={body}
        onChange={(e) => onChange({ body: e.target.value })}
        placeholder="Sua anotação, em Markdown (opcional)"
        aria-label="Sua anotação"
        rows={3}
        className={fieldClass}
      />
      <input
        value={tags}
        onChange={(e) => onChange({ tags: e.target.value })}
        placeholder="Tags, separadas por vírgula"
        aria-label="Tags"
        className={fieldClass}
      />
    </div>
  );
}

function NewNote({ workId, fileId, getLocator }) {
  const create = useCreateNote(workId);
  const [draft, setDraft] = useState({ quote: '', body: '', tags: '' });
  const locator = getLocator();
  const empty = !draft.quote.trim() && !draft.body.trim();

  const place = { ...(fileId && locator ? { fileId, locator } : {}) };

  const save = () =>
    create.mutate(
      {
        kind: draft.body.trim() ? 'note' : 'highlight',
        quote: draft.quote,
        body: draft.body,
        tags: parseTags(draft.tags),
        ...place,
      },
      { onSuccess: () => setDraft({ quote: '', body: '', tags: '' }) }
    );

  const bookmark = () => create.mutate({ kind: 'bookmark', ...place });

  return (
    <section aria-label="Nova anotação" className="rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <div className="mb-3 flex items-center justify-between gap-2">
        <h3 className="font-display text-xl text-ink">Nova anotação</h3>
        <span className="rounded-md bg-surface-alt px-2 py-1 font-mono text-[10px] uppercase tracking-wide text-ink-soft">Só sua</span>
      </div>
      <NoteFields {...draft} onChange={(change) => setDraft((d) => ({ ...d, ...change }))} />
      <p className="mt-2 font-mono text-[11px] text-ink-soft">
        {locator ? `Fica ligada a: ${placeLabel(locator)}` : 'Sem ponto do arquivo ainda: avance uma página para ligar a nota a ele.'}
      </p>
      {create.isError && <p className="mt-2 text-xs text-red-700">{reason(create.error, 'Não foi possível salvar.')}</p>}
      <div className="mt-3 flex justify-between gap-2">
        <button
          onClick={bookmark}
          disabled={create.isPending || !locator || !fileId}
          className="min-h-10 rounded-lg border border-border-hairline bg-surface-alt px-3 py-2 text-xs font-medium text-ink-soft hover:bg-[#e5ddd1] disabled:opacity-40"
          title="Guarda só este ponto do arquivo"
        >
          🔖 Marcar aqui
        </button>
        <button
          onClick={save}
          disabled={create.isPending || empty}
          className="min-h-10 rounded-lg bg-brand px-3 py-2 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40"
        >
          Salvar nota
        </button>
      </div>
    </section>
  );
}

function NoteItem({ note, onOpenAt }) {
  const update = useUpdateNote();
  const remove = useDeleteNote();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [draft, setDraft] = useState({ quote: note.quote, body: note.body, tags: note.tags.join(', ') });
  const place = placeLabel(note.locator);

  const save = () =>
    update.mutate(
      { id: note.id, quote: draft.quote, body: draft.body, tags: parseTags(draft.tags) },
      { onSuccess: () => setEditing(false) }
    );

  return (
    <li className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm" aria-label={`${KIND_LABEL[note.kind]} ${note.id}`}>
      <div className="flex items-center justify-between gap-2 font-mono text-[11px] text-ink-soft">
        <span className="rounded-md bg-brand/10 px-2 py-1 uppercase tracking-wide text-brand">{KIND_LABEL[note.kind]}</span>
        <span>{place ?? new Date(note.createdAt).toLocaleDateString('pt-BR')}</span>
      </div>

      {editing ? (
        <>
          <NoteFields {...draft} onChange={(change) => setDraft((d) => ({ ...d, ...change }))} />
          {update.isError && <p className="text-xs text-red-700">{reason(update.error, 'Não foi possível salvar.')}</p>}
          <div className="flex justify-end gap-2">
            <button onClick={() => setEditing(false)} className="min-h-10 px-3 py-2 text-xs text-ink-soft hover:text-ink">
              Cancelar
            </button>
            <button
              onClick={save}
              disabled={update.isPending}
              className="min-h-10 rounded-lg bg-brand px-3 py-2 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40"
            >
              Salvar
            </button>
          </div>
        </>
      ) : (
        <>
          {note.quote && <blockquote className="border-l-2 border-brand pl-3 font-display text-base italic leading-relaxed text-ink">{note.quote}</blockquote>}
          {/* Markdown, shown as text: raw HTML in it is not interpreted. */}
          {note.body && (
            <div className="prose prose-sm max-w-none rounded-lg bg-[#f5f0e9] p-3 font-body text-sm text-ink [&_a]:text-brand">
              <ReactMarkdown>{note.body}</ReactMarkdown>
            </div>
          )}
          {note.tags.length > 0 && (
            <p className="flex flex-wrap gap-1">
              {note.tags.map((tag) => (
                <span key={tag} className="rounded-full bg-surface-alt px-2 py-1 font-mono text-[11px] text-ink-soft">
                  #{tag}
                </span>
              ))}
            </p>
          )}
          <div className="flex flex-wrap items-center gap-3 border-t border-border-hairline pt-3 text-xs">
            {note.fileId && note.locator && note.fileAvailable && (
              <button onClick={() => onOpenAt(note)} className="min-h-9 rounded-md bg-brand/10 px-2.5 font-semibold text-brand hover:bg-brand/20">
                Abrir neste ponto
              </button>
            )}
            {note.locator && !note.fileAvailable && <span className="text-ink-faint">arquivo indisponível</span>}
            <button onClick={() => setEditing(true)} className="ml-auto min-h-9 text-ink-soft hover:text-ink">
              Editar
            </button>
            {confirming ? (
              <button
                onClick={() => remove.mutate(note.id)}
                disabled={remove.isPending}
                className="min-h-9 font-semibold text-red-700 hover:text-red-900"
              >
                Confirmar exclusão
              </button>
            ) : (
              <button onClick={() => setConfirming(true)} className="min-h-9 text-ink-soft hover:text-red-700">
                Excluir
              </button>
            )}
          </div>
        </>
      )}
    </li>
  );
}

/**
 * The person's marginalia for the book being read (RF-016, RF-017): notes, highlights and
 * bookmarks, tied to the exact place in the file when there is one. Nothing here is shown to
 * anyone else. The text of a note is Markdown and is rendered without HTML.
 */
export function NotesPanel({ workId, fileId, getLocator, onOpenAt, onClose }) {
  const { data, isLoading, isError } = useWorkNotes(workId);
  const notes = data?.data ?? [];
  const total = data?.total ?? notes.length;
  const [exportError, setExportError] = useState(false);
  const [filter, setFilter] = useState('all');
  const closeButtonRef = useRef(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const filters = [
    ['all', 'Todas'], ['note', 'Notas'], ['highlight', 'Destaques'], ['bookmark', 'Marcadores'],
  ];
  const filteredNotes = filter === 'all' ? notes : notes.filter((note) => note.kind === filter);

  useEffect(() => {
    const previousFocus = document.activeElement;
    closeButtonRef.current?.focus();
    const closeOnEscape = (event) => {
      if (event.key === 'Escape') {
        event.stopPropagation();
        onCloseRef.current();
      }
    };
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('keydown', closeOnEscape);
      if (previousFocus?.isConnected) previousFocus.focus();
    };
  }, []);

  const exportAs = async (format) => {
    setExportError(false);
    try {
      await downloadFile(`/notes/export?format=${format}&workId=${workId}`, `codice-anotacoes.${format}`);
    } catch {
      setExportError(true);
    }
  };

  return (
    <aside
      aria-label="Anotações"
      // Above the toolbars of every viewer (they are z-50): on a phone the panel covers the page, and
      // a toolbar drawn over its fields hides them.
      className="fixed inset-y-0 right-0 z-[60] flex w-full max-w-lg flex-col border-l border-border-hairline bg-[#faf8f4] font-body shadow-2xl"
    >
      <header className="flex shrink-0 items-start justify-between gap-4 border-b border-border-hairline bg-[#faf8f4] px-4 py-4 sm:px-5">
        <div>
          <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-brand">À margem da leitura</p>
          <h2 className="mt-1 font-display text-3xl leading-none text-ink">Suas anotações</h2>
          <p className="mt-1 text-xs text-ink-soft">
            {total > notes.length
              ? `${notes.length} de ${total} registros carregados`
              : `${notes.length} ${notes.length === 1 ? 'registro privado' : 'registros privados'}`}
          </p>
        </div>
        <button
          ref={closeButtonRef}
          onClick={onClose}
          className="flex min-h-11 min-w-11 items-center justify-center rounded-lg bg-surface-alt text-lg text-ink-soft hover:bg-border-hairline hover:text-ink"
          aria-label="Fechar anotações"
        >
          ✕
        </button>
      </header>

      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-4 py-5 sm:px-5">
        <NewNote workId={workId} fileId={fileId} getLocator={getLocator} />

        <section aria-label="Anotações salvas">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="font-display text-xl text-ink">Marginalia do livro</h3>
            <span className="rounded-md bg-surface-alt px-2 py-1 font-mono text-[11px] text-ink-soft">{notes.length}</span>
          </div>
          <div className="mb-4 flex flex-wrap gap-2" role="group" aria-label="Filtrar anotações por tipo">
            {filters.map(([kind, label]) => (
              <button
                key={kind}
                onClick={() => setFilter(kind)}
                aria-pressed={filter === kind}
                className={`min-h-9 rounded-full px-3 py-1.5 text-xs font-medium transition-colors ${filter === kind ? 'bg-brand text-white' : 'bg-surface-alt text-ink-soft hover:bg-border-hairline hover:text-ink'}`}
              >
                {label} ({kind === 'all' ? notes.length : notes.filter((note) => note.kind === kind).length})
              </button>
            ))}
          </div>

          {isLoading && <p className="animate-pulse text-sm text-ink-soft">Carregando…</p>}
          {isError && <p className="text-sm text-red-700">Não foi possível carregar as anotações.</p>}
          {!isLoading && !isError && notes.length === 0 && (
            <p className="rounded-xl border border-dashed border-border-hairline bg-white p-4 text-sm text-ink-soft">Nenhuma anotação neste livro ainda. Só você as vê.</p>
          )}
          {!isLoading && !isError && notes.length > 0 && filteredNotes.length === 0 && (
            <p className="rounded-xl border border-dashed border-border-hairline bg-white p-4 text-sm text-ink-soft">Nenhum registro desse tipo neste livro.</p>
          )}
          <ul className="flex flex-col gap-3">
            {filteredNotes.map((note) => (
              <NoteItem key={note.id} note={note} onOpenAt={onOpenAt} />
            ))}
          </ul>
        </section>
      </div>

      {notes.length > 0 && (
        <footer className="flex shrink-0 flex-col gap-1 border-t border-border-hairline bg-[#faf8f4] px-4 py-3 sm:px-5">
          <div className="flex flex-wrap items-center gap-3 text-xs text-ink-soft">
            <span>Exportar deste livro:</span>
            <button onClick={() => exportAs('md')} className="min-h-9 font-semibold text-brand hover:underline">
              Markdown
            </button>
            <button onClick={() => exportAs('json')} className="min-h-9 font-semibold text-brand hover:underline">
              JSON
            </button>
          </div>
          {exportError && <p className="text-xs text-red-700">Não foi possível exportar.</p>}
        </footer>
      )}
    </aside>
  );
}
