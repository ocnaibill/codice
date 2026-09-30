import React, { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useWork } from '../api/useWork';
import { useReadingHeartbeat } from '../api/useReadingHeartbeat';
import { useFavoriteToggle } from '../api/useFavoriteToggle';
import { useFileProgress } from '../api/useFileProgress';
import { findFile, languageName, positionFromLocator } from '../files';
import { otherVersionsInProgress } from '../finishPrompt';
import { useSetWorkFinished } from '../api/useCompletion';
import { useAcceptEquivalentPosition, useEquivalentPosition } from '../api/useEquivalentPosition';
import { api } from '../../../lib/api';
import { NotesPanel } from './NotesPanel';
import { FinishWorkPrompt } from './FinishWorkPrompt';
import { EquivalentPositionPrompt } from './EquivalentPositionPrompt';
import { ErrorBoundary } from '../../../components/ErrorBoundary';
import { authenticatedUrl } from '../../../lib/api';

// Dynamic imports (Lazy Loading) - Readers are loaded on demand
const PdfViewer = lazy(() => import('./viewers/PdfViewer'));
const EpubViewer = lazy(() => import('./viewers/EpubViewer'));
const MangaViewer = lazy(() => import('./viewers/MangaViewer'));
const TextViewer = lazy(() => import('./viewers/TextViewer'));
const MarkdownViewer = lazy(() => import('./viewers/MarkdownViewer'));
const AudioViewer = lazy(() => import('./viewers/AudioViewer'));

export function Reader() {
  const activeBookId = useGlobalStore((state) => state.activeBookId);
  const closeBook = useGlobalStore((state) => state.closeBook);

  const activeFileId = useGlobalStore((state) => state.activeFileId);
  const fromStart = useGlobalStore((state) => state.fromStart);

  const { data: book, isLoading, isError } = useWork(activeBookId);
  // The file being read: the one chosen on the sheet, or the work's primary. Its position,
  // its format and its reading time are its own.
  const file = useMemo(() => findFile(book, activeFileId), [book, activeFileId]);
  const progress = useFileProgress(file?.id);
  useReadingHeartbeat(activeBookId, file?.id);
  const favoriteToggle = useFavoriteToggle(activeBookId);
  const [showNotes, setShowNotes] = useState(false);
  const openBook = useGlobalStore((state) => state.openBook);
  const seek = useGlobalStore((state) => state.seek);

  // Where the person is in this file, as a locator: what the viewers last reported, or the saved
  // position until they report one. A note or bookmark made now is tied to it.
  const currentLocator = useRef(null);
  useEffect(() => {
    currentLocator.current = fromStart ? null : (seek?.locator ?? progress.data?.locator ?? null);
  }, [file?.id, fromStart, seek, progress.data]);
  const saveProgress = progress.save;

  // When this version is finished and another is still in progress, ask once whether the whole
  // work is finished (DEC-080). It is asked when the file goes from not finished to finished, not
  // every time a finished file saves its last page.
  const [finishPrompt, setFinishPrompt] = useState(null);
  const setWorkFinished = useSetWorkFinished();
  const wasCompleted = useRef(false);
  useEffect(() => {
    wasCompleted.current = !!progress.data?.completed;
  }, [file?.id, progress.data?.completed]);
  const workId = activeBookId;
  const fileId = file?.id;

  // Another version of this book may be mid-way (DEC-079's "continue" file). Opening a different
  // one from it, while it is still in progress, offers a one-time jump to where the wording puts
  // it there (RF-042); asked once per opening, and never if there is nothing to offer.
  const otherVersion = book?.inProgress && book.continue?.fileId && book.continue.fileId !== fileId ? book.continue : null;
  const equivalent = useEquivalentPosition(fileId, otherVersion?.fileId);
  const acceptEquivalent = useAcceptEquivalentPosition(fileId);
  const [equivalentDeclined, setEquivalentDeclined] = useState(false);
  const showEquivalentPrompt =
    !!otherVersion && !equivalentDeclined && (equivalent.data?.status === 'found' || equivalent.data?.status === 'ambiguous');

  const askIfWorkIsFinished = useCallback(async () => {
    try {
      const { data: detail } = await api.get(`/works/${workId}`);
      const others = detail.finished ? [] : otherVersionsInProgress(detail, fileId);
      if (others.length > 0) setFinishPrompt({ others });
    } catch {
      // Not asking is the safe way to fail.
    }
  }, [workId, fileId]);

  const onProgress = useCallback(
    (locator, extras) => {
      currentLocator.current = locator;
      const saved = saveProgress(locator, extras);
      saved?.then?.((state) => {
        if (!state) return;
        if (state.completed && !wasCompleted.current) askIfWorkIsFinished();
        wasCompleted.current = !!state.completed;
      });
      return saved;
    },
    [saveProgress, askIfWorkIsFinished]
  );

  if (isLoading || (file && progress.isLoading)) {
    return (
      <div className="flex h-dvh items-center justify-center bg-[#faf8f4]">
        <span className="animate-pulse font-body text-sm text-ink-soft">Carregando o livro…</span>
      </div>
    );
  }

  if (isError || !book || !file?.url || file.availability === 'missing') {
    return (
      <div className="flex h-dvh flex-col items-center justify-center gap-4 bg-[#faf8f4] px-6 text-center">
        <span className="font-body text-sm font-medium text-red-700">
          {file?.availability === 'missing' ? 'Este arquivo não está mais no disco do servidor.' : 'Não foi possível abrir este arquivo.'}
        </span>
        <button
          onClick={closeBook}
          className="min-h-11 rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-white hover:bg-brand-light"
        >
          Voltar ao acervo
        </button>
      </div>
    );
  }

  // Determine file format from backend metadata, fallback to file extension
  const format = (file.format || file.url.split('.').pop() || '').toLowerCase();
  const fileUrl = file.url;
  // The saved position of this file, unless the person chose to start over. Older readers
  // understand it as text (a CFI, a page number, seconds): the server keeps that form in sync.
  const initialProgress = seek ? positionFromLocator(seek.locator) : fromStart ? undefined : progress.data?.position || undefined;

  const renderViewer = () => {
    switch (format) {
      case 'pdf':
        return <PdfViewer fileUrl={fileUrl} onProgress={onProgress} initialProgress={initialProgress} />;
      case 'epub':
        return <EpubViewer fileUrl={fileUrl} onProgress={onProgress} initialProgress={initialProgress} />;
      case 'cbz':
      case 'cbr':
        return <MangaViewer fileUrl={fileUrl} onProgress={onProgress} workId={book.id} initialProgress={initialProgress} />;
      case 'txt':
        return <TextViewer fileUrl={fileUrl} onProgress={onProgress} initialProgress={initialProgress} />;
      case 'md':
        return <MarkdownViewer fileUrl={fileUrl} onProgress={onProgress} initialProgress={initialProgress} />;
      case 'mp3':
      case 'm4a':
      case 'm4b':
      case 'ogg':
      case 'wav':
      case 'flac':
        return <AudioViewer fileUrl={fileUrl} onProgress={onProgress} initialProgress={initialProgress} />;
      case 'mobi':
      case 'azw':
      case 'azw3':
        return (
          <div className="flex h-full flex-col items-center justify-center gap-4 p-6 text-center text-ink-soft">
            <p>Este formato MOBI/AZW não pode ser lido no navegador.</p>
            <a href={authenticatedUrl(fileUrl)}
               download
               className="rounded-lg bg-brand px-4 py-2 text-white hover:bg-brand-light">
              Baixar arquivo
            </a>
          </div>
        );
      default:
        return (
          <div className="mt-10 px-6 text-center font-medium text-ink-soft">
            O leitor ainda não oferece suporte ao formato .{format}.
          </div>
        );
    }
  };

  return (
    <div className="flex h-dvh min-h-[320px] flex-col bg-[#faf8f4]">
      <header className="sticky top-0 z-20 flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline bg-[#faf8f4]/95 px-3 py-2 shadow-sm backdrop-blur-md sm:px-6">
        <div className="flex min-w-0 items-center gap-3">
          <button
            onClick={closeBook}
            className="flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded-lg bg-surface-alt text-xl text-ink hover:bg-border-hairline"
            aria-label="Voltar ao acervo"
            title="Voltar ao acervo"
          >
            ←
          </button>
          <div className="min-w-0">
            <p className="truncate font-mono text-[10px] uppercase tracking-widest text-ink-faint">
              {book.author} · {format.toUpperCase()}{file.edition?.language ? ` · ${languageName(file.edition.language)}` : ''}
            </p>
            <h2 className="truncate font-display text-xl leading-tight text-ink sm:text-2xl">{book.title}</h2>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5 sm:gap-2">
          <button
            onClick={() => favoriteToggle.mutate(!book.isFavorite)}
            disabled={favoriteToggle.isPending}
            className={`flex min-h-11 items-center gap-1.5 rounded-lg px-3 text-sm transition-all disabled:opacity-50 ${
              book.isFavorite
                ? 'bg-brand/10 text-brand'
                : 'bg-surface-alt text-ink-soft hover:bg-border-hairline hover:text-ink'
            }`}
            aria-label={book.isFavorite ? 'Remover dos favoritos' : 'Adicionar aos favoritos'}
            aria-pressed={!!book.isFavorite}
            title={book.isFavorite ? 'Remover dos favoritos' : 'Adicionar aos favoritos'}
          >
            <span aria-hidden="true">{book.isFavorite ? '★' : '☆'}</span>
            <span className="hidden sm:inline">{book.isFavorite ? 'Favorito' : 'Favoritar'}</span>
          </button>
          <button
            onClick={() => setShowNotes((v) => !v)}
            className={`flex min-h-11 items-center gap-1.5 rounded-lg px-3 text-sm transition-all ${
              showNotes
                ? 'bg-brand text-white'
                : 'bg-surface-alt text-ink-soft hover:bg-border-hairline hover:text-ink'
            }`}
            aria-expanded={showNotes}
            aria-label="Notas, destaques e marcadores deste livro"
            title="Notas, destaques e marcadores deste livro"
          >
            <span aria-hidden="true">✎</span>
            <span className="hidden sm:inline">Notas</span>
          </button>
        </div>
      </header>

      {showNotes && (
        <NotesPanel
          workId={book.id}
          fileId={file.id}
          getLocator={() => currentLocator.current}
          onOpenAt={(note) => {
            setShowNotes(false);
            openBook(book.id, note.fileId, { locator: note.locator });
          }}
          onClose={() => setShowNotes(false)}
        />
      )}

      {showEquivalentPrompt && (
        <EquivalentPositionPrompt
          from={otherVersion}
          sourceExcerpt={equivalent.data.sourceExcerpt}
          status={equivalent.data.status}
          candidates={equivalent.data.candidates}
          busy={acceptEquivalent.isPending}
          onDecline={() => setEquivalentDeclined(true)}
          onAccept={(candidate) => {
            acceptEquivalent.mutate({
              sourceFileId: otherVersion.fileId, locator: candidate.locator,
              method: candidate.method, confidence: candidate.confidence, precision: candidate.precision,
            });
            setEquivalentDeclined(true);
            openBook(workId, fileId, { locator: candidate.locator });
          }}
        />
      )}

      {finishPrompt && (
        <FinishWorkPrompt
          finished={`${(file.format || '').toUpperCase()}${file.edition?.language ? ` (${languageName(file.edition.language)})` : ''}: concluído.`}
          others={finishPrompt.others}
          busy={setWorkFinished.isPending}
          onKeep={() => setFinishPrompt(null)}
          onFinish={() => setWorkFinished.mutate({ workId, finished: true }, { onSuccess: () => setFinishPrompt(null) })}
        />
      )}

      {/* Dynamic Reader Router Viewport */}
      <div className="min-h-0 flex-1 overflow-y-auto bg-[#eae5dc]">
        <Suspense fallback={<div className="flex justify-center p-10 text-sm text-ink-soft animate-pulse">Preparando o leitor…</div>}>
          <ErrorBoundary>
            <React.Fragment key={`${file.id}-${seek?.n ?? 0}`}>{renderViewer()}</React.Fragment>
          </ErrorBoundary>
        </Suspense>
      </div>
    </div>
  );
}
