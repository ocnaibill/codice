// Where on the screen a touch on the book is. The book is drawn inside an iframe, whose events speak of its own corner:
// they are brought to the screen's, and then to the place on the page, before the rules of pdfGestures say what they mean.

/** The position on the screen of a point of an iframe, given where the iframe is on the screen (it moves as pages turn). */
export function toScreen({ frameLeft, frameTop, clientX, clientY }) {
  return { x: frameLeft + clientX, y: frameTop + clientY };
}

/** How far across the page a point of the screen is: 0 at its left edge, 1 at its right. */
export function acrossPage({ x, left, width }) {
  return (x - left) / (width || 1);
}

/**
 * How long a finger may stay on the page and still be a tap (#180). A phone takes a finger that stays for about half a
 * second as the start of a selection and selects the word under it, so a tap that is allowed to last that long turns the page
 * and selects a word at once: shorter than that, a tap is only a tap, and a press that stays is the person selecting.
 */
export const TOUCH_TAP_MAX_MS = 300;

/**
 * For this long after a tap or a swipe turned the page, a selection that appears is not the person's: it is the word that
 * was under the finger, which the phone selected a moment late (a press that stays about half a second, and a tap is shorter than 300 ms), and the menu of notes and the dictionary must not open on
 * it (they would be there for a page that has already gone).
 */
export const QUIET_AFTER_TURN_MS = 400;

/**
 * What a lifted finger or mouse means for the page, given what was selected when it went down and when it came up:
 *  - 'ignore': a link was touched, or a mouse that selects (a double click, a drag): not a page's tap;
 *  - 'dismiss': the finger came down on a page that had a selection: the tap only lets it go, it does not turn anything;
 *  - 'gesture': it may be a tap or a swipe. For a finger that includes a selection that appeared during the touch: the phone
 *    selected the word under a tap, and the tap is still a tap.
 */
export function liftMeaning({ pointerType, selectedAtDown, selectedNow, onLink }) {
  if (onLink) return 'ignore';
  if (pointerType === 'mouse') return selectedNow ? 'ignore' : 'gesture';
  return selectedAtDown ? 'dismiss' : 'gesture';
}

/**
 * The page of the book is in an iframe of its own, whose touch rules are its own: without this, a finger that drags
 * across it is taken by the browser to move the page (and the swipe that turns it is cancelled halfway, with nothing said). Up
 * and down is the browser's (the book scrolls, a text zooms by pinch); across is ours.
 */
export const TOUCH_CSS = 'html, body { touch-action: pan-y pinch-zoom; }';
