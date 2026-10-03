import React, { useState, useEffect, useRef, useCallback } from 'react';
import ePub from 'epubjs';
import { api } from '../../../../lib/api';
import { Skeleton } from '../../../../components/ui/Skeleton';
import { buildEpubProgress } from '../../epubProgress';
import { applyEpubSettings, fontFaceCss, sanitizeSettings, READING_THEMES } from '../../epubThemes';
import { toScreen, acrossPage } from '../../epubGestures';
import { flattenToc } from '../../epubToc';
import { tapAction, swipeAction } from '../../pdfGestures';
import { getEpubSettings, saveEpubSettings } from '../../preferences';
import { epubPlaceProblem } from '../../placeCheck';
import ReadingSettingsPanel from './ReadingSettingsPanel';

const iconProps = { viewBox: '0 0 24 24', width: 18, height: 18, fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true };
const Chevron = ({ dir }) => (
  <svg {...iconProps}>
    <path d={dir === 'left' ? 'M15 5l-7 7 7 7' : 'M9 5l7 7-7 7'} />
  </svg>
);

// The room kept under the page for the controls (4.5rem), in pixels.
const ROOM_FOR_CONTROLS = 72;

const control =
  'flex h-11 min-w-11 items-center justify-center rounded-full text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95 disabled:opacity-30 disabled:hover:bg-transparent disabled:active:scale-100';

/**
 * The reader of an EPUB, with epubjs driven directly (not react-reader).
 *
 * react-reader wraps epubjs in a class component whose lifecycle / internal
 * queue has timing issues that produce "No Section Found" errors when
 * displaying sections whose TOC-hrefs don't match the OPF-relative spine
 * hrefs.  By driving epubjs ourselves we side-step all of that.
 *
 * The controls are at the bottom, where the thumb is: previous and next page, how far into the book, the look of the
 * text (size, page color, font, spacing), the table of contents and a button that hides everything but the page. A tap
 * on the left or right of the page turns it, a tap in the middle hides or shows the controls, and a swipe turns it too;
 * on a keyboard the arrows, Page Up and Page Down do.
 */
export default function EpubViewer({ fileUrl, onProgress, initialProgress, locator, onPlaceFailed, immersive = false, onImmersiveChange }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [attempt, setAttempt] = useState(0); // opening the book again
  const [toc, setToc] = useState([]);
  const [panel, setPanel] = useState(null); // 'settings', 'toc' or nothing
  const [settings, setSettings] = useState(() => getEpubSettings());
  const [percent, setPercent] = useState(null); // how far through the book, once the positions of the book are known
  const [room, setRoom] = useState(null); // the height the reader has with its header shown: the text is laid out for it

  const viewerRef = useRef(null);
  const bookRef = useRef(null);
  const renditionRef = useRef(null);
  const timeoutRef = useRef(null);
  const settingsRef = useRef(settings);
  settingsRef.current = settings;
  const gesture = useRef(null);
  const touchRef = useRef({ down() {}, up() {}, cancel() {} });
  const keyRef = useRef(() => {});
  const surfaceRef = useRef(null);
  const placeRef = useRef(null); // where the page in view starts (a CFI), to come back to it when the text is laid out again
  const layoutRef = useRef(null);
  const backTimer = useRef(null);
  const settlingRef = useRef(false); // the text is being laid out again: what epub.js says of the place meanwhile is not the person's
  const settleRef = useRef(null);
  const immersiveRef = useRef(immersive);
  immersiveRef.current = immersive;

  // ── Helpers ──────────────────────────────────────────────────────

  /**
   * Find a spine Section whose href matches `rawHref` (which may be
   * relative to nav.xhtml, e.g. "chapter03.xhtml").  The spine stores
   * hrefs relative to the OPF (e.g. "Text/chapter03.xhtml"), so a
   * simple suffix match is needed.
   */
  const resolveToSpine = useCallback((book, rawHref) => {
    if (!book || !rawHref) return null;
    const clean = rawHref.split('#')[0];
    // 1) Direct lookup
    let section = book.spine.get(clean);
    if (section) return section;
    // 2) Suffix match against spine items
    section = book.spine.spineItems.find(
      (s) => s.href === clean || s.href.endsWith('/' + clean),
    );
    return section || null;
  }, []);

  // ── Initialise book + rendition ──────────────────────────────────
  useEffect(() => {
    let cancelled = false;
    let book = null;
    let rendition = null;

    async function init() {
      try {
        setLoading(true);
        setError(null);

        // 1. Fetch EPUB binary via authenticated axios request
        const response = await api.get(fileUrl, {
          responseType: 'arraybuffer',
          timeout: 60000,
        });
        if (cancelled) return;

        // 2. Create book – pass ArrayBuffer + force openAs 'binary'
        book = ePub(response.data, { openAs: 'binary' });
        bookRef.current = book;

        // 3. Wait for navigation (TOC) to be ready
        const nav = await book.loaded.navigation;
        if (cancelled) return;
        setToc(nav.toc || []);

        // 4. Wait for book to be fully opened (spine + replacements done)
        await book.opened;
        if (cancelled) return;

        // 5. Ensure the container element is available
        if (!viewerRef.current) {
          throw new Error('Viewer container not mounted');
        }

        // 6. Create rendition — book.opened already resolved so the
        //    internal queue processes immediately.
        rendition = book.renderTo(viewerRef.current, {
          width: '100%',
          height: '100%',
          flow: 'paginated',
          manager: 'default',
          allowScriptedContent: true,
        });
        renditionRef.current = rendition;

        // 7. What the book looks like, and what a finger or a key does on it (the book is in an iframe of its own)
        applyEpubSettings(rendition, settingsRef.current);
        rendition.hooks.content.register((contents) => {
          contents.addStylesheetCss(fontFaceCss(), 'codice-fonts');
          const doc = contents.document;
          const frame = doc.defaultView?.frameElement;
          const point = (e) => {
            const at = frame ? frame.getBoundingClientRect() : { left: 0, top: 0 };
            return toScreen({ frameLeft: at.left, frameTop: at.top, clientX: e.clientX, clientY: e.clientY });
          };
          doc.addEventListener('pointerdown', (e) => touchRef.current.down(point(e), e));
          doc.addEventListener('pointerup', (e) =>
            touchRef.current.up(point(e), e, {
              selected: !!doc.defaultView.getSelection?.()?.toString(),
              onLink: !!e.target?.closest?.('a'),
            }));
          doc.addEventListener('pointercancel', () => touchRef.current.cancel());
          doc.addEventListener('keyup', (e) => keyRef.current(e));
        });

        // 8. Track location changes
        //    Each one saves the exact place (CFI), the chapter and how far into it, and, once the
        //    positions of the whole book are known, how far through the book (a percentage).
        let lastLocation = null;
        const percentageFromCfi = (cfi) =>
          book.locations.length() > 0 ? book.locations.percentageFromCfi(cfi) : null;
        const report = (location) => {
          if (timeoutRef.current) clearTimeout(timeoutRef.current);
          if (!onProgress) return;
          timeoutRef.current = setTimeout(() => {
            const progress = buildEpubProgress(location, percentageFromCfi);
            if (progress) {
              onProgress(progress.locator, progress.extras)
                ?.catch?.((err) => console.error('Failed to save progress:', err));
            }
          }, 1000);
        };
        const accept = (location) => {
          lastLocation = location;
          setPercent(percentageFromCfi(location.start.cfi));
          report(location);
        };
        rendition.on('relocated', (location) => {
          if (!location || !location.start || settlingRef.current) return;
          placeRef.current = location.start.cfi;
          accept(location);
        });
        // After the text is laid out again: back to the place, and what is saved is the page that shows it.
        settleRef.current = async (place) => {
          await rendition.display(place);
          const location = rendition.currentLocation?.();
          if (!cancelled && location?.start) accept(location);
        };

        // The positions of the whole book take a moment to compute: in the background, and the
        // place the person is at is reported again when they are ready, now with a percentage.
        book.locations
          .generate(1600)
          .then(() => {
            if (cancelled || !lastLocation) return;
            setPercent(percentageFromCfi(lastLocation.start.cfi));
            report(lastLocation);
          })
          .catch(() => {
            // Without them there is no percentage, and everything else works.
          });

        // 9. Display the initial location
        //    - CFI strings ("epubcfi(...)") are opened where they point, if the book still has that chapter
        //      and it is still the one the place was saved in
        //    - Raw hrefs from old progress data need resolving through the spine because TOC hrefs and
        //      spine hrefs may differ
        //    - No progress → display() with no args = first linear item
        //    A place that cannot be opened is said (onPlaceFailed), and the start is shown: never in silence.
        const failPlace = async (reason) => {
          console.warn('[EpubViewer] Could not open the place asked for:', reason);
          await rendition.display();
          onPlaceFailed?.({ reason });
        };
        try {
          if (initialProgress && initialProgress.startsWith('epubcfi(')) {
            let section = null;
            try {
              section = book.spine.get(initialProgress);
            } catch {
              section = null;
            }
            const problem = epubPlaceProblem({ section, locator });
            if (problem) await failPlace(problem);
            else {
              try {
                await rendition.display(initialProgress);
              } catch {
                await failPlace('Não foi possível abrir este ponto no EPUB: o arquivo pode ter mudado.');
              }
            }
          } else if (initialProgress) {
            // Legacy raw-href progress – resolve to spine
            const section = resolveToSpine(book, initialProgress);
            if (section) await rendition.display(section.href);
            else await failPlace('O capítulo onde o ponto estava não existe mais neste EPUB: o arquivo pode ter mudado.');
          } else {
            await rendition.display();
          }
        } catch (displayErr) {
          console.warn('[EpubViewer] Display failed, trying first spine item:', displayErr);
          try {
            const firstHref = book.spine.spineItems[0]?.href;
            if (firstHref) {
              await rendition.display(firstHref);
            } else {
              throw displayErr;
            }
          } catch (fallbackErr) {
            console.error('[EpubViewer] Fallback also failed:', fallbackErr);
            throw fallbackErr;
          }
        }

        if (!cancelled) setLoading(false);
      } catch (err) {
        if (!cancelled) {
          console.error('[EpubViewer] init error:', err);
          setError(err.message);
          setLoading(false);
        }
      }
    }

    init();

    return () => {
      cancelled = true;
      if (rendition) {
        rendition.destroy();
      }
      if (book) {
        book.destroy();
      }
      bookRef.current = null;
      renditionRef.current = null;
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fileUrl, attempt]);


  // ── What the person chooses for the look of the text ──────────
  const change = (patch) => {
    const next = sanitizeSettings({ ...settingsRef.current, ...patch });
    saveEpubSettings(next);
    setSettings(next);
  };
  // A bigger letter, another font or another spacing lays the text out again: the book goes back to where the page in
  // view started, and not to the same page number, which would be elsewhere in the text.
  useEffect(() => {
    const rendition = renditionRef.current;
    const layout = `${settings.size}|${settings.font}|${settings.spacing}`;
    const changed = layoutRef.current !== null && layoutRef.current !== layout;
    layoutRef.current = layout;
    applyEpubSettings(rendition, settings);
    if (!rendition || !changed || !placeRef.current) return;
    // The place is the one the person was at before the change: epub.js says other places while it lays the text out.
    const place = placeRef.current;
    settlingRef.current = true;
    clearTimeout(backTimer.current);
    backTimer.current = setTimeout(async () => {
      try {
        await settleRef.current?.(place);
      } catch {
        // the page stays where it is
      }
      backTimer.current = setTimeout(() => { settlingRef.current = false; }, 250);
    }, 80);
  }, [settings]);
  useEffect(() => () => clearTimeout(backTimer.current), []);

  // The text is laid out for the room there is with the header shown, and stays at the bottom of the screen: when the
  // header folds away to leave only the page, the text does not move and is not laid out again (it would start at
  // another place). The room is measured again once things settle, but not while the controls are hidden.
  useEffect(() => {
    const el = surfaceRef.current;
    if (!el) return undefined;
    let timer = null;
    const measure = () => {
      if (!immersiveRef.current && el.clientHeight) setRoom(el.clientHeight);
    };
    const settle = () => {
      clearTimeout(timer);
      timer = setTimeout(measure, 250);
    };
    measure();
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', settle);
      return () => {
        clearTimeout(timer);
        window.removeEventListener('resize', settle);
      };
    }
    const observer = new ResizeObserver(settle);
    observer.observe(el);
    return () => {
      clearTimeout(timer);
      observer.disconnect();
    };
  }, []);

  // The page of the book follows the room it is given (the header folds away, a phone is turned): the book is
  // laid out again, at the same place.
  useEffect(() => {
    const el = viewerRef.current;
    if (!el || typeof ResizeObserver === 'undefined') return undefined;
    let timer = null;
    const observer = new ResizeObserver(() => {
      clearTimeout(timer);
      timer = setTimeout(() => renditionRef.current?.resize(), 150);
    });
    observer.observe(el);
    return () => {
      clearTimeout(timer);
      observer.disconnect();
    };
  }, []);

  // ── Navigation ──────────────────────────────────────────────────
  const prevPage = () => renditionRef.current?.prev();
  const nextPage = () => renditionRef.current?.next();

  const goToTocItem = (href) => {
    if (!renditionRef.current || !bookRef.current) return;

    // TOC hrefs are relative to nav.xhtml (e.g. "chapter03.xhtml#ch3")
    // but spine hrefs are relative to the OPF (e.g. "Text/chapter03.xhtml").
    // Resolve via suffix matching.
    const section = resolveToSpine(bookRef.current, href);
    if (section) {
      renditionRef.current.display(section.href);
    } else {
      // Last resort: pass the raw href and hope epubjs can figure it out
      renditionRef.current.display(href);
    }
    setPanel(null);
  };

  // ── The keyboard ──────────────────────────────────────────────
  // Not taken from someone who is typing, nor from a shortcut. The book is in an iframe, which has its own keys:
  // they are given to the same function.
  keyRef.current = (e) => {
    if (e.target?.closest?.('input, textarea, select, [contenteditable="true"]') || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === 'ArrowLeft' || e.key === 'PageUp') prevPage();
    else if (e.key === 'ArrowRight' || e.key === 'PageDown') nextPage();
    else if (e.key === 'Escape') {
      if (panel) setPanel(null);
      else if (immersive) onImmersiveChange?.(false);
    }
  };
  useEffect(() => {
    const onKey = (e) => keyRef.current(e);
    document.addEventListener('keyup', onKey);
    return () => document.removeEventListener('keyup', onKey);
  }, []);

  // ── A finger on the page ──────────────────────────────────────
  // A tap turns the page at the sides and shows or hides the controls in the middle; a finger swiped across turns it
  // too. With a panel open, a tap on the page only closes it. Whoever is selecting text, or touches a link, is not
  // turning a page. A mouse turns by click but not by dragging.
  touchRef.current = {
    down(point, event) {
      if (event.button > 0) return;
      gesture.current = { ...point, at: Date.now() };
    },
    up(point, event, { selected = false, onLink = false } = {}) {
      const start = gesture.current;
      gesture.current = null;
      if (!start || selected || onLink) return;
      const dx = point.x - start.x;
      const dy = point.y - start.y;
      const ms = Date.now() - start.at;
      if (panel) {
        if (tapAction({ dx, dy, ms, x: 0.5, zoom: 1 })) setPanel(null);
        return;
      }
      if (event.pointerType !== 'mouse') {
        const swipe = swipeAction({ dx, dy, zoom: 1 });
        if (swipe) return swipe === 'next' ? nextPage() : prevPage();
      }
      const rect = viewerRef.current.getBoundingClientRect();
      const tap = tapAction({ dx, dy, ms, x: acrossPage({ x: point.x, left: rect.left, width: rect.width }), zoom: 1 });
      if (tap === 'next') nextPage();
      else if (tap === 'prev') prevPage();
      else if (tap === 'toggle') onImmersiveChange?.(!immersive);
    },
    cancel() {
      gesture.current = null;
    },
  };
  const onSurfaceDown = (event) => touchRef.current.down({ x: event.clientX, y: event.clientY }, event);
  const onSurfaceUp = (event) => {
    if (event.target?.closest?.('[data-chrome], a, button, input')) {
      gesture.current = null;
      return;
    }
    touchRef.current.up({ x: event.clientX, y: event.clientY }, event, { selected: !!window.getSelection?.()?.toString() });
  };

  // The panels close by a press outside them. The bar they open from is not outside (its buttons close them by
  // themselves, and a press that closed them first would let the click open them again), and nor is the page: a tap on
  // it closes them (above), and a finger that swipes across it does not.
  useEffect(() => {
    if (!panel) return undefined;
    const onPointer = (event) => {
      if (!surfaceRef.current?.contains(event.target)) setPanel(null);
    };
    document.addEventListener('pointerdown', onPointer);
    return () => document.removeEventListener('pointerdown', onPointer);
  }, [panel]);
  // Hiding the controls takes the panels with them.
  useEffect(() => {
    if (immersive) setPanel(null);
  }, [immersive]);

  // ── Cleanup debounce timer ────────────────────────────────────
  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  // ── Render ───────────────────────────────────────────────────────
  const theme = READING_THEMES.find((t) => t.id === settings.theme);
  const shownPercent = percent == null ? '–' : `${Math.round(percent * 100)}%`;
  const entries = flattenToc(toc);

  return (
    <div
      ref={surfaceRef}
      onPointerDown={onSurfaceDown}
      onPointerUp={onSurfaceUp}
      onPointerCancel={() => touchRef.current.cancel()}
      style={{ backgroundColor: theme.background, touchAction: 'pan-y pinch-zoom' }}
      className="relative h-full overflow-hidden transition-colors duration-300"
    >
      {/* The page. Room is left under it for the controls, so that showing them does not move the text. */}
      <div
        className="absolute inset-x-0 bottom-[4.5rem] mx-auto w-full max-w-3xl px-2 sm:px-6"
        style={{ height: room ? room - ROOM_FOR_CONTROLS - 12 : 'calc(100% - 4.5rem - 0.75rem)' }}
      >
        <div ref={viewerRef} data-epub-page className="h-full w-full overflow-hidden" />
      </div>

      {loading && !error && (
        <div className="absolute inset-x-0 bottom-[4.5rem] flex h-[min(calc(100%-4.5rem),32rem)] justify-center px-4 pt-6">
          <Skeleton label="Carregando o livro" className="h-[85%] w-[min(100%,40rem)]" />
        </div>
      )}

      {error && (
        <div role="alert" className="absolute inset-x-0 bottom-[4.5rem] flex h-[min(calc(100%-4.5rem),32rem)] flex-col items-center justify-center gap-3 px-6 text-center">
          <p className="text-sm font-medium text-danger">Não foi possível abrir este EPUB.</p>
          <p className="max-w-md text-xs text-ink-soft">{error}</p>
          <button
            onClick={() => setAttempt((n) => n + 1)}
            className="min-h-11 rounded-full border border-border-hairline bg-white px-5 text-sm text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95"
          >
            Tentar de novo
          </button>
        </div>
      )}

      {/* The controls, at the bottom. Hidden, they leave a small mark of how far the person is. */}
      <div className="pointer-events-none absolute inset-x-0 bottom-[calc(env(safe-area-inset-bottom)+0.75rem)] z-30 flex justify-center">
        <div
          data-chrome={immersive ? 'hidden' : 'shown'}
          inert={immersive}
          className={`relative flex items-center gap-0.5 rounded-full border border-border-hairline bg-white/95 px-1.5 py-1 shadow-lg backdrop-blur transition-[opacity,transform] duration-200 ease-out ${
            immersive ? 'pointer-events-none translate-y-3 opacity-0' : 'pointer-events-auto opacity-100'
          }`}
        >
          {panel === 'settings' && (
            <div className="absolute bottom-full left-1/2 mb-2 max-h-[calc(100dvh-9rem)] w-[min(24rem,calc(100vw-1.5rem))] -translate-x-1/2 animate-rise-in overflow-y-auto rounded-xl border border-border-hairline bg-white p-4 shadow-xl">
              <ReadingSettingsPanel settings={settings} onChange={change} />
            </div>
          )}
          {panel === 'toc' && entries.length > 0 && (
            <nav
              aria-label="Sumário do livro"
              className="absolute bottom-full left-1/2 mb-2 max-h-[55vh] w-[min(26rem,calc(100vw-1.5rem))] -translate-x-1/2 animate-rise-in overflow-y-auto rounded-xl border border-border-hairline bg-white p-1.5 shadow-xl"
            >
              {entries.map((entry, i) => (
                <button
                  key={`${i}-${entry.id || entry.href}`}
                  onClick={() => goToTocItem(entry.href)}
                  style={{ paddingLeft: `${0.75 + entry.depth * 1}rem` }}
                  className="flex min-h-11 w-full items-center rounded-lg py-2.5 pr-3 text-left text-sm text-ink-soft transition-colors hover:bg-surface-alt hover:text-ink"
                  title={entry.label}
                >
                  <span className="truncate">{entry.label}</span>
                </button>
              ))}
            </nav>
          )}

          <button onClick={prevPage} aria-label="Página anterior" className={control}>
            <Chevron dir="left" />
          </button>
          <div title="Quanto do livro já foi lido" className="shrink-0 whitespace-nowrap px-1 text-center font-mono text-sm text-ink" style={{ minWidth: '3.25ch' }}>
            {shownPercent}
          </div>
          <button onClick={nextPage} aria-label="Próxima página" className={control}>
            <Chevron dir="right" />
          </button>

          <span className="mx-1 h-6 w-px bg-border-hairline max-sm:hidden" aria-hidden="true" />
          <button
            onClick={() => setPanel((p) => (p === 'settings' ? null : 'settings'))}
            aria-expanded={panel === 'settings'}
            aria-label="Aparência do texto"
            title="Aparência do texto"
            className={`${control} ${panel === 'settings' ? 'bg-brand/10 text-brand' : ''}`}
          >
            <span aria-hidden="true" className="font-display text-lg leading-none">Aa</span>
          </button>
          {entries.length > 0 && (
            <button
              onClick={() => setPanel((p) => (p === 'toc' ? null : 'toc'))}
              aria-expanded={panel === 'toc'}
              aria-label="Sumário do livro"
              title="Sumário do livro"
              className={`${control} ${panel === 'toc' ? 'bg-brand/10 text-brand' : ''}`}
            >
              <svg {...iconProps}><path d="M4 6h16M4 12h10M4 18h16" /></svg>
            </button>
          )}
          <button onClick={() => onImmersiveChange?.(true)} aria-label="Esconder os controles" title="Esconder os controles" className={control}>
            <svg {...iconProps}><path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" /></svg>
          </button>
        </div>

        <div
          data-chrome-mark
          aria-hidden={!immersive}
          style={{ backgroundColor: theme.text, color: theme.background }}
          className={`pointer-events-none absolute bottom-0 rounded-full px-3 py-1 font-mono text-[11px] opacity-0 transition-opacity duration-200 ${immersive ? '!opacity-60' : ''}`}
        >
          {shownPercent}
        </div>
      </div>
    </div>
  );
}
