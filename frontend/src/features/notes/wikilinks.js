// [[Concept]] links in the text of a note (#21, DEC-110). This reads them the way the server does
// (backend/internal/graph/wikilink.go), and both are tested against the same cases
// (backend/internal/graph/testdata/wikilinks.json): a link must be shown the way it is kept.
//
// What is not a link: a "[[" with no "]]" on the same line, or with a bracket inside, or with nothing but spaces or
// punctuation as the name, or a name longer than 120 characters; one inside a code span or a fenced block, or inside
// a formula ($$...$$ in a line, or a block between two lines of $$); one whose first bracket is escaped with a
// backslash.

const MAX_NAME = 120;
const MAX_TEXT = 300;

const length = (s) => [...s].length;
// What a name reads as needs a letter or a digit (the server compares names without case, accents or punctuation).
const hasSubstance = (s) => /[\p{L}\p{N}]/u.test(s.normalize('NFD').replace(/\p{Mn}/gu, ''));
const isPunct = (c) => /[!-/:-@[-`{-~]/.test(c);

// A fence is three or more ` or ~, or two or more $ (a block of formula). One of ` or $ cannot hold its character in
// its info text: a line like "$$x$$" is a formula in the text, not the start of a block.
function opensFence(line) {
  const s = line.replace(/^ +/, '');
  if (line.length - s.length > 3 || s.length < 2 || (s[0] !== '`' && s[0] !== '~' && s[0] !== '$')) return '';
  let n = 0;
  while (n < s.length && s[n] === s[0]) n++;
  const min = s[0] === '$' ? 2 : 3;
  if (n < min || (s[0] !== '~' && s.slice(n).includes(s[0]))) return '';
  return s.slice(0, n);
}

function closesFence(line, fence) {
  const s = line.replace(/^ +/, '');
  if (line.length - s.length > 3 || s[0] !== fence[0]) return false;
  let n = 0;
  while (n < s.length && s[n] === fence[0]) n++;
  return n >= fence.length && s.slice(n).trim() === '';
}

function spanEnd(text, from, to, n, c) {
  for (let i = from; i < to; ) {
    if (text[i] !== c) {
      i++;
      continue;
    }
    let run = 0;
    while (i + run < to && text[i + run] === c) run++;
    if (run === n) return i + run;
    i += run;
  }
  return -1;
}

function linkAt(text, i, to) {
  let j = i + 2;
  while (j < to && text[j] !== ']' && text[j] !== '[' && text[j] !== '\n') j++;
  if (j + 1 >= to || text[j] !== ']' || text[j + 1] !== ']') return null;
  const inner = text.slice(i + 2, j);
  if (length(inner) > MAX_TEXT) return null;
  let name = inner;
  let label = '';
  const bar = inner.indexOf('|');
  if (bar >= 0) {
    name = inner.slice(0, bar);
    label = inner.slice(bar + 1);
  }
  name = name.trim();
  label = label.trim();
  if (length(name) > MAX_NAME || !hasSubstance(name)) return null;
  return { name, label: label || name, start: i, end: j + 2 };
}

function paragraphLinks(text, from, to) {
  const out = [];
  for (let i = from; i < to; ) {
    const c = text[i];
    if (c === '\\' && i + 1 < to && isPunct(text[i + 1])) {
      i += 2;
    } else if (c === '`' || c === '$') {
      let n = 0;
      while (i + n < to && text[i + n] === c) n++;
      // A span ends at the next run of exactly as many of the same character. A single $ is a dollar sign (a
      // price), not a formula: it takes two.
      const close = c === '`' || n >= 2 ? spanEnd(text, i + n, to, n, c) : -1;
      i = close >= 0 ? close : i + n;
    } else if (c === '[' && i + 1 < to && text[i + 1] === '[') {
      const link = linkAt(text, i, to);
      if (link) {
        out.push(link);
        i = link.end;
      } else {
        i++;
      }
    } else {
      i++;
    }
  }
  return out;
}

/** The [[links]] of a note's text, in order: { name, label, start, end } (offsets in the string). */
export function findLinks(text) {
  const out = [];
  let fence = '';
  let start = -1;
  const flush = (end) => {
    if (start >= 0) {
      out.push(...paragraphLinks(text, start, end));
      start = -1;
    }
  };
  for (let pos = 0; pos < text.length; ) {
    const eol = text.indexOf('\n', pos);
    const next = eol >= 0 ? eol + 1 : text.length;
    const line = text.slice(pos, next);
    const open = opensFence(line);
    if (fence !== '') {
      if (closesFence(line, fence)) fence = '';
    } else if (open !== '') {
      flush(pos);
      fence = open;
    } else if (line.trim() === '') {
      flush(pos);
    } else if (start < 0) {
      start = pos;
    }
    pos = next;
  }
  flush(text.length);
  return out;
}

const SCHEME = 'wikilink:';

/**
 * The text with each [[link]] turned into a Markdown link to "wikilink:N", N being its place in `links` (what
 * findLinks returned), so that a renderer can show it as it likes. The shown text is escaped so that nothing in it
 * is read as Markdown. Everything else is as the person wrote it.
 */
export function linkify(text, links) {
  let out = '';
  let at = 0;
  links.forEach((link, n) => {
    out += text.slice(at, link.start) + `[${link.label.replace(/[!-/:-@[-`{-~]/g, '\\$&')}](${SCHEME}${n})`;
    at = link.end;
  });
  return out + text.slice(at);
}

/** Which link an href made by linkify is, or -1. */
export function linkIndex(href) {
  return typeof href === 'string' && href.startsWith(SCHEME) ? Number(href.slice(SCHEME.length)) : -1;
}
