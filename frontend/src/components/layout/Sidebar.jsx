import { LibraryIcon } from '../ui/LibraryIcon';
import { useGlobalStore } from '../../store/useGlobalStore';

const LIBRARIES = [
  { key: 'all', label: 'Todas as obras', icon: 'library' },
  {
    key: 'ebooks',
    label: 'Livros digitais',
    icon: 'book',
    format: 'EPUB / PDF',
  },
  {
    key: 'comics',
    label: 'Quadrinhos & mangás',
    icon: 'comics',
    format: 'CBZ / CBR',
  },
  { key: 'audio', label: 'Audiolivros', icon: 'audio', format: 'ÁUDIO' },
];
const COLLECTION = [
  { key: 'reading', label: 'Em leitura', icon: 'bookmark' },
  { key: 'favorites', label: 'Favoritos', icon: 'heart' },
];

export function Sidebar({ onGoHome, onNavigate, canAdmin, onOpenAdmin }) {
  const view = useGlobalStore((state) => state.libraryView);
  const search = useGlobalStore((state) => state.searchQuery);
  const adminOpen = useGlobalStore((state) => state.adminOpen);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const navigate = (key) => {
    setView(key);
    onNavigate?.();
  };
  const navItem = (item) => (
    <button
      key={item.key}
      onClick={() => navigate(item.key)}
      aria-current={
        !search && !adminOpen && view === item.key ? 'page' : undefined
      }
      className="library-nav-item"
    >
      <LibraryIcon name={item.icon} />
      <span>{item.label}</span>
      {item.format && <small>{item.format}</small>}
    </button>
  );
  return (
    <div className="library-sidebar-content">
      <button
        className="library-brand"
        onClick={() => {
          onGoHome?.();
          onNavigate?.();
        }}
      >
        <LibraryIcon name="book" />
        <span>
          <strong>Códice</strong>
          <small>Acervo & leitura</small>
        </span>
      </button>
      <nav aria-label="Bibliotecas" className="library-nav">
        <p className="library-eyebrow">
          Biblioteca <span>[ 04 ]</span>
        </p>
        {LIBRARIES.map(navItem)}
        <p className="library-eyebrow library-nav-group">Sua coleção</p>
        {COLLECTION.map(navItem)}
        {canAdmin && (
          <>
            <p className="library-eyebrow library-nav-group">
              Gestão do acervo
            </p>
            <button
              className="library-nav-item"
              aria-current={adminOpen ? 'page' : undefined}
              onClick={() => {
                onOpenAdmin?.();
                onNavigate?.();
              }}
            >
              <LibraryIcon name="settings" />
              <span>Administração</span>
            </button>
          </>
        )}
      </nav>
      <div className="library-sidebar-footer">
        <LibraryIcon name="book" />
        <p>
          Um lugar para suas leituras.
          <br />
          <span>Seu acervo, no seu tempo.</span>
        </p>
      </div>
    </div>
  );
}
