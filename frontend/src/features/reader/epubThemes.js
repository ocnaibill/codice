// How the reader of an EPUB looks (#77, #106): the colors of the page, the font, the size, the space between lines, the margins
// and whether the lines are justified.
// The choices come from short closed lists, so that every combination of them can be read: no color is typed by hand.
import { FONT_FILES } from './epubFonts';

/** The pages the reader can have. Every pair of colors is checked to be readable (epubThemes.test.js). */
export const READING_THEMES = [
  { id: 'branco', label: 'Branco', background: '#ffffff', text: '#18181b', link: '#1d4ed8' },
  { id: 'papel', label: 'Papel', background: '#faf8f4', text: '#18181b', link: '#1d4ed8' },
  { id: 'sepia', label: 'Sépia', background: '#f2e8d5', text: '#4a3728', link: '#8a3b12' },
  { id: 'cinza', label: 'Cinza', background: '#d8d8dc', text: '#1f1f23', link: '#1d4ed8' },
  { id: 'escuro', label: 'Escuro', background: '#18181b', text: '#e4e4e7', link: '#93c5fd' },
  { id: 'preto', label: 'Preto', background: '#000000', text: '#d4d4d8', link: '#93c5fd' },
];

/** The fonts: the book's own (the default), two of the app's, and one made for dyslexia (OpenDyslexic), all hosted with it. */
export const READING_FONTS = [
  { id: 'livro', label: 'Do livro', stack: null, appStack: null },
  // `stack` is for the page of a book (an iframe, where the fonts are declared again); `appStack` for the app's own page.
  { id: 'serifada', label: 'Serifada', stack: '"Codice Serif", Georgia, serif', appStack: '"Newsreader Variable", Georgia, serif' },
  { id: 'sem-serifa', label: 'Sem serifa', stack: '"Codice Sans", system-ui, sans-serif', appStack: '"Plus Jakarta Sans Variable", system-ui, sans-serif' },
  { id: 'dislexia', label: 'Dislexia', stack: '"Codice OpenDyslexic", system-ui, sans-serif', appStack: '"OpenDyslexic", system-ui, sans-serif' },
];

/** The margins: the page's own, or the room left on each side of the text (a share of the width of the page). */
export const READING_MARGINS = [
  { id: 'livro', label: 'Do livro', value: null },
  { id: 'estreita', label: 'Estreita', value: '0px' },
  { id: 'media', label: 'Média', value: '6%' },
  { id: 'larga', label: 'Larga', value: '14%' },
];

/** The space between lines: the book's own, or a given one. */
export const READING_SPACING = [
  { id: 'livro', label: 'Do livro', value: null },
  { id: 'media', label: 'Média', value: 1.5 },
  { id: 'ampla', label: 'Ampla', value: 1.8 },
];

export const SIZE_MIN = 80;
export const SIZE_MAX = 200;
export const SIZE_STEP = 10;

export const DEFAULT_SETTINGS = { theme: 'papel', font: 'livro', size: 100, spacing: 'livro', margins: 'livro', justify: false };

const has = (list, id) => list.some((item) => item.id === id);

/** What is kept of a choice: every value is one of the lists, and the size is one of the steps. Anything else is the default. */
export function sanitizeSettings(raw) {
  const value = raw && typeof raw === 'object' ? raw : {};
  const size = Number(value.size);
  const onStep = Number.isInteger(size) && size >= SIZE_MIN && size <= SIZE_MAX && (size - SIZE_MIN) % SIZE_STEP === 0;
  return {
    theme: has(READING_THEMES, value.theme) ? value.theme : DEFAULT_SETTINGS.theme,
    font: has(READING_FONTS, value.font) ? value.font : DEFAULT_SETTINGS.font,
    size: onStep ? size : DEFAULT_SETTINGS.size,
    spacing: has(READING_SPACING, value.spacing) ? value.spacing : DEFAULT_SETTINGS.spacing,
    margins: has(READING_MARGINS, value.margins) ? value.margins : DEFAULT_SETTINGS.margins,
    justify: value.justify === true,
  };
}

const channel = (hex, at) => {
  const v = parseInt(hex.slice(at, at + 2), 16) / 255;
  return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
};
export const SELECTION_ON_LIGHT = 'rgba(148, 69, 22, 0.3)'; // the brand, #944516
export const SELECTION_ON_DARK = 'rgba(179, 93, 44, 0.55)'; // the lighter brand, #b35d2c
const luminance = (hex) => 0.2126 * channel(hex, 1) + 0.7152 * channel(hex, 3) + 0.0722 * channel(hex, 5);

/** The contrast of two colors written #rrggbb, from 1 (none) to 21 (black on white): WCAG asks 4.5 for text. */
export function contrastRatio(a, b) {
  const [light, dark] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (light + 0.05) / (dark + 0.05);
}

/** The @font-face of the app's fonts, to be declared inside the page of a book. `base` is where the files are served from. */
export function fontFaceCss(base = window.location.href) {
  const face = (family, file, style, weight = '200 800') =>
    `@font-face{font-family:"${family}";font-style:${style};font-weight:${weight};font-display:swap;src:url("${new URL(file, base).href}") format("woff2");}`;
  return [
    face('Codice Serif', FONT_FILES.serif, 'normal'),
    face('Codice Serif', FONT_FILES.serifItalic, 'italic'),
    face('Codice Sans', FONT_FILES.sans, 'normal'),
    face('Codice Sans', FONT_FILES.sansItalic, 'italic'),
    face('Codice OpenDyslexic', FONT_FILES.dyslexic, 'normal', '400'),
    face('Codice OpenDyslexic', FONT_FILES.dyslexicBold, 'normal', '700'),
    face('Codice OpenDyslexic', FONT_FILES.dyslexicItalic, 'italic', '400'),
  ].join('\n');
}

/** The room on each side of the text, as an inline style for the box that holds the page; nothing for "Do livro". */
export function marginStyle(settings) {
  const value = READING_MARGINS.find((m) => m.id === sanitizeSettings(settings).margins)?.value;
  return value ? { paddingInline: value } : {};
}

const TEXT_ELEMENTS = '*:not(pre):not(code):not(kbd):not(samp):not(pre *)';

/** The name of the theme epub.js is given for these settings: it is a class on the page of the book. */
export const themeName = (settings) => `read-${settings.theme}-${settings.font}-${settings.spacing}-${settings.justify ? 'j' : 'l'}`;

/**
 * The rules for the page of the book. The reader wins over what the book forces (a book can set black text, or its own
 * font), but a font for code is left alone, and with "Do livro" the font and the spacing are the book's.
 */
export function epubRules(settings) {
  const s = sanitizeSettings(settings);
  const theme = READING_THEMES.find((t) => t.id === s.theme);
  const font = READING_FONTS.find((f) => f.id === s.font);
  const spacing = READING_SPACING.find((p) => p.id === s.spacing);
  const n = `.${themeName(s)}`;
  const rules = {
    [n]: { background: `${theme.background} !important` },
    [`${n} body`]: { background: `${theme.background} !important` },
    [`${n}, ${n} *`]: { color: `${theme.text} !important` },
    [`${n} a, ${n} a *`]: { color: `${theme.link} !important` },
  };
  // The selection is terracotta, as in the rest of the app; on a dark page the lighter terracotta, and stronger, to be seen.
  rules[`${n}::selection, ${n} *::selection`] = { background: `${luminance(theme.background) < 0.2 ? SELECTION_ON_DARK : SELECTION_ON_LIGHT} !important` };
  if (font.stack) rules[`${n} ${TEXT_ELEMENTS}`] = { 'font-family': `${font.stack} !important` };
  if (spacing.value) rules[`${n} p, ${n} li, ${n} blockquote, ${n} dd, ${n} div`] = { 'line-height': `${spacing.value} !important` };
  // Justified lines, with hyphens where the book says its language: only the paragraphs of text, and not a heading, a table or a poem's lines.
  if (s.justify) rules[`${n} p, ${n} li, ${n} blockquote, ${n} dd`] = { 'text-align': 'justify !important', hyphens: 'auto !important', '-webkit-hyphens': 'auto !important' };
  return rules;
}

/** Puts the settings on the book that is shown: the colors, the font, the spacing and the size. */
export function applyEpubSettings(rendition, settings) {
  if (!rendition) return;
  const s = sanitizeSettings(settings);
  const name = themeName(s);
  rendition.themes.register(name, epubRules(s));
  rendition.themes.select(name);
  rendition.themes.fontSize(`${s.size}%`);
}
