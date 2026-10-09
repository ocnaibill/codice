import { useEffect, useRef } from 'react';
import { useCategories } from '../api/useCategories';
import { useWorks } from '../../library/api/useWorks';
import { LibraryGrid } from '../../home/components/LibraryGrid';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { ancestorsOf } from '../tree';

const PAGE_SIZE = 12;

/**
 * The page of a category (DEC-140): where it is in the tree, its subcategories, and the works in it and under it, a page at a time.
 * It takes the place of the shelves of the library; the way back is the library, or one of the categories above it.
 */
export function CategoryPage({ id }) {
  const { data: categories, isLoading: loadingTree, isError: treeFailed, refetch: retryTree } = useCategories();
  const page = useGlobalStore((state) => state.libraryPage);
  const setPage = useGlobalStore((state) => state.setLibraryPage);
  const viewMode = useGlobalStore((state) => state.libraryViewMode);
  const openCategory = useGlobalStore((state) => state.openCategory);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const works = useWorks({ page, limit: PAGE_SIZE, category: id, sort: 'title' });
  // Opening a category from the row at the bottom of the home, or one of its subcategories, shows the page from the top.
  const top = useRef(null);
  useEffect(() => {
    top.current?.scrollIntoView?.({ block: 'start' });
  }, [id]);
  const category = (categories ?? []).find((item) => item.id === id);
  const children = (categories ?? []).filter((item) => item.parentId === id && item.works > 0);
  const totalPages = works.data?.totalPages ?? 1;

  const back = (
    <button className="library-button" onClick={() => setView('all')}>← Acervo</button>
  );
  if (treeFailed) {
    return (
      <div ref={top} className="library-dashboard">
        {back}
        <div role="alert" className="library-error">
          <p>Não foi possível carregar a categoria.</p>
          <button className="library-button" onClick={() => retryTree()}>Tentar novamente</button>
        </div>
      </div>
    );
  }
  if (!loadingTree && !category) {
    return (
      <div ref={top} className="library-dashboard">
        {back}
        <p role="alert" className="py-6 text-[14px] text-ink-soft">Esta categoria não existe mais.</p>
      </div>
    );
  }
  const trail = category ? ancestorsOf(categories, id) : [];

  return (
    <div ref={top} className="library-dashboard">
      <nav aria-label="Onde você está" className="flex flex-wrap items-center gap-2 text-[13px] text-ink-soft">
        {back}
        {trail.map((above) => (
          <span key={above.id} className="flex items-center gap-2">
            <span aria-hidden="true">›</span>
            <button className="library-text-link" style={{ marginLeft: 0 }} onClick={() => openCategory(above.id)}>{above.name}</button>
          </span>
        ))}
      </nav>
      {children.length > 0 && (
        <section aria-label="Subcategorias" className="mt-4">
          <ul className="flex flex-wrap gap-2">
            {children.map((child) => (
              <li key={child.id}>
                <button
                  onClick={() => openCategory(child.id)}
                  className="rounded-full border-[0.5px] border-border-hairline bg-white px-4 py-2 text-[13px] text-ink hover:bg-surface/60"
                >
                  {child.name} <span className="text-ink-faint">· {child.works}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
      <section aria-label="Obras da categoria" className="mt-6">
        {works.isError ? (
          <div role="alert" className="library-error">
            <p>Não foi possível carregar as obras desta categoria.</p>
            <button className="library-button" onClick={() => works.refetch()}>Tentar novamente</button>
          </div>
        ) : (
          <>
            <LibraryGrid
              items={works.data?.data ?? []}
              isLoading={works.isLoading || loadingTree}
              isFetching={works.isFetching}
              title={category?.name ?? 'Categoria'}
              viewMode={viewMode}
              total={works.data?.total}
              countWord="obras"
            />
            {totalPages > 1 && (
              <nav className="library-pagination" aria-label="Páginas da categoria">
                <button className="library-button" disabled={page <= 1 || works.isFetching} onClick={() => setPage(page - 1)}>Anterior</button>
                <span aria-live="polite">{page} de {totalPages}</span>
                <button className="library-button" disabled={page >= totalPages || works.isFetching} onClick={() => setPage(page + 1)}>Próxima</button>
              </nav>
            )}
          </>
        )}
      </section>
    </div>
  );
}
