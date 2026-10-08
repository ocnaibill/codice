import iconActionRead from '../../../assets/icons/card-action-read.svg';
import iconActionDownload from '../../../assets/icons/card-action-download.svg';
import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { WorkCover } from '../../../components/ui/WorkCover';
import { LibraryIcon } from '../../../components/ui/LibraryIcon';
import { useFavoriteToggle } from '../../reader/api/useFavoriteToggle';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { AuthorLinks } from '../../people/components/AuthorLinks';
import { authenticatedUrl } from '../../../lib/api';
import { readLabel, readTarget } from '../../reader/readTarget';
import { formatBadge, formatCount } from '../utils/format';
import { goOnText, newText, seriesCounts } from '../../collections/text';
import { cardTitle } from '../../reader/titles';

const STATUS_LABEL = {
  READY: 'Pronto para ler',
  UNKNOWN: 'Aguardando análise',
  ANALYZING: 'Processando',
  QUEUED: 'Na fila',
  ERROR: 'Erro ao processar',
};

/** The heart of a card: to favorite a work without opening it (#179). */
function FavoriteButton({ item }) {
  const toggle = useFavoriteToggle(item.id);
  const label = item.isFavorite ? 'Remover dos favoritos' : 'Adicionar aos favoritos';
  return (
    <button
      type="button"
      className="library-favorite"
      onClick={() => toggle.mutate(!item.isFavorite)}
      disabled={toggle.isPending}
      aria-pressed={!!item.isFavorite}
      aria-label={`${label}: ${item.title}`}
      title={label}
    >
      <LibraryIcon name="heart" />
    </button>
  );
}

function BookCard({ item, onOpen, onSheet }) {
  const title = cardTitle(item); // the name of the edition being read, when someone wrote one for it
  const isReady = item.mediaStatus === 'READY' || !item.mediaStatus;
  return (
    <article className="library-book">
      <div className="library-book-body">
        <button
          onClick={() => onSheet(item.id)}
          title="Ver edições e arquivos"
          className="library-book-cover"
        >
          <WorkCover item={{ ...item, title }} />
          {item.format && (
            <span className="absolute left-1.5 top-1.5 rounded-sm bg-ink/90 px-1.5 py-1 font-mono text-[9px] text-white">
              {formatBadge(item)}
            </span>
          )}
        </button>
        <div className="library-book-meta">
          <p
            className="library-book-status"
            style={!isReady ? { color: 'var(--color-ink-soft)' } : undefined}
          >
            {STATUS_LABEL[item.mediaStatus] ??
              item.mediaStatus ??
              'Pronto para ler'}
          </p>
          <h3 title={title}>{title}</h3>
          <p className="library-author"><AuthorLinks authors={item.authors} fallback={item.author} /></p>
          {item.tags?.length > 0 && (
            <p className="library-book-tags">{item.tags.join(' · ')}</p>
          )}
        </div>
      </div>
      <div className="library-book-actions">
        <button
          onClick={() => {
            const target = readTarget(item);
            if (target.kind === 'sheet') onSheet(item.id);
            else onOpen(item.id, target.fileId);
          }}
          title={readLabel(item)}
          aria-label={`${readLabel(item)}: ${title}`}
        >
          <img src={iconActionRead} alt="" className="h-4 w-5" />
        </button>
        <FavoriteButton item={item} />
        {item.fileUrl ? (
          <a
            href={authenticatedUrl(item.fileUrl)}
            title="Baixar"
            aria-label={`Baixar: ${title}`}
          >
            <img src={iconActionDownload} alt="" className="size-4" />
          </a>
        ) : (
          <span className="opacity-30" title="Arquivo ainda não disponível">
            <img src={iconActionDownload} alt="" className="size-4" />
          </span>
        )}
      </div>
    </article>
  );
}

/**
 * A series in the grid of the library (#187): one card for the whole of it, in the place of its newest work. The cover and the name
 * open the collection; the button goes on with the series, where the page of the collection would.
 */
function SeriesBookCard({ item, onCollection, onOpen }) {
  const series = item.collapsed;
  const go = series.continue;
  return (
    <article className="library-book" data-series="true">
      <div className="library-book-body">
        <button
          onClick={() => onCollection(series.collectionId)}
          title="Ver as obras da série"
          aria-label={`Abrir a série ${series.name}`}
          className="library-book-cover"
        >
          <WorkCover item={{ ...item, title: series.name }} />
          <span className="absolute left-1.5 top-1.5 rounded-sm bg-brand px-1.5 py-1 font-mono text-[9px] text-white">SÉRIE</span>
          {series.newCount > 0 && (
            <span
              className="absolute right-1.5 top-1.5 rounded-sm bg-success px-1.5 py-1 font-mono text-[9px] text-white"
              title="Chegaram na última semana e você ainda não terminou"
            >
              {newText(series.newCount)}
            </span>
          )}
        </button>
        <div className="library-book-meta">
          <p className="library-book-status">Série</p>
          <h3 title={series.name}>{series.name}</h3>
          <p className="library-author"><AuthorLinks authors={item.authors} fallback={item.author} /></p>
          <p className="library-book-tags">{seriesCounts(series)}</p>
        </div>
      </div>
      <div className="library-book-actions">
        {go ? (
          <button className="library-series-go" onClick={() => onOpen(go.id)} title={go.title}>
            {goOnText(go)}
          </button>
        ) : (
          <span className="library-series-done">Tudo lido</span>
        )}
      </div>
    </article>
  );
}

export function LibraryGrid({
  items,
  isLoading,
  isFetching,
  title = 'Adicionados recentemente',
  viewMode = 'grid',
  total,
  countWord = 'obras',
}) {
  const openBook = useGlobalStore((state) => state.openBook);
  const openWork = useGlobalStore((state) => state.openWork);
  const openCollection = useGlobalStore((state) => state.openCollection);
  return (
    <section aria-busy={!!(isLoading || isFetching)}>
      <div className="library-section-heading">
        <h2>{title}</h2>
        {total != null && (
          <span className="library-eyebrow">
            [ {formatCount(total)} {countWord} ]
          </span>
        )}
      </div>
      {isLoading ? (
        <div className="library-books" data-view={viewMode}>
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton
              key={i}
              className={
                viewMode === 'list' ? 'h-28 w-full' : 'h-[300px] w-full'
              }
            />
          ))}
        </div>
      ) : items.length === 0 ? (
        <EmptyState>Nenhuma obra encontrada nessa categoria ainda.</EmptyState>
      ) : (
        <div className="library-books" data-view={viewMode}>
          {items.map((item) =>
            item.collapsed ? (
              <SeriesBookCard key={item.id} item={item} onCollection={openCollection} onOpen={openBook} />
            ) : (
              <BookCard key={item.id} item={item} onOpen={openBook} onSheet={openWork} />
            )
          )}
        </div>
      )}
    </section>
  );
}
