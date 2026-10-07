import { describe, expect, it } from 'vitest';
import { IMMERSIVE_MAX_READING_WIDTH, MAX_READING_WIDTH, SIDE_ZONE, SWIPE_MIN, SWIPE_RATIO, TAP_MAX_MOVE, TAP_MAX_MS, ZOOMS, readingWidth, swipeAction, tapAction } from './pdfGestures';

describe('how wide a PDF page is drawn', () => {
  it('takes what the screen gives, less a gutter at each side', () => {
    expect(readingWidth(360)).toBe(328);
    expect(readingWidth(800)).toBe(768);
  });
  it('stops at a comfortable measure on a wide screen', () => {
    expect(readingWidth(1920)).toBe(MAX_READING_WIDTH);
    expect(readingWidth(MAX_READING_WIDTH + 32)).toBe(MAX_READING_WIDTH);
    expect(readingWidth(MAX_READING_WIDTH + 31)).toBe(MAX_READING_WIDTH - 1);
  });
  it('never gets narrower than a page can be read', () => {
    expect(readingWidth(100)).toBe(240);
    expect(readingWidth(0)).toBe(240);
  });
  it('takes nearly all of the screen when only the page is on it (#181), and goes further on a big one', () => {
    expect(readingWidth(360, true)).toBe(356);
    expect(readingWidth(375, true)).toBe(371);
    expect(readingWidth(1200, true)).toBe(1196);
    expect(readingWidth(1920, true)).toBe(IMMERSIVE_MAX_READING_WIDTH);
    expect(readingWidth(IMMERSIVE_MAX_READING_WIDTH + 4, true)).toBe(IMMERSIVE_MAX_READING_WIDTH);
    expect(readingWidth(IMMERSIVE_MAX_READING_WIDTH + 3, true)).toBe(IMMERSIVE_MAX_READING_WIDTH - 1);
    expect(IMMERSIVE_MAX_READING_WIDTH).toBeGreaterThan(MAX_READING_WIDTH);
  });
  it('is wider with only the page, on every size of screen', () => {
    for (const available of [320, 375, 768, 1024, 1440, 1920]) {
      expect(readingWidth(available, true)).toBeGreaterThan(readingWidth(available, false));
    }
  });
  it('never gets narrower than a page can be read, hidden controls or not', () => {
    expect(readingWidth(100, true)).toBe(240);
  });
  it('has steps of zoom that start at the width of the screen and grow', () => {
    expect(ZOOMS[0]).toBe(1);
    expect([...ZOOMS].sort((a, b) => a - b)).toEqual(ZOOMS);
    expect(ZOOMS.length).toBeGreaterThan(3);
  });
});

describe('what a tap does', () => {
  const tap = (x, over = {}) => tapAction({ dx: 0, dy: 0, ms: 100, x, zoom: 1, ...over });
  it('turns back at the left, forward at the right, and shows or hides the controls in the middle', () => {
    expect(tap(0.05)).toBe('prev');
    expect(tap(SIDE_ZONE - 0.01)).toBe('prev');
    expect(tap(SIDE_ZONE)).toBe('toggle');
    expect(tap(0.5)).toBe('toggle');
    expect(tap(1 - SIDE_ZONE)).toBe('toggle');
    expect(tap(1 - SIDE_ZONE + 0.01)).toBe('next');
    expect(tap(0.99)).toBe('next');
  });
  it('is not a tap when the finger moved or stayed', () => {
    expect(tap(0.1, { dx: TAP_MAX_MOVE })).toBe('prev');
    expect(tap(0.1, { dx: TAP_MAX_MOVE + 1 })).toBeNull();
    expect(tap(0.1, { dx: -(TAP_MAX_MOVE + 1) })).toBeNull();
    expect(tap(0.1, { dy: TAP_MAX_MOVE + 1 })).toBeNull();
    expect(tap(0.1, { dy: -(TAP_MAX_MOVE + 1) })).toBeNull();
    expect(tap(0.1, { ms: TAP_MAX_MS })).toBe('prev');
    expect(tap(0.1, { ms: TAP_MAX_MS + 1 })).toBeNull();
  });
  it('wants the finger to stay still up and down as much as across, to the very pixel', () => {
    expect(tap(0.1, { dy: TAP_MAX_MOVE })).toBe('prev');
    expect(tap(0.1, { dy: -TAP_MAX_MOVE })).toBe('prev');
    expect(tap(0.1, { dx: -TAP_MAX_MOVE })).toBe('prev');
  });
  it('has the sides at three tenths of the page, as a thumb knows them', () => {
    expect(SIDE_ZONE).toBe(0.3);
    expect(tap(0.29)).toBe('prev');
    expect(tap(0.31)).toBe('toggle');
    expect(tap(0.69)).toBe('toggle');
    expect(tap(0.71)).toBe('next');
  });
  it('only shows or hides the controls when the page is zoomed: the sides are for moving it', () => {
    expect(tap(0.05, { zoom: 1.25 })).toBe('toggle');
    expect(tap(0.95, { zoom: 2 })).toBe('toggle');
    expect(tap(0.05, { zoom: 1.25, dx: 30 })).toBeNull();
  });
});

describe('what a swipe does', () => {
  const swipe = (dx, dy = 0, zoom = 1) => swipeAction({ dx, dy, zoom });
  it('brings the next page to the left and the one before to the right', () => {
    expect(swipe(-SWIPE_MIN)).toBe('next');
    expect(swipe(-200)).toBe('next');
    expect(swipe(SWIPE_MIN)).toBe('prev');
    expect(swipe(200)).toBe('prev');
  });
  it('wants the finger to go far enough', () => {
    expect(swipe(-(SWIPE_MIN - 1))).toBeNull();
    expect(swipe(SWIPE_MIN - 1)).toBeNull();
    expect(swipe(0)).toBeNull();
  });
  it('is not a swipe when it goes more along than across: that is reading', () => {
    expect(swipe(-100, 50)).toBe('next'); // 100 >= 50 * 1.5
    expect(swipe(-100, 100 / SWIPE_RATIO)).toBe('next');
    expect(swipe(-100, 100 / SWIPE_RATIO + 1)).toBeNull();
    expect(swipe(100, -100)).toBeNull();
    expect(swipe(-70, 200)).toBeNull();
  });
  it('is nothing when the page is zoomed: the finger is moving the page', () => {
    expect(swipe(-200, 0, 1.25)).toBeNull();
    expect(swipe(200, 0, 3)).toBeNull();
  });
});
