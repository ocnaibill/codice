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

  const [toast, setToast] = useState(null);

  useEffect(() => {
    if (!isAuthenticated) return;

    let socket = null;
    let reconnectTimeout = null;
    let isCancelled = false;

    const connect = () => {
      try {
        socket = new WebSocket(wsUrl('/ws'));

        socket.onopen = () => {
          console.log('⚡ Real-time WebSocket connected');
        };

        socket.onmessage = (event) => {
          try {
            const data = JSON.parse(event.data);
            if (data.type === 'WORK_READY') {
              console.log(`🎉 Book processing completed: "${data.title}"`);
              queryClient.invalidateQueries({ queryKey: ['works'] });
              setToast({
                type: 'success',
                message: `✨ Metadata updated: ${data.title || 'Book ready'}`,
              });
              setTimeout(() => setToast(null), 5000);
            } else if (data.type === 'WORK_ANALYZING') {
              console.log(`🔍 Book analyzing: ID ${data.work_id}`);
              queryClient.invalidateQueries({ queryKey: ['works'] });
            } else if (data.type === 'WORK_ERROR') {
              console.warn(`❌ Processing error for Work ID ${data.work_id}:`, data.error);
              queryClient.invalidateQueries({ queryKey: ['works'] });
              setToast({
                type: 'error',
                message: `❌ Failed to process book: ${data.error || 'Unknown error'}`,
              });
              setTimeout(() => setToast(null), 6000);
            }
          } catch (err) {
            console.error('Error parsing WebSocket message:', err);
          }
        };

        socket.onclose = () => {
          if (!isCancelled) {
            console.log('WebSocket connection closed. Retrying in 3s...');
            reconnectTimeout = setTimeout(connect, 3000);
          }
        };

        socket.onerror = (err) => {
          console.error('WebSocket error:', err);
        };
      } catch (err) {
        console.error('Failed to create WebSocket:', err);
        if (!isCancelled) {
          reconnectTimeout = setTimeout(connect, 3000);
        }
      }
    };

    connect();

    return () => {
      isCancelled = true;
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (socket) socket.close();
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
    <div className="min-h-screen bg-zinc-950 flex flex-col relative">
      {/* Toast Notification Banner */}
      {toast && (
        <div className={`fixed top-4 right-4 z-50 px-4 py-3 rounded-lg shadow-xl border text-sm font-medium transition-all duration-300 flex items-center gap-3 ${
          toast.type === 'error' 
            ? 'bg-red-950/90 text-red-200 border-red-800' 
            : 'bg-zinc-900/90 text-emerald-300 border-emerald-800/60 backdrop-blur'
        }`}>
          <span>{toast.message}</span>
          <button 
            onClick={() => setToast(null)}
            className="text-zinc-500 hover:text-zinc-300 text-xs ml-2"
          >
            ✕
          </button>
        </div>
      )}
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