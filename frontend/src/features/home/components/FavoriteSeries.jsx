import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkCover } from '../../../components/ui/WorkCover';

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
              key={item.workId}
              onClick={() => openWork(item.workId)}
              className="flex items-center gap-3 rounded-[10px] border-[0.5px] border-border-hairline bg-surface/30 p-3 text-left hover:bg-surface/60"
            >
              <WorkCover item={item} className="h-[60px] w-[40px] shrink-0 overflow-hidden rounded-sm object-cover" />
              <div className="min-w-0 flex-1">
                <p className="truncate font-body text-[11px] tracking-[0.44px] text-ink">
                  {item.title} — {item.author}
                </p>
                {item.seriesTotal > 0 && <p className="font-display italic text-[11px] tracking-[0.44px] text-ink">
                  Leu {item.seriesCompleted} de {item.seriesTotal}
                </p>}
              </div>
              <span className="font-display text-2xl italic text-ink">→</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
