import React, { lazy, Suspense, useMemo, useState } from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useWork } from '../api/useWork';
import { useReadingHeartbeat } from '../api/useReadingHeartbeat';
import { useFavoriteToggle } from '../api/useFavoriteToggle';
import { useCreateNote } from '../api/useCreateNote';
import { useFileProgress } from '../api/useFileProgress';
import { findFile, languageName } from '../files';
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
  const createNote = useCreateNote(activeBookId);
  const [showNoteForm, setShowNoteForm] = useState(false);
  const [noteText, setNoteText] = useState('');

  const handleSaveNote = () => {
    const trimmed = noteText.trim();
    if (!trimmed) return;
    createNote.mutate(trimmed, {
      onSuccess: () => {
        setNoteText('');
        setShowNoteForm(false);
      },
    });
  };

  if (isLoading || (file && progress.isLoading)) {
    return (
      <div className="flex h-[calc(100vh-4rem)] items-center justify-center bg-zinc-950">
        <span className="text-zinc-500 animate-pulse font-medium">Loading book details...</span>
      </div>
    );
  }

  if (isError || !book || !file?.url || file.availability === 'missing') {
    return (
      <div className="flex h-[calc(100vh-4rem)] flex-col items-center justify-center bg-zinc-950 gap-4">
        <span className="text-red-400 font-medium">
          {file?.availability === 'missing' ? 'Este arquivo não está mais no disco do servidor.' : 'Error: File not found on server.'}
        </span>
        <button 
          onClick={closeBook} 
          className="text-sm font-medium bg-zinc-800 border border-zinc-700 text-zinc-300 px-4 py-2 rounded-md hover:bg-zinc-700 hover:text-zinc-100 transition-colors"
        >
          Back to Library
        </button>
      </div>
    );
  }

  // Determine file format from backend metadata, fallback to file extension
  const format = (file.format || file.url.split('.').pop() || '').toLowerCase();
  const fileUrl = file.url;
  // The saved position of this file, unless the person chose to start over. Older readers
  // understand it as text (a CFI, a page number, seconds): the server keeps that form in sync.
  const initialProgress = fromStart ? undefined : progress.data?.position || undefined;
  const onProgress = progress.save;

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
          <div className="flex flex-col items-center justify-center h-full gap-4 text-zinc-400">
            <p>MOBI/AZW files cannot be viewed in the browser.</p>
            <a href={authenticatedUrl(fileUrl)}
               download
               className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-500">
              Download File
            </a>
          </div>
        );
      default:
        return (
          <div className="text-zinc-400 text-center mt-10 font-medium">
            File format (.{format}) is not supported yet by the reader.
          </div>
        );
    }
  };

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)] bg-zinc-950">
      {/* Sticky Reader Header */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-zinc-900 bg-zinc-950 shadow-sm z-10 sticky top-0">
        <div>
          <h2 className="text-zinc-200 font-medium">{book.title}</h2>
          <p className="text-xs text-zinc-500">
            {book.author}
            {' · '}
            {format.toUpperCase()}
            {file.edition?.language ? ` · ${languageName(file.edition.language)}` : ''}
          </p>
        </div>
        <div className="flex gap-4 items-center">
          <button
            onClick={() => favoriteToggle.mutate(!book.isFavorite)}
            disabled={favoriteToggle.isPending}
            className={`p-2 rounded-md border transition-all ${
              book.isFavorite
                ? 'bg-amber-500/10 border-amber-600/60 text-amber-400'
                : 'bg-zinc-900 border-zinc-800 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100'
            }`}
            title={book.isFavorite ? 'Remover dos favoritos' : 'Adicionar aos favoritos'}
          >
            {book.isFavorite ? '★ Favorito' : '☆ Favoritar'}
          </button>
          <button
            onClick={() => setShowNoteForm((v) => !v)}
            className="p-2 rounded-md bg-zinc-900 border border-zinc-800 hover:bg-zinc-800 text-zinc-400 hover:text-zinc-100 transition-all"
            title="Salvar uma citação deste livro"
          >
            + Nota
          </button>
          <button
            onClick={closeBook}
            className="p-2 rounded-md bg-zinc-900 border border-zinc-800 hover:bg-zinc-800 text-zinc-400 hover:text-zinc-100 transition-all"
            title="Close reader"
          >
            ✕ Close
          </button>
        </div>
      </div>

      {showNoteForm && (
        <div className="px-6 py-3 border-b border-zinc-900 bg-zinc-950 flex flex-col gap-2">
          <textarea
            value={noteText}
            onChange={(e) => setNoteText(e.target.value)}
            placeholder="Cole ou digite a citação que quer guardar..."
            rows={3}
            className="w-full rounded-md bg-zinc-900 border border-zinc-800 text-zinc-200 text-sm p-3 outline-none focus:border-zinc-600"
          />
          <div className="flex gap-2 justify-end">
            <button
              onClick={() => setShowNoteForm(false)}
              className="px-3 py-1.5 text-xs text-zinc-400 hover:text-zinc-200"
            >
              Cancelar
            </button>
            <button
              onClick={handleSaveNote}
              disabled={createNote.isPending || !noteText.trim()}
              className="px-3 py-1.5 text-xs font-medium bg-blue-600 text-white rounded-md hover:bg-blue-500 disabled:opacity-40"
            >
              Salvar nota
            </button>
          </div>
        </div>
      )}

      {/* Dynamic Reader Router Viewport */}
      <div className="flex-1 overflow-y-auto bg-zinc-900/30">
        <Suspense fallback={<div className="flex justify-center p-10 text-zinc-500 animate-pulse">Initializing reading engine...</div>}>
          <ErrorBoundary>
            {renderViewer()}
          </ErrorBoundary>
        </Suspense>
      </div>
    </div>
  );
}