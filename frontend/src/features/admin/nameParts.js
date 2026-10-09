// How the screen of the surnames reads a name: as words, of which a person picks the ones that make the surname. The server decides
// nothing from this: it checks that the parts are the words of the name (it tells which is which, it does not rename).

/** The words of a name, with no commas ("Herbert, Frank" is two words). */
export function wordsOf(name) {
  return String(name ?? '').replace(/,/g, ' ').split(/\s+/).filter(Boolean);
}

/** The places in `words` where `part` (a surname, as the server says it) is: the run of words that spell it, or none. */
export function placesOf(words, part) {
  const wanted = wordsOf(part);
  for (let start = 0; start + wanted.length <= words.length; start += 1) {
    if (wanted.every((word, i) => words[start + i] === word)) return wanted.map((_, i) => start + i);
  }
  return [];
}

/** The places of the surname the screen starts from: the one proposed, or, for someone already dealt with, the one they have. */
export function initialPlaces(person) {
  const words = wordsOf(person.name);
  return placesOf(words, person.suggestion?.family ?? person.family ?? '');
}

/** The surname and the given names that the chosen places make, each in the order its words have in the name. */
export function partsOf(name, places) {
  const words = wordsOf(name);
  const chosen = new Set(places);
  return {
    family: words.filter((_, i) => chosen.has(i)).join(' '),
    given: words.filter((_, i) => !chosen.has(i)).join(' '),
  };
}

/** How the name reads with the surname first, as a person who prefers that order sees it. */
export function familyFirst({ family, given }) {
  return given ? `${family}, ${given}` : family;
}
