// What the screen of the dictionaries says of a package (#109): the languages, how well it covers each, how big it is and
// where its installation is. Kept apart from the screen so that the wording can be tested without drawing it.
import { formatBytes, formatDate } from './format';

const LANGUAGES = { pt: 'Português', en: 'Inglês', es: 'Espanhol', fr: 'Francês', de: 'Alemão', it: 'Italiano', ja: 'Japonês', zh: 'Chinês' };
const LEVELS = { complete: 'completo', partial: 'parcial', weak: 'só tradução' };

export const languageLabel = (code) => LANGUAGES[code] ?? code;
export const levelLabel = (level) => LEVELS[level] ?? level;

/** "Português · completo". */
export const coverageLabel = (language) => `${languageLabel(language.code)} · ${levelLabel(language.level)}`;

/** How far, as a whole number from 0 to 100; anything that is not a fraction is taken as the start. */
export function percent(fraction) {
  const n = Number(fraction);
  if (!Number.isFinite(n)) return 0;
  return Math.round(Math.min(1, Math.max(0, n)) * 100);
}

const count = (n) => Number(n || 0).toLocaleString('pt-BR');

/** A date the source gave in the form of an HTTP header ("Mon, 28 Sep 2026 15:20:37 GMT"), as a date. */
export function sourceDay(value) {
  if (!value) return '';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString('pt-BR');
}

/** The size the download has, "35,4 MB", and what it takes in the database when that was measured. */
export function sizeLine(pkg) {
  const download = `Download de ${formatBytes(pkg.downloadBytes)}`;
  return pkg.storageBytes ? `${download} · ocupa cerca de ${formatBytes(pkg.storageBytes)} no banco` : download;
}

/** Where an installation is, in words. */
export function progressLine(pkg) {
  const done = percent(pkg.progress);
  switch (pkg.stage) {
    case 'downloading':
      return pkg.bytesTotal
        ? `Baixando: ${done}% (${formatBytes(pkg.bytesDone)} de ${formatBytes(pkg.bytesTotal)})`
        : `Baixando: ${formatBytes(pkg.bytesDone)}`;
    case 'importing':
      return pkg.entries > 0 ? `Importando: ${done}% (${count(pkg.entries)} verbetes lidos)` : `Importando: ${done}%`;
    case 'retrying':
      return `Tentando de novo${pkg.error ? `: ${pkg.error}` : ''}`;
    default:
      return 'Na fila, esperando o worker';
  }
}

/** What an installed package has, in words. */
export function readyLine(pkg) {
  const parts = [`Instalado em ${formatDate(pkg.installedAt)}`, `${count(pkg.entries)} verbetes`];
  const day = sourceDay(pkg.sourceDate);
  if (day) parts.push(`arquivo de ${day}`);
  return parts.join(' · ');
}
