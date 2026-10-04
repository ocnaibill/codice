import React, { useState, useEffect, useLayoutEffect, useRef } from 'react';
import { Skeleton } from '../../../../components/ui/Skeleton';
import { tapAction } from '../../pdfGestures';
import { getEpubSettings, saveEpubSettings } from '../../preferences';
import { pushReadingSettings } from '../../readingSync';
import { readingStyle } from '../../readingStyle';
import { sanitizeSettings } from '../../epubThemes';
import { scrollFraction, scrollerOf, useScrollPosition } from '../../scrollPosition';
import { useSelectionWatcher } from '../../useSelectionWatcher';
import ReadingSettingsPanel from './ReadingSettingsPanel';

const iconProps = { viewBox: '0 0 24 24', width: 18, height: 18, fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true };

const control =
  'flex h-11 min-w-11 items-center justify-center rounded-full text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95 disabled:opacity-30 disabled:hover:bg-transparent disabled:active:scale-100';

/**
 * The page of a text that is read by scrolling (plain text, Markdown): the color of the page, the font, the size and the
 * space between lines the person chose (the same as the EPUB reader's, kept for the account on this device), and the
 * controls at the bottom, where the thumb is: a screen up and down, how far through the text, the look of the text and a
 * button that hides everything but the text. A tap on the text (a finger, not a mouse) hides or shows the controls.
 * `children` draws the text, given the style it is drawn in.
 */
export default function ReadingSurface({
  content, loading, error, onRetry, loadingLabel, errorTitle, emptyText,
  immersive = false, onImmersiveChange, onProgress, initialProgress, onSelection, children,
}) {
  const [settings, setSettings] = useState(() => getEpubSettings());
  const [panel, setPanel] = useState(false);
  const [percent, setPercent] = useState(0);
  const rootRef = useRef(null);
  const textRef = useRef(null);
  const gesture = useRef(null);
  const fractionRef = useRef(0); // how far through the text the person is, to stay there when the text is laid out again
  const ready = !loading && !error;
  useScrollPosition({ ref: textRef, ready, length: content.length, onProgress, initialProgress });
  // What is selected in the text is told to whoever offers what to do with it (the menu by the selection).
  // It is tied to where it is in the text: the offset of the file, which for a plain text is exact (the page is the
  // file), and for Markdown is the same fraction of the text (what is shown is not what is written).
  useSelectionWatcher(
    textRef,
    (found) => {
      if (!onSelection) return;
      if (!found) return onSelection(null);
      const { before, total, ...rest } = found;
      const offset = total > 0 ? Math.round((before / total) * content.length) : 0;
      onSelection({ ...rest, locator: { type: 'text', offset } });
    },
    ready && !!onSelection
  );

  const style = readingStyle(settings);
  const change = (patch) => {
    const next = sanitizeSettings({ ...settings, ...patch });
    saveEpubSettings(next);
    pushReadingSettings(next);
    setSettings(next);
  };

  // How far through the text, as it is scrolled.
  useEffect(() => {
    if (!ready || !rootRef.current) return undefined;
    const scroller = scrollerOf(rootRef.current);
    const target = scroller === document.scrollingElement || scroller === document.documentElement ? window : scroller;
    const measure = () => {
      fractionRef.current = scrollFraction(scroller.scrollTop, scroller.scrollHeight, scroller.clientHeight);
      setPercent(Math.round(fractionRef.current * 100));
    };
    measure();
    target.addEventListener('scroll', measure, { passive: true });
    return () => target.removeEventListener('scroll', measure);
  }, [ready, content.length]);

  // A bigger letter, another font or another spacing makes the text longer or shorter: it stays at the same
  // fraction of it, and not at the same pixels, which would be elsewhere in the text.
  const layout = `${settings.size}|${settings.font}|${settings.spacing}`;
  const layoutRef = useRef(layout);
  useLayoutEffect(() => {
    if (layoutRef.current === layout) return;
    layoutRef.current = layout;
    if (!rootRef.current) return;
    const scroller = scrollerOf(rootRef.current);
    const range = Math.max(0, scroller.scrollHeight - scroller.clientHeight);
    scroller.scrollTop = Math.round(fractionRef.current * range);
  }, [layout]);

  const scrollScreen = (direction) => {
    const scroller = scrollerOf(rootRef.current);
    scroller.scrollBy?.({ top: direction * scroller.clientHeight * 0.9, behavior: 'smooth' });
  };

  // The panel closes by Escape, then the controls come back.
  useEffect(() => {
    const onKey = (e) => {
      if (e.key !== 'Escape' || e.target?.closest?.('input, textarea, select, [contenteditable="true"]')) return;
      if (panel) setPanel(false);
      else if (immersive) onImmersiveChange?.(false);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
  // The panel closes by a press outside the page; hiding the controls takes it with them.
  useEffect(() => {
    if (!panel) return undefined;
    const onPointer = (event) => {
      if (!rootRef.current?.contains(event.target)) setPanel(false);
    };
    document.addEventListener('pointerdown', onPointer);
    return () => document.removeEventListener('pointerdown', onPointer);
  }, [panel]);
  useEffect(() => {
    if (immersive) setPanel(false);
  }, [immersive]);

  // A tap with a finger on the text shows or hides the controls (or closes the panel, if it is open). Whoever is
  // selecting text, or touches a link, is not asking for that, and a finger that moves is scrolling.
  const onPointerDown = (event) => {
    if (event.pointerType === 'mouse' || event.button > 0) return;
    gesture.current = { x: event.clientX, y: event.clientY, at: Date.now() };
  };
  const onPointerUp = (event) => {
    const start = gesture.current;
    gesture.current = null;
    if (!start) return;
    if (window.getSelection?.()?.toString()) return;
    if (event.target?.closest?.('a, button, input, [data-chrome]')) return;
    const tap = tapAction({ dx: event.clientX - start.x, dy: event.clientY - start.y, ms: Date.now() - start.at, x: 0.5, zoom: 1 });
    if (!tap) return;
    if (panel) setPanel(false);
    else onImmersiveChange?.(!immersive);
  };

  let body;
  if (loading) {
    body = (
      <div className="flex justify-center px-4 pt-8">
        <Skeleton label={loadingLabel} className="h-[60vh] w-[min(100%,40rem)]" />
      </div>
    );
  } else if (error) {
    body = (
      <div role="alert" className="flex flex-col items-center gap-3 px-6 pt-24 text-center">
        <p className="text-sm font-medium text-danger">{errorTitle}</p>
        <p className="max-w-md text-xs text-ink-soft">{error}</p>
        <button
          onClick={onRetry}
          className="min-h-11 rounded-full border border-border-hairline bg-white px-5 text-sm text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95"
        >
          Tentar de novo
        </button>
      </div>
    );
  } else if (content.trim() === '') {
    body = <p className="px-6 pt-24 text-center text-sm" style={{ color: style.theme.text }}>{emptyText}</p>;
  } else {
    body = (
      <div
        ref={textRef}
        data-reading-text
        className="mx-auto w-full max-w-3xl px-5 pb-28 pt-8 sm:px-8"
        style={{ ...style.text, ...style.margins, '--reading-link': style.theme.link }}
      >
        {children}
      </div>
    );
  }

  return (
    <div
      ref={rootRef}
      onPointerDown={onPointerDown}
      onPointerUp={onPointerUp}
      onPointerCancel={() => { gesture.current = null; }}
      style={{ ...style.page, touchAction: 'pan-y pinch-zoom' }}
      className="relative min-h-full transition-colors duration-300"
    >
      {body}

      {/* The controls, at the bottom. They float over the text; hidden, they leave a small mark of how far the person is. */}
      <div className="pointer-events-none sticky bottom-[calc(env(safe-area-inset-bottom)+0.75rem)] z-30 h-0 w-full">
        <div className="absolute inset-x-0 bottom-0 flex justify-center">
          <div
            data-chrome={immersive ? 'hidden' : 'shown'}
            inert={immersive}
            className={`relative flex items-center gap-0.5 rounded-full border border-border-hairline bg-white/95 px-1.5 py-1 shadow-lg backdrop-blur transition-[opacity,transform] duration-200 ease-out ${
              immersive ? 'pointer-events-none translate-y-3 opacity-0' : 'pointer-events-auto opacity-100'
            }`}
          >
            {panel && (
              <div className="absolute bottom-full left-1/2 mb-2 max-h-[calc(100dvh-9rem)] w-[min(24rem,calc(100vw-1.5rem))] -translate-x-1/2 animate-rise-in overflow-y-auto rounded-xl border border-border-hairline bg-white p-4 shadow-xl">
                <ReadingSettingsPanel settings={settings} onChange={change} onClose={() => setPanel(false)} bookLabel="Padrão" />
              </div>
            )}
            <button onClick={() => scrollScreen(-1)} aria-label="Rolar para cima" className={control}>
              <svg {...iconProps}><path d="M5 15l7-7 7 7" /></svg>
            </button>
            <div title="Quanto do texto já foi lido" className="shrink-0 whitespace-nowrap px-1 text-center font-mono text-sm text-ink" style={{ minWidth: '3.25ch' }}>
              {ready ? `${percent}%` : '–'}
            </div>
            <button onClick={() => scrollScreen(1)} aria-label="Rolar para baixo" className={control}>
              <svg {...iconProps}><path d="M5 9l7 7 7-7" /></svg>
            </button>
            <span className="mx-1 h-6 w-px bg-border-hairline max-sm:hidden" aria-hidden="true" />
            <button
              onClick={() => setPanel((v) => !v)}
              aria-expanded={panel}
              aria-label="Aparência do texto"
              title="Aparência do texto"
              className={`${control} ${panel ? 'bg-brand/10 text-brand' : ''}`}
            >
              <span aria-hidden="true" className="font-display text-lg leading-none">Aa</span>
            </button>
            <button onClick={() => onImmersiveChange?.(true)} aria-label="Esconder os controles" title="Esconder os controles" className={control}>
              <svg {...iconProps}><path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" /></svg>
            </button>
          </div>
          <div
            data-chrome-mark
            aria-hidden={!immersive}
            style={{ backgroundColor: style.theme.text, color: style.theme.background }}
            className={`pointer-events-none absolute bottom-0 rounded-full px-3 py-1 font-mono text-[11px] opacity-0 transition-opacity duration-200 ${immersive ? '!opacity-60' : ''}`}
          >
            {percent}%
          </div>
        </div>
      </div>
    </div>
  );
}
