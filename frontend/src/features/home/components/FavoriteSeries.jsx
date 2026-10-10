import { useRef, useState } from 'react';
import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { useDialog } from '../../../lib/useDialog';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkCover } from '../../../components/ui/WorkCover';
import { collectionLine } from '../../collections/text';

/** How many favorites the home shows; the rest are one click away ("Ver todos"). */
export const SHOWN = 4;

// What the card says of the reading: a collection is told by how many of its works were read, a work by whether it was.
export function readingLine(item) {
  if (item.kind === 'collection') return collectionLine(item);
  return item.completed ? 'Já lido' : 'Ainda não lido';
}

function FavoriteRow({ item, onOpen }) {
  const isCollection = item.kind === 'collection';
  return (
    <button
      onClick={() => onOpen(item)}
      className="flex w-full items-center gap-3 rounded-[10px] border-[0.5px] border-border-hairline bg-surface/30 p-3 text-left hover:bg-surface/60"
    >
      <WorkCover item={item} className="h-[60px] w-[40px] shrink-0 overflow-hidden rounded-sm object-cover" />
      <div className="min-w-0 flex-1">
        <p className="truncate font-body text-[11px] tracking-[0.44px] text-ink">{isCollection ? item.title : `${item.title} — ${item.author}`}</p>
        <p className="font-display italic text-[11px] tracking-[0.44px] text-ink">
          {isCollection && <span className="not-italic">{item.collectionKind === 'personal' ? 'Lista · ' : 'Coleção · '}</span>}
          {readingLine(item)}
        </p>
      </div>
      <span className="font-display text-2xl italic text-ink">→</span>
    </button>
  );
}

const keyOf = (item) => (item.kind === 'collection' ? `collection-${item.collectionId}` : `work-${item.workId}`);

/** Every favorite, in a window over the home, for when there are more than the home shows. */
function AllFavorites({ items, onOpen, onClose }) {
  const dialogRef = useRef(null);
  const closeRef = useRef(null);
  useDialog(dialogRef, { active: true, initialFocus: closeRef, onEscape: onClose });
  return (
    <div ref={dialogRef} className="fixed inset-0 z-50 flex items-center justify-center bg-ink/60 backdrop-blur-sm sm:p-4" role="dialog" aria-modal="true" aria-label="Todos os favoritos">
      <div className="flex h-full w-full flex-col overflow-hidden bg-[#faf8f4] shadow-2xl sm:h-auto sm:max-h-[85vh] sm:max-w-xl sm:rounded-2xl">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline px-4 py-3">
          <h2 className="font-display text-xl text-ink">Seus favoritos <span className="font-body text-[11px] text-ink-soft">{items.length} no total</span></h2>
          <button ref={closeRef} onClick={onClose} className="flex min-h-11 min-w-11 items-center justify-center rounded-lg text-xl text-ink-soft hover:bg-surface-alt hover:text-brand" aria-label="Fechar">✕</button>
        </div>
        <div className="flex min-h-0 flex-col gap-2 overflow-y-auto p-4">
          {items.map((item) => (
            <FavoriteRow key={keyOf(item)} item={item} onOpen={onOpen} />
          ))}
        </div>
      </div>
    </div>
  );
}

/**
 * "Seus favoritos": the collections and lists the person favorited, one card each, and the favorite works that are not under
 * one of those (#184, #208). A collection opens its page, a work opens its sheet. The home shows the first few; "Ver todos" opens
 * all of them, so that a person with many does not fill the page.
 */
export function FavoriteSeries({ items, total, isLoading }) {
  const openWork = useGlobalStore((state) => state.openWork);
  const openCollection = useGlobalStore((state) => state.openCollection);
  const [all, setAll] = useState(false);
  const open = (item) => {
    setAll(false);
    if (item.kind === 'collection') openCollection(item.collectionId);
    else openWork(item.workId);
  };
  const more = items.length > SHOWN;

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
          {items.slice(0, SHOWN).map((item) => (
            <FavoriteRow key={keyOf(item)} item={item} onOpen={open} />
          ))}
          {more && (
            <button type="button" className="library-text-link self-end" onClick={() => setAll(true)}>
              Ver todos ({items.length}) <span aria-hidden="true">→</span>
            </button>
          )}
        </div>
      )}
      {all && <AllFavorites items={items} onOpen={open} onClose={() => setAll(false)} />}
    </div>
  );
}
