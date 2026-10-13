import { Carousel } from '../../../components/ui/Carousel';
import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { LibraryIcon } from '../../../components/ui/LibraryIcon';
import { formatCount } from '../utils/format';
import { BookCard, SeriesBookCard } from './LibraryGrid';
import { countWordFor } from '../utils/format';

/**
 * A shelf: a heading with how many there are and a link to the rest, and the works in a row that goes on past the edge. It is "Adicionados
 * recentemente" and one for each category of the home; a series is one card on it, as it is in the grid. `accent` puts a dot in a color of
 * the system before the heading (the page of a person tells its kinds of shelf apart by it); the home has none.
 */
const ACCENTS = { brand: 'bg-brand', success: 'bg-success' };

export function Shelf({ title, total, countWord = 'obras', items, isLoading, emptyText = 'Nenhuma obra encontrada nessa categoria ainda.', seeAll, headingId, accent }) {
  const openBook = useGlobalStore((state) => state.openBook);
  const openWork = useGlobalStore((state) => state.openWork);
  const openCollection = useGlobalStore((state) => state.openCollection);

  return (
    <section aria-busy={!!isLoading} aria-labelledby={headingId} className="library-shelf">
      <div className="library-section-heading">
        {accent && ACCENTS[accent] && <span aria-hidden="true" className={`size-2.5 shrink-0 self-center rounded-full ${ACCENTS[accent]}`} />}
        <h2 id={headingId}>{title}</h2>
        {total != null && <span className="library-eyebrow">[ {formatCount(total)} {countWordFor(total, countWord)} ]</span>}
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
