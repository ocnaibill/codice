import React, { useState, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Navbar } from './components/Navbar';
import { BookGrid } from './features/library/components/BookGrid';
import { Reader } from './features/reader/components/Reader';
import { useGlobalStore } from './store/useGlobalStore';
import { UploadModal } from './features/upload/components/UploadModal';
import { Auth } from './features/auth/components/Auth';
import { FirstRunSetup } from './features/auth/components/FirstRunSetup';
import { api, wsUrl } from './lib/api';

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isFirstRun, setIsFirstRun] = useState(false);
  const [checkingStatus, setCheckingStatus] = useState(true);

  const queryClient = useQueryClient();
  const activeBookId = useGlobalStore((state) => state.activeBookId);

  useEffect(() => {
    const checkStatusAndToken = async () => {
      try {
        const res = await api.get('/auth/setup-status');
        if (res.data.isFirstRun) {
          // Always show first-run wizard regardless of stale tokens
          localStorage.removeItem('codice_token');
          setIsFirstRun(true);
        } else {
          const token = localStorage.getItem('codice_token');
          if (token) {
            setIsAuthenticated(true);
          }
        }
      } catch (err) {
        console.error('Error checking setup status:', err);
      } finally {
        setCheckingStatus(false);
      }
    };

    checkStatusAndToken();
  }, []);

  useEffect(() => {
    if (!isAuthenticated) return;

    // Establish WebSocket connection with dynamic host/protocol
    const ws = new WebSocket(wsUrl('/ws'));

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

  if (checkingStatus) {
    return (
      <div className="flex h-screen items-center justify-center bg-zinc-950 text-zinc-400 font-mono text-sm">
        Initializing Códice environment...
      </div>
    );
  }

  if (isFirstRun) {
    return (
      <FirstRunSetup 
        onSetupComplete={() => {
          setIsFirstRun(false);
          setIsAuthenticated(true);
        }} 
      />
    );
  }

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