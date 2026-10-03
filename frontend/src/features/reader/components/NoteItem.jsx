import React, { useState } from 'react';
import { NoteMarkdown } from '../../notes/components/NoteMarkdown';
import { useDeleteNote, useUpdateNote } from '../api/useWorkNotes';
import { parseTags, placeLabel } from '../files';
import { KIND_LABEL, reason } from '../noteText';
import { HIGHLIGHT_COLORS, highlightColor } from '../highlightColors';

const fieldClass =
  'w-full rounded-lg border border-border-hairline bg-white px-3 py-2.5 font-body text-sm text-ink placeholder:text-ink-faint outline-none focus:border-brand focus:ring-2 focus:ring-brand/15';

/** The fields of a note: the passage kept from the book, the person's own words, tags. */
export function NoteFields({ quote, body, tags, onChange }) {
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
        placeholder="Sua anotação, em Markdown (opcional). [[Conceito]] liga a um conceito."
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

/** The four colors of a passage, to choose one of. */
export function ColorChoice({ value, onChange }) {
  return (
    <div role="radiogroup" aria-label="Cor do destaque" className="flex items-center gap-1">
      {HIGHLIGHT_COLORS.map((c) => (
        <button
          key={c.id}
          type="button"
          role="radio"
          aria-checked={c.id === value}
          aria-label={c.name}
          title={c.name}
          onClick={() => onChange(c.id)}
          className="flex size-11 items-center justify-center rounded-full"
        >
          <span
            className="block size-6 rounded-full"
            style={{ backgroundColor: c.hex, boxShadow: c.id === value ? `0 0 0 2px #fff, 0 0 0 4px ${c.hex}` : undefined }}
          />
        </button>
      ))}
    </div>
  );
}

/**
 * One note, highlight or bookmark, with editing and deleting. In a list of every note (`showSource`) it also says
 * which work it is from, which a click narrows the list to, and says so when that work has left the library.
 * `onFilterTag` makes the tags buttons that narrow the list to them.
 */
export function NoteItem({ note, onOpenAt, showSource = false, onFilterWork, onFilterTag }) {
  const update = useUpdateNote();
  const remove = useDeleteNote();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [draft, setDraft] = useState({ quote: note.quote, body: note.body, tags: note.tags.join(', '), color: highlightColor(note.color).id });
  const place = placeLabel(note.locator);
  // A passage that is painted: a highlight, or a note on a passage. A bookmark has no color to choose.
  const painted = note.kind !== 'bookmark';
  const color = highlightColor(note.color);

  const save = () =>
    update.mutate(
      { id: note.id, quote: draft.quote, body: draft.body, tags: parseTags(draft.tags), ...(painted && draft.color !== color.id ? { color: draft.color } : {}) },
      { onSuccess: () => setEditing(false) }
    );

  return (
    <li className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm" aria-label={`${KIND_LABEL[note.kind]} ${note.id}`}>
      <div className="flex items-center justify-between gap-2 font-mono text-[11px] text-ink-soft">
        <span className="flex items-center gap-1.5 rounded-md bg-brand/10 px-2 py-1 uppercase tracking-wide text-brand">
          {KIND_LABEL[note.kind]}
          {painted && <span role="img" aria-label={`Cor: ${color.name}`} title={color.name} className="block size-2.5 rounded-full" style={{ backgroundColor: color.hex }} />}
        </span>
        <span>{place ?? new Date(note.createdAt).toLocaleDateString('pt-BR')}</span>
      </div>

      {editing ? (
        <>
          <NoteFields {...draft} onChange={(change) => setDraft((d) => ({ ...d, ...change }))} />
          {painted && <ColorChoice value={draft.color} onChange={(c) => setDraft((d) => ({ ...d, color: c }))} />}
          {update.isError && <p className="text-xs text-danger">{reason(update.error, 'Não foi possível salvar.')}</p>}
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
          {showSource && (
            <p className="text-xs text-ink-soft">
              {note.sourceAvailable && note.workId && onFilterWork ? (
                <button onClick={() => onFilterWork(note)} className="font-semibold text-ink hover:text-brand" title="Ver só as anotações desta obra">
                  {note.workTitle}
                </button>
              ) : (
                <span className="font-semibold text-ink">{note.workTitle}</span>
              )}
              {note.workAuthor && <span> · {note.workAuthor}</span>}
              {!note.sourceAvailable && (
                <span className="ml-2 rounded bg-warning-soft/50 px-1.5 py-0.5 font-mono text-[10px] uppercase text-warning" title="A obra saiu do acervo; o texto da anotação ficou">
                  fonte indisponível
                </span>
              )}
            </p>
          )}
          {note.quote && (
            <blockquote className="border-l-2 border-brand pl-3 font-display text-base italic leading-relaxed text-ink" style={{ borderColor: painted ? color.hex : undefined }}>
              {note.quote}
            </blockquote>
          )}
          {/* Markdown, shown as text: raw HTML in it is not interpreted. [[Concept]] links are marked. */}
          {note.body && (
            <div className="prose prose-sm max-w-none rounded-lg bg-[#f5f0e9] p-3 font-body text-sm text-ink [&_a]:text-brand">
              <NoteMarkdown body={note.body} links={note.links} />
            </div>
          )}
          {note.tags.length > 0 && (
            <p className="flex flex-wrap gap-1">
              {note.tags.map((tag) =>
                onFilterTag ? (
                  <button
                    key={tag}
                    onClick={() => onFilterTag(tag)}
                    className="rounded-full bg-surface-alt px-2 py-1 font-mono text-[11px] text-ink-soft hover:bg-border-hairline hover:text-ink"
                    title="Ver só as anotações com esta tag"
                  >
                    #{tag}
                  </button>
                ) : (
                  <span key={tag} className="rounded-full bg-surface-alt px-2 py-1 font-mono text-[11px] text-ink-soft">
                    #{tag}
                  </span>
                )
              )}
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
                className="min-h-9 font-semibold text-danger hover:text-danger"
              >
                Confirmar exclusão
              </button>
            ) : (
              <button onClick={() => setConfirming(true)} className="min-h-9 text-ink-soft hover:text-danger">
                Excluir
              </button>
            )}
          </div>
        </>
      )}
    </li>
  );
}

