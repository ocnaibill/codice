/** 1:05 for a short audio, 1:02:10 for a long one (an audiobook is hours long). */
export function formatTime(t) {
  const total = Math.max(0, Math.floor(Number.isFinite(t) ? t : 0));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const ss = s.toString().padStart(2, '0');
  return h > 0 ? `${h}:${m.toString().padStart(2, '0')}:${ss}` : `${m}:${ss}`;
}

/** The time as it is said, for a screen reader: "1 hora, 2 minutos e 10 segundos". */
export function spoken(t) {
  const total = Math.max(0, Math.floor(Number.isFinite(t) ? t : 0));
  const parts = [];
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h) parts.push(`${h} ${h === 1 ? 'hora' : 'horas'}`);
  if (m) parts.push(`${m} ${m === 1 ? 'minuto' : 'minutos'}`);
  if (s || parts.length === 0) parts.push(`${s} ${s === 1 ? 'segundo' : 'segundos'}`);
  return parts.length > 1 ? `${parts.slice(0, -1).join(', ')} e ${parts.at(-1)}` : parts[0];
}

