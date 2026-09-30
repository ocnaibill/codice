export const epubThemes = {
  light: {
    '.light': { background: '#faf8f4' },
    '.light, .light *': { color: '#18181b !important' },
    '.light a': { color: '#1d4ed8 !important' },
  },
  dark: {
    '.dark': { background: '#09090b' },
    // EPUBs can set black text explicitly; the reader theme must win on a dark page.
    '.dark, .dark *': { color: '#e4e4e7 !important' },
    '.dark a': { color: '#93c5fd !important' },
  },
};

export function applyEpubTheme(rendition, theme) {
  if (!rendition) return;
  rendition.themes.register('light', epubThemes.light);
  rendition.themes.register('dark', epubThemes.dark);
  rendition.themes.select(theme);
}
