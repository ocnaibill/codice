const UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];

/** 1536 -> "1,5 KB". */
export function formatBytes(bytes) {
  let value = Number(bytes) || 0;
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const text = unit === 0 ? String(Math.round(value)) : value.toFixed(1).replace('.', ',');
  return `${text} ${UNITS[unit]}`;
}

export function formatDate(value) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('pt-BR');
}

/** How long ago, in Portuguese: 90 s -> "há 1 minuto", 3 h -> "há 3 horas", 50 h -> "há 2 dias". `ms` is the age. */
export function formatAge(ms) {
  const seconds = Math.max(0, Math.floor((Number(ms) || 0) / 1000));
  if (seconds < 60) return 'agora há pouco';
  const unit = (n, one, many) => `há ${n} ${n === 1 ? one : many}`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return unit(minutes, 'minuto', 'minutos');
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return unit(hours, 'hora', 'horas');
  return unit(Math.floor(hours / 24), 'dia', 'dias');
}

/** How much of a time that is still ahead is left, in Portuguese: 20 min -> "menos de 1 hora", 5 h 40 min -> "5 horas", 25 h -> "1 dia". */
export function formatLeft(ms) {
  const minutes = Math.floor((Number(ms) || 0) / 60000);
  if (minutes < 60) return 'menos de 1 hora';
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return hours === 1 ? '1 hora' : `${hours} horas`;
  const days = Math.floor(hours / 24);
  return days === 1 ? '1 dia' : `${days} dias`;
}
