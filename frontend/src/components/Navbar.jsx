import React from 'react';
import { useGlobalStore } from '../store/useGlobalStore';

export function Navbar() {
  // Puxando o estado e a ação do Zustand
  const searchQuery = useGlobalStore((state) => state.searchQuery);
  const setSearchQuery = useGlobalStore((state) => state.setSearchQuery);
  const activeBookId = useGlobalStore((state) => state.activeBookId);
  const closeBook = useGlobalStore((state) => state.closeBook);
  const openUploadModal = useGlobalStore((state) => state.openUploadModal);

  return (
    <nav className="sticky top-0 z-50 w-full bg-zinc-950/80 backdrop-blur-md border-b border-zinc-800">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex justify-between items-center h-16">
          {/* Logo - Agora clica para voltar para a estante */}
          <div 
            className="flex-shrink-0 flex items-center gap-2 cursor-pointer"
            onClick={closeBook}
          >
            <span className="text-2xl">📚</span>
            <span className="font-bold text-xl tracking-tight text-zinc-100">Códice</span>
          </div>

          {/* Search Bar FTS - Esconde se estivermos lendo um livro */}
          {!activeBookId && (
            <div className="flex-1 max-w-xl mx-8">
              <div className="relative group">
                <input
                  type="text"
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="block w-full pl-10 pr-3 py-2 border border-zinc-800 rounded-lg bg-zinc-900 text-zinc-300 placeholder-zinc-500 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all sm:text-sm"
                  placeholder="Buscar por título, autor ou conteúdo..."
                />
              </div>
            </div>
          )}

          {/* Menu Actions */}
          <div className="flex items-center gap-4">
            <button className="text-sm font-medium text-zinc-400 hover:text-zinc-100 transition-colors">
              Painel Admin
            </button>
            <button 
              onClick={openUploadModal}
              className="text-sm font-medium bg-zinc-800 border border-zinc-700 text-zinc-300 px-3 py-1.5 rounded-md hover:bg-zinc-700 hover:text-zinc-100 transition-colors">
              + Novo Livro
            </button>
            <div className="h-8 w-8 rounded-full bg-zinc-800 border border-zinc-700 flex items-center justify-center cursor-pointer hover:ring-2 hover:ring-blue-500 transition-all">
              <span className="text-xs font-bold text-zinc-300">BO</span>
            </div>
          </div>
        </div>
      </div>
    </nav>
  );
}