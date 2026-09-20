import { describe, it, expect, beforeEach } from 'vitest';
import { ACTIVE_WINDOW_MS, isActive, markActivity, resetActivity, watchActivity } from './activity';

describe('activity', () => {
  beforeEach(resetActivity);

  it('is idle until something happens, active for 90 seconds after, then idle again', () => {
    expect(isActive(1000)).toBe(false);
    markActivity(1000);
    expect(isActive(1000 + ACTIVE_WINDOW_MS)).toBe(true);
    expect(isActive(1000 + ACTIVE_WINDOW_MS + 1)).toBe(false);
  });

  it('counts opening the reader and every kind of interaction, and stops listening when closed', () => {
    const target = new EventTarget();
    const stop = watchActivity(target);
    expect(isActive()).toBe(true); // opening the reader is an interaction

    for (const name of ['pointerdown', 'keydown', 'wheel', 'touchstart', 'scroll']) {
      resetActivity();
      target.dispatchEvent(new Event(name));
      expect(isActive(), name).toBe(true);
    }

    stop();
    resetActivity();
    target.dispatchEvent(new Event('keydown'));
    expect(isActive()).toBe(false);
  });
});
