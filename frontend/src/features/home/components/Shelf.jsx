import { Carousel } from '../../../components/ui/Carousel';
import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { LibraryIcon } from '../../../components/ui/LibraryIcon';
import { formatCount } from '../utils/format';
import { BookCard, SeriesBookCard } from './LibraryGrid';

/**
 * A shelf: a heading with how many there are and a link to the rest, and the works in a row that goes on past the edge. It is "Adicionados
 * recentemente" and one for each category of the home; a series is one card on it, as it is in the grid.
 */
export function Shelf({ title, total, countWord = 'obras', items, isLoading, emptyText = 'Nenhuma obra encontrada nessa categoria ainda.', seeAll, headingId }) {
  const openBook = useGlobalStore((state) => state.openBook);
  const openWork = useGlobalStore((state) => state.openWork);
  const openCollection = useGlobalStore((state) => state.openCollection);

  return (
    <section aria-busy={!!isLoading} aria-labelledby={headingId} className="library-shelf">
      <div className="library-section-heading">
        <h2 id={headingId}>{title}</h2>
        {total != null && <span className="library-eyebrow">[ {formatCount(total)} {countWord} ]</span>}
        {seeAll && (
          <button type="button" className="library-text-link" onClick={seeAll.onClick}>
            {seeAll.label} <LibraryIcon name="arrow" />
          </button>
        )}
      </div>
      {isLoading ? (
        <div className="library-carousel-track" aria-label="Carregando">
          {Array.from({ length: 5 }, (_, i) => (
            <div key={i} className="library-carousel-item">
              <Skeleton className="h-[300px] w-full" />
            </div>
          ))}
        </div>
      ) : items.length === 0 ? (
        <EmptyState>{emptyText}</EmptyState>
      ) : (
        <Carousel label={title}>
          {items.map((item) =>
            item.collapsed ? (
              <SeriesBookCard key={item.id} item={item} onCollection={openCollection} onOpen={openBook} />
            ) : (
              <BookCard key={item.id} item={item} onOpen={openBook} onSheet={openWork} />
            )
          )}
        </Carousel>
      )}
    </section>
  );
}
