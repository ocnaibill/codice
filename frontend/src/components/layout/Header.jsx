import { useEffect, useRef, useState } from 'react';
import { useGlobalStore } from '../../store/useGlobalStore';
import { LibraryIcon } from '../ui/LibraryIcon';

export function Header({
  searchQuery = '',
  onSearchChange,
  onLogout,
  onChangePassword,
  onOpenPreferences,
  onOpenApps,
  onOpenAbout,
  onOpenSessions,
  canAdmin = false,
  onOpenAdmin,
  onGoHome,
  onOpenMenu,
  menuButtonRef,
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const account = useRef(null);
  const accountButton = useRef(null);
  const openUploadModal = useGlobalStore((state) => state.openUploadModal);
  // Choosing from the menu closes it, and the item that was chosen goes away with it: the focus goes to the button of the
  // account first, so that what the item opens (a dialog) gives the focus back to something that is still there.
  const pick = (action) => () => {
    setMenuOpen(false);
    accountButton.current?.focus();
    action?.();
  };
  useEffect(() => {
    if (menuOpen) account.current?.querySelector('[role="menuitem"]')?.focus();
    const onKey = (event) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        document.getElementById('library-search')?.focus();
      }
      if (event.key === 'Escape' && menuOpen) {
        setMenuOpen(false);
        accountButton.current?.focus();
      }
      if (
        menuOpen &&
        account.current?.contains(event.target) &&
        ['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)
      ) {
        event.preventDefault();
        const items = [
          ...account.current.querySelectorAll('[role="menuitem"]'),
        ];
        const current = items.indexOf(document.activeElement);
        const next =
          event.key === 'Home'
            ? 0
            : event.key === 'End'
              ? items.length - 1
              : (current +
                  (event.key === 'ArrowDown' ? 1 : -1) +
                  items.length) %
                items.length;
        items[next]?.focus();
      }
    };
    const onPointer = (event) => {
      if (!account.current?.contains(event.target)) setMenuOpen(false);
    };
    document.addEventListener('keydown', onKey);
    document.addEventListener('pointerdown', onPointer);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('pointerdown', onPointer);
    };
  }, [menuOpen]);

  return (
    <header className="library-header">
      <div className="library-mobile-brand">
        <button
          ref={menuButtonRef}
          className="library-icon-button"
          aria-label="Abrir menu da biblioteca"
          aria-haspopup="dialog"
          onClick={onOpenMenu}
        >
          <LibraryIcon name="menu" />
        </button>
        <button className="library-wordmark" onClick={onGoHome}>
          Códice
        </button>
      </div>
      <div className="library-search" role="search">
        <LibraryIcon name="search" />
        <input
          id="library-search"
          type="search"
          aria-label="Buscar no acervo"
          value={searchQuery}
          onChange={(event) => onSearchChange?.(event.target.value)}
          placeholder="Buscar obras, passagens e anotações…"
        />
        <kbd>⌘ / Ctrl K</kbd>
      </div>
      <div className="library-header-actions">
        {canAdmin && (
          <button
            onClick={openUploadModal}
            className="library-button library-button-primary"
            title="Adicionar um livro, mangá ou áudio"
          >
            <LibraryIcon name="plus" />
            <span>Adicionar</span>
          </button>
        )}
        <div
          ref={account}
          className="library-account"
          onBlur={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget))
              setMenuOpen(false);
          }}
        >
          <button
            ref={accountButton}
            onClick={() => setMenuOpen((open) => !open)}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            title="Minha conta"
            aria-label="Minha conta"
            className="library-avatar"
          >
            <LibraryIcon name="user" />
          </button>
          {menuOpen && (
            <div
              role="menu"
              aria-label="Minha conta"
              className="library-account-menu"
            >
              {canAdmin && (
                <button
                  role="menuitem"
                  onClick={pick(onOpenAdmin)}
                >
                  Administração
                </button>
              )}
              {onOpenPreferences && (
                <button
                  role="menuitem"
                  onClick={pick(onOpenPreferences)}
                >
                  Preferências
                </button>
              )}
              {onOpenApps && (
                <button
                  role="menuitem"
                  onClick={pick(onOpenApps)}
                >
                  Aplicativos
                </button>
              )}
              {onOpenSessions && (
                <button
                  role="menuitem"
                  onClick={pick(onOpenSessions)}
                >
                  Sessões e dispositivos
                </button>
              )}
              {onOpenAbout && (
                <button
                  role="menuitem"
                  onClick={pick(onOpenAbout)}
                >
                  Sobre
                </button>
              )}
              {onChangePassword && (
                <button
                  role="menuitem"
                  onClick={pick(onChangePassword)}
                >
                  Alterar senha
                </button>
              )}
              <button
                role="menuitem"
                onClick={pick(onLogout)}
              >
                Sair
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
