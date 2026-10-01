// The reference sources whose identifiers a person may hold (DEC-095), as a reader knows them.
const SOURCES = { openlibrary: 'Open Library', comicvine: 'ComicVine' };

export function sourceName(scheme) {
  return SOURCES[scheme] || scheme;
}

/** "openlibrary:OL79034A" -> "Open Library OL79034A" */
export function keyLabel(key) {
  const at = key.indexOf(':');
  return `${sourceName(key.slice(0, at))} ${key.slice(at + 1)}`;
}
