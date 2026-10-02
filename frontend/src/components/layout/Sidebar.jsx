import { LibraryIcon } from '../ui/LibraryIcon';
import { useGlobalStore } from '../../store/useGlobalStore';
import { useStats } from '../../features/home/api/useStats';
import { bracketCount } from '../../features/home/utils/format';

// What each item counts, from GET /stats (retired works are not counted). "Todas as obras" is always there; a kind of
// work (books, comics, audiobooks) is there only when the library has at least one of it, so a library with no
// audiobooks does not offer a shelf of them (`dynamic`). Nothing is offered until the counts are known.
const LIBRARIES = [
  { key: 'all', label: 'Todas as obras', icon: 'library', count: (stats) => stats?.worksTotal },
  { key: 'ebooks', label: 'Livros digitais', icon: 'book', dynamic: true, count: (stats) => stats?.libraryBreakdown?.livros },
  { key: 'comics', label: 'Quadrinhos & mangás', icon: 'comics', dynamic: true, count: (stats) => stats?.libraryBreakdown?.mangas },
  { key: 'audio', label: 'Audiolivros', icon: 'audio', dynamic: true, count: (stats) => stats?.libraryBreakdown?.audio },
];
const COLLECTION = [
  { key: 'reading', label: 'Em leitura', icon: 'bookmark', count: (stats) => stats?.inProgressCount },
  { key: 'favorites', label: 'Favoritos', icon: 'heart' },
];

export function Sidebar({ onGoHome, onNavigate, canAdmin, onOpenAdmin }) {
  const view = useGlobalStore((state) => state.libraryView);
  const search = useGlobalStore((state) => state.searchQuery);
  const adminOpen = useGlobalStore((state) => state.adminOpen);
  const notesOpen = useGlobalStore((state) => state.notesOpen);
  const openNotes = useGlobalStore((state) => state.openNotes);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const { data: stats } = useStats();
  const navigate = (key) => {
    setView(key);
    onNavigate?.();
  };
  const navItem = (item) =>
    item.dynamic && !(item.count(stats) > 0) ? null : (
    <button
      key={item.key}
      onClick={() => navigate(item.key)}
      aria-current={
        !search && !adminOpen && !notesOpen && view === item.key ? 'page' : undefined
      }
      className="library-nav-item"
    >
      <LibraryIcon name={item.icon} />
      <span>{item.label}</span>
      {item.count && bracketCount(item.count(stats)) && <small>{bracketCount(item.count(stats))}</small>}
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
          Biblioteca {stats?.worksTotal != null && <span>{bracketCount(stats.worksTotal)}</span>}
        </p>
        {LIBRARIES.map(navItem)}
        <p className="library-eyebrow library-nav-group">Sua coleção</p>
        {COLLECTION.map(navItem)}
        <button
          className="library-nav-item"
          aria-current={notesOpen && !search ? 'page' : undefined}
          onClick={() => {
            openNotes();
            onNavigate?.();
          }}
        >
          <LibraryIcon name="note" />
          <span>Anotações</span>
        </button>
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
