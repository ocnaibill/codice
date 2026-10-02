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
