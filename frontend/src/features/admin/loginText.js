import { describeUserAgent } from '../../lib/userAgent';
import { formatAge, formatDate } from './format';

/** What each result of the record of sign-ins is called, and in which tone it is shown (DEC-121). */
export const RESULTS = {
  success: { label: 'Entrou', tone: 'ok' },
  bad_password: { label: 'Senha errada', tone: 'bad' },
  unknown_user: { label: 'Usuário que não existe', tone: 'bad' },
  blocked: { label: 'Conta bloqueada', tone: 'bad' },
  directory_unavailable: { label: 'Diretório fora do ar', tone: 'warn' },
  rate_limited: { label: 'Barrado pelo limite de tentativas', tone: 'warn' },
  link_offered: { label: 'Pediu para ligar a conta ao diretório', tone: 'neutral' },
  bad_app_token: { label: 'Aplicativo com senha errada', tone: 'bad' },
};

export const METHODS = {
  local: 'senha da conta',
  ldap: 'diretório (LDAP)',
  invite: 'convite',
  setup: 'primeira configuração',
  app: 'aplicativo (OPDS)',
};

/** The results the filter offers, in the order a person looks for them. */
export const RESULT_FILTERS = ['success', 'bad_password', 'unknown_user', 'blocked', 'rate_limited', 'directory_unavailable', 'bad_app_token', 'link_offered'];

/** Who it was about: the account when there is one; the name that was typed, when the owner may see it; otherwise what it was. */
export function loginWho(entry) {
  if (entry.username) return entry.username;
  if (entry.typed) return `“${entry.typed}”`;
  if (entry.result === 'unknown_user') return 'um nome que não existe';
  return 'sem conta';
}

/** When it was: once, or, for a run of the same failure, from when to when and how many times. */
export function loginWhen(entry, now = Date.now()) {
  const last = new Date(entry.lastAt ?? entry.at);
  const when = formatAge(now - last.getTime());
  if (entry.count > 1) {
    return { text: `${entry.count} vezes, de ${formatDate(entry.at)} a ${formatDate(entry.lastAt)}`, title: when };
  }
  return { text: when, title: formatDate(entry.at) };
}

/** The device, from the browser's text; nothing when the entry has none (an app, a refused request). */
export function loginDevice(entry) {
  return entry.userAgent ? describeUserAgent(entry.userAgent).label : '';
}

/** One line of detail: the way in, the address and the device, whichever there are. */
export function loginDetail(entry) {
  return [
    `por ${METHODS[entry.method] ?? entry.method}`,
    entry.ip ? `endereço ${entry.ip}` : null,
    loginDevice(entry) || null,
  ].filter(Boolean).join(' · ');
}
