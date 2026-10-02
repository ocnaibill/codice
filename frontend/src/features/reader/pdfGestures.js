// What a finger or a window does to a PDF page (#77, #107): how wide the page is, how far it can be zoomed, and what a
// touch means. Kept apart from the viewer so that the rules can be tested without a screen.

/** The steps of the zoom, 1 being "fit the width". */
export const ZOOMS = [1, 1.25, 1.5, 2, 2.5, 3];

/** The widest a page is drawn at zoom 1: beyond this a line of text is too long to read, and the screen has room to spare. */
export const MAX_READING_WIDTH = 900;
const MIN_READING_WIDTH = 240;
const SIDE_GUTTER = 16;

/** The width of the page at zoom 1, given what the screen gives: all of it, up to a comfortable measure. */
export function readingWidth(available) {
  return Math.max(MIN_READING_WIDTH, Math.min(MAX_READING_WIDTH, Math.round(available - SIDE_GUTTER * 2)));
}

/** A tap is a touch that hardly moved and did not stay. */
export const TAP_MAX_MOVE = 10;
export const TAP_MAX_MS = 500;
/** The share of the page, at each side, where a tap turns it. */
export const SIDE_ZONE = 0.3;
/** A swipe: far enough, and more across than along. */
export const SWIPE_MIN = 60;
export const SWIPE_RATIO = 1.5;

/**
 * What a tap does, at `x` (0 at the left edge of the page, 1 at the right): 'prev' at the left, 'next' at the right,
 * 'toggle' (show or hide the controls) in the middle, and everywhere when the page is zoomed, where the sides are
 * for moving the page, not for turning it. null when it was not a tap.
 */
export function tapAction({ dx, dy, ms, x, zoom }) {
  if (Math.abs(dx) > TAP_MAX_MOVE || Math.abs(dy) > TAP_MAX_MOVE || ms > TAP_MAX_MS) return null;
  if (zoom > 1) return 'toggle';
  if (x < SIDE_ZONE) return 'prev';
  if (x > 1 - SIDE_ZONE) return 'next';
  return 'toggle';
}

/** A swipe to the left brings the next page, to the right the one before; none when the page is zoomed (it is being moved). */
export function swipeAction({ dx, dy, zoom }) {
  if (zoom > 1 || Math.abs(dx) < SWIPE_MIN || Math.abs(dx) < Math.abs(dy) * SWIPE_RATIO) return null;
  return dx < 0 ? 'next' : 'prev';
}
