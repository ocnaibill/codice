// How the text looks follows the person from one device to another (#106), by kind of device (#180): what they choose is kept on the device at once (the
// reader needs it to start) and sent to the server a moment after the last change, so that a person who moves a slider
// through ten steps sends one request and not ten. The server's answer is not waited for: a failure leaves the choice on
// the device, and the next change tries again.
import { api } from '../../lib/api';
import { sanitizeSettings } from './epubThemes';
import { deviceClass } from './deviceClass';

export const PUSH_DELAY = 600;

let timer = null;
let pending = null;

/** Sends what is waiting to be sent, now. */
export function flushReadingSettings() {
  clearTimeout(timer);
  timer = null;
  if (!pending) return;
  const reader = pending;
  pending = null;
  try {
    // The choice is of this kind of device: the server keeps the other kind's as it is (or changes it too, if the person asked
    // for the same choice everywhere).
    Promise.resolve(api.put('/auth/preferences', { reader: { device: deviceClass(), settings: reader } })).catch(() => {});
  } catch {
    // not being able to send it is not a problem: it is on this device
  }
}

/** Keeps the choice for the server, to be sent when the person stops changing it. */
export function pushReadingSettings(settings, delay = PUSH_DELAY) {
  pending = sanitizeSettings(settings);
  clearTimeout(timer);
  if (delay <= 0) {
    flushReadingSettings();
    return;
  }
  timer = setTimeout(flushReadingSettings, delay);
}

// A choice that is waiting is not lost when the page is left or hidden.
if (typeof window !== 'undefined') {
  window.addEventListener('pagehide', flushReadingSettings);
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') flushReadingSettings();
  });
}
