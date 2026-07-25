import React, { useState, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Navbar } from './components/Navbar';
import { BookGrid } from './features/library/components/BookGrid';
import { Reader } from './features/reader/components/Reader';
import { useGlobalStore } from './store/useGlobalStore';
import { UploadModal } from './features/upload/components/UploadModal';
import { Auth } from './features/auth/components/Auth';

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const queryClient = useQueryClient();
  const activeBookId = useGlobalStore((state) => state.activeBookId);

  useEffect(() => {
    // Check if token exists in localStorage on app mount
    const token = localStorage.getItem('codice_token');
    if (token) {
      setIsAuthenticated(true);
    }
  }, []);

  useEffect(() => {
    if (!isAuthenticated) return;

    // Establish WebSocket connection with Go backend when authenticated
    const ws = new WebSocket('ws://localhost:8080/ws');

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'WORK_READY') {
          console.log(`🎉 Book processing completed: "${data.title}"`);
          queryClient.invalidateQueries({ queryKey: ['works'] });
        }
      } catch (err) {
        console.error('Error parsing WebSocket message:', err);
      }
    };

    return () => {
      ws.close();
    };
  }, [queryClient, isAuthenticated]);

  const handleLogout = () => {
    localStorage.removeItem('codice_token');
    setIsAuthenticated(false);
  };

  if (!isAuthenticated) {
    return <Auth onLoginSuccess={() => setIsAuthenticated(true)} />;
  }

  return (
    <div className="min-h-screen bg-zinc-950 flex flex-col">
      <Navbar onLogout={handleLogout} />
      <UploadModal />
      <main className="flex-1">
        {/* Virtual routing view switch */}
        {activeBookId ? <Reader /> : <BookGrid />}
      </main>
    </div>
  );
}

export default App;