import iconPin from '../../../assets/icons/s2-pin.svg';
import iconChapter from '../../../assets/icons/s2-chapter.svg';
import iconContinueBtn from '../../../assets/icons/s2-continue-btn.svg';
import iconPage from '../../../assets/icons/s2-page.svg';
import iconViewerBtn from '../../../assets/icons/s2-viewer-btn.svg';
import { ProgressBar } from '../../../components/ui/ProgressBar';
import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { WorkCover } from '../../../components/ui/WorkCover';
import { LibraryIcon } from '../../../components/ui/LibraryIcon';

const COMIC_FORMATS = new Set(['cbz', 'cbr']);
const AUDIO_FORMATS = new Set(['mp3', 'm4a', 'm4b', 'ogg', 'wav', 'flac']);

function pageLabel(format, progress) {
  const pageNum = Number.parseInt(progress, 10);
  if (Number.isNaN(pageNum)) return null;
  return COMIC_FORMATS.has(format) ? `Página ${pageNum + 1}` : `Página ${pageNum}`;
}

function InProgressCard({ item, onOpen }) {
  // What to continue is the file read last, which is not always the book's primary one: someone
  // reading the English EPUB of a book whose main file is the Portuguese one continues that.
  const last = item.continue;
  const format = (last?.format || item.format || '').toLowerCase();
  const readingProgress = last ? last.position : item.readingProgress;
  const percentComplete = last ? last.percentComplete : item.percentComplete;
  const isComic = COMIC_FORMATS.has(format);
  const isAudio = AUDIO_FORMATS.has(format);
  const color = isComic ? 'success' : 'brand';
  const progressLabel = isComic ? 'Leitura visual' : 'Progresso global';
  const action = isComic
    ? { label: 'Abrir Visor', variant: 'outline' }
    : { label: isAudio ? 'Continuar Ouvindo' : 'Continuar Leitura', variant: 'solid' };
  const location = isAudio ? null : pageLabel(format, readingProgress);
  const genre = item.tags?.[0] || (format ? format.toUpperCase() : null);

  return (
    <article className="library-reading-card relative flex flex-col justify-between">
      <div className="flex w-full items-start gap-4">
        <div className="relative h-36 w-24 shrink-0 overflow-hidden rounded-sm bg-surface-alt shadow-[0px_1px_2px_0px_rgba(0,0,0,0.05)]">
          <WorkCover item={item} className="h-full w-full object-cover" />
          {(last?.format || item.format) && (
            <span className="absolute left-1 top-1 rounded-sm bg-[rgba(26,28,31,0.85)] px-1.5 py-0.5 font-body text-[10px] font-bold text-white">
              {(last?.format || item.format).toUpperCase()}
              {last?.language ? ` · ${last.language.toUpperCase()}` : ''}
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
          <span className={`font-bold ${isComic ? 'text-success' : 'text-brand'}`}>{Math.round(percentComplete)}%</span>
        </div>
        <ProgressBar percent={percentComplete} color={color} />
        <div className="mt-2 flex items-center justify-end pt-1">
          <button
            onClick={() => onOpen(item.id, last?.fileId ?? null)}
            className={`library-reading-action flex items-center gap-1 rounded-sm px-3 py-1.5 font-body text-[11px] tracking-[0.44px] shadow-[0px_1px_1px_rgba(0,0,0,0.05)] ${
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

export function ContinueReading({ items, isLoading, onViewAll }) {
  const openBook = useGlobalStore((state) => state.openBook);

  return (
    <section className="min-w-0" aria-label="Continuar lendo e ouvindo">
      <div className="library-section-heading">
        <h2>Continuar lendo & ouvindo</h2>
        {!isLoading && <span className="library-eyebrow">[{items.length} EM FOCO]</span>}
        {onViewAll && items.length > 0 && <button className="library-text-link" onClick={onViewAll}>Ver todas <LibraryIcon name="arrow" /></button>}
      </div>

      {isLoading ? (
        <div className="library-reading-track" aria-label="Carregando leituras">
          <Skeleton className="h-[270px] w-[320px]" />
          <Skeleton className="h-[270px] w-[320px]" />
        </div>
      ) : items.length === 0 ? (
        <EmptyState>Nenhuma leitura em andamento. Abra um livro da sua biblioteca para começar.</EmptyState>
      ) : (
        <div className="library-reading-track" tabIndex={0} aria-label="Leituras em andamento">
          {items.map((item) => (
            <InProgressCard key={item.id} item={item} onOpen={openBook} />
          ))}
        </div>
      )}
    </section>
  );
}
