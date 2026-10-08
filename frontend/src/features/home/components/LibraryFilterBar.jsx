import { useEffect } from 'react';
import { LibraryIcon } from '../../../components/ui/LibraryIcon';
import { bracketCount } from '../utils/format';

export function LibraryFilterBar({
  worksTotal,
  breakdown,
  activeFilter,
  onFilterChange,
  viewMode,
  onViewModeChange,
  sort = 'added',
  onSortChange,
  collectionsTotal = 0,
  canManageCollections = false,
  listsTotal,
}) {
  // "Todos" is always there. A kind of work is there only when the library has at least one of it, and none is
  // offered before the counts are known.
  const kinds = [
    { key: 'ebooks', label: 'Livros digitais', count: breakdown?.livros },
    { key: 'comics', label: 'Mangás & HQs', count: breakdown?.mangas },
    { key: 'audio', label: 'Audiolivros', count: breakdown?.audio },
  ];
  // Collections come after the kinds: when there is one, or for the staff, who make the first.
  const collections = collectionsTotal > 0 || canManageCollections ? [{ key: 'collections', label: 'Coleções', count: collectionsTotal }] : [];
  // The lists of the person, once their number is known (anybody can keep them).
  const lists = listsTotal == null ? [] : [{ key: 'lists', label: 'Minhas listas', count: listsTotal }];
  const filters = [{ key: 'all', label: 'Todos', count: worksTotal }, ...kinds.filter((kind) => kind.count > 0), ...collections, ...lists];
  // The kind on screen has no work left (the last one went away): back to all of them, not to an empty shelf.
  const gone = breakdown && kinds.some((kind) => kind.key === activeFilter && !(kind.count > 0));
  useEffect(() => {
    if (gone) onFilterChange('all');
  }, [gone, onFilterChange]);
  return (
    <div className="library-filters">
      <div
        className="library-filter-options"
        role="group"
        aria-label="Filtrar acervo"
      >
        {filters.map((filter) => (
          <button
            key={filter.key}
            onClick={() => onFilterChange(filter.key)}
            aria-pressed={filter.key === activeFilter}
          >
            {filter.label}
            {filter.count != null && <span>{bracketCount(filter.count)}</span>}
          </button>
        ))}
      </div>
      {onSortChange && (
        <label className="library-sort">
          <span>Ordenar</span>
          <select value={sort} onChange={(event) => onSortChange(event.target.value)} aria-label="Ordenar o acervo">
            <option value="added">Mais recentes</option>
            <option value="title">Título</option>
            <option value="author">Autor</option>
          </select>
        </label>
      )}
      <div
        className="library-view-toggle"
        role="group"
        aria-label="Visualização do acervo"
      >
        <button
          onClick={() => onViewModeChange('grid')}
          aria-label="Grade"
          aria-pressed={viewMode === 'grid'}
        >
          <LibraryIcon name="grid" />
        </button>
        <button
          onClick={() => onViewModeChange('list')}
          aria-label="Lista"
          aria-pressed={viewMode === 'list'}
        >
          <LibraryIcon name="list" />
        </button>
      </div>
    </div>
  );
}
