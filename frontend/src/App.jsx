import React, { useState, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { AppShell } from './components/layout/AppShell';
import { HomePage } from './pages/HomePage';
import { Reader } from './features/reader/components/Reader';
import { useGlobalStore } from './store/useGlobalStore';
import { UploadModal } from './features/upload/components/UploadModal';
import { Auth } from './features/auth/components/Auth';
import { FirstRunSetup } from './features/auth/components/FirstRunSetup';
import { api, wsUrl, refreshAssetToken, clearAssetToken, UNAUTHORIZED_EVENT } from './lib/api';

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isFirstRun, setIsFirstRun] = useState(false);
  const [checkingStatus, setCheckingStatus] = useState(true);
  const [assetsReady, setAssetsReady] = useState(false);

  const queryClient = useQueryClient();
  const activeBookId = useGlobalStore((state) => state.activeBookId);
  const closeBook = useGlobalStore((state) => state.closeBook);
  const searchQuery = useGlobalStore((state) => state.searchQuery);
  const setSearchQuery = useGlobalStore((state) => state.setSearchQuery);

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

  // The server no longer accepts our session (expired, revoked, blocked): back to login.
  useEffect(() => {
    const onSessionLost = () => setIsAuthenticated(false);
    window.addEventListener(UNAUTHORIZED_EVENT, onSessionLost);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onSessionLost);
  }, []);

  // Covers, files and pages load via <img>/<audio>, which cannot send headers, so
  // they carry a short-lived asset token that api.js keeps renewed.
  useEffect(() => {
    if (!isAuthenticated) {
      clearAssetToken();
      setAssetsReady(false);
      return;
    }
    let cancelled = false;
    refreshAssetToken()
      .catch((err) => console.error('Could not obtain the asset token:', err))
      .finally(() => {
        if (!cancelled) setAssetsReady(true);
      });
    return () => {
      cancelled = true;
    };
  }, [isAuthenticated]);

  const [toast, setToast] = useState(null);

  useEffect(() => {
    if (!isAuthenticated) return;

    let socket = null;
    let reconnectTimeout = null;
    let isCancelled = false;

    const connect = async () => {
      try {
        // Each connection needs its own 60-second ticket from the server.
        const url = await wsUrl('/ws');
        if (isCancelled) return;
        socket = new WebSocket(url);

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

  const handleLogout = async () => {
    try {
      // Revoke the session on the server so the token is dead everywhere.
      await api.post('/auth/logout');
    } catch (err) {
      console.error('Logout request failed:', err);
    }
    localStorage.removeItem('codice_token');
    clearAssetToken();
    setIsAuthenticated(false);
  };

  if (checkingStatus) {
    return (
      <div className="flex h-screen items-center justify-center bg-surface text-ink-soft font-mono text-sm">
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

  if (!assetsReady) {
    return (
      <div className="flex h-screen items-center justify-center bg-surface text-ink-soft font-mono text-sm">
        Initializing Códice environment...
      </div>
    );
  }

  return (
    <div className="relative">
      {/* Toast Notification Banner */}
      {toast && (
        <div className={`fixed top-4 right-4 z-50 px-4 py-3 rounded-lg shadow-xl border text-sm font-medium transition-all duration-300 flex items-center gap-3 ${
          toast.type === 'error'
            ? 'bg-red-50 text-red-700 border-red-200'
            : 'bg-white text-success border-success/30 backdrop-blur'
        }`}>
          <span>{toast.message}</span>
          <button
            onClick={() => setToast(null)}
            className="text-ink-faint hover:text-ink text-xs ml-2"
          >
            ✕
          </button>
        </div>
      )}
      <UploadModal />
      {activeBookId ? (
        <Reader />
      ) : (
        <AppShell
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          onGoHome={closeBook}
          onLogout={handleLogout}
        >
          <HomePage />
        </AppShell>
      )}
    </div>
  );
}

export default App;