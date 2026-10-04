import { describeUserAgent } from '../../lib/userAgent';
import { formatAge, formatDate } from '../../features/admin/format';

// A session used in the last two minutes is "active now": the use is written at most once a minute.
const ACTIVE_WITHIN = 2 * 60 * 1000;

const KIND = { phone: 'Celular', tablet: 'Tablet', computer: 'Computador' };

/** What a line of the list of sessions says, from what the server sent. */
export function sessionLines(session, now = Date.now()) {
  const device = describeUserAgent(session.userAgent);
  const ago = session.lastSeenAt ? now - new Date(session.lastSeenAt).getTime() : null;
  let use = 'Último uso não registrado';
  if (ago !== null) use = ago < ACTIVE_WITHIN ? 'Ativa agora' : `Último uso ${formatAge(ago)}`;
  return {
    title: device.label,
    kind: KIND[device.kind],
    // The address is the owner's alone: the list of another account has none, and the lines say nothing about it then.
    address: session.ip === undefined ? null : session.ip ? `Endereço ${session.ip}` : 'Endereço não registrado',
    entered: `Entrou em ${formatDate(session.createdAt)}`,
    use,
  };
}
