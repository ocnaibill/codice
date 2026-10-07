import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkCover } from '../../../components/ui/WorkCover';

// What the card says of the reading: a series is told by how many of it were read, a loose work by whether it was.
export function readingLine(item) {
  if (item.kind !== 'series') return item.seriesCompleted > 0 ? 'Já lido' : 'Ainda não lido';
  const read = `Leu ${item.seriesCompleted} de ${item.seriesTotal}`;
  return item.favoriteCount > 1 ? `${read} · ${item.favoriteCount} favoritos` : read;
}

export function FavoriteSeries({ items, total, isLoading }) {
  const openWork = useGlobalStore((state) => state.openWork);

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
        <EmptyState>Ainda sem favoritos. Marque um livro como favorito enquanto lê para vê-lo aqui.</EmptyState>
      ) : (
        <div className="flex flex-col gap-2">
          {items.map((item) => (
            <button
              key={`${item.kind}-${item.workId}`}
              onClick={() => openWork(item.workId)}
              className="flex items-center gap-3 rounded-[10px] border-[0.5px] border-border-hairline bg-surface/30 p-3 text-left hover:bg-surface/60"
            >
              <WorkCover item={item} className="h-[60px] w-[40px] shrink-0 overflow-hidden rounded-sm object-cover" />
              <div className="min-w-0 flex-1">
                <p className="truncate font-body text-[11px] tracking-[0.44px] text-ink">
                  {item.title} — {item.author}
                </p>
                <p className="font-display italic text-[11px] tracking-[0.44px] text-ink">{readingLine(item)}</p>
              </div>
              <span className="font-display text-2xl italic text-ink">→</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
