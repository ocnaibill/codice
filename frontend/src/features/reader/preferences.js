// The comic reader's mode is remembered between comics, on this device (DEC-078). Nothing else
// about how a book is shown is: a font size or a zoom should not follow the person to another book.
const KEY = 'codice:comic-mode';
export const COMIC_MODES = ['ltr', 'rtl', 'webtoon', 'double'];

export function getComicMode() {
  try {
    const value = localStorage.getItem(KEY);
    return COMIC_MODES.includes(value) ? value : 'ltr';
  } catch {
    return 'ltr'; // storage can be blocked or empty: the default is fine
  }
}

export function saveComicMode(mode) {
  if (!COMIC_MODES.includes(mode)) return;
  try {
    localStorage.setItem(KEY, mode);
  } catch {
    // not being able to remember it is not a problem
  }
}
