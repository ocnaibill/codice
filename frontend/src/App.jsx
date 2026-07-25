import React, { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Navbar } from './components/Navbar';
import { BookGrid } from './features/library/components/BookGrid';
import { Reader } from './features/reader/components/Reader';
import { useGlobalStore } from './store/useGlobalStore';
import { UploadModal } from './features/upload/components/UploadModal';

function App() {
  const queryClient = useQueryClient();
  // Check if there is an active book opened in Store
  const activeBookId = useGlobalStore((state) => state.activeBookId);

  useEffect(() => {
    // Establish WebSocket connection with Go backend
    const ws = new WebSocket('ws://localhost:8080/ws');

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'WORK_READY') {
          console.log(`🎉 Book processing completed: "${data.title}"`);
          // Silently invalidate works query cache to refresh library shelf in real time
          queryClient.invalidateQueries({ queryKey: ['works'] });
        }
      } catch (err) {
        console.error('Error parsing WebSocket message:', err);
      }
    };

    return () => {
      ws.close();
    };
  }, [queryClient]);

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