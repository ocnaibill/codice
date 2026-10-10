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

/** What the last test of a provider came to, in words (DEC-145). `{ tone, text }`, or null when it never was tested. */
export function describeTest(provider, now = Date.now()) {
  const test = provider.test;
  if (!test) return null;
  const when = formatAge(now - new Date(test.testedAt).getTime());
  const env = KEY_ENV[provider.id];
  const seconds = `${(test.ms / 1000).toFixed(1).replace('.', ',')} s`;
  switch (test.state) {
    case 'ok':
      return { tone: 'ok', text: `Teste ${when}: respondeu em ${seconds}, com ${test.results} ${test.results === 1 ? 'resultado' : 'resultados'}.` };
    case 'key':
      return { tone: 'danger', text: `Teste ${when}: a chave foi recusada (HTTP ${test.status}).${env ? ` Confira ${env} no ambiente do worker e reinicie-o.` : ''}` };
    case 'quota':
      return { tone: 'warn', text: `Teste ${when}: o limite de uso foi atingido (HTTP ${test.status}). Espere, ou use uma chave com cota maior.` };
    case 'down':
      return { tone: 'danger', text: `Teste ${when}: não respondeu (rede ou serviço fora do ar).` };
    case 'nokey':
      return { tone: 'warn', text: `Teste ${when}: faltou a chave de API, então nada foi perguntado.${env ? ` Defina ${env} no ambiente do worker e reinicie-o.` : ''}` };
    case 'empty':
      return { tone: 'warn', text: `Teste ${when}: respondeu em ${seconds}, mas sem nada para uma pergunta que tem resposta.` };
    default:
      return { tone: 'warn', text: `Teste ${when}: respondeu com erro (HTTP ${test.status}).` };
  }
}

export const TONE_CLASS = { ok: 'text-success', warn: 'text-warning', danger: 'text-danger', faint: 'text-ink-faint' };
