import { useState } from 'react';
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

const GRID_TITLES = {
  all: 'Adicionados Recentemente & Sincronizados',
  ebooks: 'Livros Digitais',
  comics: 'Mangás & HQs',
  audio: 'Áudiolivros',
};

export function HomePage() {
  const { data: me } = useMe();
  const [formatFilter, setFormatFilter] = useState('all');
  const [viewMode, setViewMode] = useState('grid');

  const { data: stats, isLoading: statsLoading } = useStats();
  const { data: inProgressResult, isLoading: inProgressLoading } = useWorks({ inProgress: true, limit: 5 });
  const { data: gridResult, isLoading: gridLoading } = useWorks({ limit: 10, formatGroup: formatFilter });
  const { data: favoritesResult, isLoading: favoritesLoading } = useFavorites();
  const { data: notesResult, isLoading: notesLoading } = useNotes({ limit: 4 });

  return (
    <div className="mx-auto flex w-full max-w-[1440px] flex-col gap-8 px-4 py-8 sm:px-6 lg:flex-row lg:items-start lg:gap-6 lg:px-10">
      <div className="flex min-w-0 flex-1 flex-col gap-8">
        <GreetingStats userName={me?.username} stats={stats} isLoading={statsLoading} />
        <ContinueReading items={inProgressResult?.data ?? []} isLoading={inProgressLoading} />
        <LibraryFilterBar
          worksTotal={stats?.worksTotal ?? 0}
          breakdown={stats?.libraryBreakdown}
          activeFilter={formatFilter}
          onFilterChange={setFormatFilter}
          viewMode={viewMode}
          onViewModeChange={setViewMode}
        />
        <LibraryGrid items={gridResult?.data ?? []} isLoading={gridLoading} title={GRID_TITLES[formatFilter]} />
      </div>

      <div className="flex w-full flex-col gap-6 lg:w-[348px] lg:shrink-0">
        <FavoriteSeries items={favoritesResult?.data ?? []} total={favoritesResult?.total ?? 0} isLoading={favoritesLoading} />
        <NotesQuotes notes={notesResult?.data ?? []} isLoading={notesLoading} />
      </div>
    </div>
  );
}
