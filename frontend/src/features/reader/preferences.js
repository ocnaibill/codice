// The comic reader's mode is remembered between comics, on this device and for this account
// (DEC-078, RF-013: a preference must not affect other users, and two people can share a
// browser). Nothing else about how a book is shown is: a font size or a zoom should not follow
// the person to another book.
const PREFIX = 'codice:comic-mode:';
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
