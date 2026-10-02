/** The table of contents of a book as a flat list, each entry with how deep it is. */
export function flattenToc(items, depth = 0) {
  return (items || []).flatMap((item) => [
    { id: item.id, href: item.href, label: item.label?.trim() || '', depth },
    ...flattenToc(item.subitems, depth + 1),
  ]);
}
