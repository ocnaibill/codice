// Which pages of a comic are asked of the server, and in what order (#181). A browser asks for the images in the order they
// are made, and only a few at a time: the pages that are on the screen must be asked for before any other, and after them
// the ones that are about to be turned to, before the ones behind. Asked from the first one that exists, the page of the right
// (the second of two) waited behind the pages that had already been read.

/**
 * The pages to ask for, in order, given the first page on the screen and how many are shown at once (1, or 2 for two pages
 * together): the pages on the screen, then the ones ahead nearest first (as many as `count` turns of the page ahead), then the ones
 * behind nearest first (`count` of them).
 */
export function preloadOrder({ current, shown = 1, count = 2, total }) {
  const wanted = [];
  const add = (page) => {
    if (page >= 0 && page < total) wanted.push(page); // the three runs below never meet, so no page is there twice
  };
  for (let i = 0; i < shown; i += 1) add(current + i);
  for (let i = 0; i < count * shown; i += 1) add(current + shown + i);
  for (let i = 1; i <= count; i += 1) add(current - i);
  return wanted;
}
