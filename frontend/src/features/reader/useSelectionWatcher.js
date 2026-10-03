import { useEffect, useRef } from 'react';
import { selectionIn } from './selection';

/** How long the selection must rest before the menu is offered: a mouse is done at once, a finger drags handles. */
export const MOUSE_DELAY = 120;
export const TOUCH_DELAY = 350;

/**
 * Tells what is selected inside `rootRef` (a text of the app's own page: the PDF, a text, a Markdown), once the
 * selection has rested, as { text, rect, touch, clear }, and null when it is gone or the person scrolls. While the
 * selection is being changed (a handle dragged) it says null at once, and again when it rests.
 */
export function useSelectionWatcher(rootRef, onSelection, enabled = true) {
  const callback = useRef(onSelection);
  callback.current = onSelection;

  useEffect(() => {
    if (!enabled) return undefined;
    let timer = null;
    let touch = false;
    let shown = false;
    const say = (value) => {
      shown = value !== null;
      callback.current(value);
    };
    const read = () => {
      // Without a page to look at (not drawn yet, or gone) there is no selection that is its own.
      const found = rootRef.current ? selectionIn(rootRef.current) : null;
      if (found) say({ ...found, touch, clear: () => window.getSelection()?.removeAllRanges() });
      else if (shown) say(null);
    };
    const onChange = () => {
      clearTimeout(timer);
      if (shown) say(null);
      timer = setTimeout(read, touch ? TOUCH_DELAY : MOUSE_DELAY);
    };
    const onPointer = (event) => { touch = event.pointerType !== 'mouse'; };
    const onScroll = () => { if (shown) say(null); };
    document.addEventListener('selectionchange', onChange);
    document.addEventListener('pointerdown', onPointer, true);
    document.addEventListener('scroll', onScroll, true);
    return () => {
      clearTimeout(timer);
      document.removeEventListener('selectionchange', onChange);
      document.removeEventListener('pointerdown', onPointer, true);
      document.removeEventListener('scroll', onScroll, true);
      if (shown) callback.current(null);
    };
  }, [rootRef, enabled]);
}
