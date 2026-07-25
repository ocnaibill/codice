import React from 'react';
import { Navbar } from './components/Navbar';
import { BookGrid } from './features/library/components/BookGrid';
import { Reader } from './features/reader/components/Reader';
import { useGlobalStore } from './store/useGlobalStore';
import { UploadModal } from './features/upload/components/UploadModal';

function App() {
  // Check if there is an active book opened in Store
  const activeBookId = useGlobalStore((state) => state.activeBookId);

  return (
    <div className="min-h-screen bg-zinc-950 flex flex-col">
      <Navbar />
      <UploadModal />
      <main className="flex-1">
        {/* Virtual routing view switch */}
        {activeBookId ? <Reader /> : <BookGrid />}
      </main>
    </div>
  );
}

export default App;