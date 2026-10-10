import { GreetingStats } from '../features/home/components/GreetingStats';
import { ContinueReading } from '../features/home/components/ContinueReading';
import { LibraryFilterBar } from '../features/home/components/LibraryFilterBar';
import { LibraryGrid } from '../features/home/components/LibraryGrid';
import { FavoriteSeries } from '../features/home/components/FavoriteSeries';
import { NotesQuotes } from '../features/home/components/NotesQuotes';
import { useMe } from '../features/auth/api/useMe';
import { useStats } from '../features/home/api/useStats';
import { useFavorites } from '../features/home/api/useFavorites';
import { useNotes } from '../features/home/api/useNotes';
import { useWorks } from '../features/library/api/useWorks';
import { useGlobalStore } from '../store/useGlobalStore';
import { SearchPage } from '../features/search/SearchPage';
import { CollectionsGrid } from '../features/collections/components/CollectionsGrid';
import { useCollections } from '../features/collections/api/useCollections';
import { isStaff } from '../features/auth/api/useMe';
import { CategoryShelf } from '../features/home/components/CategoryShelf';
import { CategoryRows } from '../features/home/components/CategoryRows';
import { Shelf } from '../features/home/components/Shelf';
import { CategoryPage } from '../features/categories/components/CategoryPage';

// "Adicionados recentemente" says how the catalog is sorted, so it only holds while it is: sorted by title or
// by author the heading says that instead (the other views are named by what they hold, not by the order).
const SORTED_TITLES = { title: 'Todas as obras, por título', author: 'Todas as obras, por autor' };
const GRID_TITLES = {
  all: 'Adicionados recentemente',
  ebooks: 'Livros digitais',
  comics: 'Quadrinhos',
  mangas: 'Mangás',
  audio: 'Audiolivros',
  reading: 'Em leitura',
  favorites: 'Obras favoritas',
  collections: 'Coleções',
};

function QueryError({ children, onRetry }) {
  return (
    <div role="alert" className="library-error">
      <p>{children}</p>
      <button className="library-button" onClick={onRetry}>
        Tentar novamente
      </button>
    </div>
  );
}

export function HomePage({ searchQuery = '' }) {
  const categoryId = useGlobalStore((state) => state.categoryPageId);
  if (searchQuery.trim()) return <SearchPage query={searchQuery} />;
  // A category has a page of its own, in the place of the shelves (DEC-140).
  if (categoryId) return <CategoryPage id={categoryId} />;
  return <HomeDashboard />;
}

function HomeDashboard() {
  const { data: me } = useMe();
  const view = useGlobalStore((state) => state.libraryView);
  const page = useGlobalStore((state) => state.libraryPage);
  const viewMode = useGlobalStore((state) => state.libraryViewMode);
  const sort = useGlobalStore((state) => state.librarySort);
  const setSort = useGlobalStore((state) => state.setLibrarySort);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const setPage = useGlobalStore((state) => state.setLibraryPage);
  const setViewMode = useGlobalStore((state) => state.setLibraryViewMode);
  const stats = useStats();
  const inProgress = useWorks({ inProgress: true, limit: 5 });
  const grid = useWorks({
    page,
    limit: 12,
    formatGroup: ['ebooks', 'comics', 'mangas', 'audio'].includes(view) ? view : 'all',
    inProgress: view === 'reading',
    favorite: view === 'favorites',
    sort,
    // A series is one card in the shelves of the library; the other views list the works (#187).
    series: ['all', 'ebooks', 'comics', 'mangas', 'audio'].includes(view) ? 'collapse' : undefined,
  });
  const favorites = useFavorites();
  const collections = useCollections({ limit: 1 });
  const lists = useCollections({ limit: 1, kind: 'personal' });
  const notes = useNotes({ limit: 2, sample: true });
  const totalPages = grid.data?.totalPages ?? 1;
  const changePage = (next) => {
    setPage(next);
    document
      .getElementById('library-catalog')
      ?.scrollIntoView({ block: 'start' });
  };

  // The home is shelves while it is the plain one: all the works, the newest first, as a grid. Another kind, another order or the list is
  // the paginated catalog, in the same place, so that every work can be reached.
  const shelves = view === 'all' && sort === 'added' && viewMode === 'grid';
  const catalogTitle = (view === 'all' && SORTED_TITLES[sort]) || GRID_TITLES[view];
  const personal = (
    <div className="home-rail">
      <div className="home-favorites">
        {favorites.isError ? (
          <QueryError onRetry={() => favorites.refetch()}>Não foi possível carregar os favoritos.</QueryError>
        ) : (
          <FavoriteSeries items={favorites.data?.data ?? []} total={favorites.data?.total ?? 0} isLoading={favorites.isLoading} />
        )}
      </div>
      <div className="home-notes">
        {notes.isError ? (
          <QueryError onRetry={() => notes.refetch()}>Não foi possível carregar as anotações.</QueryError>
        ) : (
          <NotesQuotes notes={notes.data?.data ?? []} isLoading={notes.isLoading} />
        )}
      </div>
    </div>
  );

  return (
    <div className="library-dashboard">
      <GreetingStats
        userName={me?.displayName || me?.username}
        stats={stats.data}
        isLoading={stats.isLoading}
        error={stats.isError}
        onRetry={() => stats.refetch()}
      />
      <div className="home-columns" data-catalog={shelves ? 'shelves' : 'full'}>
        <div className="home-continue">
          {inProgress.isError ? (
            <QueryError onRetry={() => inProgress.refetch()}>Não foi possível carregar as leituras em andamento.</QueryError>
          ) : (
            <ContinueReading items={inProgress.data?.data ?? []} isLoading={inProgress.isLoading} onViewAll={() => setView('reading')} />
          )}
        </div>
        <section id="library-catalog" aria-label="Acervo" className="home-catalog" style={{ scrollMarginTop: 96 }}>
          <LibraryFilterBar
            worksTotal={stats.data?.worksTotal}
            breakdown={stats.data?.libraryBreakdown}
            activeFilter={view}
            onFilterChange={setView}
            viewMode={viewMode}
            onViewModeChange={setViewMode}
            sort={sort}
            onSortChange={view === 'collections' || view === 'lists' ? undefined : setSort}
            collectionsTotal={collections.data?.total ?? 0}
            canManageCollections={isStaff(me)}
            listsTotal={lists.data?.total}
          />
          {view === 'collections' || view === 'lists' ? (
            <CollectionsGrid viewMode={viewMode} kind={view === 'lists' ? 'personal' : 'official'} />
          ) : grid.isError ? (
            <QueryError onRetry={() => grid.refetch()}>Não foi possível carregar o acervo.</QueryError>
          ) : shelves ? (
            <Shelf
              headingId="recent-heading"
              title={catalogTitle}
              total={grid.data?.total}
              countWord={grid.data?.series ? 'itens' : 'obras'}
              items={grid.data?.data ?? []}
              isLoading={grid.isLoading}
              emptyText="Nenhuma obra encontrada nessa categoria ainda."
            />
          ) : (
            <>
              <LibraryGrid
                items={grid.data?.data ?? []}
                isLoading={grid.isLoading}
                isFetching={grid.isFetching}
                title={catalogTitle}
                viewMode={viewMode}
                total={grid.data?.total}
                countWord={grid.data?.series ? 'itens' : 'obras'}
              />
              {totalPages > 1 && (
                <nav className="library-pagination" aria-label="Páginas do acervo">
                  <button className="library-button" disabled={page <= 1 || grid.isFetching} onClick={() => changePage(page - 1)}>
                    Anterior
                  </button>
                  <span aria-live="polite">
                    {page} de {totalPages}
                  </span>
                  <button className="library-button" disabled={page >= totalPages || grid.isFetching} onClick={() => changePage(page + 1)}>
                    Próxima
                  </button>
                </nav>
              )}
            </>
          )}
        </section>
        {personal}
      </div>
      {shelves && <CategoryRows />}
      {view === 'all' && <CategoryShelf />}
    </div>
  );
}
