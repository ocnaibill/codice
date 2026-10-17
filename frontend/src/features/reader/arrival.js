// "Your Kindle stopped right here": when a book is opened at a place another device left it (a KOReader that syncs with the Códice), the
// reader of the web says so, and underlines the paragraph for a moment. Said once for each save: opening the same book again, with nothing
// new from the device, is not told again.

const KEY = (fileId) => `codice:arrival:${fileId}`;
const PENDING_KEY = (fileId) => `codice:pending:${fileId}`;

/**
 * How long ago, in words, for a time of the server: "há 2 horas", "ontem", "há 5 dias", "há 3 semanas", "há 2 meses". Empty for the last
 * few minutes (nothing to say) and for a time that is not one.
 */
export function ago(iso, now = Date.now()) {
  const time = Date.parse(iso);
  if (!Number.isFinite(time)) return '';
  const minutes = Math.floor((now - time) / 60000);
  if (minutes < 10) return '';
  const unit = (n, one, many) => `há ${n} ${n === 1 ? one : many}`;
  if (minutes < 60) return unit(minutes, 'minuto', 'minutos');
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return unit(hours, 'hora', 'horas');
  const days = Math.floor(hours / 24);
  if (days === 1) return 'ontem';
  if (days < 14) return unit(days, 'dia', 'dias');
  if (days < 60) return unit(Math.floor(days / 7), 'semana', 'semanas');
  return unit(Math.floor(days / 30), 'mês', 'meses');
}

/** When the place kept apart was last told of (its time, as the server wrote it); '' when never. */
export function pendingTold(fileId) {
  try {
    return localStorage.getItem(PENDING_KEY(fileId)) || '';
  } catch {
    return '';
  }
}

export function markPendingTold(fileId, at) {
  try {
    localStorage.setItem(PENDING_KEY(fileId), String(at));
  } catch {
    // Without storage it is said again the next time: a small thing.
  }
}

/**
 * The place a device sent that was kept apart because the web had a newer one (DEC-191): where it is, how long ago, and the locator to go to.
 * Null when there is none, or when this one was already told of.
 */
export function pendingOf(progress, told = '', now = Date.now()) {
  const p = progress?.pending;
  if (!p || !p.locator || String(p.at) === told) return null;
  const device = String(p.device || '').trim() || 'aparelho';
  const percent = Number.isFinite(p.percent) ? ` (${Math.round(p.percent)}%)` : '';
  const when = ago(p.at, now);
  return { device, at: String(p.at), locator: p.locator, message: `Seu ${device} parou em outro ponto${percent}${when ? `, ${when}` : ''}` };
}

/** The revision of the save already told of for a file; 0 when none, or when the browser keeps nothing. */
export function lastTold(fileId) {
  try {
    return Number(localStorage.getItem(KEY(fileId))) || 0;
  } catch {
    return 0;
  }
}

export function markTold(fileId, revision) {
  try {
    localStorage.setItem(KEY(fileId), String(revision));
  } catch {
    // Without storage it is said again the next time: a small thing.
  }
}

/**
 * What to say about a place, or null when there is nothing to say: the last save was a device's (not this reader's), it is one that was not
 * told of yet, and it says where it is. `progress` is what the server answers (`source`, `device`, `revision`, `locator`).
 */
export function arrivalOf(progress, told = 0, now = Date.now()) {
  if (!progress || progress.source !== 'koreader' || !(progress.revision > told) || !progress.locator) return null;
  const device = String(progress.device || '').trim() || 'aparelho';
  const locator = progress.locator;
  let message = `Seu ${device} parou aqui`;
  let cfi = null;
  if (locator.type === 'epub' && locator.cfi) {
    message = `Seu ${device} parou bem aqui`;
    cfi = locator.cfi;
  } else if (locator.type === 'pdf' && Number.isInteger(locator.page)) {
    message = `Seu ${device} parou na página ${locator.page + 1}`;
  } else if (locator.type === 'image' && Number.isInteger(locator.index)) {
    message = `Seu ${device} parou na página ${locator.index + 1}`;
  }
  const when = ago(progress.updatedAt, now);
  return { device, message: when ? `${message}, ${when}` : message, cfi, revision: progress.revision };
}

export const ARRIVAL_CLASS = 'codice-arrival';
/** How long the paragraph stays underlined. */
export const ARRIVAL_MS = 3500;

/** The rules the page of the book is given: an outline as well as a color (a color alone says nothing to everyone), and no motion for who asked for none. */
export const ARRIVAL_RULES = {
  [`.${ARRIVAL_CLASS}`]: {
    outline: '2px solid #9c4918 !important',
    'outline-offset': '6px !important',
    'border-radius': '3px !important',
    'background-color': 'rgba(156, 73, 24, 0.10) !important',
    transition: 'outline-color 400ms ease, background-color 400ms ease !important',
  },
  [`.${ARRIVAL_CLASS}.codice-arrival-out`]: {
    'outline-color': 'transparent !important',
    'background-color': 'transparent !important',
  },
  '@media (prefers-reduced-motion: reduce)': {
    [`.${ARRIVAL_CLASS}`]: { transition: 'none !important' },
  },
};

/**
 * Underlines the paragraph of `cfi` in the page `contents` (an epub.js Contents) for a moment and lets it go. The CFI leads to an element,
 * or into the text of one: the paragraph is the element that holds it. Never throws: a place that cannot be found is not underlined, and
 * the book is read all the same. Returns a function that takes the mark away at once.
 */
export function paintArrival(contents, cfi, { delay = ARRIVAL_MS, timers = { set: setTimeout, clear: clearTimeout } } = {}) {
  try {
    const range = contents?.range?.(cfi);
    let node = range?.startContainer ?? null;
    if (node && node.nodeType !== 1) node = node.parentElement ?? null;
    // a CFI to the body or the page itself would underline all of it
    if (!node || /^(body|html)$/i.test(node.tagName || '')) return () => {};
    contents.addStylesheetRules?.(ARRIVAL_RULES);
    node.classList.add(ARRIVAL_CLASS);
    let fade;
    const clear = timers.set(() => {
      node.classList.add('codice-arrival-out');
      fade = timers.set(() => node.classList.remove(ARRIVAL_CLASS, 'codice-arrival-out'), 450);
    }, delay);
    return () => {
      timers.clear(clear);
      if (fade) timers.clear(fade);
      node.classList.remove(ARRIVAL_CLASS, 'codice-arrival-out');
    };
  } catch {
    return () => {};
  }
}
