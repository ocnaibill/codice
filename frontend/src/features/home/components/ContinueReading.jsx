import iconPin from '../../../assets/icons/s2-pin.svg';
import iconChapter from '../../../assets/icons/s2-chapter.svg';
import iconContinueBtn from '../../../assets/icons/s2-continue-btn.svg';
import iconPage from '../../../assets/icons/s2-page.svg';
import iconViewerBtn from '../../../assets/icons/s2-viewer-btn.svg';
import { ProgressBar } from '../../../components/ui/ProgressBar';
import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { authenticatedUrl } from '../../../lib/api';

const COMIC_FORMATS = new Set(['cbz', 'cbr']);
const AUDIO_FORMATS = new Set(['mp3', 'm4a', 'm4b', 'ogg', 'wav', 'flac']);

function pageLabel(format, progress) {
  const pageNum = Number.parseInt(progress, 10);
  if (Number.isNaN(pageNum)) return null;
  return COMIC_FORMATS.has(format) ? `Página ${pageNum + 1}` : `Página ${pageNum}`;
}

function InProgressCard({ item, onOpen }) {
  const format = (item.format || '').toLowerCase();
  const isComic = COMIC_FORMATS.has(format);
  const isAudio = AUDIO_FORMATS.has(format);
  const color = isComic ? 'success' : 'brand';
  const progressLabel = isComic ? 'Leitura visual' : 'Progresso global';
  const action = isComic
    ? { label: 'Abrir Visor', variant: 'outline' }
    : { label: isAudio ? 'Continuar Ouvindo' : 'Continuar Leitura', variant: 'solid' };
  const location = pageLabel(format, item.readingProgress);
  const genre = item.tags?.[0] || (item.format ? item.format.toUpperCase() : null);

  return (
    <article className="relative flex w-full max-w-[320px] shrink-0 flex-col overflow-hidden rounded-lg bg-white p-4 shadow-[0px_4px_6px_-1px_rgba(0,0,0,0.1),0px_2px_4px_-2px_rgba(0,0,0,0.1)]">
      <div className="flex w-full items-start gap-4">
        <div className="relative h-36 w-24 shrink-0 overflow-hidden rounded-sm bg-surface-alt shadow-[0px_1px_2px_0px_rgba(0,0,0,0.05)]">
          <img src={authenticatedUrl(item.coverUrl)} alt={item.title} className="h-full w-full object-cover" />
          {item.format && (
            <span className="absolute left-1 top-1 rounded-sm bg-[rgba(26,28,31,0.85)] px-1.5 py-0.5 font-body text-[10px] font-bold text-white">
              {item.format.toUpperCase()}
            </span>
          )}
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex items-center justify-between">
            {genre && (
              <span className={`font-body text-[11px] uppercase tracking-[0.44px] ${isComic ? 'text-success' : 'text-ink-warm'}`}>
                {genre}
              </span>
            )}
            {item.isFavorite && <img src={iconPin} alt="Favorito" className="size-[9px]" />}
          </div>
          <h3 className="pt-1 font-body text-xl font-bold tracking-[-0.2px] text-ink">{item.title}</h3>
          <p className="font-body text-[13px] tracking-[0.065px] text-ink-soft">{item.author}</p>
          {location && (
            <div className="mt-3 flex items-center gap-1 rounded-sm bg-surface px-2 py-1">
              <img src={isAudio ? iconPage : iconChapter} alt="" className="size-[11px]" />
              <span className="font-body text-[11px] tracking-[0.44px] text-ink-soft">{location}</span>
            </div>
          )}
        </div>
      </div>

      <div className="mt-4 flex flex-col gap-1 pt-2">
        <div className="flex items-center justify-between font-body text-[11px] tracking-[0.44px]">
          <span className="text-ink-soft">{progressLabel}</span>
          <span className={`font-bold ${isComic ? 'text-success' : 'text-brand'}`}>{Math.round(item.percentComplete)}%</span>
        </div>
        <ProgressBar percent={item.percentComplete} color={color} />
        <div className="mt-2 flex items-center justify-end pt-1">
          <button
            onClick={() => onOpen(item.id)}
            className={`flex items-center gap-1 rounded-sm px-3 py-1.5 font-body text-[11px] tracking-[0.44px] shadow-[0px_1px_1px_rgba(0,0,0,0.05)] ${
              action.variant === 'solid' ? 'bg-brand text-white' : 'bg-surface-alt text-ink'
            }`}
          >
            <img src={action.variant === 'solid' ? iconContinueBtn : iconViewerBtn} alt="" className="size-3" />
            {action.label}
          </button>
        </div>
      </div>
    </article>
  );
}

export function ContinueReading({ items, isLoading }) {
  const openBook = useGlobalStore((state) => state.openBook);

  return (
    <section className="flex w-full flex-col gap-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="font-display text-2xl font-medium tracking-[-0.12px] text-ink">Continuar Lendo & Ouvindo</h2>
          {!isLoading && <span className="font-mono text-[11px] tracking-[0.44px] text-ink-faint">[{items.length} EM FOCO]</span>}
        </div>
      </div>

      {isLoading ? (
        <div className="flex gap-4">
          <Skeleton className="h-[270px] w-[320px]" />
          <Skeleton className="h-[270px] w-[320px]" />
        </div>
      ) : items.length === 0 ? (
        <EmptyState>Nenhuma leitura em andamento. Abra um livro da sua biblioteca para começar.</EmptyState>
      ) : (
        <div className="flex flex-nowrap items-stretch gap-4 overflow-x-auto pb-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {items.map((item) => (
            <InProgressCard key={item.id} item={item} onOpen={openBook} />
          ))}
        </div>
      )}
    </section>
  );
}
