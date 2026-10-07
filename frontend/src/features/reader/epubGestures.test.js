import { describe, it, expect } from 'vitest';
import { QUIET_AFTER_TURN_MS, TOUCH_TAP_MAX_MS, acrossPage, liftMeaning, toScreen } from './epubGestures';
import { TAP_MAX_MS, tapAction } from './pdfGestures';

describe('the places of a touch on the book', () => {
  it('brings a point of the iframe to the screen, and says how far across the page a point is', () => {
    expect(toScreen({ frameLeft: -800, frameTop: 20, clientX: 840, clientY: 300 })).toEqual({ x: 40, y: 320 });
    expect(acrossPage({ x: 130, left: 100, width: 400 })).toBe(0.075);
    expect(acrossPage({ x: 5, left: 0, width: 0 })).toBe(5); // a page with no width does not divide by nothing
  });
});

describe('what a lifted finger or mouse means for the page (#180)', () => {
  const lift = (over) => liftMeaning({ pointerType: 'touch', selectedAtDown: false, selectedNow: false, onLink: false, ...over });

  it('a link is touched, not turned', () => {
    expect(lift({ onLink: true })).toBe('ignore');
    expect(lift({ onLink: true, pointerType: 'mouse' })).toBe('ignore');
  });

  it('a finger that comes down on a selection only lets it go', () => {
    expect(lift({ selectedAtDown: true, selectedNow: true })).toBe('dismiss');
    expect(lift({ selectedAtDown: true, selectedNow: false })).toBe('dismiss');
  });

  it('a finger that taps is still a tap when the phone selected the word under it', () => {
    expect(lift({ selectedAtDown: false, selectedNow: true })).toBe('gesture');
    expect(lift({})).toBe('gesture');
  });

  it('a mouse that selects (a double click, a drag) is not a tap, and a plain click is', () => {
    expect(lift({ pointerType: 'mouse', selectedNow: true })).toBe('ignore');
    expect(lift({ pointerType: 'mouse', selectedNow: false })).toBe('gesture');
    expect(lift({ pointerType: 'mouse', selectedAtDown: true, selectedNow: false })).toBe('gesture');
  });

  it('a pen is a finger', () => {
    expect(lift({ pointerType: 'pen', selectedAtDown: true })).toBe('dismiss');
  });
});

describe('how long a tap lasts under a finger', () => {
  it('is shorter than what a phone takes as the start of a selection (about half a second)', () => {
    expect(TOUCH_TAP_MAX_MS).toBe(300);
    expect(TOUCH_TAP_MAX_MS).toBeLessThan(TAP_MAX_MS);
  });

  it('turns by tap at 300 ms and not at 301, with the limit of a finger; the usual limit stays for the rest', () => {
    const tap = (ms, maxMs) => tapAction({ dx: 0, dy: 0, ms, x: 0.9, zoom: 1, maxMs });
    expect(tap(300, TOUCH_TAP_MAX_MS)).toBe('next');
    expect(tap(301, TOUCH_TAP_MAX_MS)).toBeNull();
    expect(tap(500, undefined)).toBe('next');
    expect(tap(501, undefined)).toBeNull();
  });

  it('keeps the late selection of a turn out for a short while, long enough for the phone and no longer', () => {
    expect(QUIET_AFTER_TURN_MS).toBe(400);
    expect(QUIET_AFTER_TURN_MS).toBeGreaterThan(500 - TOUCH_TAP_MAX_MS); // the press that selects comes ~500 ms after the finger went down
  });
});
