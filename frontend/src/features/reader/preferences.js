// The comic reader's mode and how the EPUB reader looks (page color, font, size, spacing) are remembered between
// books, on this device and for this account (DEC-078, RF-013: a preference must not affect other users, and two
// people can share a browser). Nothing else about how a book is shown is: a zoom should not follow the person to
// another book.
import { DEFAULT_SETTINGS, sanitizeSettings } from './epubThemes';
import { LOOKUP_LANGUAGES } from './dictionaryLookup';
import { DEFAULT_HIGHLIGHT_COLOR, isHighlightColor } from './highlightColors';

const PREFIX = 'codice:comic-mode:';
const EPUB_PREFIX = 'codice:epub-settings:';
const DICTIONARY_PREFIX = 'codice:dictionary-target:';
const HIGHLIGHT_PREFIX = 'codice:highlight-color:';
const KEEP_SCREEN_ON_PREFIX = 'codice:keep-screen-on:';
export const COMIC_MODES = ['ltr', 'rtl', 'webtoon', 'double'];

let owner = null;

/** Tells the preferences whose they are: the signed-in account, or nobody (nothing is remembered). */
export function setPreferenceOwner(userId) {
  owner = userId || null;
}

export function getComicMode() {
  if (!owner) return 'ltr';
  try {
    const value = localStorage.getItem(PREFIX + owner);
    return COMIC_MODES.includes(value) ? value : 'ltr';
  } catch {
    return 'ltr'; // storage can be blocked or empty: the default is fine
  }
}

export function saveComicMode(mode) {
  if (!owner || !COMIC_MODES.includes(mode)) return;
  try {
    localStorage.setItem(PREFIX + owner, mode);
  } catch {
    // not being able to remember it is not a problem
  }
}

/** How the EPUB reader looks for this account on this device: what was chosen, with anything unreadable as the default. */
export function getEpubSettings() {
  if (!owner) return { ...DEFAULT_SETTINGS };
  try {
    return sanitizeSettings(JSON.parse(localStorage.getItem(EPUB_PREFIX + owner)));
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
}

/** Whether this account has chosen how the text looks on this device (and not only the default the reader starts from). */
export function hasSavedEpubSettings() {
  if (!owner) return false;
  try {
    return localStorage.getItem(EPUB_PREFIX + owner) !== null;
  } catch {
    return false;
  }
}

export function saveEpubSettings(settings) {
  if (!owner) return;
  try {
    localStorage.setItem(EPUB_PREFIX + owner, JSON.stringify(sanitizeSettings(settings)));
  } catch {
    // not being able to remember it is not a problem
  }
}

/** The language this account wants definitions in, when the dictionary has them in more than one (#109): Portuguese until chosen. */
export function getDictionaryTarget() {
  if (!owner) return 'pt';
  try {
    const value = localStorage.getItem(DICTIONARY_PREFIX + owner);
    return LOOKUP_LANGUAGES.some(([code]) => code === value) ? value : 'pt';
  } catch {
    return 'pt';
  }
}

export function saveDictionaryTarget(code) {
  if (!owner || !LOOKUP_LANGUAGES.some(([c]) => c === code)) return;
  try {
    localStorage.setItem(DICTIONARY_PREFIX + owner, code);
  } catch {
    // not being able to remember it is not a problem
  }
}

/** The color this account painted its last highlight with, which the next one has (terracotta until one is chosen). */
export function getHighlightColor() {
  if (!owner) return DEFAULT_HIGHLIGHT_COLOR;
  try {
    const value = localStorage.getItem(HIGHLIGHT_PREFIX + owner);
    return isHighlightColor(value) ? value : DEFAULT_HIGHLIGHT_COLOR;
  } catch {
    return DEFAULT_HIGHLIGHT_COLOR;
  }
}

export function saveHighlightColor(color) {
  if (!owner || !isHighlightColor(color)) return;
  try {
    localStorage.setItem(HIGHLIGHT_PREFIX + owner, color);
  } catch {
    // not being able to remember it is not a problem
  }
}

/** Whether the screen is kept on while this account reads on this device (#180): yes until it is turned off. It is of the device,
 *  not of the account everywhere: it is about the battery of this phone. */
export function getKeepScreenOn() {
  if (!owner) return true;
  try {
    return localStorage.getItem(KEEP_SCREEN_ON_PREFIX + owner) !== 'off';
  } catch {
    return true;
  }
}

export function saveKeepScreenOn(on) {
  if (!owner) return;
  try {
    localStorage.setItem(KEEP_SCREEN_ON_PREFIX + owner, on ? 'on' : 'off');
  } catch {
    // not being able to remember it is not a problem
  }
}
