// The fonts of the reader that are the app's own, hosted with it (no call to a service outside). A book is drawn in
// an iframe that does not see the app's styles, so each one is declared again inside it (epubThemes.fontFaceCss).
import serif from '@fontsource-variable/newsreader/files/newsreader-latin-wght-normal.woff2?url';
import serifItalic from '@fontsource-variable/newsreader/files/newsreader-latin-wght-italic.woff2?url';
import sans from '@fontsource-variable/plus-jakarta-sans/files/plus-jakarta-sans-latin-wght-normal.woff2?url';
import sansItalic from '@fontsource-variable/plus-jakarta-sans/files/plus-jakarta-sans-latin-wght-italic.woff2?url';

export const FONT_FILES = { serif, serifItalic, sans, sansItalic };
