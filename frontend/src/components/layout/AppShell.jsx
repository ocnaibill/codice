import { useRef } from 'react';
import { Sidebar } from './Sidebar';
import { Header } from './Header';
import { LibraryIcon } from '../ui/LibraryIcon';
import { useGlobalStore } from '../../store/useGlobalStore';
import './library-shell.css';

export function AppShell({
  searchQuery,
  onSearchChange,
  onGoHome,
  onLogout,
  onChangePassword,
  canAdmin,
  onOpenAdmin,
  children,
}) {
  const drawer = useRef(null);
  const menuButton = useRef(null);
  const view = useGlobalStore((state) => state.libraryView);
  const adminOpen = useGlobalStore((state) => state.adminOpen);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const goHome = () => {
    onGoHome?.();
    setView('all');
    onSearchChange?.('');
  };
  const closeMenu = () => {
    drawer.current?.close();
    menuButton.current?.focus();
  };
  const sidebarProps = { onGoHome: goHome, canAdmin, onOpenAdmin };
  const focusSearch = () => document.getElementById('library-search')?.focus();

  return (
    <div className="library-shell">
      <a className="library-skip-link" href="#main-content">
        Ir para o conteúdo
      </a>
      <aside className="library-sidebar">
        <Sidebar {...sidebarProps} />
      </aside>
      <dialog
        ref={drawer}
        className="library-drawer"
        aria-label="Menu da biblioteca"
        onClick={(event) => {
          if (event.target === event.currentTarget) closeMenu();
        }}
      >
        <button
          className="library-icon-button library-drawer-close"
          aria-label="Fechar menu"
          onClick={closeMenu}
        >
          <LibraryIcon name="close" />
        </button>
        <Sidebar {...sidebarProps} onNavigate={closeMenu} />
      </dialog>
      <div className="library-main-column">
        <Header
          searchQuery={searchQuery}
          onSearchChange={onSearchChange}
          onLogout={onLogout}
          onChangePassword={onChangePassword}
          canAdmin={canAdmin}
          onOpenAdmin={onOpenAdmin}
          onGoHome={goHome}
          menuButtonRef={menuButton}
          onOpenMenu={() => drawer.current?.showModal()}
        />
        <main id="main-content" tabIndex={-1}>
          {children}
        </main>
      </div>
      <nav className="library-bottom-nav" aria-label="Navegação principal">
        {[
          ['all', 'library', 'Acervo'],
          ['reading', 'bookmark', 'Em leitura'],
          ['favorites', 'heart', 'Favoritos'],
        ].map(([key, icon, label]) => (
          <button
            key={key}
            aria-current={
              !searchQuery &&
              !adminOpen &&
              (view === key ||
                (key === 'all' && ['ebooks', 'comics', 'audio'].includes(view)))
                ? 'page'
                : undefined
            }
            onClick={() => setView(key)}
          >
            <LibraryIcon name={icon} />
            <span>{label}</span>
          </button>
        ))}
        <button
          onClick={focusSearch}
          aria-current={searchQuery ? 'page' : undefined}
        >
          <LibraryIcon name="search" />
          <span>Buscar</span>
        </button>
      </nav>
    </div>
  );
}
