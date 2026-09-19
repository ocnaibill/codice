import iconCheck from '../../../assets/icons/card-check.svg';
import iconActionRead from '../../../assets/icons/card-action-read.svg';
import iconActionDownload from '../../../assets/icons/card-action-download.svg';
import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { authenticatedUrl } from '../../../lib/api';

const FORMAT_BADGE_STYLE = {
  MP3: 'bg-brand text-white',
  M4B: 'bg-brand text-white',
  M4A: 'bg-brand text-white',
};

const STATUS_LABEL = {
  READY: 'Pronto',
  ANALYZING: 'Processando',
  QUEUED: 'Na fila',
  ERROR: 'Erro ao processar',
};

function BookCard({ item, onOpen }) {
  const statusLabel = STATUS_LABEL[item.mediaStatus] ?? item.mediaStatus;
  const isReady = item.mediaStatus === 'READY' || !item.mediaStatus;

  return (
    <article className="flex w-[145px] shrink-0 flex-col justify-between rounded bg-white p-2 shadow-[0px_1px_1px_rgba(0,0,0,0.05)]">
      <div className="flex flex-col gap-2">
        <button
          onClick={() => onOpen(item.id)}
          className="relative block overflow-hidden rounded-sm bg-surface-alt shadow-[inset_0px_2px_4px_0px_rgba(0,0,0,0.05)]"
        >
          <img src={authenticatedUrl(item.coverUrl)} alt={item.title} className="h-[194px] w-full object-cover" />
          {item.format && (
            <span
              className={`absolute left-1.5 top-1.5 rounded-sm px-1 py-0.5 font-body text-[9px] font-bold ${
                FORMAT_BADGE_STYLE[item.format.toUpperCase()] ?? 'bg-[rgba(26,28,31,0.9)] text-white'
              }`}
            >
              {item.format.toUpperCase()}
            </span>
          )}
        </button>
        <div className="flex flex-col px-1">
          <p className={`flex items-center gap-1 font-body text-[11px] ${isReady ? 'text-success' : 'text-ink-faint'}`}>
            {isReady && <img src={iconCheck} alt="" className="size-2.5" />}
            {statusLabel}
          </p>
          <h4 className="pt-0.5 truncate font-body text-base font-bold text-ink">{item.title}</h4>
          <p className="truncate font-body text-[13px] tracking-[0.065px] text-ink-soft">{item.author}</p>
          {item.tags?.length > 0 && (
            <p className="truncate pt-1 font-body text-[11px] tracking-[0.44px] text-ink-faint">{item.tags.join(' • ')}</p>
          )}
        </div>
      </div>
      <div className="mt-3 flex items-center justify-between border-t border-surface-alt pt-2 px-1">
        <button onClick={() => onOpen(item.id)} className="p-1" title="Ler">
          <img src={iconActionRead} alt="Ler" className="h-3 w-[15px]" />
        </button>
        {item.fileUrl ? (
          <a href={authenticatedUrl(item.fileUrl)} className="p-1" title="Baixar">
            <img src={iconActionDownload} alt="Baixar" className="size-3" />
          </a>
        ) : (
          <span className="p-1 opacity-30" title="Arquivo ainda não disponível">
            <img src={iconActionDownload} alt="Baixar" className="size-3" />
          </span>
        )}
      </div>
    </article>
  );
}

export function LibraryGrid({ items, isLoading, title = 'Adicionados Recentemente & Sincronizados' }) {
  const openBook = useGlobalStore((state) => state.openBook);

  return (
    <section className="flex w-full flex-col gap-4">
      <h2 className="font-display text-2xl font-medium tracking-[-0.12px] text-ink">{title}</h2>
      {isLoading ? (
        <div className="flex flex-wrap gap-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-[280px] w-[145px]" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <EmptyState>Nenhuma obra encontrada nessa categoria ainda.</EmptyState>
      ) : (
        <div className="flex flex-wrap justify-center gap-3 sm:justify-start">
          {items.map((item) => (
            <BookCard key={item.id} item={item} onOpen={openBook} />
          ))}
        </div>
      )}
    </section>
  );
}
