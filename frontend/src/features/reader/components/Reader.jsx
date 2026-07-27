import React, { lazy, Suspense } from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useWork } from '../api/useWork';

// Dynamic imports (Lazy Loading) - Readers are loaded on demand
const PdfViewer = lazy(() => import('./viewers/PdfViewer'));
const EpubViewer = lazy(() => import('./viewers/EpubViewer'));
const MangaViewer = lazy(() => import('./viewers/MangaViewer'));

export function Reader() {
  const activeBookId = useGlobalStore((state) => state.activeBookId);
  const closeBook = useGlobalStore((state) => state.closeBook);

  const { data: book, isLoading, isError } = useWork(activeBookId);

  if (isLoading) {
    return (
      <div className="flex h-[calc(100vh-4rem)] items-center justify-center bg-zinc-950">
        <span className="text-zinc-500 animate-pulse font-medium">Loading book details...</span>
      </div>
    );
  }

  if (isError || !book || !book.fileUrl) {
    return (
      <div className="flex h-[calc(100vh-4rem)] flex-col items-center justify-center bg-zinc-950 gap-4">
        <span className="text-red-400 font-medium">Error: File not found on server.</span>
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
  const format = (book.format || book.fileUrl.split('.').pop() || '').toLowerCase();

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)] bg-zinc-950">
      {/* Sticky Reader Header */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-zinc-900 bg-zinc-950 shadow-sm z-10 sticky top-0">
        <div>
          <h2 className="text-zinc-200 font-medium">{book.title}</h2>
          <p className="text-xs text-zinc-500">{book.author}</p>
        </div>
        <div className="flex gap-4 items-center">
          <button 
            onClick={closeBook}
            className="p-2 rounded-md bg-zinc-900 border border-zinc-800 hover:bg-zinc-800 text-zinc-400 hover:text-zinc-100 transition-all"
            title="Close reader"
          >
            ✕ Close
          </button>
        </div>
      </div>

      {/* Dynamic Reader Router Viewport */}
      <div className="flex-1 overflow-y-auto px-4 py-8 bg-zinc-900/30">
        <Suspense fallback={<div className="flex justify-center p-10 text-zinc-500 animate-pulse">Initializing reading engine...</div>}>
          
          {format === 'pdf' && (
            <PdfViewer 
              fileUrl={book.fileUrl} 
              bookId={book.id} 
              initialProgress={book.readingProgress} 
            />
          )}
          {format === 'epub' && (
            <EpubViewer 
              fileUrl={book.fileUrl} 
              bookId={book.id} 
              initialProgress={book.readingProgress} 
            />
          )}
          {['cbz', 'cbr'].includes(format) && (
            <MangaViewer 
              fileUrl={book.fileUrl} 
              bookId={book.id} 
              initialProgress={book.readingProgress} 
            />
          )}
          
          {!['pdf', 'epub', 'cbz', 'cbr'].includes(format) && (
            <div className="text-zinc-400 text-center mt-10 font-medium">
              File format (.{format}) is not supported yet by the reader.
            </div>
          )}

        </Suspense>
      </div>
    </div>
  );
}