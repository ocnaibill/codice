import { useEffect } from 'react';

/**
 * Keeps the screen of a phone on while `active` (#180): a person who reads for minutes without touching the screen should not
 * find it dark. It uses the Screen Wake Lock API (a secure address only; where it is missing or refused nothing happens, and
 * the phone does what it always did). The browser lets go of the lock when the page is hidden, so it is asked for again when
 * the page is shown, and it is let go when `active` ends or the component goes (the screen is the person's again at once).
 */
export function useWakeLock(active) {
  useEffect(() => {
    if (!active || typeof navigator === 'undefined' || !navigator.wakeLock?.request) return undefined;
    let lock = null;
    let gone = false;
    const acquire = async () => {
      try {
        const granted = await navigator.wakeLock.request('screen');
        if (gone) {
          granted.release().catch(() => {}); // it was not wanted any more by the time it came
          return;
        }
        lock = granted;
        granted.addEventListener?.('release', () => {
          if (lock === granted) lock = null;
        });
      } catch {
        // refused (low battery, a mode of power saving, a page that is not visible): nothing to do but read on
      }
    };
    const onVisible = () => {
      if (document.visibilityState === 'visible' && !lock) acquire();
    };
    acquire();
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      gone = true;
      document.removeEventListener('visibilitychange', onVisible);
      lock?.release().catch(() => {});
      lock = null;
    };
  }, [active]);
}
