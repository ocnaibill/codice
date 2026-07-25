import React from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useWorks } from '../api/useWorks';

export function BookGrid() {
  const searchQuery = useGlobalStore((state) => state.searchQuery);
  const openBook = useGlobalStore((state) => state.openBook);

  // Consume TanStack Query
  const { data: works, isLoading, isError } = useWorks();

  // UI handling for transitional states
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

  // Client-side reactive filtering
  const filteredWorks = works?.filter((work) => 
    work.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
    work.author.toLowerCase().includes(searchQuery.toLowerCase())
  ) || [];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <h2 className="text-lg font-semibold text-zinc-100 mb-6 flex items-center gap-2">
        <span>📖</span> Recently Added
      </h2>
      
      {filteredWorks.length === 0 ? (
        <div className="text-zinc-500 mt-10">No books found for "{searchQuery}".</div>
      ) : (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-6">
          {filteredWorks.map((work) => (
            <div 
              key={work.id} 
              className="group flex flex-col gap-3 cursor-pointer"
              onClick={() => openBook(work.id)} 
            >
              <div className="relative aspect-[2/3] w-full overflow-hidden rounded-md border border-zinc-800 bg-zinc-900 transition-all duration-300 group-hover:border-zinc-600 group-hover:shadow-lg group-hover:shadow-blue-900/20 group-hover:-translate-y-1">
                <img 
                  src={work.coverUrl} 
                  alt={work.title} 
                  className="h-full w-full object-cover transition-opacity duration-300 group-hover:opacity-90"
                />
              </div>

              <div>
                <h3 className="text-sm font-medium text-zinc-200 truncate group-hover:text-blue-400 transition-colors">
                  {work.title}
                </h3>
                <p className="text-xs text-zinc-500 truncate">
                  {work.author}
                </p>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}