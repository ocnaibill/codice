import React from 'react';
import { Navbar } from './components/Navbar';
import { BookGrid } from './features/library/components/BookGrid';
import { Reader } from './features/reader/components/Reader';
import { useGlobalStore } from './store/useGlobalStore';
import { UploadModal } from './features/upload/components/UploadModal';

function App() {
  // Observa se há algum livro aberto na Store
  const activeBookId = useGlobalStore((state) => state.activeBookId);

  return (
    <div className="min-h-screen bg-zinc-950 flex flex-col">
      <Navbar />
      <UploadModal />
      <main className="flex-1">
        {/* O switch de roteamento virtual */}
        {activeBookId ? <Reader /> : <BookGrid />}
      </main>
    </div>
  );
}

export default App;