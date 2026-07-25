import React from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';

export function Reader() {
  const activeBookId = useGlobalStore((state) => state.activeBookId);
  const closeBook = useGlobalStore((state) => state.closeBook);
  const books = useGlobalStore((state) => state.books);

  // Find metadata for current book
  const book = books.find((b) => b.id === activeBookId);

  if (!book) return null;

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)] bg-zinc-950">
      {/* Reader Internal Control Bar */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-zinc-900 bg-zinc-950 shadow-sm z-10">
        <div>
          <h2 className="text-zinc-200 font-medium">{book.title}</h2>
          <p className="text-xs text-zinc-500">{book.author}</p>
        </div>
        <div className="flex gap-4 items-center">
          <span className="text-xs font-mono text-zinc-500">{book.progress}% Completed</span>
          <button 
            onClick={closeBook}
            className="p-2 rounded-md bg-zinc-900 border border-zinc-800 hover:bg-zinc-800 text-zinc-400 hover:text-zinc-100 transition-all"
            title="Back to library shelf"
          >
            ✕ Close
          </button>
        </div>
      </div>

      {/* Centralized Reading Area */}
      <div className="flex-1 overflow-y-auto px-4 py-12 md:py-20 scroll-smooth bg-zinc-900/30">
        <div className="max-w-2xl mx-auto text-zinc-300 font-serif leading-relaxed space-y-6 text-lg">
          
          {/* Content mock to test visual layout and scrolling */}
          <h1 className="text-3xl font-bold text-zinc-100 mb-8 font-sans">Prologue</h1>
          
          <p>
            The servers hummed quietly in the metal rack. It was the sound of an entire library being 
            processed in the background, powered by Docker containers and Python scripts that never slept.
          </p>
          <p>
            To ensure continuous virtualization worked seamlessly, this sample text is long enough to test 
            scrolling mechanics and serif font contrast against the dark interface background.
          </p>
          <p>
            In production Códice, this section will be replaced by dynamic rendering of EPUB content, 
            extracted in chunks and rendered on demand using IntersectionObserver so the browser smoothly handles 800-page books.
          </p>
          
          {/* Section placeholder for scrolling test */}
          <div className="h-96 border-l-2 border-zinc-800 pl-6 my-12 text-zinc-500 italic flex items-center">
            [Placeholder for loading the next chapter from database...]
          </div>
          
        </div>
      </div>
    </div>
  );
}