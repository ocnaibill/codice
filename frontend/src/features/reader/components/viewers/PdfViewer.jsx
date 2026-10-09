import React, { useState, useEffect, useLayoutEffect, useRef } from 'react';
import { useDialog } from '../../../../lib/useDialog';
import { Document, Page, pdfjs } from 'react-pdf';
import { authenticatedUrl } from '../../../../lib/api';
import { Skeleton } from '../../../../components/ui/Skeleton';
import { PdfPassword } from './PdfPassword';
import { completionFor } from '../../progressRules';
import { pdfPlaceProblem } from '../../placeCheck';
import { loadOutline } from '../../pdfOutline';
import { ZOOMS, readingWidth, tapAction, swipeAction } from '../../pdfGestures';
import { QUIET_AFTER_TURN_MS, TOUCH_TAP_MAX_MS, liftMeaning } from '../../epubGestures';
import { useSelectionWatcher } from '../../useSelectionWatcher';

import 'react-pdf/dist/Page/AnnotationLayer.css';
import 'react-pdf/dist/Page/TextLayer.css';

// Vite worker configuration for PDF.js
pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url,
).toString();

const iconProps = { viewBox: '0 0 24 24', width: 18, height: 18, fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true };
const Chevron = ({ dir }) => (
  <svg {...iconProps}>
    <path d={dir === 'left' ? 'M15 5l-7 7 7 7' : 'M9 5l7 7-7 7'} />
  </svg>
);

const control =
  'flex h-11 min-w-11 items-center justify-center rounded-full text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95 disabled:opacity-30 disabled:hover:bg-transparent disabled:active:scale-100';

/**
 * The reader of a PDF, one page at a time. The controls are at the bottom, where the thumb is: the page and a way to
 * jump to another, the zoom, the table of contents and a button that hides everything but the page. On a touch screen
 * a tap on the left or the right of the page turns it, a tap in the middle hides or shows the controls, and a swipe
 * turns it too; on a keyboard the arrows, Page Up and Page Down, Home and End do, and + and - zoom.
 */
// What the PDF reader says when it asks for the password again: the one given was not the right one (pdf.js: PasswordResponses).
const INCORRECT_PASSWORD = 2;

export default function PdfViewer({ fileUrl, onProgress, initialProgress, onPlaceFailed, immersive = false, onImmersiveChange, onSelection, onCancelPassword }) {
  // The reader of the PDF asks for a password: what it asks with, and whether the last answer was wrong.
  const [asking, setAsking] = useState(null);
  const [numPages, setNumPages] = useState(null);
  const [pageNumber, setPageNumber] = useState(initialProgress ? parseInt(initialProgress, 10) || 1 : 1);
  const [outline, setOutline] = useState([]);
  const [showOutline, setShowOutline] = useState(false);
  const [draft, setDraft] = useState(null); // what is being typed in the page box, until Enter
  const [zoom, setZoom] = useState(1);
  const [available, setAvailable] = useState(() => window.innerWidth);
  const timeoutRef = useRef(null);
  const rootRef = useRef(null);
  const gesture = useRef(null);
  const quietUntil = useRef(0); // until when a selection that appears is the phone's own, late (see QUIET_AFTER_TURN_MS)
  const outlineRef = useRef(null);
  // What is selected on the page is told to whoever offers what to do with it (the menu by the selection).
  // It is tied to the page it is on (the page number is the person's: the one shown now).
  useSelectionWatcher(
    rootRef,
    (found) => onSelection?.(found && { text: found.text, rect: found.rect, touch: found.touch, clear: found.clear, locator: { type: 'pdf', page: pageNumber - 1 } }),
    !!onSelection,
    () => Date.now() < quietUntil.current
  );
  const outlineOpenerRef = useRef(null);
  // The contents: the focus goes in, Escape closes them and the focus goes back to the button that opened them.
  useDialog(outlineRef, { active: showOutline && outline.length > 0, onEscape: () => setShowOutline(false), trap: false });

  // How wide the page may be: what the screen gives, up to a comfortable measure, and it follows the screen
  // (a phone turned on its side, a window made larger).
  useLayoutEffect(() => {
    const el = rootRef.current;
    if (!el) return undefined;
    const measure = () => setAvailable(el.clientWidth || window.innerWidth);
    measure();
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure);
      return () => window.removeEventListener('resize', measure);
    }
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  function onDocumentLoadSuccess(pdf) {
    setNumPages(pdf.numPages);
    // A page the file does not have is said, not hidden by opening the first one in silence.
    const problem = pdfPlaceProblem(initialProgress, pdf.numPages);
    if (problem) {
      setPageNumber(1);
      onPlaceFailed?.({ reason: problem });
    }
    loadOutline(pdf).then(setOutline).catch(() => setOutline([]));
  }

  // Debounced progress saving when pageNumber changes
  const changePage = (newPage) => {
    setPageNumber(newPage);
    // A new page starts at its top, not where the last one was left.
    rootRef.current?.parentElement?.scrollTo?.({ top: 0 });

    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    if (onProgress) {
      timeoutRef.current = setTimeout(() => {
        const percent = numPages ? (newPage / numPages) * 100 : undefined;
        const completed = completionFor(percent);
        onProgress({ type: 'pdf', page: newPage - 1 }, { percent, completed })
          .catch((err) => console.error("Failed to save PDF reading progress:", err));
      }, 1000);
    }
  };

  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  const goTo = (page) => {
    if (Number.isInteger(page) && page >= 1 && (!numPages || page <= numPages) && page !== pageNumber) changePage(page);
  };
  const prevPage = () => goTo(pageNumber - 1);
  // Forward needs to know how long the document is: before that there is no "next".
  const nextPage = () => numPages && goTo(pageNumber + 1);
  const zoomBy = (step) => {
    const at = ZOOMS.indexOf(zoom);
    setZoom(ZOOMS[Math.min(ZOOMS.length - 1, Math.max(0, at + step))]);
  };

  // The keyboard. It is not taken from someone who is typing (the box of the page, a note).
  useEffect(() => {
    const onKey = (event) => {
      const target = event.target;
      if (target?.closest?.('input, textarea, select, [contenteditable="true"]') || event.metaKey || event.ctrlKey || event.altKey) return;
      const zoomed = zoom > 1; // then the arrows move the page, they do not turn it
      switch (event.key) {
        case 'ArrowRight':
        case 'PageDown':
          if (event.key === 'PageDown' || !zoomed) { event.preventDefault(); nextPage(); }
          break;
        case 'ArrowLeft':
        case 'PageUp':
          if (event.key === 'PageUp' || !zoomed) { event.preventDefault(); prevPage(); }
          break;
        case 'Home': event.preventDefault(); goTo(1); break;
        case 'End': if (numPages) { event.preventDefault(); goTo(numPages); } break;
        case '+': case '=': zoomBy(1); break;
        case '-': zoomBy(-1); break;
        case '0': setZoom(1); break;
        case 'Escape':
          if (showOutline) setShowOutline(false);
          else if (immersive) onImmersiveChange?.(false);
          break;
        default:
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // The table of contents closes by a click outside it. Its own button is not outside: a press on it would close the
  // list before the click toggles it open again.
  useEffect(() => {
    if (!showOutline) return undefined;
    const onPointer = (event) => {
      if (!outlineRef.current?.contains(event.target) && !outlineOpenerRef.current?.contains(event.target)) setShowOutline(false);
    };
    document.addEventListener('pointerdown', onPointer);
    return () => document.removeEventListener('pointerdown', onPointer);
  }, [showOutline]);

  // Touch: a tap turns the page at the sides and shows or hides the controls in the middle; a swipe turns it. Whoever
  // is selecting text, or touches a link, is not turning a page. A finger that taps is not selecting: a phone may select the
  // word under it anyway, and that word goes (#180), as it does on the page of an EPUB.
  const selectedNow = () => !!window.getSelection?.()?.toString();
  const onPointerDown = (event) => {
    if (event.pointerType !== 'touch' && event.pointerType !== 'pen') return;
    gesture.current = { x: event.clientX, y: event.clientY, at: Date.now(), selected: selectedNow() };
  };
  const onPointerUp = (event) => {
    const start = gesture.current;
    gesture.current = null;
    if (!start) return;
    const meaning = liftMeaning({
      pointerType: event.pointerType, selectedAtDown: start.selected, selectedNow: selectedNow(), onLink: !!event.target?.closest?.('a, button, input'),
    });
    if (meaning === 'ignore') return;
    if (meaning === 'dismiss') {
      window.getSelection?.()?.removeAllRanges?.();
      return;
    }
    const dx = event.clientX - start.x;
    const dy = event.clientY - start.y;
    const acted = () => {
      quietUntil.current = Date.now() + QUIET_AFTER_TURN_MS;
      if (selectedNow()) window.getSelection?.()?.removeAllRanges?.();
    };
    if (!selectedNow()) {
      const swipe = swipeAction({ dx, dy, zoom });
      if (swipe) {
        acted();
        return swipe === 'next' ? nextPage() : prevPage();
      }
    }
    const rect = event.currentTarget.getBoundingClientRect();
    const tap = tapAction({ dx, dy, ms: Date.now() - start.at, x: (event.clientX - rect.left) / (rect.width || 1), zoom, maxMs: TOUCH_TAP_MAX_MS });
    if (!tap) return;
    acted();
    if (tap === 'next') nextPage();
    else if (tap === 'prev') prevPage();
    else if (tap === 'toggle') onImmersiveChange?.(!immersive);
  };

  const pageWidth = Math.round(readingWidth(available, immersive) * zoom);
  const commitDraft = () => {
    if (draft !== null && draft !== '') goTo(parseInt(draft, 10));
    setDraft(null);
  };

  return (
    <div ref={rootRef} className={`relative flex min-h-full flex-col items-center ${immersive ? 'pb-6 pt-0' : 'px-2 pb-6 pt-3 sm:px-6'}`}>
      {/* The page. A tap or a swipe on it turns it; a page larger than the screen (zoomed) moves inside its frame. */}
      <div
        data-pdf-page-area
        onPointerDown={onPointerDown}
        onPointerUp={onPointerUp}
        onPointerCancel={() => { gesture.current = null; }}
        className={`max-w-full overflow-auto bg-white ${immersive ? '' : 'rounded-sm border border-border-hairline shadow-lg'}`}
        style={{ touchAction: zoom > 1 ? 'pan-x pan-y' : 'pan-y' }}
      >
        <Document
          file={authenticatedUrl(fileUrl)}
          onLoadSuccess={onDocumentLoadSuccess}
          onPassword={(callback, reason) => setAsking({ callback, wrong: reason === INCORRECT_PASSWORD })}
          loading={<Skeleton label="Carregando o PDF" className="h-[70vh] w-[min(100vw-1.5rem,56rem)] rounded-none" />}
          error={<p role="alert" className="p-10 text-sm font-medium text-danger">Não foi possível ler este PDF.</p>}
        >
          <Page pageNumber={pageNumber} renderTextLayer={true} renderAnnotationLayer={false} width={pageWidth} />
        </Document>
      </div>

      {asking && (
        <PdfPassword
          wrong={asking.wrong}
          onSubmit={(password) => {
            const { callback } = asking;
            setAsking(null);
            callback(password);
          }}
          onCancel={() => {
            setAsking(null);
            onCancelPassword?.();
          }}
        />
      )}

      {/* The controls, at the bottom. Hidden, they leave a small mark of the page so that the place is not lost. */}
      <div className="sticky bottom-[calc(env(safe-area-inset-bottom)+0.75rem)] z-30 mt-4 flex w-full justify-center">
        <div
          data-chrome={immersive ? 'hidden' : 'shown'}
          inert={immersive}
          className={`relative flex items-center gap-0.5 rounded-full border border-border-hairline bg-white/95 px-1.5 py-1 shadow-lg backdrop-blur transition-[opacity,transform] duration-200 ease-out ${
            immersive ? 'pointer-events-none translate-y-3 opacity-0' : 'opacity-100'
          }`}
        >
          {showOutline && outline.length > 0 && (
            <nav
              ref={outlineRef}
              aria-label="Sumário do PDF"
              className="absolute bottom-full left-1/2 mb-2 max-h-[55vh] w-[min(26rem,calc(100vw-1.5rem))] -translate-x-1/2 animate-rise-in overflow-y-auto rounded-xl border border-border-hairline bg-white p-1.5 shadow-xl"
            >
              {outline.map((entry, i) => (
                <button
                  key={`${i}-${entry.page}`}
                  onClick={() => {
                    goTo(entry.page);
                    setShowOutline(false);
                  }}
                  style={{ paddingLeft: `${0.75 + entry.depth * 1}rem` }}
                  className="flex min-h-11 w-full items-baseline justify-between gap-3 rounded-lg py-2.5 pr-3 text-left text-sm text-ink-soft transition-colors hover:bg-surface-alt hover:text-ink"
                  title={entry.title}
                >
                  <span className="truncate">{entry.title}</span>
                  <span className="shrink-0 font-mono text-[11px] text-ink-faint">{entry.page}</span>
                </button>
              ))}
            </nav>
          )}

          <button onClick={prevPage} disabled={pageNumber <= 1} aria-label="Página anterior" className={control}>
            <Chevron dir="left" />
          </button>
          <div className="flex shrink-0 items-center gap-1 whitespace-nowrap px-1 font-mono text-sm text-ink">
            <input
              type="text"
              inputMode="numeric"
              pattern="[0-9]*"
              value={draft ?? String(pageNumber)}
              onFocus={(event) => event.target.select()}
              onChange={(event) => setDraft(event.target.value.replace(/\D/g, '').slice(0, 6))}
              onBlur={commitDraft}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault();
                  commitDraft();
                  event.target.blur();
                }
                if (event.key === 'Escape') {
                  setDraft(null);
                  event.target.blur();
                }
              }}
              aria-label="Ir para a página"
              style={{ width: `${Math.max(2, String(draft ?? pageNumber).length) + 1}ch` }}
              className="rounded-md bg-surface-alt px-1 py-1.5 text-center text-sm text-ink outline-none focus:ring-2 focus:ring-brand/30"
            />
            <span className="text-ink-soft">/ {numPages || '–'}</span>
          </div>
          <button onClick={nextPage} disabled={numPages ? pageNumber >= numPages : true} aria-label="Próxima página" className={control}>
            <Chevron dir="right" />
          </button>

          <span className="mx-1 h-6 w-px bg-border-hairline max-sm:hidden" aria-hidden="true" />
          <button onClick={() => zoomBy(-1)} disabled={zoom <= ZOOMS[0]} aria-label="Diminuir o zoom" className={`${control} max-sm:hidden`}>
            <svg {...iconProps}><path d="M5 12h14" /></svg>
          </button>
          <button
            onClick={() => setZoom(1)}
            disabled={zoom === 1}
            aria-label={`Zoom ${Math.round(zoom * 100)}%, voltar a ajustar à largura`}
            className="hidden h-11 min-w-12 items-center justify-center rounded-full px-1 font-mono text-[11px] text-ink-soft transition-colors hover:bg-surface-alt disabled:hover:bg-transparent sm:flex"
          >
            {Math.round(zoom * 100)}%
          </button>
          <button onClick={() => zoomBy(1)} disabled={zoom >= ZOOMS[ZOOMS.length - 1]} aria-label="Aumentar o zoom" className={`${control} max-sm:hidden`}>
            <svg {...iconProps}><path d="M12 5v14M5 12h14" /></svg>
          </button>
          {/* On a phone there is room for one: it goes through the steps and back to the width of the screen. */}
          <button
            onClick={() => setZoom(ZOOMS[(ZOOMS.indexOf(zoom) + 1) % ZOOMS.length])}
            aria-label={`Zoom ${Math.round(zoom * 100)}%, mudar`}
            className="flex h-11 min-w-12 items-center justify-center rounded-full px-1 font-mono text-[11px] text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95 sm:hidden"
          >
            {Math.round(zoom * 100)}%
          </button>

          {outline.length > 0 && (
            <>
              <span className="mx-1 h-6 w-px bg-border-hairline max-sm:hidden" aria-hidden="true" />
              <button
                ref={outlineOpenerRef}
                onClick={() => setShowOutline((v) => !v)}
                aria-expanded={showOutline}
                aria-label="Sumário do PDF"
                title="Sumário do PDF"
                className={`${control} ${showOutline ? 'bg-brand/10 text-brand' : ''}`}
              >
                <svg {...iconProps}><path d="M4 6h16M4 12h10M4 18h16" /></svg>
              </button>
            </>
          )}
          <span className="mx-1 h-6 w-px bg-border-hairline max-sm:hidden" aria-hidden="true" />
          <button onClick={() => onImmersiveChange?.(true)} aria-label="Esconder os controles" title="Esconder os controles" className={control}>
            <svg {...iconProps}><path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" /></svg>
          </button>
        </div>

        {/* With the controls hidden, a quiet mark of where the person is. A tap in the middle of the page brings them back. */}
        <div
          data-chrome-mark
          aria-hidden={!immersive}
          className={`pointer-events-none absolute bottom-0 rounded-full bg-ink/70 px-3 py-1 font-mono text-[11px] text-white transition-opacity duration-200 ${immersive ? 'opacity-100' : 'opacity-0'}`}
        >
          {pageNumber} / {numPages || '–'}
        </div>
      </div>
    </div>
  );
}
