import { formatAge } from './format';

/** The environment variable that holds the key of each provider, to tell where to look when a key is refused. */
const KEY_ENV = { google_books: 'GOOGLE_BOOKS_API_KEY', comicvine: 'COMICVINE_API_KEY' };

/**
 * How a provider answered the last time the worker asked it, in words (DEC-144): the key was refused, the quota is used up, the service did not
 * answer. A refused key, a used-up quota and a service that is down all look like "found nothing" from outside. Returns `{ tone, text }`, with the
 * tone one of ok, warn, danger or faint, or null when there is nothing to say (a provider that is off and was never asked).
 */
export function describeHealth(provider, now = Date.now()) {
  const health = provider.health;
  if (!health) return provider.enabled ? { tone: 'faint', text: 'Ainda não foi perguntado: nada a dizer sobre ele.' } : null;
  const ago = (value) => formatAge(now - new Date(value).getTime());
  const asked = `último pedido ${ago(health.checkedAt)}`;
  const worked = health.lastOkAt ? ` Funcionou pela última vez ${ago(health.lastOkAt)}.` : ' Ainda não respondeu bem.';
  const env = KEY_ENV[provider.id];
  switch (health.state) {
    case 'ok':
      return health.empty
        ? { tone: 'warn', text: `Respondeu, mas sem nada nas últimas buscas seguidas (${asked}). Se as obras existem, confira a chave e a conexão.` }
        : { tone: 'ok', text: `Respondendo normalmente (${asked}).` };
    case 'key':
      return { tone: 'danger', text: `A chave foi recusada (HTTP ${health.status}, ${asked}). ${env ? `Confira ${env} no ambiente do worker e reinicie-o.` : 'Confira o acesso a este serviço.'}${worked}` };
    case 'quota':
      return { tone: 'warn', text: `O limite de uso foi atingido (HTTP ${health.status}, ${asked}). Espere, ou use uma chave com cota maior.${worked}` };
    case 'down':
      return { tone: 'danger', text: `Não respondeu: rede ou serviço fora do ar (${asked}).${worked}` };
    default:
      return { tone: 'warn', text: `Respondeu com erro (HTTP ${health.status}, ${asked}).${worked}` };
  }
}

export const TONE_CLASS = { ok: 'text-success', warn: 'text-warning', danger: 'text-danger', faint: 'text-ink-faint' };
