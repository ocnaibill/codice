import React, { useState, useEffect, useRef, useCallback } from 'react';
import { authenticatedUrl } from '../../../../lib/api';
import { Skeleton } from '../../../../components/ui/Skeleton';
import { completionFor } from '../../progressRules';
import { imagePlaceProblem } from '../../placeCheck';
import { getComicMode, saveComicMode } from '../../preferences';
import { MAX_READING_WIDTH, tapAction, swipeAction } from '../../pdfGestures';

// Number of pages to preload ahead and behind
const PRELOAD_COUNT = 2;
// Loading timeout per page in ms
const PAGE_TIMEOUT = 30000;

// How a file can say it is read (#19). The file's word is a fact; the remembered mode is the person's taste, which
// fills in where the file says nothing.
const FILE_MODES = ['rtl', 'webtoon'];

const MODES = [
  { id: 'ltr', label: 'Esquerda para a direita' },
  { id: 'rtl', label: 'Direita para a esquerda' },
  { id: 'webtoon', label: 'Tira para rolar' },
  { id: 'double', label: 'Página dupla' },
];
const FILE_SAID = {
  rtl: 'O arquivo indica leitura da direita para a esquerda.',
  webtoon: 'O arquivo indica que é uma tira para rolar.',
};

const iconProps = { viewBox: '0 0 24 24', width: 18, height: 18, fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true };
const Chevron = ({ dir }) => (
  <svg {...iconProps}>
    <path d={dir === 'left' ? 'M15 5l-7 7 7 7' : 'M9 5l7 7-7 7'} />
  </svg>
);

const control =
  'flex h-11 min-w-11 items-center justify-center rounded-full text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95 disabled:opacity-30 disabled:hover:bg-transparent disabled:active:scale-100';

/**
 * The reader of a comic: a page at a time (left to right, right to left, or two together) or a strip to scroll. The
 * controls are at the bottom, where the thumb is. A tap on the left or the right of the page turns it (to the side the
 * comic is read toward), a tap in the middle hides or shows the controls, and a swipe turns it too; on a keyboard the
 * arrows do.
 */
export default function MangaViewer({ fileUrl, onProgress, initialProgress, workId, onPlaceFailed, declaredMode, immersive = false, onImmersiveChange }) {
  const [pages, setPages] = useState([]);
  const [currentPage, setCurrentPage] = useState(
    initialProgress ? parseInt(initialProgress, 10) || 0 : 0
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [attempt, setAttempt] = useState(0); // asking for the page list again
  // 'ltr', 'rtl', 'webtoon' or 'double'. A comic that declares how it is read opens that way; otherwise it opens in
  // the mode remembered from the last comic read on this device (DEC-078). Opening one that declares a mode does
  // not touch what is remembered: that was not a choice of the person, who still makes it by changing the mode.
  const declared = FILE_MODES.includes(declaredMode) ? declaredMode : null;
  const [readingDirection, setDirection] = useState(() => declared || getComicMode());
  const [fromFile, setFromFile] = useState(!!declared);
  const [showModes, setShowModes] = useState(false);
  const setReadingDirection = (mode) => {
    setDirection(mode);
    setFromFile(false);
    saveComicMode(mode);
  };
  const [pageStatus, setPageStatus] = useState({}); // { [pageNum]: 'loading'|'loaded'|'error' }
  const timeoutRef = useRef(null);
  const rootRef = useRef(null);
  const modesRef = useRef(null);
  const modesOpenerRef = useRef(null);
  const gesture = useRef(null);
  // Read when the page list arrives, not reasons to ask for it again.
  const placeRef = useRef({ initialProgress, onPlaceFailed });
  placeRef.current = { initialProgress, onPlaceFailed };

  // Fetch page list from backend
  useEffect(() => {
    let cancelled = false;

    const loadPageList = async () => {
      try {
        // Derive work ID from fileUrl: /files/<workId>/...
        // Fallback: use workId prop or extract from fileUrl
        const id = workId || (fileUrl ? fileUrl.split('/').filter(Boolean)[1] : null);
        if (!id) {
          setError('Não foi possível saber de qual obra é este arquivo.');
          setLoading(false);
          return;
        }

        const url = authenticatedUrl(`/works/${id}/pages`);
        const response = await fetch(url);
        if (!response.ok) {
          throw new Error(`O servidor respondeu com o erro ${response.status}.`);
        }
        const data = await response.json();

        if (!cancelled) {
          setPages(data);
          setLoading(false);
          // An image the file does not have is said, not hidden by showing another one.
          const problem = imagePlaceProblem(placeRef.current.initialProgress, data.length);
          if (problem) {
            setCurrentPage(0);
            placeRef.current.onPlaceFailed?.({ reason: problem });
          }
        }
      } catch (err) {
        if (!cancelled) {
          setError(err.message);
          setLoading(false);
        }
      }
    };

    setLoading(true);
    setError(null);
    loadPageList();
    return () => { cancelled = true; };
  }, [fileUrl, workId, attempt]);

  // Derive work ID from fileUrl or workId prop
  const workIdValue = workId || (fileUrl ? fileUrl.split('/').filter(Boolean)[1] : null);

  // Get authenticated URL for a specific page
  const getPageUrl = useCallback((pageNum) => {
    if (!workIdValue) return null;
    return authenticatedUrl(`/works/${workIdValue}/pages/${pageNum}`);
  }, [workIdValue]);

  // Get thumbnail URL for a specific page
  const getThumbnailUrl = useCallback((pageNum) => {
    if (!workIdValue) return null;
    return authenticatedUrl(`/works/${workIdValue}/pages/${pageNum}/thumbnail`);
  }, [workIdValue]);

  // Preload adjacent pages
  useEffect(() => {
    const toPreload = [];
    for (let i = currentPage - PRELOAD_COUNT; i <= currentPage + PRELOAD_COUNT; i++) {
      if (i >= 0 && i < pages.length && !pageStatus[i]) {
        toPreload.push(i);
      }
    }

    toPreload.forEach((pageNum) => {
      setPageStatus((prev) => ({ ...prev, [pageNum]: 'loading' }));
      const img = new Image();
      const timer = setTimeout(() => {
        setPageStatus((prev) => ({ ...prev, [pageNum]: 'error' }));
      }, PAGE_TIMEOUT);

      img.onload = () => {
        clearTimeout(timer);
        setPageStatus((prev) => ({ ...prev, [pageNum]: 'loaded' }));
      };
      img.onerror = () => {
        clearTimeout(timer);
        setPageStatus((prev) => ({ ...prev, [pageNum]: 'error' }));
      };
      img.src = getPageUrl(pageNum);
    });
  }, [currentPage, pages.length, getPageUrl, pageStatus]);

  // Debounced progress saving when currentPage changes
  const changePage = useCallback((newPage) => {
    const clampedPage = Math.max(0, Math.min(newPage, pages.length - 1));
    setCurrentPage(clampedPage);

    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    if (onProgress) {
      timeoutRef.current = setTimeout(() => {
        const percent = pages.length ? ((clampedPage + 1) / pages.length) * 100 : undefined;
        const completed = completionFor(percent);
        onProgress({ type: 'image', index: clampedPage }, { percent, completed })
          ?.catch?.((err) => console.error('Failed to save reading progress:', err));
      }, 1000);
    }
  }, [onProgress, pages.length]);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  const webtoon = readingDirection === 'webtoon';
  const step = readingDirection === 'double' ? 2 : 1;
  // Forward is the way the comic is read: to a higher page, whichever side of the screen that is.
  const canForward = currentPage + step < pages.length;
  const canBack = currentPage > 0;
  const forward = () => { if (canForward) changePage(currentPage + step); };
  const back = () => { if (canBack) changePage(currentPage - step); };
  // What is at each side of the screen: a comic read right to left turns forward at the left.
  const rtl = readingDirection === 'rtl';
  const toTheLeft = rtl ? forward : back;
  const toTheRight = rtl ? back : forward;
  const canGoLeft = rtl ? canForward : canBack;
  const canGoRight = rtl ? canBack : canForward;

  // A strip is read by scrolling; the buttons scroll it a screen at a time.
  const scroller = () => rootRef.current?.parentElement;
  const scrollScreen = (direction) => {
    const el = scroller();
    el?.scrollBy?.({ top: direction * el.clientHeight * 0.9, behavior: 'smooth' });
  };

  // The keyboard. It is not taken from someone who is typing.
  useEffect(() => {
    const handleKeyDown = (e) => {
      if (e.target?.closest?.('input, textarea, select, [contenteditable="true"]') || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === 'Escape') {
        if (showModes) setShowModes(false);
        else if (immersive) onImmersiveChange?.(false);
        return;
      }
      if (webtoon) {
        if (e.key === 'ArrowDown') changePage(currentPage + 1);
        if (e.key === 'ArrowUp') changePage(currentPage - 1);
      } else {
        if (e.key === 'ArrowLeft') toTheLeft();
        if (e.key === 'ArrowRight') toTheRight();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  });

  // Scroll detection for webtoon mode
  useEffect(() => {
    const container = scroller();
    if (!webtoon || !container) return undefined;

    const handleScroll = () => {
      const scrollBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
      if (scrollBottom < 200) {
        // Near bottom, load more pages
        const visiblePages = Math.ceil(container.scrollTop / container.clientHeight) + 2;
        setCurrentPage(Math.min(visiblePages, pages.length - 1));
      }
    };

    container.addEventListener('scroll', handleScroll);
    return () => container.removeEventListener('scroll', handleScroll);
  }, [webtoon, pages.length, loading]);

  // The list of reading modes closes by a click outside it. The button that opens it is not outside: it closes it
  // by itself, and a press there that closed the list would let the click open it again.
  useEffect(() => {
    if (!showModes) return undefined;
    const onPointer = (event) => {
      if (!modesRef.current?.contains(event.target) && !modesOpenerRef.current?.contains(event.target)) setShowModes(false);
    };
    document.addEventListener('pointerdown', onPointer);
    return () => document.removeEventListener('pointerdown', onPointer);
  }, [showModes]);

  // A tap turns the page at the sides and shows or hides the controls in the middle; a finger swiped across turns it
  // too. A strip has nothing to turn: a tap anywhere on it shows or hides the controls.
  const onPointerDown = (event) => {
    if (event.button > 0) return;
    gesture.current = { x: event.clientX, y: event.clientY, at: Date.now() };
  };
  const onPointerUp = (event) => {
    const start = gesture.current;
    gesture.current = null;
    if (!start) return;
    if (window.getSelection?.()?.toString()) return;
    if (event.target?.closest?.('a, button, input')) return;
    const dx = event.clientX - start.x;
    const dy = event.clientY - start.y;
    if (!webtoon && event.pointerType !== 'mouse') {
      const swipe = swipeAction({ dx, dy, zoom: 1 });
      // A swipe to the left brings what is at the right of the page, whichever way the comic is read.
      if (swipe) return swipe === 'next' ? toTheRight() : toTheLeft();
    }
    const rect = event.currentTarget.getBoundingClientRect();
    const tap = tapAction({ dx, dy, ms: Date.now() - start.at, x: (event.clientX - rect.left) / (rect.width || 1), zoom: 1 });
    if (tap && webtoon) onImmersiveChange?.(!immersive);
    else if (tap === 'next') toTheRight();
    else if (tap === 'prev') toTheLeft();
    else if (tap === 'toggle') onImmersiveChange?.(!immersive);
  };

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <Skeleton label="Carregando as páginas" className="h-[70%] w-[min(100%,28rem)]" />
      </div>
    );
  }

  if (error) {
    return (
      <div role="alert" className="flex h-full flex-col items-center justify-center gap-3 p-10 text-center">
        <p className="text-sm font-medium text-danger">Não foi possível abrir este quadrinho.</p>
        <p className="text-xs text-ink-soft">{error}</p>
        <button
          onClick={() => setAttempt((n) => n + 1)}
          className="min-h-11 rounded-full border border-border-hairline bg-white px-5 text-sm text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95"
        >
          Tentar de novo
        </button>
      </div>
    );
  }

  if (pages.length === 0) {
    return (
      <div role="alert" className="flex h-full items-center justify-center p-10 text-center text-sm font-medium text-danger">
        Este arquivo não tem páginas de imagem que possam ser lidas.
      </div>
    );
  }

  const leftPage = currentPage;
  const rightPage = currentPage + 1;
  const hasRight = rightPage < pages.length;
  const position = readingDirection === 'double'
    ? `${leftPage + 1}${hasRight ? `-${rightPage + 1}` : ''}`
    : String(currentPage + 1);
  const mode = MODES.find((m) => m.id === readingDirection);

  const retryPage = () => {
    setPageStatus((prev) => ({ ...prev, [currentPage]: 'loading' }));
    const img = new Image();
    img.onload = () => setPageStatus((prev) => ({ ...prev, [currentPage]: 'loaded' }));
    img.onerror = () => setPageStatus((prev) => ({ ...prev, [currentPage]: 'error' }));
    img.src = getPageUrl(currentPage);
  };

  const pageImage = (idx, className, extra = {}) => (
    <img
      src={getPageUrl(idx)}
      alt={`Página ${idx + 1}`}
      draggable={false}
      className={`select-none ${className}`}
      onError={(e) => {
        if (!webtoon && idx === currentPage) setPageStatus((prev) => ({ ...prev, [idx]: 'error' }));
        // Fallback to thumbnail
        e.target.src = getThumbnailUrl(idx);
      }}
      onLoad={() => setPageStatus((prev) => ({ ...prev, [idx]: 'loaded' }))}
      {...extra}
    />
  );

  const area = {
    'data-comic-page-area': true,
    onPointerDown,
    onPointerUp,
    onPointerCancel: () => { gesture.current = null; },
    style: { touchAction: 'pan-y pinch-zoom' },
  };

  return (
    <div ref={rootRef} className={webtoon ? 'relative flex min-h-full flex-col items-center pb-6' : 'relative h-full'}>
      {webtoon ? (
        <div {...area} className="flex w-full flex-col items-center gap-1 py-2">
          {pages.map((page, idx) => (
            <React.Fragment key={idx}>
              {pageImage(idx, 'h-auto w-full', {
                style: { maxWidth: MAX_READING_WIDTH },
                loading: idx <= currentPage + PRELOAD_COUNT ? 'eager' : 'lazy',
              })}
            </React.Fragment>
          ))}
        </div>
      ) : (
        <div {...area} className="flex h-full items-center justify-center gap-1 p-2 sm:p-4">
          {readingDirection === 'double' ? (
            <>
              {pageImage(leftPage, 'max-h-full max-w-[50%] object-contain')}
              {hasRight && pageImage(rightPage, 'max-h-full max-w-[50%] object-contain')}
            </>
          ) : pageStatus[currentPage] === 'error' ? (
            <div className="flex flex-col items-center gap-3 text-center">
              <span role="alert" className="text-sm font-medium text-danger">Não foi possível carregar esta página.</span>
              <button
                onClick={retryPage}
                className="min-h-11 rounded-full border border-border-hairline bg-white px-5 text-sm text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95"
              >
                Tentar de novo
              </button>
            </div>
          ) : (
            pageImage(currentPage, 'max-h-full max-w-full object-contain')
          )}
        </div>
      )}

      {/* The controls, at the bottom. They float over the page, so that hiding them does not move it. */}
      <div
        className={
          webtoon
            ? 'pointer-events-none sticky bottom-[calc(env(safe-area-inset-bottom)+0.75rem)] z-30 h-0 w-full'
            : 'pointer-events-none absolute inset-x-0 bottom-[calc(env(safe-area-inset-bottom)+0.75rem)] z-30'
        }
      >
        <div className="absolute inset-x-0 bottom-0 flex justify-center">
          <div
            data-chrome={immersive ? 'hidden' : 'shown'}
            inert={immersive}
            className={`relative flex items-center gap-0.5 rounded-full border border-border-hairline bg-white/95 px-1.5 py-1 shadow-lg backdrop-blur transition-[opacity,transform] duration-200 ease-out ${
              immersive ? 'pointer-events-none translate-y-3 opacity-0' : 'pointer-events-auto opacity-100'
            }`}
          >
            {showModes && (
              <div
                ref={modesRef}
                role="menu"
                aria-label="Modo de leitura"
                className="absolute bottom-full left-1/2 mb-2 w-[min(18rem,calc(100vw-1.5rem))] -translate-x-1/2 animate-rise-in rounded-xl border border-border-hairline bg-white p-1.5 shadow-xl"
              >
                {MODES.map((m) => (
                  <button
                    key={m.id}
                    role="menuitemradio"
                    aria-checked={m.id === readingDirection}
                    onClick={() => {
                      setReadingDirection(m.id);
                      setShowModes(false);
                    }}
                    className={`flex min-h-11 w-full items-center justify-between gap-3 rounded-lg px-3 text-left text-sm transition-colors hover:bg-surface-alt ${
                      m.id === readingDirection ? 'font-medium text-brand' : 'text-ink-soft hover:text-ink'
                    }`}
                  >
                    {m.label}
                    {m.id === readingDirection && <span aria-hidden="true">✓</span>}
                  </button>
                ))}
              </div>
            )}
            {!showModes && fromFile && (
              <span
                title={FILE_SAID[readingDirection]}
                className="absolute bottom-full left-1/2 mb-2 -translate-x-1/2 whitespace-nowrap rounded-full bg-ink/70 px-3 py-1 text-[11px] text-white"
              >
                {mode.label} · pelo arquivo
              </span>
            )}

            {webtoon ? (
              <>
                <button onClick={() => scrollScreen(-1)} aria-label="Rolar para cima" className={control}>
                  <svg {...iconProps}><path d="M5 15l7-7 7 7" /></svg>
                </button>
                <div className="shrink-0 whitespace-nowrap px-1 font-mono text-sm text-ink-soft">{pages.length} {pages.length === 1 ? 'página' : 'páginas'}</div>
                <button onClick={() => scrollScreen(1)} aria-label="Rolar para baixo" className={control}>
                  <svg {...iconProps}><path d="M5 9l7 7 7-7" /></svg>
                </button>
              </>
            ) : (
              <>
                <button onClick={toTheLeft} disabled={!canGoLeft} aria-label={rtl ? 'Próxima página, à esquerda' : 'Página anterior, à esquerda'} className={control}>
                  <Chevron dir="left" />
                </button>
                <div className="shrink-0 whitespace-nowrap px-1 font-mono text-sm text-ink">
                  {position} <span className="text-ink-soft">/ {pages.length}</span>
                </div>
                <button onClick={toTheRight} disabled={!canGoRight} aria-label={rtl ? 'Página anterior, à direita' : 'Próxima página, à direita'} className={control}>
                  <Chevron dir="right" />
                </button>
              </>
            )}

            <span className="mx-1 h-6 w-px bg-border-hairline max-sm:hidden" aria-hidden="true" />
            <button
              ref={modesOpenerRef}
              onClick={() => setShowModes((v) => !v)}
              aria-haspopup="menu"
              aria-expanded={showModes}
              aria-label={`Modo de leitura: ${mode.label}`}
              title={`Modo de leitura: ${mode.label}`}
              className={`${control} ${showModes ? 'bg-brand/10 text-brand' : ''}`}
            >
              <svg {...iconProps}><path d="M4 6h16M4 12h16M4 18h10" /></svg>
            </button>
            <button onClick={() => onImmersiveChange?.(true)} aria-label="Esconder os controles" title="Esconder os controles" className={control}>
              <svg {...iconProps}><path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" /></svg>
            </button>
          </div>

          {/* With the controls hidden, a quiet mark of where the person is. A tap in the middle of the page brings them back. */}
          {!webtoon && (
            <div
              data-chrome-mark
              aria-hidden={!immersive}
              className={`pointer-events-none absolute bottom-0 rounded-full bg-ink/70 px-3 py-1 font-mono text-[11px] text-white transition-opacity duration-200 ${immersive ? 'opacity-100' : 'opacity-0'}`}
            >
              {position} / {pages.length}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
