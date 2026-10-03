import React, { useEffect, useRef, useState } from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useWork } from '../../reader/api/useWork';
import { useCandidates } from '../../reader/api/useCandidates';
import { reasonOf } from '../../reader/api/useVersions';
import { WorkSuggestions } from '../../reader/components/WorkSuggestions';
import { ConfirmDialog } from '../../admin/components/ConfirmDialog';
import { isTopmostDialog } from '../../../lib/topDialog';

const LOCKS = [
  ['title', 'Título'], ['author', 'Autor'], ['series', 'Série'], ['cover', 'Capa'], ['isbn', 'ISBN'],
  ['publisher', 'Editora'], ['language', 'Idioma'], ['publication_date', 'Data'], ['description', 'Sinopse'],
];

const inputClass = 'w-full rounded-lg border border-border-hairline bg-white px-3 py-2 text-sm text-ink outline-none focus:border-brand disabled:opacity-50';

function Field({ label, children }) {
  return (
    <label className="flex min-w-0 flex-col gap-1 text-xs font-medium text-ink-soft">
      {label}
      {children}
    </label>
  );
}

/**
 * The fields of a work, to be corrected by hand: the last resort when what the file says and what the
 * providers suggest are not right. Whatever is saved is locked against the automatic extraction. The form
 * starts from the full record and exists only once it has loaded, so nothing is saved over a field that
 * was never read.
 */
function EditForm({ work, onClose }) {
  const queryClient = useQueryClient();
  const closeSheet = useGlobalStore((state) => state.closeSheet);
  const meta = work.metadata || {};
  const [title, setTitle] = useState(work.title || '');
  // The first author as it is stored: what the sheet shows for a work with several authors, or for an account
  // that sees surnames first, is not a name, and saving it back would make a person of it.
  const firstAuthor = meta.firstAuthor === 'Unknown Author' ? '' : meta.firstAuthor || '';
  const [author, setAuthor] = useState(firstAuthor);
  const [series, setSeries] = useState(meta.series || '');
  const [seriesIndex, setSeriesIndex] = useState(meta.seriesIndex ? String(meta.seriesIndex) : '');
  const [isbn, setIsbn] = useState(meta.isbn || '');
  const [publisher, setPublisher] = useState(meta.publisher || '');
  const [language, setLanguage] = useState(meta.language || '');
  const [publicationDate, setPublicationDate] = useState(meta.publicationDate || '');
  const [description, setDescription] = useState(meta.description || '');
  const [tags, setTags] = useState((work.tags || []).join(', '));
  const [locks, setLocks] = useState(meta.locks || {});
  const [retiring, setRetiring] = useState(false);

  const save = useMutation({
    mutationFn: (body) => api.put(`/works/${work.id}`, body),
    onSuccess: () => {
      refreshLibrary(queryClient);
      onClose();
    },
  });
  const retire = useMutation({
    mutationFn: () => api.delete(`/works/${work.id}`),
    onSuccess: () => {
      refreshLibrary(queryClient);
      closeSheet();
      onClose();
    },
  });
  const busy = save.isPending || retire.isPending;

  const submit = (event) => {
    event.preventDefault();
    save.mutate({
      title,
      // Only when it was changed: the other authors of the work stay as they are.
      ...(author !== firstAuthor ? { author } : {}),
      series,
      series_index: seriesIndex ? parseFloat(seriesIndex) : 0,
      isbn,
      publisher,
      language,
      publication_date: publicationDate,
      description,
      tags: tags.split(',').map((t) => t.trim()).filter(Boolean),
      title_lock: !!locks.title,
      author_lock: !!locks.author,
      series_lock: !!locks.series,
      cover_lock: !!locks.cover,
      isbn_lock: !!locks.isbn,
      publisher_lock: !!locks.publisher,
      language_lock: !!locks.language,
      description_lock: !!locks.description,
      publication_date_lock: !!locks.publication_date,
    });
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <p className="text-sm text-ink-soft">
        Corrija à mão o que o arquivo e os provedores não acertaram. O que você salva fica travado contra a extração automática.
      </p>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Título">
          <input className={inputClass} value={title} onChange={(e) => setTitle(e.target.value)} required disabled={busy} />
        </Field>
        <Field label="Primeiro autor">
          <input className={inputClass} value={author} onChange={(e) => setAuthor(e.target.value)} disabled={busy} />
        </Field>
        <Field label="Série">
          <input className={inputClass} value={series} onChange={(e) => setSeries(e.target.value)} disabled={busy} />
        </Field>
        <Field label="Número na série">
          <input className={inputClass} type="number" step="0.1" value={seriesIndex} onChange={(e) => setSeriesIndex(e.target.value)} disabled={busy} />
        </Field>
        <Field label="ISBN">
          <input className={inputClass} value={isbn} onChange={(e) => setIsbn(e.target.value)} disabled={busy} />
        </Field>
        <Field label="Editora">
          <input className={inputClass} value={publisher} onChange={(e) => setPublisher(e.target.value)} disabled={busy} />
        </Field>
        <Field label="Idioma">
          <input className={inputClass} value={language} onChange={(e) => setLanguage(e.target.value)} placeholder="pt, pt-BR, en…" disabled={busy} />
        </Field>
        <Field label="Data de publicação">
          <input className={inputClass} value={publicationDate} onChange={(e) => setPublicationDate(e.target.value)} disabled={busy} />
        </Field>
      </div>
      <Field label="Etiquetas (separadas por vírgula)">
        <input className={inputClass} value={tags} onChange={(e) => setTags(e.target.value)} disabled={busy} />
      </Field>
      <Field label="Sinopse">
        <textarea className={`${inputClass} resize-y`} rows={4} value={description} onChange={(e) => setDescription(e.target.value)} disabled={busy} />
      </Field>

      <fieldset className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-border-hairline pt-4">
        <legend className="mb-1 text-xs font-medium text-ink-soft">Travar contra a extração automática</legend>
        {LOCKS.map(([field, label]) => (
          <label key={field} className="flex items-center gap-2 text-xs text-ink-soft">
            <input
              type="checkbox"
              checked={!!locks[field]}
              onChange={(e) => setLocks((prev) => ({ ...prev, [field]: e.target.checked }))}
              disabled={busy}
            />
            {label}
          </label>
        ))}
      </fieldset>

      {save.isError && <p role="alert" className="text-sm text-danger">{reasonOf(save.error, 'Não foi possível salvar.')}</p>}
      {retire.isError && <p role="alert" className="text-sm text-danger">{reasonOf(retire.error, 'Não foi possível retirar a obra.')}</p>}

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border-hairline pt-4">
        <button
          type="button"
          onClick={() => setRetiring(true)}
          disabled={busy}
          className="min-h-10 rounded-lg border border-danger/30 bg-white px-4 py-2 text-xs text-danger hover:bg-danger-soft/40 disabled:opacity-40"
          title="Tira a obra do acervo. Arquivos, notas e progresso ficam guardados e ela pode ser restaurada."
        >
          Retirar do acervo
        </button>
        <div className="flex gap-2">
          <button type="button" onClick={onClose} disabled={busy} className="min-h-10 px-4 py-2 text-xs text-ink-soft hover:text-brand disabled:opacity-40">
            Cancelar
          </button>
          <button type="submit" disabled={busy} className="min-h-10 rounded-lg bg-brand px-5 py-2 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40">
            {save.isPending ? 'Salvando…' : 'Salvar'}
          </button>
        </div>
      </div>

      {retiring && (
        <ConfirmDialog
          title="Retirar esta obra do acervo?"
          message={<p>Ela sai do acervo, mas os arquivos, as notas e o progresso ficam guardados, e ela pode ser restaurada na Lixeira, em Administração.</p>}
          choices={[{ label: 'Retirar do acervo', value: true, tone: 'danger' }]}
          onChoose={() => { setRetiring(false); retire.mutate(); }}
          onCancel={() => setRetiring(false)}
        />
      )}
    </form>
  );
}

/**
 * The metadata of a work, for owner and admin (#70): what the providers suggested, to accept or reject, and
 * the fields, to correct by hand. It is opened from the sheet of the work and from the queue in Administração.
 */
export function EditBookModal({ workId, tab: initialTab = 'suggestions', onClose }) {
  const [tab, setTab] = useState(initialTab);
  const closeRef = useRef(null);
  const dialogRef = useRef(null);
  const { data: work, isLoading, isError, refetch, isRefetching } = useWork(workId, { fresh: true });
  const { data: candidates } = useCandidates(workId);
  const pending = candidates?.length ?? 0;

  useEffect(() => {
    closeRef.current?.focus();
    const onKeyDown = (event) => {
      if (event.key === 'Escape' && isTopmostDialog(dialogRef.current)) onClose();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  const tabs = [['suggestions', pending > 0 ? `Sugestões (${pending})` : 'Sugestões'], ['edit', 'Editar']];

  return (
    <div ref={dialogRef} className="fixed inset-0 z-[60] flex items-center justify-center bg-ink/60 backdrop-blur-sm sm:p-4" role="dialog" aria-modal="true" aria-label="Metadados da obra">
      <div className="flex h-full w-full flex-col overflow-hidden bg-[#faf8f4] shadow-2xl sm:max-h-[92vh] sm:h-auto sm:max-w-2xl sm:rounded-2xl">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline px-4 py-3 sm:px-6">
          <div className="min-w-0">
            <p className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">Metadados da obra</p>
            <h2 className="truncate font-display text-xl text-ink sm:text-2xl">{work?.title ?? 'Carregando…'}</h2>
          </div>
          <button ref={closeRef} onClick={onClose} className="flex min-h-11 min-w-11 items-center justify-center rounded-lg text-xl text-ink-soft hover:bg-surface-alt hover:text-brand" aria-label="Fechar">
            ✕
          </button>
        </div>
        <div role="tablist" className="flex shrink-0 gap-1 border-b border-border-hairline px-4 sm:px-6">
          {tabs.map(([key, label]) => (
            <button
              key={key}
              role="tab"
              aria-selected={tab === key}
              onClick={() => setTab(key)}
              className={`-mb-px min-h-11 border-b-2 px-4 py-2 text-[13px] ${tab === key ? 'border-brand text-brand' : 'border-transparent text-ink-soft hover:text-ink'}`}
            >
              {label}
            </button>
          ))}
        </div>
        <div className="min-h-0 overflow-y-auto px-4 py-5 sm:px-6">
          {isLoading && <p className="animate-pulse text-sm text-ink-faint">Carregando a obra…</p>}
          {isError && <LoadError onRetry={refetch} retrying={isRefetching}>Não foi possível abrir esta obra.</LoadError>}
          {work && tab === 'suggestions' && <WorkSuggestions workId={work.id} emptyText="Nenhuma sugestão esperando decisão." />}
          {work && tab === 'edit' && <EditForm work={work} onClose={onClose} />}
        </div>
      </div>
    </div>
  );
}
