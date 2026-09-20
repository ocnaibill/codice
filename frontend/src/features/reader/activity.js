// Reading time is active time (DEC-076): it counts only while the person is doing something
// with the book: turning a page, scrolling, typing, touching, or listening to audio that is
// playing. A tab left open and untouched adds nothing.
export const ACTIVE_WINDOW_MS = 90_000;

let lastActivity = 0;

export function markActivity(now = Date.now()) {
  lastActivity = now;
}

export function isActive(now = Date.now(), windowMs = ACTIVE_WINDOW_MS) {
  return lastActivity > 0 && now - lastActivity <= windowMs;
}

export function resetActivity() {
  lastActivity = 0;
}

const EVENTS = ['pointerdown', 'keydown', 'wheel', 'touchstart', 'scroll'];

/** Starts listening for the person's interaction; opening the reader counts as one. Returns the stop function. */
export function watchActivity(target = window) {
  const onEvent = () => markActivity();
  for (const name of EVENTS) target.addEventListener(name, onEvent, { passive: true, capture: true });
  markActivity();
  return () => {
    for (const name of EVENTS) target.removeEventListener(name, onEvent, { capture: true });
  };
}
