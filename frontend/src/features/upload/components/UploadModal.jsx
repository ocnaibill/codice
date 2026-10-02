import React, { useEffect, useRef, useState } from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { toast } from '../../../components/ui/toast';
import { formatBytes } from '../../admin/format';
import { ACCEPT_ATTRIBUTE, extensionOf, isAccepted } from '../accepted';
import { useUploadBook, describeUploadError } from '../api/useUploadBook';

const FORMATS_TEXT = 'PDF, EPUB, CBZ, CBR, TXT, MD, MOBI e áudio (MP3, M4A, M4B, FLAC, OGG, WAV)';

/**
 * Where a file is added to the library: chosen from the computer or dropped on the area, one at a time. It says what
 * was chosen, how far the sending is, and why it failed in words a person can act on. While it is sending nothing
 * closes it (the file would be lost half way). When it is sent, the notice says it is on its way to be read.
 */
export function UploadModal() {
  const isOpen = useGlobalStore((state) => state.isUploadModalOpen);
  const closeModal = useGlobalStore((state) => state.closeUploadModal);
  const [file, setFile] = useState(null);
  const [progress, setProgress] = useState(0);
  const [problem, setProblem] = useState('');
  const [dragging, setDragging] = useState(false);
  const panel = useRef(null);
  const { mutate: uploadBook, isPending, isError, error, reset } = useUploadBook();

  const close = () => {
    if (isPending) return;
    setFile(null);
    setProgress(0);
    setProblem('');
    setDragging(false);
    reset();
    closeModal();
  };

  useEffect(() => {
    if (!isOpen) return undefined;
    panel.current?.focus();
    const onKey = (event) => {
      if (event.key === 'Escape') close();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
    // close reads the state of the moment: it is made again with each render, so the listener follows it.
  });

  if (!isOpen) return null;

  const choose = (files) => {
    const [first] = files;
    if (!first) return;
    reset();
    setProgress(0);
    if (!isAccepted(first.name)) {
      setFile(null);
      const ext = extensionOf(first.name);
      setProblem(`Esse tipo de arquivo não é aceito${ext ? ` (.${ext})` : ''}. Aceitamos ${FORMATS_TEXT}.`);
      return;
    }
    setFile(first);
    setProblem(files.length > 1 ? 'Um arquivo por vez: ficou o primeiro. Envie os outros depois.' : '');
  };

  const send = () => {
    if (!file || isPending) return;
    setProgress(0);
    setProblem('');
    uploadBook(
      { file, onProgress: setProgress },
      {
        onSuccess: () => {
          toast.success('Arquivo enviado', { message: `${file.name} será lido agora e logo aparece no acervo.` });
          setFile(null);
          setProgress(0);
          closeModal();
        },
      }
    );
  };

  const shown = isError ? describeUploadError(error) : problem;
  const ext = file ? extensionOf(file.name) : '';

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
            disabled={isPending}
            aria-label="Fechar"
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-ink-faint transition-colors hover:bg-surface-alt hover:text-ink disabled:opacity-30"
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden="true">
              <path d="M6 6l12 12M18 6L6 18" />
            </svg>
          </button>
        </div>

        <label
          onDragEnter={(event) => { event.preventDefault(); if (!isPending) setDragging(true); }}
          onDragOver={(event) => { event.preventDefault(); if (!isPending) setDragging(true); }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget)) setDragging(false); }}
          onDrop={(event) => {
            event.preventDefault();
            setDragging(false);
            if (!isPending) choose([...event.dataTransfer.files]);
          }}
          data-dragging={dragging}
          className={`flex min-h-36 cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed px-4 py-6 text-center transition-colors duration-150 focus-within:border-brand focus-within:ring-2 focus-within:ring-brand/15 ${
            dragging ? 'border-brand bg-brand/5' : 'border-border-hairline bg-surface hover:border-brand/60 hover:bg-surface-alt'
          } ${isPending ? 'pointer-events-none opacity-60' : ''}`}
        >
          <span className="flex h-11 w-11 items-center justify-center rounded-full bg-info-soft text-brand" aria-hidden="true">
            <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <path d="M12 16V4M7 9l5-5 5 5M5 20h14" />
            </svg>
          </span>
          <p className="text-sm text-ink">
            <span className="font-semibold text-brand">Escolha um arquivo</span>
            <span className="hidden sm:inline"> ou arraste até aqui</span>
          </p>
          <p className="max-w-xs text-xs leading-relaxed text-ink-soft">{FORMATS_TEXT}</p>
          <input
            type="file"
            className="sr-only"
            accept={ACCEPT_ATTRIBUTE}
            aria-label="Escolher um arquivo"
            disabled={isPending}
            onChange={(event) => {
              choose([...event.target.files]);
              event.target.value = '';
            }}
          />
        </label>

        {file && (
          <div className="flex items-center gap-3 rounded-xl border border-border-hairline bg-surface px-3 py-2.5">
            <span className="rounded-md bg-ink px-1.5 py-1 font-mono text-[10px] uppercase text-white">{ext}</span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-ink" title={file.name}>{file.name}</p>
              <p className="font-mono text-[11px] text-ink-soft">{formatBytes(file.size)}</p>
            </div>
            <button
              onClick={() => { setFile(null); setProblem(''); setProgress(0); reset(); }}
              disabled={isPending}
              aria-label={`Tirar ${file.name}`}
              className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-ink-faint transition-colors hover:bg-white hover:text-ink disabled:opacity-30"
            >
              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden="true">
                <path d="M6 6l12 12M18 6L6 18" />
              </svg>
            </button>
          </div>
        )}

        {isPending && (
          <div className="flex flex-col gap-2 rounded-xl border border-border-hairline bg-surface px-4 py-3">
            <div className="flex justify-between font-mono text-xs text-ink-soft">
              <span>{progress < 100 ? 'Enviando…' : 'Entregue; preparando a leitura…'}</span>
              <span>{progress}%</span>
            </div>
            <div
              role="progressbar"
              aria-label="Andamento do envio"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={progress}
              className="h-2 w-full overflow-hidden rounded-full bg-surface-alt"
            >
              <div className="h-full rounded-full bg-brand transition-[width] duration-300 ease-out" style={{ width: `${progress}%` }} />
            </div>
          </div>
        )}

        {shown && (
          <p
            role="alert"
            className={`rounded-lg px-3 py-2.5 text-[13px] leading-snug ${isError || problem.startsWith('Esse tipo') ? 'bg-danger-soft text-danger' : 'bg-warning-soft text-warning'}`}
          >
            {shown}
          </p>
        )}

        <div className="flex justify-end gap-2">
          <button
            onClick={close}
            disabled={isPending}
            className="min-h-10 rounded-lg px-4 py-2 text-sm font-medium text-ink-soft transition-colors hover:bg-surface-alt hover:text-ink disabled:opacity-30"
          >
            Cancelar
          </button>
          <button
            onClick={send}
            disabled={!file || isPending}
            className="min-h-10 rounded-lg bg-brand px-5 py-2 text-sm font-semibold text-white transition-[filter,transform] duration-150 hover:brightness-110 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-40 disabled:active:scale-100"
          >
            {isPending ? 'Enviando…' : 'Enviar'}
          </button>
        </div>
      </div>
    </div>
  );
}
