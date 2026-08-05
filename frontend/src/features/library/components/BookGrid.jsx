import React, { useState, useMemo } from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useWorks } from '../api/useWorks';
import { EditBookModal } from './EditBookModal';
import { authenticatedUrl } from '../../../lib/api';

export function BookGrid() {
  const [activeFilter, setActiveFilter] = useState('All');
  const [editingBook, setEditingBook] = useState(null);
  const [page, setPage] = useState(1);
  const limit = 50;

  const searchQuery = useGlobalStore((state) => state.searchQuery);
  const openBook = useGlobalStore((state) => state.openBook);

  // Consume TanStack Query with pagination
  const { data: result, isLoading, isError } = useWorks({ page, limit, search: searchQuery });

  const works = result?.data || [];
  const totalPages = result?.totalPages || 1;
  const total = result?.total || 0;

  // Discover all unique tags available in the collection
  const availableTags = useMemo(() => {
    const tags = new Set();
    works.forEach(work => work.tags?.forEach(tag => tags.add(tag)));
    return ['All', ...Array.from(tags).sort()];
  }, [works]);

  // Client-side tag filtering (search is now server-side)
  const filteredWorks = useMemo(() => {
    return works.filter((work) => {
      return activeFilter === 'All' || (work.tags && work.tags.includes(activeFilter));
    });
  }, [works, activeFilter]);

  // Reset to page 1 when search changes
  React.useEffect(() => {
    setPage(1);
  }, [searchQuery]);

  if (isLoading) {
    return (
      <div className="max-w-7xl mx-auto px-4 py-20 flex justify-center">
        <span className="text-zinc-500 animate-pulse font-medium">Loading collection from server...</span>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="max-w-7xl mx-auto px-4 py-20 flex justify-center">
        <span className="text-red-400 font-medium">❌ Error communicating with Go server.</span>
      </div>
    );
  }

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      {/* Tag Filter Pills Bar */}
      {availableTags.length > 1 && (
        <div className="flex gap-2 overflow-x-auto pb-4 mb-6 scrollbar-none">
          {availableTags.map(tag => (
            <button
              key={tag}
              onClick={() => setActiveFilter(tag)}
              className={`px-4 py-1.5 rounded-full text-xs font-medium transition-all whitespace-nowrap border ${
                activeFilter === tag 
                  ? 'bg-zinc-100 text-zinc-900 border-zinc-100 shadow-sm' 
                  : 'bg-zinc-900 text-zinc-400 border-zinc-800 hover:bg-zinc-800 hover:text-zinc-200'
              }`}
            >
              {tag}
            </button>
          ))}
        </div>
      )}

      <h2 className="text-lg font-semibold text-zinc-100 mb-6 flex items-center gap-2">
        <span>📖</span> Recently Added <span className="text-sm text-zinc-500 font-normal">({total} books)</span>
      </h2>
      
      {filteredWorks.length === 0 ? (
        <div className="text-zinc-500 mt-10 font-medium">No books found matching criteria.</div>
      ) : (
        <>
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-6">
            {filteredWorks.map((work) => (
              <div 
                key={work.id} 
                className="group flex flex-col gap-3 cursor-pointer"
                onClick={() => openBook(work.id)} 
              >
                <div className="relative aspect-[2/3] w-full overflow-hidden rounded-md border border-zinc-800 bg-zinc-900 transition-all duration-300 group-hover:border-zinc-600 group-hover:shadow-lg group-hover:shadow-blue-900/20 group-hover:-translate-y-1">
                  {(work.mediaStatus === 'ANALYZING' || work.mediaStatus === 'QUEUED') && (
                    <div className="absolute top-2 left-2 z-10 bg-amber-500/90 text-zinc-950 font-semibold text-[10px] px-2 py-0.5 rounded-full backdrop-blur-sm shadow-md animate-pulse flex items-center gap-1">
                      <span className="w-1.5 h-1.5 rounded-full bg-zinc-950 animate-ping" />
                      Analyzing...
                    </div>
                  )}
                  <button 
                    onClick={(e) => {
                      e.stopPropagation();
                      setEditingBook(work);
                    }}
                    className="absolute top-2 right-2 p-1.5 bg-black/75 backdrop-blur-sm text-zinc-300 rounded-md opacity-0 group-hover:opacity-100 transition-all duration-200 z-10 hover:bg-blue-600 hover:text-white shadow-md"
                    title="Edit metadata"
                  >
                    ✎
                  </button>
                  <img 
                    src={authenticatedUrl(work.coverUrl)} 
                    alt={work.title} 
                    className="h-full w-full object-cover transition-opacity duration-300 group-hover:opacity-90"
                  />
                </div>
                <div>
                  <h3 className="text-sm font-medium text-zinc-200 truncate group-hover:text-blue-400 transition-colors">
                    {work.title}
                  </h3>
                  <p className="text-xs text-zinc-500 truncate">{work.author}</p>
                  {work.tags && work.tags.length > 0 && (
                    <div className="flex gap-1 mt-1 overflow-hidden">
                      {work.tags.slice(0, 2).map(tag => (
                        <span key={tag} className="text-[10px] px-1.5 py-0.5 bg-zinc-800 text-zinc-400 rounded-sm font-mono truncate">
                          {tag}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-center gap-4 mt-8">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page <= 1}
                className="px-4 py-2 text-sm font-medium bg-zinc-900 border border-zinc-800 text-zinc-300 rounded-md hover:bg-zinc-800 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
              >
                ← Previous
              </button>
              <span className="text-zinc-400 text-sm font-mono">
                Page {page} of {totalPages}
              </span>
              <button
                onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
                className="px-4 py-2 text-sm font-medium bg-zinc-900 border border-zinc-800 text-zinc-300 rounded-md hover:bg-zinc-800 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
              >
                Next →
              </button>
            </div>
          )}
        </>
      )}

      {editingBook && (
        <EditBookModal 
          book={editingBook} 
          onClose={() => setEditingBook(null)} 
        />
      )}
    </div>
  );
}