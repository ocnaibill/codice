import iconActionRead from '../../../assets/icons/card-action-read.svg';
import iconActionDownload from '../../../assets/icons/card-action-download.svg';
import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { WorkCover } from '../../../components/ui/WorkCover';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { authenticatedUrl } from '../../../lib/api';
import { readLabel, readTarget } from '../../reader/readTarget';
import { formatCount } from '../utils/format';

const STATUS_LABEL = {
  READY: 'Pronto para ler',
  UNKNOWN: 'Aguardando análise',
  ANALYZING: 'Processando',
  QUEUED: 'Na fila',
  ERROR: 'Erro ao processar',
};

function BookCard({ item, onOpen, onSheet }) {
  const isReady = item.mediaStatus === 'READY' || !item.mediaStatus;
  return (
    <article className="library-book">
      <div className="library-book-body">
        <button
          onClick={() => onSheet(item.id)}
          title="Ver edições e arquivos"
          className="library-book-cover"
        >
          <WorkCover item={item} />
          {item.format && (
            <span className="absolute left-1.5 top-1.5 rounded-sm bg-ink/90 px-1.5 py-1 font-mono text-[9px] text-white">
              {item.format.toUpperCase()}
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
          <h3 title={item.title}>{item.title}</h3>
          <p className="library-author">{item.author}</p>
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
          aria-label={`${readLabel(item)}: ${item.title}`}
        >
          <img src={iconActionRead} alt="" className="h-4 w-5" />
        </button>
        {item.fileUrl ? (
          <a
            href={authenticatedUrl(item.fileUrl)}
            title="Baixar"
            aria-label={`Baixar: ${item.title}`}
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

export function LibraryGrid({
  items,
  isLoading,
  isFetching,
  title = 'Adicionados recentemente',
  viewMode = 'grid',
  total,
}) {
  const openBook = useGlobalStore((state) => state.openBook);
  const openWork = useGlobalStore((state) => state.openWork);
  return (
    <section aria-busy={!!(isLoading || isFetching)}>
      <div className="library-section-heading">
        <h2>{title}</h2>
        {total != null && (
          <span className="library-eyebrow">
            [ {formatCount(total)} obras ]
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
          {items.map((item) => (
            <BookCard
              key={item.id}
              item={item}
              onOpen={openBook}
              onSheet={openWork}
            />
          ))}
        </div>
      )}
    </section>
  );
}
