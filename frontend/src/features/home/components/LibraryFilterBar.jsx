import iconGrid from '../../../assets/icons/s3-grid.svg';
import iconList from '../../../assets/icons/s3-list.svg';
import iconChevron from '../../../assets/icons/s3-chevron.svg';
import iconFilter from '../../../assets/icons/s3-filter.svg';
import { formatCount } from '../utils/format';

export function LibraryFilterBar({ worksTotal, breakdown, activeFilter, onFilterChange, viewMode, onViewModeChange }) {
  const filters = [
    { key: 'all', label: 'Todos', count: worksTotal },
    { key: 'ebooks', label: 'Livros Digitais', count: breakdown?.livros },
    { key: 'comics', label: 'Mangás & HQs', count: breakdown?.mangas },
    { key: 'audio', label: 'Áudiolivros', count: breakdown?.audio },
  ];

  return (
    <div className="flex flex-col gap-3 rounded-lg bg-surface p-3 shadow-[0px_1px_1px_rgba(0,0,0,0.05)] sm:flex-row sm:items-center sm:justify-between">
      <div className="flex flex-wrap items-center gap-2">
        {filters.map((filter) => {
          const isActive = filter.key === activeFilter;
          return (
            <button
              key={filter.key}
              onClick={() => onFilterChange(filter.key)}
              className={`flex items-center gap-1 rounded px-3 py-1.5 font-body text-[13px] tracking-[0.26px] shadow-[0px_1px_1px_rgba(0,0,0,0.05)] ${
                isActive ? 'bg-brand text-white' : 'bg-surface-alt text-ink'
              }`}
            >
              {filter.label}
              <span className={isActive ? 'text-white/80' : 'text-ink-soft'}>[{formatCount(filter.count)}]</span>
            </button>
          );
        })}
      </div>

      <div className="flex items-center gap-2">
        <div className="flex items-center rounded bg-surface-alt p-0.5">
          <button
            onClick={() => onViewModeChange('grid')}
            className={`flex items-center justify-center rounded-sm px-1.5 py-1.5 ${viewMode === 'grid' ? 'bg-white shadow-[0px_1px_1px_rgba(0,0,0,0.05)]' : ''}`}
          >
            <img src={iconGrid} alt="Grade" className="size-[13.5px]" />
          </button>
          <button
            onClick={() => onViewModeChange('list')}
            className={`flex items-center justify-center rounded-sm px-1.5 py-1.5 ${viewMode === 'list' ? 'bg-white shadow-[0px_1px_1px_rgba(0,0,0,0.05)]' : ''}`}
          >
            <img src={iconList} alt="Lista" className="h-3 w-[15px]" />
          </button>
        </div>
        <span className="h-4 w-px bg-[#dbc1b6]" />
        <div className="relative">
          <select className="appearance-none rounded bg-surface-alt py-1.5 pl-3 pr-6 font-body text-[11px] tracking-[0.44px] text-ink outline-none">
            <option>Mais recentes adicionados</option>
          </select>
          <img src={iconChevron} alt="" className="pointer-events-none absolute right-2 top-1/2 h-[5px] w-2 -translate-y-1/2" />
        </div>
        <button className="flex items-center gap-1 rounded bg-surface-alt px-3 py-1.5">
          <img src={iconFilter} alt="" className="size-3" />
          <span className="font-body text-[11px] tracking-[0.44px] text-ink">Filtros</span>
        </button>
      </div>
    </div>
  );
}
