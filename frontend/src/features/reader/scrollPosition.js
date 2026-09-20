import { useEffect } from 'react';
import { completionFor } from './progressRules';

// A text has no pages, so its place is a character offset. What is measured is the scroll, so the
// offset is the fraction scrolled times the length: close enough to come back to the same
// passage whatever the window size or font, which is what a position is for.

export function scrollFraction(scrollTop, scrollHeight, clientHeight) {
  const range = scrollHeight - clientHeight;
  if (!(range > 0)) return 1; // all of it is on screen: the person has seen the end
  return Math.min(1, Math.max(0, scrollTop / range));
}

export function offsetFromFraction(fraction, length) {
  return Math.round(Math.min(1, Math.max(0, fraction)) * length);
}

export function scrollTopFromOffset(offset, length, scrollHeight, clientHeight) {
  if (!(length > 0)) return 0;
  const range = Math.max(0, scrollHeight - clientHeight);
  return Math.round(Math.min(1, Math.max(0, offset / length)) * range);
}

/** The element that actually scrolls the text: the nearest ancestor that overflows, else the page. */
export function scrollerOf(element) {
  for (let el = element?.parentElement; el; el = el.parentElement) {
    const overflow = getComputedStyle(el).overflowY;
    if ((overflow === 'auto' || overflow === 'scroll') && el.scrollHeight > el.clientHeight + 1) return el;
  }
  return document.scrollingElement || document.documentElement;
}

/**
 * Saves the reading position of a text as it is scrolled (debounced), and returns to the saved
 * one when it is shown. `ref` is the element holding the text, `length` its number of
 * characters, `initialProgress` the saved offset as text (or undefined).
 */
export function useScrollPosition({ ref, ready, length, onProgress, initialProgress, debounceMs = 1000 }) {
  useEffect(() => {
    const element = ref.current;
    if (!ready || !element || !(length > 0)) return undefined;

    const scroller = scrollerOf(element);
    const at = () => scrollFraction(scroller.scrollTop, scroller.scrollHeight, scroller.clientHeight);

    const saved = Number.parseInt(initialProgress, 10);
    if (Number.isFinite(saved) && saved > 0) {
      scroller.scrollTop = scrollTopFromOffset(saved, length, scroller.scrollHeight, scroller.clientHeight);
    }

    // Putting the text back where it was makes the browser scroll, which is not the person
    // reading on: an offset that is where the last save (or the returned-to place) already is,
    // give or take a thousandth of the text, is not saved again.
    let lastOffset = Number.isFinite(saved) && saved > 0 ? saved : null;
    let timer = null;
    const report = () => {
      if (!onProgress) return;
      const fraction = at();
      const percent = Math.round(fraction * 1000) / 10;
      const offset = offsetFromFraction(fraction, length);
      if (lastOffset !== null && Math.abs(offset - lastOffset) <= length / 1000) return;
      lastOffset = offset;
      onProgress({ type: 'text', offset }, { percent, completed: completionFor(percent) })
        ?.catch?.((err) => console.error('Failed to save reading progress:', err));
    };
    const onScroll = () => {
      clearTimeout(timer);
      timer = setTimeout(report, debounceMs);
    };

    // A text short enough to be seen whole has nothing to scroll: it has been read.
    if (!(scroller.scrollHeight - scroller.clientHeight > 1) && !(saved > 0)) report();

    const target = scroller === document.scrollingElement || scroller === document.documentElement ? window : scroller;
    target.addEventListener('scroll', onScroll, { passive: true });
    return () => {
      clearTimeout(timer);
      target.removeEventListener('scroll', onScroll);
    };
  }, [ref, ready, length, onProgress, initialProgress, debounceMs]);
}
