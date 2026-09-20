import React, { useState } from 'react';
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
  'w-full rounded-md bg-zinc-900 border border-zinc-800 text-zinc-200 text-sm p-2 outline-none focus:border-zinc-600';

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
    <div className="flex flex-col gap-2 border-b border-zinc-800 pb-4">
      <NoteFields {...draft} onChange={(change) => setDraft((d) => ({ ...d, ...change }))} />
      <p className="text-[11px] text-zinc-500">
        {locator ? `Fica ligada a: ${placeLabel(locator)}` : 'Sem ponto do arquivo ainda: avance uma página para ligar a nota a ele.'}
      </p>
      {create.isError && <p className="text-xs text-red-400">{reason(create.error, 'Não foi possível salvar.')}</p>}
      <div className="flex justify-between gap-2">
        <button
          onClick={bookmark}
          disabled={create.isPending || !locator || !fileId}
          className="rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-40"
          title="Guarda só este ponto do arquivo"
        >
          🔖 Marcar aqui
        </button>
        <button
          onClick={save}
          disabled={create.isPending || empty}
          className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-40"
        >
          Salvar nota
        </button>
      </div>
    </div>
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
    <li className="flex flex-col gap-2 rounded-lg border border-zinc-800 bg-zinc-950 p-3" aria-label={`${KIND_LABEL[note.kind]} ${note.id}`}>
      <div className="flex items-center justify-between text-[11px] text-zinc-500">
        <span className="rounded bg-zinc-800 px-1.5 py-0.5 uppercase tracking-wide text-zinc-400">{KIND_LABEL[note.kind]}</span>
        <span>{place ?? new Date(note.createdAt).toLocaleDateString('pt-BR')}</span>
      </div>

      {editing ? (
        <>
          <NoteFields {...draft} onChange={(change) => setDraft((d) => ({ ...d, ...change }))} />
          {update.isError && <p className="text-xs text-red-400">{reason(update.error, 'Não foi possível salvar.')}</p>}
          <div className="flex justify-end gap-2">
            <button onClick={() => setEditing(false)} className="px-3 py-1.5 text-xs text-zinc-400 hover:text-zinc-200">
              Cancelar
            </button>
            <button
              onClick={save}
              disabled={update.isPending}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-40"
            >
              Salvar
            </button>
          </div>
        </>
      ) : (
        <>
          {note.quote && <blockquote className="border-l-2 border-zinc-700 pl-3 text-sm italic text-zinc-300">{note.quote}</blockquote>}
          {/* Markdown, shown as text: raw HTML in it is not interpreted. */}
          {note.body && (
            <div className="prose prose-invert prose-sm max-w-none text-sm text-zinc-200 [&_a]:text-blue-400">
              <ReactMarkdown>{note.body}</ReactMarkdown>
            </div>
          )}
          {note.tags.length > 0 && (
            <p className="flex flex-wrap gap-1">
              {note.tags.map((tag) => (
                <span key={tag} className="rounded-full bg-zinc-800 px-2 py-0.5 text-[11px] text-zinc-400">
                  #{tag}
                </span>
              ))}
            </p>
          )}
          <div className="flex items-center gap-3 text-xs">
            {note.fileId && note.locator && note.fileAvailable && (
              <button onClick={() => onOpenAt(note)} className="text-blue-400 hover:text-blue-300">
                Abrir neste ponto
              </button>
            )}
            {note.locator && !note.fileAvailable && <span className="text-zinc-600">arquivo indisponível</span>}
            <button onClick={() => setEditing(true)} className="ml-auto text-zinc-500 hover:text-zinc-300">
              Editar
            </button>
            {confirming ? (
              <button
                onClick={() => remove.mutate(note.id)}
                disabled={remove.isPending}
                className="text-red-400 hover:text-red-300"
              >
                Confirmar exclusão
              </button>
            ) : (
              <button onClick={() => setConfirming(true)} className="text-zinc-500 hover:text-red-400">
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
  const [exportError, setExportError] = useState(false);

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
      className="fixed inset-y-0 right-0 z-30 flex w-full max-w-md flex-col gap-4 overflow-y-auto border-l border-zinc-800 bg-zinc-900 p-5 shadow-2xl"
    >
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">Suas anotações</h2>
        <button onClick={onClose} className="text-zinc-500 hover:text-zinc-300" aria-label="Fechar anotações">
          ✕
        </button>
      </div>

      <NewNote workId={workId} fileId={fileId} getLocator={getLocator} />

      {isLoading && <p className="animate-pulse text-sm text-zinc-500">Carregando…</p>}
      {isError && <p className="text-sm text-red-400">Não foi possível carregar as anotações.</p>}
      {!isLoading && !isError && notes.length === 0 && (
        <p className="text-sm text-zinc-500">Nenhuma anotação neste livro ainda. Só você as vê.</p>
      )}
      <ul className="flex flex-col gap-3">
        {notes.map((note) => (
          <NoteItem key={note.id} note={note} onOpenAt={onOpenAt} />
        ))}
      </ul>

      {notes.length > 0 && (
        <div className="mt-auto flex flex-col gap-1 border-t border-zinc-800 pt-3">
          <div className="flex items-center gap-3 text-xs text-zinc-500">
            <span>Exportar as anotações deste livro:</span>
            <button onClick={() => exportAs('md')} className="text-blue-400 hover:text-blue-300">
              Markdown
            </button>
            <button onClick={() => exportAs('json')} className="text-blue-400 hover:text-blue-300">
              JSON
            </button>
          </div>
          {exportError && <p className="text-xs text-red-400">Não foi possível exportar.</p>}
        </div>
      )}
    </aside>
  );
}
