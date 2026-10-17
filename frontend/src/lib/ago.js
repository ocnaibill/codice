// How long ago, in words, for a time of the server (the notices of the reader and the list of the accesses use it).

/**
 * How long ago, in words, for a time of the server: "há 2 horas", "ontem", "há 5 dias", "há 3 semanas", "há 2 meses". Empty for the last
 * few minutes (nothing to say) and for a time that is not one.
 */
export function ago(iso, now = Date.now()) {
  const time = Date.parse(iso);
  if (!Number.isFinite(time)) return '';
  const minutes = Math.floor((now - time) / 60000);
  if (minutes < 10) return '';
  const unit = (n, one, many) => `há ${n} ${n === 1 ? one : many}`;
  if (minutes < 60) return unit(minutes, 'minuto', 'minutos');
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return unit(hours, 'hora', 'horas');
  const days = Math.floor(hours / 24);
  if (days === 1) return 'ontem';
  if (days < 14) return unit(days, 'dia', 'dias');
  if (days < 60) return unit(Math.floor(days / 7), 'semana', 'semanas');
  return unit(Math.floor(days / 30), 'mês', 'meses');
}
