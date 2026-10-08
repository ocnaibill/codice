import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkCover } from '../../../components/ui/WorkCover';
import { collectionLine } from '../../collections/text';

// What the card says of the reading: a collection is told by how many of its works were read, a work by whether it was.
export function readingLine(item) {
  if (item.kind === 'collection') return collectionLine(item);
  return item.completed ? 'Já lido' : 'Ainda não lido';
}

/**
 * "Seus favoritos": the collections and lists the person favorited, one card each, and the favorite works that are not under
 * one of those (#184, #208). A collection opens its page, a work opens its sheet.
 */
export function FavoriteSeries({ items, total, isLoading }) {
  const openWork = useGlobalStore((state) => state.openWork);
  const openCollection = useGlobalStore((state) => state.openCollection);

  return (
    <div className="library-panel">
      <div className="flex items-center justify-between pb-3">
        <h3 className="font-display text-xl text-ink">Seus favoritos</h3>
        {!isLoading && <span className="font-body text-[11px] tracking-[0.44px] text-ink-soft">{total} no total</span>}
      </div>

      {isLoading ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-[72px] w-full" />
          <Skeleton className="h-[72px] w-full" />
        </div>
      ) : items.length === 0 ? (
        <EmptyState>Ainda sem favoritos. Marque um livro, uma coleção ou uma lista como favorito para vê-lo aqui.</EmptyState>
      ) : (
        <div className="flex flex-col gap-2">
          {items.map((item) => {
            const isCollection = item.kind === 'collection';
            return (
              <button
                key={isCollection ? `collection-${item.collectionId}` : `work-${item.workId}`}
                onClick={() => (isCollection ? openCollection(item.collectionId) : openWork(item.workId))}
                className="flex items-center gap-3 rounded-[10px] border-[0.5px] border-border-hairline bg-surface/30 p-3 text-left hover:bg-surface/60"
              >
                <WorkCover item={item} className="h-[60px] w-[40px] shrink-0 overflow-hidden rounded-sm object-cover" />
                <div className="min-w-0 flex-1">
                  <p className="truncate font-body text-[11px] tracking-[0.44px] text-ink">
                    {isCollection ? item.title : `${item.title} — ${item.author}`}
                  </p>
                  <p className="font-display italic text-[11px] tracking-[0.44px] text-ink">
                    {isCollection && <span className="not-italic">{item.collectionKind === 'personal' ? 'Lista · ' : 'Coleção · '}</span>}
                    {readingLine(item)}
                  </p>
                </div>
                <span className="font-display text-2xl italic text-ink">→</span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
