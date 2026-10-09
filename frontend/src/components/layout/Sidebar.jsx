import { LibraryIcon } from '../ui/LibraryIcon';
import { useGlobalStore } from '../../store/useGlobalStore';
import { useStats } from '../../features/home/api/useStats';
import { useCollections } from '../../features/collections/api/useCollections';
import { bracketCount } from '../../features/home/utils/format';

// What each item counts, from GET /stats (retired works are not counted). "Todas as obras" is always there; a kind of
// work (books, comics, mangas, audiobooks) is there only when the library has at least one of it, so a library with no
// audiobooks does not offer a shelf of them (`dynamic`). Nothing is offered until the counts are known.
const LIBRARIES = [
  { key: 'all', label: 'Todas as obras', icon: 'library', count: (stats) => stats?.worksTotal },
  { key: 'ebooks', label: 'Livros digitais', icon: 'book', dynamic: true, count: (stats) => stats?.libraryBreakdown?.livros },
  { key: 'comics', label: 'Quadrinhos', icon: 'comics', dynamic: true, count: (stats) => stats?.libraryBreakdown?.quadrinhos },
  { key: 'mangas', label: 'Mangás', icon: 'comics', dynamic: true, count: (stats) => stats?.libraryBreakdown?.mangas },
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
  const categoryOpen = useGlobalStore((state) => state.categoryPageId != null);
  const openNotes = useGlobalStore((state) => state.openNotes);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const { data: stats } = useStats();
  // Collections are offered once there is one; the staff always has them, to make the first (#206).
  const collections = useCollections({ limit: 1 }).data?.total ?? 0;
  // The lists of the person are always there: anybody can keep them (#207).
  const lists = useCollections({ limit: 1, kind: 'personal' }).data?.total;
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
        !search && !adminOpen && !notesOpen && !categoryOpen && view === item.key ? 'page' : undefined
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
        {(collections > 0 || canAdmin) && navItem({ key: 'collections', label: 'Coleções', icon: 'grid', count: () => collections })}
        <p className="library-eyebrow library-nav-group">Sua coleção</p>
        {COLLECTION.map(navItem)}
        {navItem({ key: 'lists', label: 'Minhas listas', icon: 'list', count: () => lists })}
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
