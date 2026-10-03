// What the person selected in a text, and where to put the menu that offers what to do with it (#108).

/** The longest quotation the server keeps; a longer selection is cut there (and said, in the menu's title). */
export const MAX_QUOTE = 2000;

/** The text of a selection as a quotation: lines and runs of spaces become one space, and the ends are trimmed. */
export function cleanQuote(text) {
  return String(text ?? '').replace(/\s+/g, ' ').trim();
}

export const MENU_GAP = 8;
/** On a touch screen the system draws its own handles and magnifier at the selection: the menu keeps further from it. */
export const TOUCH_GAP = 28;
export const EDGE = 8;

/**
 * Where the menu goes, given where the selection is on the screen (`rect`), the screen and the size of the menu.
 * With a mouse it goes above the selection, with a finger below it (the system's own menu is above); where there is
 * no room on that side it goes to the other, and it never leaves the screen. Returns its top left corner.
 */
export function menuPlacement({ rect, viewport, size, touch = false }) {
  const gap = touch ? TOUCH_GAP : MENU_GAP;
  const centered = rect.left + rect.width / 2 - size.width / 2;
  const left = Math.max(EDGE, Math.min(centered, viewport.width - size.width - EDGE));
  const above = rect.top - size.height - gap;
  const below = rect.bottom + gap;
  const fitsAbove = above >= EDGE;
  const fitsBelow = below + size.height <= viewport.height - EDGE;
  let top;
  if (touch) top = fitsBelow ? below : fitsAbove ? above : below;
  else top = fitsAbove ? above : fitsBelow ? below : above;
  return { left, top: Math.max(EDGE, Math.min(top, viewport.height - size.height - EDGE)) };
}

/** The selection of a document when it is inside `root` and is not empty: { text, rect }, or null. */
export function selectionIn(root, doc = document) {
  const selection = doc.getSelection?.();
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) return null;
  const range = selection.getRangeAt(0);
  if (root && !root.contains(range.commonAncestorContainer)) return null;
  const text = cleanQuote(selection.toString());
  if (!text) return null;
  const box = range.getBoundingClientRect();
  // How many characters of the root come before the selection, and how many it has: where in the text it is.
  let before = 0;
  if (root) {
    const head = doc.createRange();
    head.selectNodeContents(root);
    head.setEnd(range.startContainer, range.startOffset);
    before = head.toString().length;
  }
  return {
    text,
    before,
    total: root ? root.textContent.length : 0,
    rect: { left: box.left, top: box.top, width: box.width, height: box.height, bottom: box.bottom, right: box.right },
  };
}
