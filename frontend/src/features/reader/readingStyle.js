// How a text that is not a book (plain text, Markdown) is drawn, from the same choices as the EPUB reader's (#77, #106):
// the page color, the font, the size and the space between lines. The size is a percentage of the usual letter.
import { READING_THEMES, READING_FONTS, READING_SPACING, marginStyle, sanitizeSettings } from './epubThemes';

export const BASE_SIZE = 16;
/** The space between lines when the person has not chosen one. */
export const DEFAULT_LINE_HEIGHT = 1.7;

export function readingStyle(settings) {
  const s = sanitizeSettings(settings);
  const theme = READING_THEMES.find((t) => t.id === s.theme);
  const font = READING_FONTS.find((f) => f.id === s.font);
  const spacing = READING_SPACING.find((p) => p.id === s.spacing);
  return {
    theme,
    page: { backgroundColor: theme.background },
    margins: marginStyle(s),
    text: {
      color: theme.text,
      fontSize: `${(BASE_SIZE * s.size) / 100}px`,
      lineHeight: spacing.value ?? DEFAULT_LINE_HEIGHT,
      ...(font.appStack ? { fontFamily: font.appStack } : {}),
      ...(s.justify ? { textAlign: 'justify', hyphens: 'auto' } : {}),
    },
  };
}
