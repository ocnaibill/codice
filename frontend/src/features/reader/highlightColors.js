// The four colors a passage can be painted with (the server keeps the id, and refuses any other): the person's choice, and
// nothing else. Terracotta is what a highlight has always been, and what one has until another is chosen.

export const HIGHLIGHT_COLORS = [
  { id: 'terracotta', name: 'Terracota', hex: '#944516' },
  { id: 'sepia', name: 'Sépia', hex: '#a8801c' },
  { id: 'sage', name: 'Sálvia', hex: '#4f7a55' },
  { id: 'indigo', name: 'Índigo', hex: '#3c4d9c' },
];

export const DEFAULT_HIGHLIGHT_COLOR = 'terracotta';

export const isHighlightColor = (id) => HIGHLIGHT_COLORS.some((c) => c.id === id);

/** The color for an id; one that is not one of the four (or none, from a server that does not say) is the default. */
export const highlightColor = (id) => HIGHLIGHT_COLORS.find((c) => c.id === id) ?? HIGHLIGHT_COLORS[0];
