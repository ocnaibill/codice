import React, { useState, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { AppShell } from './components/layout/AppShell';
import { HomePage } from './pages/HomePage';
import { Reader } from './features/reader/components/Reader';
import { WorkSheet } from './features/reader/components/WorkSheet';
import { NotesPage } from './features/notes/NotesPage';
import { EditBookModal } from './features/library/components/EditBookModal';
import { useGlobalStore } from './store/useGlobalStore';
import { UploadModal } from './features/upload/components/UploadModal';
import { Auth } from './features/auth/components/Auth';
import { useMe, isStaff } from './features/auth/api/useMe';
import { setPreferenceOwner } from './features/reader/preferences';
import { AdminPage } from './features/admin/AdminPage';
import { NoPermission } from './components/ui/PermissionNote';
import { FirstRunSetup } from './features/auth/components/FirstRunSetup';
import { OwnershipBanner } from './features/ownership/OwnershipBanner';
import { ReadingPreferencesSync } from './features/reader/ReadingPreferencesSync';
import { ChangePasswordModal } from './components/layout/ChangePasswordModal';
import { PreferencesModal } from './components/layout/PreferencesModal';
import { AppsModal } from './components/layout/AppsModal';
import { AboutModal } from './components/layout/AboutModal';
import { SessionsModal } from './components/layout/SessionsModal';
import { ResetPassword } from './features/auth/components/ResetPassword';
import { AcceptInvite } from './features/auth/components/AcceptInvite';
import { api, wsUrl, refreshAssetToken, clearAssetToken, UNAUTHORIZED_EVENT } from './lib/api';
import { refreshLibrary } from './lib/refreshLibrary';
import { ToastRegion } from './components/ui/ToastRegion';
import { useToasts } from './components/ui/toast';
import { noticeForWorkEvent } from './features/library/workNotice';

// What the server says of a work (ready, or failed) becomes a notice, with a way to open the work.
function showWorkNotice(event) {
  const notice = noticeForWorkEvent(event, useGlobalStore.getState().openWork);
  if (notice) useToasts.getState().show(notice);
}

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isFirstRun, setIsFirstRun] = useState(false);
  const [checkingStatus, setCheckingStatus] = useState(true);
  const [assetsReady, setAssetsReady] = useState(false);
  const [changingPassword, setChangingPassword] = useState(false);
  const [preferencesOpen, setPreferencesOpen] = useState(false);
  const [appsOpen, setAppsOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);
  const [sessionsOpen, setSessionsOpen] = useState(false);
  // A link like /?invite=<secret> opens the sign-up page for that invitation.
  const [inviteToken, setInviteToken] = useState(() => new URLSearchParams(window.location.search).get('invite'));
  const [resetToken, setResetToken] = useState(() => new URLSearchParams(window.location.search).get('reset'));
  const leaveLink = () => {
    localStorage.removeItem('codice_token');
    window.history.replaceState(null, '', window.location.pathname);
    setInviteToken(null);
    setResetToken(null);
  };
  const leaveInvite = () => {
    window.history.replaceState(null, '', window.location.pathname);
    setInviteToken(null);
  };

  const queryClient = useQueryClient();
  const activeBookId = useGlobalStore((state) => state.activeBookId);
  const closeBook = useGlobalStore((state) => state.closeBook);
  const searchQuery = useGlobalStore((state) => state.searchQuery);
  const setSearchQuery = useGlobalStore((state) => state.setSearchQuery);
  const adminOpen = useGlobalStore((state) => state.adminOpen);
  const notesOpen = useGlobalStore((state) => state.notesOpen);
  const metadataWorkId = useGlobalStore((state) => state.metadataWorkId);
  const metadataTab = useGlobalStore((state) => state.metadataTab);
  const closeMetadata = useGlobalStore((state) => state.closeMetadata);
  const openAdmin = useGlobalStore((state) => state.openAdmin);
  const { data: me } = useMe(isAuthenticated);
  const staff = isStaff(me);

  // Display preferences belong to the account that is signed in, not to the browser.
  useEffect(() => {
    setPreferenceOwner(isAuthenticated ? me?.id : null);
  }, [isAuthenticated, me?.id]);

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
          // A reset link always shows the reset form, even when there is a stored
          // session. That session is cleared when returning to the login page.
          const hasResetLink = new URLSearchParams(window.location.search).has('reset');
          if (token && !hasResetLink) {
            setIsAuthenticated(true);
          }
        }
      } catch (err) {
        console.error('Error checking setup status:', err);
        // The question "has the server been set up?" failing (the limit on those calls, a moment without network) says
        // nothing about the session: the person stays where they were, and the first thing that needs the server asks
        // again and sends them to the login if the session really is gone.
        const hasResetLink = new URLSearchParams(window.location.search).has('reset');
        if (localStorage.getItem('codice_token') && !hasResetLink) setIsAuthenticated(true);
      } finally {
        setCheckingStatus(false);
      }
    };

    checkStatusAndToken();
  }, []);

  // The server no longer accepts our session (expired, revoked, blocked): back to login.
  useEffect(() => {
    const onSessionLost = () => {
      queryClient.removeQueries({ queryKey: ['search'] });
      queryClient.removeQueries({ queryKey: ['notes'] });
      setSearchQuery('');
      setIsAuthenticated(false);
    };
    window.addEventListener(UNAUTHORIZED_EVENT, onSessionLost);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onSessionLost);
  }, [queryClient, setSearchQuery]);

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

  // Reading moves progress and the counters; refresh them when the reader closes.
  const readerWasOpen = React.useRef(false);
  useEffect(() => {
    if (activeBookId) readerWasOpen.current = true;
    else if (readerWasOpen.current) {
      readerWasOpen.current = false;
      refreshLibrary(queryClient);
      queryClient.invalidateQueries({ queryKey: ['notes'] });
    }
  }, [activeBookId, queryClient]);

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
              refreshLibrary(queryClient);
              showWorkNotice(data);
            } else if (data.type === 'WORK_ANALYZING') {
              console.log(`🔍 Book analyzing: ID ${data.work_id}`);
              refreshLibrary(queryClient);
            } else if (data.type === 'WORK_ERROR') {
              console.warn(`❌ Processing error for Work ID ${data.work_id}:`, data.error);
              refreshLibrary(queryClient);
              showWorkNotice(data);
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
    queryClient.removeQueries({ queryKey: ['me'] });
    queryClient.removeQueries({ queryKey: ['admin'] });
    queryClient.removeQueries({ queryKey: ['search'] });
    queryClient.removeQueries({ queryKey: ['notes'] });
    setSearchQuery('');
    closeBook();
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

  if (!isAuthenticated && resetToken) {
    return <ResetPassword token={resetToken} onDone={leaveLink} />;
  }

  if (!isAuthenticated && inviteToken) {
    return (
      <AcceptInvite
        token={inviteToken}
        onAccepted={() => { leaveInvite(); setIsAuthenticated(true); }}
        onCancel={leaveInvite}
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
      <ToastRegion />
      <UploadModal />
      <OwnershipBanner me={me} />
      <ReadingPreferencesSync key={me?.id ?? "none"} userId={me?.id} />
      {changingPassword && <ChangePasswordModal onClose={() => setChangingPassword(false)} />}
      {preferencesOpen && <PreferencesModal onClose={() => setPreferencesOpen(false)} />}
      {appsOpen && <AppsModal onClose={() => setAppsOpen(false)} />}
      {aboutOpen && <AboutModal onClose={() => setAboutOpen(false)} />}
      {sessionsOpen && <SessionsModal onClose={() => setSessionsOpen(false)} onOpenApps={() => setAppsOpen(true)} />}
      <WorkSheet />
      {metadataWorkId && <EditBookModal key={`${metadataWorkId}-${metadataTab}`} workId={metadataWorkId} tab={metadataTab} onClose={closeMetadata} />}
      {activeBookId ? (
        <Reader />
      ) : (
        <AppShell
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          onGoHome={closeBook}
          onLogout={handleLogout}
          onChangePassword={() => setChangingPassword(true)}
          onOpenPreferences={() => setPreferencesOpen(true)}
          onOpenApps={() => setAppsOpen(true)}
          onOpenAbout={() => setAboutOpen(true)}
          onOpenSessions={() => setSessionsOpen(true)}
          canAdmin={staff}
          onOpenAdmin={openAdmin}
        >
          {adminOpen && !staff ? (
            <NoPermission title="A administração é de quem cuida do acervo" onBack={closeBook}>
              Sua conta é de leitura. Se você precisa mexer no acervo, peça a quem administra.
            </NoPermission>
          ) : adminOpen ? (
            <AdminPage isOwner={me.role === 'owner'} onClose={closeBook} />
          ) : notesOpen && !searchQuery.trim() ? (
            <NotesPage />
          ) : (
            <HomePage searchQuery={searchQuery} />
          )}
        </AppShell>
      )}
    </div>
  );
}

export default App;
