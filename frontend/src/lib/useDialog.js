import { useEffect, useRef } from 'react';
import { isTopmostDialog } from './topDialog';

const FOCUSABLE = [
  'a[href]', 'button:not([disabled])', 'input:not([disabled]):not([type="hidden"])', 'select:not([disabled])',
  'textarea:not([disabled])', '[tabindex]:not([tabindex="-1"])', 'summary',
].join(',');

// What the keyboard can reach inside an element: not disabled, not hidden, not inside something inert.
export function focusablesIn(element) {
  return [...element.querySelectorAll(FOCUSABLE)].filter((el) => {
    if (el.closest('[inert], [hidden]')) return false;
    const style = getComputedStyle(el);
    return style.display !== 'none' && style.visibility !== 'hidden';
  });
}

/**
 * What a dialog owes the keyboard and the screen reader (the part that a role and aria-modal do not do by themselves):
 *  - focus goes in when it opens (to the field or button that asked for it, else the first thing that can be reached, else
 *    the dialog itself), unless something inside already took it (autoFocus);
 *  - Tab and Shift+Tab stay inside the one on top, wherever the focus was (even on the page behind it, after a click on the
 *    backdrop), so the keyboard never ends up on a page that is covered;
 *  - Escape closes only the one on top, with the answer the dialog gives (`onEscape`);
 *  - when it closes, focus goes back to what opened it, unless the app already put it somewhere else on purpose.
 *
 * `ref` is the element with role="dialog". `initialFocus` is a ref to the element that should take the focus first. `active`
 * is for a component that is always mounted and only shows the dialog while something is open (default: true, the dialog
 * is there as long as the component is). `trap: false` is for a dialog that is not modal (a card beside the page, not over
 * it): the focus goes in and comes back, Escape closes it, but Tab is free to leave.
 */
export function useDialog(ref, { onEscape, initialFocus, active = true, trap = true } = {}) {
  // Read while rendering, in the render that opens it, before the commit: an autoFocus inside the dialog moves the focus
  // during the commit, and then the opener would be lost.
  const opener = useRef(null);
  const wasActive = useRef(false);
  if (active && !wasActive.current && typeof document !== 'undefined') opener.current = document.activeElement;
  const escape = useRef(onEscape);
  escape.current = onEscape;

  useEffect(() => {
    wasActive.current = active;
    const dialog = ref.current;
    if (!active || !dialog) return undefined;
    if (!dialog.hasAttribute('tabindex')) dialog.setAttribute('tabindex', '-1');
    if (!dialog.contains(document.activeElement)) {
      (initialFocus?.current ?? focusablesIn(dialog)[0] ?? dialog).focus();
    }

    const onKey = (event) => {
      if (!isTopmostDialog(dialog)) return;
      if (event.key === 'Escape') {
        if (escape.current && !event.defaultPrevented) escape.current(event);
        return;
      }
      if (event.key !== 'Tab' || !trap) return;
      const items = focusablesIn(dialog);
      if (items.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      if (!dialog.contains(active)) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
      } else if (event.shiftKey && (active === first || active === dialog)) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && active === last) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener('keydown', onKey);

    const from = opener.current;
    return () => {
      window.removeEventListener('keydown', onKey);
      const active = document.activeElement;
      const lost = !active || active === document.body || dialog.contains(active);
      if (lost && from?.isConnected && typeof from.focus === 'function') from.focus();
    };
    // The dialog is the same element for as long as it is open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active]);
}
