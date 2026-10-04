import React, { useRef, useState } from 'react';
import { useDialog } from '../../../lib/useDialog';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { toast } from '../../../components/ui/toast';
import { formatBytes } from '../../admin/format';
import { ACCEPT_ATTRIBUTE, extensionOf } from '../accepted';
import { useUploadBook, describeUploadError } from '../api/useUploadBook';
import { MAX_FILES, STATUS, placesTaken, useUploadQueue } from '../uploadQueue';

const FORMATS_TEXT = 'PDF, EPUB, CBZ, CBR, TXT, MD, MOBI e áudio (MP3, M4A, M4B, FLAC, OGG, WAV)';

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

/** What adding files to a full list, or files that were already on it, leaves out, said once. */
function leftOut({ overCap, repeated }) {
  const parts = [];
  if (overCap) parts.push(`Cabem ${MAX_FILES} arquivos por vez: ${plural(overCap, 'ficou de fora', 'ficaram de fora')}. Envie ${overCap === 1 ? 'esse' : 'esses'} depois.`);
  if (repeated) parts.push(`${plural(repeated, 'arquivo já estava', 'arquivos já estavam')} na lista.`);
  return parts.join(' ');
}

/** What a row says on its second line, and the colour of it. */
function describe(item) {
  switch (item.status) {
    case STATUS.QUEUED: return { text: formatBytes(item.file.size), tone: 'text-ink-soft' };
    case STATUS.UPLOADING: return { text: `Enviando… ${item.progress}%`, tone: 'text-ink-soft' };
    case STATUS.DONE: return { text: 'Enviado', tone: 'text-success' };
    case STATUS.DUPLICATE: return { text: item.message, tone: 'text-warning' };
    case STATUS.CANCELLED: return { text: 'Cancelado', tone: 'text-ink-soft' };
    default: return { text: item.message, tone: 'text-danger' }; // error, refused
  }
}

/**
 * Where files are added to the library: up to five chosen from the computer or dropped on the area, sent one after the
 * other. Each file says where it is (waiting, sending, sent, already in the library, refused, failed) and why, and one
 * that failed can be sent again. While it is sending nothing closes the dialog; "Parar o resto" lets the file that is
 * going finish and cancels those that have not started. When everything is sent the dialog closes and the notice of
 * the system says how many went.
 */
export function UploadModal() {
  const isOpen = useGlobalStore((state) => state.isUploadModalOpen);
  const closeModal = useGlobalStore((state) => state.closeUploadModal);
  const [notice, setNotice] = useState('');
  const [dragging, setDragging] = useState(false);
  const panel = useRef(null);
  const { mutateAsync } = useUploadBook();
  const send = React.useCallback((file, onProgress) => mutateAsync({ file, onProgress }), [mutateAsync]);
  const queue = useUploadQueue(send, describeUploadError);
  const { items, running, position } = queue;

  const close = () => {
    if (running) return;
    queue.reset();
    setNotice('');
    setDragging(false);
    closeModal();
  };

  // close reads the state of the moment: it is made again with each render, and the hook keeps the latest one.
  // The focus starts on the panel itself, as before: the name of the dialog is read, and the first Tab reaches the first control.
  useDialog(panel, { active: isOpen, onEscape: close, initialFocus: panel });

  if (!isOpen) return null;

  const choose = (files) => {
    if (files.length === 0 || running) return;
    setNotice(leftOut(queue.add(files)));
  };

  // What the system says when a run is over, and whether the dialog has nothing left to show.
  const afterRun = ({ sent, duplicates, errors, cancelled, firstSent, allDone }) => {
    if (sent > 0) {
      const issues = [];
      if (duplicates) issues.push(`${plural(duplicates, 'já estava', 'já estavam')} no acervo`);
      if (errors) issues.push(`${plural(errors, 'com erro', 'com erro')}`);
      if (cancelled) issues.push(`${plural(cancelled, 'cancelado', 'cancelados')}`);
      if (issues.length > 0) toast.warning(plural(sent, 'arquivo enviado', 'arquivos enviados'), { message: `${issues.join(' · ')}.` });
      else if (sent === 1) toast.success('Arquivo enviado', { message: `${firstSent} será lido agora e logo aparece no acervo.` });
      else toast.success(`${sent} arquivos enviados`, { message: 'Serão lidos agora e logo aparecem no acervo.' });
    }
    if (allDone) {
      queue.reset();
      setNotice('');
      closeModal();
    }
  };

  // What was said of the last choice (a file left out) is not about the sending that begins now.
  const begin = async () => {
    setNotice('');
    afterRun(await queue.start());
  };
  const again = async (id) => {
    setNotice('');
    afterRun(await queue.retry(id));
  };

  const queued = items.filter((item) => item.status === STATUS.QUEUED).length;
  const full = placesTaken(items) >= MAX_FILES;
  const touched = items.some((item) => [STATUS.DONE, STATUS.DUPLICATE, STATUS.ERROR, STATUS.CANCELLED].includes(item.status));
  const locked = running || full;

  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in items-end justify-center bg-black/50 p-0 backdrop-blur-[2px] sm:items-center sm:p-4"
      onClick={(event) => event.target === event.currentTarget && close()}
    >
      <div
        ref={panel}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-label="Adicionar à biblioteca"
        className="flex max-h-[92dvh] w-full max-w-lg animate-pop-in flex-col gap-5 overflow-y-auto rounded-t-2xl bg-white p-6 shadow-2xl outline-none sm:rounded-xl"
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-brand">Acervo</p>
            <h2 className="font-display text-2xl text-ink">Adicionar à biblioteca</h2>
          </div>
          <button
            onClick={close}
            disabled={running}
            aria-label="Fechar"
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-ink-faint transition-colors hover:bg-surface-alt hover:text-ink disabled:opacity-30"
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden="true">
              <path d="M6 6l12 12M18 6L6 18" />
            </svg>
          </button>
        </div>

        <label
          onDragEnter={(event) => { event.preventDefault(); if (!locked) setDragging(true); }}
          onDragOver={(event) => { event.preventDefault(); if (!locked) setDragging(true); }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget)) setDragging(false); }}
          onDrop={(event) => {
            event.preventDefault();
            setDragging(false);
            if (!locked) choose([...event.dataTransfer.files]);
          }}
          data-dragging={dragging}
          className={`flex min-h-32 cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed px-4 py-5 text-center transition-colors duration-150 focus-within:border-brand focus-within:ring-2 focus-within:ring-brand/15 ${
            dragging ? 'border-brand bg-brand/5' : 'border-border-hairline bg-surface hover:border-brand/60 hover:bg-surface-alt'
          } ${locked ? 'pointer-events-none opacity-60' : ''}`}
        >
          <span className="flex h-11 w-11 items-center justify-center rounded-full bg-info-soft text-brand" aria-hidden="true">
            <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <path d="M12 16V4M7 9l5-5 5 5M5 20h14" />
            </svg>
          </span>
          <p className="text-sm text-ink">
            {full && !running ? (
              <span className="font-semibold text-ink-soft">A lista está cheia: {MAX_FILES} arquivos</span>
            ) : (
              <>
                <span className="font-semibold text-brand">Escolha até {MAX_FILES} arquivos</span>
                <span className="hidden sm:inline"> ou arraste até aqui</span>
              </>
            )}
          </p>
          <p className="max-w-xs text-xs leading-relaxed text-ink-soft">{FORMATS_TEXT}</p>
          <input
            type="file"
            multiple
            className="sr-only"
            accept={ACCEPT_ATTRIBUTE}
            aria-label="Escolher arquivos"
            disabled={locked}
            onChange={(event) => {
              choose([...event.target.files]);
              event.target.value = '';
            }}
          />
        </label>

        {items.length > 0 && (
          <ul aria-label="Arquivos para enviar" className="flex flex-col gap-2">
            {items.map((item) => {
              const line = describe(item);
              const ext = extensionOf(item.file.name);
              return (
                <li key={item.id} data-status={item.status} className="flex flex-col gap-2 rounded-xl border border-border-hairline bg-surface px-3 py-2.5">
                  <div className="flex items-center gap-3">
                    <span className="w-11 shrink-0 rounded-md bg-ink px-1 py-1 text-center font-mono text-[10px] uppercase text-white">{ext || '?'}</span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-ink" title={item.file.name}>{item.file.name}</p>
                      <p className={`text-[12px] leading-snug ${line.tone}`}>{line.text}</p>
                    </div>
                    {(item.status === STATUS.ERROR || item.status === STATUS.CANCELLED) && !running && (
                      <button
                        onClick={() => again(item.id)}
                        aria-label={`Tentar de novo ${item.file.name}`}
                        className="min-h-9 shrink-0 rounded-lg px-2.5 text-[12px] font-semibold text-brand transition-colors hover:bg-brand/10"
                      >
                        Tentar de novo
                      </button>
                    )}
                    {item.status !== STATUS.UPLOADING && (
                      <button
                        onClick={() => queue.remove(item.id)}
                        disabled={running}
                        aria-label={`Tirar ${item.file.name}`}
                        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-ink-faint transition-colors hover:bg-white hover:text-ink disabled:opacity-30"
                      >
                        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden="true">
                          <path d="M6 6l12 12M18 6L6 18" />
                        </svg>
                      </button>
                    )}
                  </div>
                  {item.status === STATUS.UPLOADING && (
                    <div
                      role="progressbar"
                      aria-label={`Andamento do envio de ${item.file.name}`}
                      aria-valuemin={0}
                      aria-valuemax={100}
                      aria-valuenow={item.progress}
                      className="h-1.5 w-full overflow-hidden rounded-full bg-surface-alt"
                    >
                      <div className="h-full rounded-full bg-brand transition-[width] duration-300 ease-out" style={{ width: `${item.progress}%` }} />
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}

        {running && (
          <p role="status" className="font-mono text-xs text-ink-soft">
            Enviando {position.index} de {position.total}
            {items.find((item) => item.status === STATUS.UPLOADING)?.progress >= 100 ? ' · entregue; preparando a leitura…' : '…'}
          </p>
        )}

        {notice && (
          <p role="alert" className="rounded-lg bg-warning-soft px-3 py-2.5 text-[13px] leading-snug text-warning">
            {notice}
          </p>
        )}

        <div className="flex flex-wrap justify-end gap-2">
          <button
            onClick={close}
            disabled={running}
            className="min-h-10 rounded-lg px-4 py-2 text-sm font-medium text-ink-soft transition-colors hover:bg-surface-alt hover:text-ink disabled:opacity-30"
          >
            {touched ? 'Fechar' : 'Cancelar'}
          </button>
          {running && (
            <button
              onClick={queue.stop}
              className="min-h-10 rounded-lg border border-border-hairline px-4 py-2 text-sm font-medium text-ink transition-colors hover:bg-surface-alt"
            >
              Parar o resto
            </button>
          )}
          <button
            onClick={begin}
            disabled={queued === 0 || running}
            className="min-h-10 rounded-lg bg-brand px-5 py-2 text-sm font-semibold text-white transition-[filter,transform] duration-150 hover:brightness-110 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-40 disabled:active:scale-100"
          >
            {running ? 'Enviando…' : queued > 1 ? `Enviar ${queued} arquivos` : 'Enviar'}
          </button>
        </div>
      </div>
    </div>
  );
}
