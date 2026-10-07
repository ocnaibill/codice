// What kind of device this is, for the choices of how the text looks (#180): one made for the fingers (a phone, a tablet), or
// one made for a mouse. A phone and a computer are read in different places and at different distances, so each has its own
// choice. The kind is how the screen is touched, not how big it is: a small window on a computer is still a computer.

/** The two kinds the server keeps a choice for. */
export const DEVICES = ['touch', 'desktop'];

/** 'touch' for a screen whose main way of pointing is a finger and that cannot hover; 'desktop' for the rest (and when it cannot be told). */
export function deviceClass() {
  try {
    return window.matchMedia?.('(hover: none) and (pointer: coarse)').matches ? 'touch' : 'desktop';
  } catch {
    return 'desktop';
  }
}

/** The other kind. */
export const otherDevice = (device) => (device === 'touch' ? 'desktop' : 'touch');
