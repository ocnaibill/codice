import { extensionOf } from './accepted';

/** What the bulk upload of a series takes in one go (DEC-200). */
export const MAX_SERIES_FILES = 500;
/** How many files go to the server at the same time. */
export const SERIES_CONCURRENCY = 3;
/** The formats of a series of chapters or volumes. */
export const SERIES_EXTENSIONS = ['cbz', 'cbr', 'pdf'];

export const isSeriesFile = (name) => SERIES_EXTENSIONS.includes(extensionOf(name));

const NUM = String.raw`(\d{1,4}(?:[.,]\d{1,2})?)`;
// "Cap 1", "Cap. 1", "Capítulo 01", "Chapter 12", "Ch 3", "c012", "#7". A letter before "c"/"v" would make it part of a word ("Voice 3").
const CHAPTER = new RegExp(String.raw`(?<![\p{L}\d])(?:cap[ií]tulo|cap\.?|chapter|ch\.?|c(?=\d))\s*[#.]?\s*${NUM}`, 'iu');
const VOLUME = new RegExp(String.raw`(?<![\p{L}\d])(?:volume|vol\.?|v(?=\d))\s*[#.]?\s*${NUM}`, 'iu');
// "Caps 18-26", "Cap 12-13", "Chapters 1 a 9": the chapters a file holds. "(18-26)" after a volume says the same.
const RANGE = new RegExp(String.raw`(?<![\p{L}\d])(?:cap[ií]tulos?|caps?\.?|chapters?|ch\.?)\s*[#.]?\s*${NUM}\s*(?:-|–|—|a|to|até)\s*${NUM}(?!\d)`, 'iu');
const PAREN_RANGE = new RegExp(String.raw`\(\s*${NUM}\s*(?:-|–|—)\s*${NUM}\s*\)`, 'u');
const HASH = new RegExp(String.raw`#\s*${NUM}`, 'u');
const TRAILING = new RegExp(String.raw`(?<![\d.,])${NUM}(?!.*\d)`, 'u');

const toNumber = (text) => Number(String(text).replace(',', '.'));

const baseName = (name) => String(name ?? '').replace(/\.[^./\\]+$/, '').replace(/_/g, ' ').trim();

/**
 * What the name of a file says of its number: { chapter, volume, bare, at }, each a number or null. `bare` is a number with no word
 * before it ("#7", or the last number of the name); `at` is where the first marker begins, to cut the series name there.
 */
export function readNumber(filename) {
  const text = baseName(filename);
  const found = { chapter: null, volume: null, bare: null, range: null, at: -1 };
  const cut = (match) => {
    if (found.at < 0 || match.index < found.at) found.at = match.index;
  };
  let m = CHAPTER.exec(text);
  if (m) { found.chapter = toNumber(m[1]); cut(m); }
  m = VOLUME.exec(text);
  if (m) { found.volume = toNumber(m[1]); cut(m); }
  m = HASH.exec(text);
  if (m) { found.bare = toNumber(m[1]); cut(m); }
  m = RANGE.exec(text) ?? PAREN_RANGE.exec(text);
  if (m && toNumber(m[2]) > toNumber(m[1])) found.range = { from: toNumber(m[1]), to: toNumber(m[2]) };
  if (found.chapter == null && found.volume == null && found.bare == null) {
    m = TRAILING.exec(text);
    if (m) { found.bare = toNumber(m[1]); cut(m); }
  }
  return found;
}

/**
 * The number and the unit a file has, given what the person chose for the series: "chapter", "volume" or "auto" (each file by what its name
 * says; a name with only a bare number is taken as a chapter). { number, unit, volumeNumber, covers }: the number is null when the name has none
 * for that unit; `volumeNumber` is the volume a chapter says it is in ("v03 c012") and `covers` the chapters the file holds ("Vol 3 Caps 18-26",
 * "Cap 12-13"), both null when the name says nothing (DEC-202).
 */
export function numberFor(read, unit) {
  const covers = read.range;
  if (unit === 'chapter') {
    return { number: read.chapter ?? read.bare, unit: 'chapter', volumeNumber: read.volume, covers };
  }
  if (unit === 'volume') {
    return { number: read.volume ?? read.bare, unit: 'volume', volumeNumber: null, covers };
  }
  // "Vol 3 Caps 18-26" is a volume that holds a range of chapters; the first of them is not the number of the file
  if (read.volume != null && read.range && read.chapter === read.range.from) return { number: read.volume, unit: 'volume', volumeNumber: null, covers };
  if (read.chapter != null) return { number: read.chapter, unit: 'chapter', volumeNumber: read.volume, covers };
  if (read.volume != null) return { number: read.volume, unit: 'volume', volumeNumber: null, covers };
  return { number: read.bare, unit: 'chapter', volumeNumber: null, covers };
}

const SEPARATORS = /^[\s\-–—_.,:;|]+|[\s\-–—_.,:;|]+$/g;

/**
 * The name the series seems to have: what comes before the number in most of the names ("Jujutsu Kaisen Modelo - Cap 1.cbz" -> "Jujutsu
 * Kaisen Modelo"). When the names say nothing the folder they came from does, and when not even that, nothing.
 */
export function guessSeries(names, folder = '') {
  const count = new Map();
  for (const name of names) {
    const read = readNumber(name);
    const text = baseName(name);
    const head = (read.at > 0 ? text.slice(0, read.at) : '').replace(SEPARATORS, '').replace(/\s+/g, ' ');
    if (head) count.set(head, (count.get(head) ?? 0) + 1);
  }
  let best = '';
  let most = 0;
  for (const [head, n] of count) {
    if (n > most) { best = head; most = n; }
  }
  return best || String(folder ?? '').trim();
}

/** "12" for 12, "10.5" for 10.5: how a number reads in a title and in the field. */
export const formatNumber = (number) => (Number.isInteger(number) ? String(number) : String(Number(number.toFixed(2))));

/** The number as the person typed it: "12", "10,5". null when it is not a number above zero. */
export function parseTyped(text) {
  const clean = String(text ?? '').trim().replace(',', '.');
  if (!/^\d{1,6}(\.\d{1,2})?$/.test(clean)) return null;
  const number = Number(clean);
  return number > 0 ? number : null;
}

/** The title of a work, by default: "Série - Cap. 12", "Série - Vol. 3". With no number, the name of the file. */
export function titleFor(series, unit, number, filename, covers = null) {
  if (number == null) return baseName(filename);
  const label = unit === 'volume' ? 'Vol.' : 'Cap.';
  // a chapter that holds more than one ("Cap 12-13") is said so; a volume is its number, whatever it holds
  const said = unit === 'chapter' && covers && covers.to > covers.from ? `${formatNumber(covers.from)}-${formatNumber(covers.to)}` : formatNumber(number);
  return series ? `${series} - ${label} ${said}` : `${label} ${said}`;
}

/** The files in the order the series reads: by number, the ones with no number last, and by name where it is a tie. */
export function sortRows(rows) {
  return [...rows].sort((a, b) => {
    if (a.number == null && b.number == null) return a.name.localeCompare(b.name, undefined, { numeric: true });
    if (a.number == null) return 1;
    if (b.number == null) return -1;
    return a.number - b.number || a.name.localeCompare(b.name, undefined, { numeric: true });
  });
}

/**
 * The rows of the review, from the files and the choices: each row has its number (what the person typed, or what the name says), its
 * unit, its title (what the person typed, or the default) and its situation: "new", "repeated" (another row has the same unit and number)
 * or "missing" (no number). `rows` are { id, name, read, typedNumber, typedTitle }.
 */
export function deriveRows(rows, { series, unit }) {
  const derived = rows.map((row) => {
    const auto = numberFor(row.read, unit);
    const typed = row.typedNumber == null ? undefined : parseTyped(row.typedNumber);
    const number = typed === undefined ? auto.number : typed;
    return { ...row, number, unit: auto.unit, volumeNumber: auto.volumeNumber, covers: auto.covers, key: number == null ? null : `${auto.unit}:${number}` };
  });
  const seen = new Map();
  for (const row of derived) if (row.key) seen.set(row.key, (seen.get(row.key) ?? 0) + 1);
  return derived.map((row) => ({
    ...row,
    title: row.typedTitle != null ? row.typedTitle : titleFor(series, row.unit, row.number, row.name, row.covers),
    situation: row.key == null ? 'missing' : seen.get(row.key) > 1 ? 'repeated' : 'new',
  }));
}

/** The whole numbers a run skips between its first and its last, in the unit most rows have ("faltam 57 e 58"). */
export function gaps(rows) {
  const byUnit = new Map();
  for (const row of rows) {
    if (row.number == null || !Number.isInteger(row.number)) continue;
    if (!byUnit.has(row.unit)) byUnit.set(row.unit, new Set());
    byUnit.get(row.unit).add(row.number);
  }
  let main = null;
  for (const [unit, set] of byUnit) if (!main || set.size > main.set.size) main = { unit, set };
  if (!main) return [];
  const numbers = [...main.set];
  const lo = Math.min(...numbers);
  const hi = Math.max(...numbers);
  const out = [];
  for (let n = lo; n <= hi; n += 1) if (!main.set.has(n)) out.push(n);
  return out;
}

/** The facts the chips show: how many have a number, how many do not, which numbers repeat, what is missing, from where to where. */
export function summarize(rows) {
  const withNumber = rows.filter((row) => row.number != null);
  const repeated = [...new Set(rows.filter((row) => row.situation === 'repeated').map((row) => row.key))];
  const numbers = withNumber.map((row) => row.number);
  return {
    total: rows.length,
    withNumber: withNumber.length,
    withoutNumber: rows.length - withNumber.length,
    repeated: repeated.map((key) => Number(key.split(':')[1])),
    repeatedRows: rows.filter((row) => row.situation === 'repeated').length,
    missing: gaps(rows),
    first: numbers.length ? Math.min(...numbers) : null,
    last: numbers.length ? Math.max(...numbers) : null,
    sendable: rows.filter((row) => row.situation === 'new').length,
  };
}

/** "57", "57 e 58", "57, 58 e 59", "57, 58, 59, 60, 61 e mais 4". */
export function listNumbers(numbers, show = 5) {
  const shown = numbers.slice(0, show).map(formatNumber);
  if (numbers.length > show) return `${shown.join(', ')} e mais ${numbers.length - show}`;
  if (shown.length <= 1) return shown.join('');
  return `${shown.slice(0, -1).join(', ')} e ${shown[shown.length - 1]}`;
}

/** The folder a file came from, when the person chose a whole folder: "Jujutsu Kaisen/Cap 1.cbz" -> "Jujutsu Kaisen". */
export function folderOf(file) {
  const path = file.webkitRelativePath || '';
  const parts = path.split('/');
  return parts.length > 1 ? parts[0] : '';
}

/** The files chosen, the ones a series takes, in the order they read, and how many were left out (other formats, or over the limit). */
export function pickSeriesFiles(files, already = []) {
  const taken = new Set(already.map((file) => `${file.name}|${file.size}|${file.lastModified}`));
  const kept = [];
  let other = 0;
  let repeated = 0;
  let over = 0;
  for (const file of files) {
    const sign = `${file.name}|${file.size}|${file.lastModified}`;
    if (!isSeriesFile(file.name)) other += 1;
    else if (taken.has(sign)) repeated += 1;
    else if (already.length + kept.length >= MAX_SERIES_FILES) over += 1;
    else { taken.add(sign); kept.push(file); }
  }
  return { kept, other, repeated, over };
}

/**
 * Runs `work(item)` for each item, up to `limit` at a time, and stops taking new ones once `stopped()` says so; the ones that were going
 * finish. Resolves when no one is left going. The order in which they start is the order of the list.
 */
export async function runPool(items, work, limit, stopped = () => false) {
  let next = 0;
  const lane = async () => {
    while (next < items.length && !stopped()) {
      const item = items[next];
      next += 1;
      await work(item);
    }
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, lane));
  return items.slice(next); // what was never started
}
