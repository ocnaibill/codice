import React, { useEffect, useRef, useState } from 'react';
import { fetchFile, saveBlob } from '../../../lib/download';
import { isTopmostDialog } from '../../../lib/topDialog';
import { filterParams } from '../api/useNotesList';
import { describeFilters, EXPORT_CAP } from '../exportText';

const PREVIEW_CHARS = 1800;

const FORMATS = [
  ['md', 'Markdown', 'Legível e com a referência de cada obra'],
  ['json', 'JSON', 'Para guardar ou levar a outro programa'],
];

/**
 * Export of the notes the list shows (FL-09): the file is made first, shown, and only saved when the person
 * confirms, so what is saved is what was reviewed. Nothing is asked of the server twice.
 */
export function ExportNotes({ filters, total, onClose }) {
  const [format, setFormat] = useState('md');
  const [state, setState] = useState({ status: 'loading' });
  const dialogRef = useRef(null);
  const key = JSON.stringify([format, filters.q, filters.kind, filters.tag, filters.work?.id]);

  useEffect(() => {
    let current = true;
    setState({ status: 'loading' });
    const params = filterParams({ ...filters, workId: filters.work?.id });
    params.set('format', format);
    fetchFile(`/notes/export?${params}`, `codice-anotacoes.${format}`)
      .then(async (file) => {
        const text = await file.blob.text();
        if (current) setState({ status: 'ready', file, text });
      })
      .catch(() => current && setState({ status: 'error' }));
    return () => {
      current = false;
    };
    // The file is made again when the format or a filter changes, not when the list reloads.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  useEffect(() => {
    const onKey = (event) => {
      if (event.key === 'Escape' && isTopmostDialog(dialogRef.current)) onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const capped = total > EXPORT_CAP;
  const ready = state.status === 'ready';
  return (
    <div
      ref={dialogRef}
      role="dialog"
      aria-modal="true"
      aria-label="Exportar anotações"
      className="fixed inset-0 z-[60] flex items-center justify-center bg-ink/60 backdrop-blur-sm sm:p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div className="flex h-full w-full flex-col overflow-hidden bg-[#faf8f4] shadow-2xl sm:h-auto sm:max-h-[90vh] sm:max-w-2xl sm:rounded-2xl">
        <div className="flex shrink-0 items-start justify-between gap-3 border-b border-border-hairline px-4 py-3 sm:px-6">
          <div>
            <p className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">Anotações</p>
            <h2 className="font-display text-2xl text-ink">Exportar</h2>
          </div>
          <button onClick={onClose} aria-label="Fechar" className="flex min-h-11 min-w-11 items-center justify-center rounded-lg text-xl text-ink-soft hover:bg-surface-alt hover:text-brand">
            ✕
          </button>
        </div>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-5 sm:px-6">
          <p className="text-sm text-ink">
            <strong>{total === 1 ? '1 anotação' : `${total} anotações`}</strong>: {describeFilters(filters)}.
          </p>
          {capped && (
            <p role="note" className="rounded-lg bg-amber-50 p-3 text-xs text-amber-900">
              Um arquivo leva no máximo {EXPORT_CAP.toLocaleString('pt-BR')} anotações, as mais antigas primeiro. Estreite os filtros para exportar o resto.
            </p>
          )}

          <fieldset className="grid gap-2 sm:grid-cols-2">
            <legend className="mb-1 text-xs font-medium text-ink-soft">Formato</legend>
            {FORMATS.map(([value, label, hint]) => (
              <label key={value} className={`flex cursor-pointer flex-col rounded-lg border p-3 text-sm ${format === value ? 'border-brand bg-brand/5' : 'border-border-hairline bg-white'}`}>
                <span className="flex items-center gap-2 font-medium text-ink">
                  <input type="radio" name="export-format" value={value} checked={format === value} onChange={() => setFormat(value)} />
                  {label}
                </span>
                <span className="text-xs text-ink-soft">{hint}</span>
              </label>
            ))}
          </fieldset>

          <section aria-label="Prévia do arquivo">
            <p className="mb-1 text-xs font-medium text-ink-soft">Prévia do arquivo</p>
            {state.status === 'loading' && <p className="animate-pulse text-sm text-ink-faint">Preparando o arquivo…</p>}
            {state.status === 'error' && <p role="alert" className="text-sm text-red-700">Não foi possível preparar o arquivo.</p>}
            {ready && (
              <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-border-hairline bg-white p-3 font-mono text-[11px] leading-relaxed text-ink">
                {state.text.length > PREVIEW_CHARS ? `${state.text.slice(0, PREVIEW_CHARS)}\n…` : state.text}
              </pre>
            )}
            {ready && <p className="mt-1 text-[11px] text-ink-faint">{state.file.name}, {Math.max(1, Math.round(state.file.blob.size / 1024))} KB</p>}
          </section>
        </div>

        <div className="flex shrink-0 flex-wrap justify-end gap-2 border-t border-border-hairline px-4 py-3 sm:px-6">
          <button onClick={onClose} className="min-h-10 px-4 py-2 text-xs text-ink-soft hover:text-brand">Cancelar</button>
          <button
            disabled={!ready}
            onClick={() => {
              saveBlob(state.file.blob, state.file.name);
              onClose();
            }}
            className="min-h-10 rounded-lg bg-brand px-5 py-2 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40"
          >
            Salvar arquivo
          </button>
        </div>
      </div>
    </div>
  );
}
