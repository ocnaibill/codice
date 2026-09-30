import { LibraryIcon } from '../../../components/ui/LibraryIcon';
import { formatCount } from '../utils/format';

export function LibraryFilterBar({
  worksTotal,
  breakdown,
  activeFilter,
  onFilterChange,
  viewMode,
  onViewModeChange,
  sort = 'added',
  onSortChange,
}) {
  const filters = [
    { key: 'all', label: 'Todos', count: worksTotal },
    { key: 'ebooks', label: 'Livros digitais', count: breakdown?.livros },
    { key: 'comics', label: 'Mangás & HQs', count: breakdown?.mangas },
    { key: 'audio', label: 'Audiolivros', count: breakdown?.audio },
  ];
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
            {filter.count != null && <span>[{formatCount(filter.count)}]</span>}
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
