// What the storage says when a file cannot be moved, in words a person can act on. The server answers in English;
// anything it says that is not here is shown as it came.

const KNOWN = [
  ['the original file is missing', 'O original não está mais na pasta de origem.'],
  ['the original file changed since it was catalogued', 'O original mudou desde que foi catalogado. Varra a pasta de novo para catalogá-lo outra vez.'],
  ['the destination is already taken', 'Já existe, no armazenamento gerenciado, um arquivo diferente no lugar de destino. Nada foi sobrescrito.'],
  ['the path is not a safe relative path', 'O caminho do arquivo não é seguro; ele não foi movido.'],
  ['the file is not in a referenced location', 'O arquivo não está mais em uma pasta referenciada (talvez já tenha sido movido).'],
  ['the file cannot be moved', 'O arquivo não pode ser movido.'],
  ['not a referenced file that is available', 'Não é um arquivo referenciado disponível (talvez já tenha sido movido).'],
  ['another file of this work is being moved', 'Outro arquivo desta obra está sendo movido: peça de novo quando terminar.'],
  ['could not queue the transfer', 'Não foi possível pôr a transferência na fila.'],
  ['the transfer was cancelled', 'A transferência foi cancelada.'],
];

/** The reason a file was not moved, in Portuguese; the server's own words when it is a reason this does not know. */
export function explainTransferError(text) {
  const raw = (text || '').trim();
  if (!raw) return 'Não foi possível mover o arquivo.';
  const lower = raw.toLowerCase();
  const found = KNOWN.find(([english]) => lower.includes(english));
  return found ? found[1] : raw;
}

const CLEANUP = [
  ['could not read the original', 'Não foi possível ler o original para conferi-lo'],
  ['the original changed after it was copied', 'O original mudou depois de copiado, então foi mantido.'],
  ['the original no longer has the content that was copied', 'O original não tem mais o conteúdo que foi copiado, então foi mantido.'],
  ['the original could not be removed', 'Não foi possível apagar o original'],
  ['waiting to be removed', 'Aguardando para ser apagado.'],
];

/** Why an original is still in its folder after the copy stood, in Portuguese; what the system said (a permission, a
 *  path) follows it, since that is what can be fixed. A reason this does not know is shown as it came. */
export function explainCleanupReason(text) {
  const raw = (text || '').trim();
  if (!raw) return '';
  const lower = raw.toLowerCase();
  const found = CLEANUP.find(([english]) => lower.startsWith(english));
  if (!found) return raw;
  const detail = raw.slice(raw.indexOf(':') + 1).trim();
  return raw.includes(':') && !found[1].endsWith('.') ? `${found[1]}: ${detail}` : found[1];
}

export const STATE_LABEL = {
  ok: 'no disco',
  missing: 'ausente do disco',
  conflict: 'mudou depois de catalogado',
};

export const STATE_HINT = {
  missing: 'O arquivo não está mais onde foi catalogado: não dá para movê-lo. Varra a pasta para atualizar.',
  conflict: 'O arquivo mudou depois de ser catalogado: moverá só se voltar a ser o mesmo. Varra a pasta para catalogá-lo de novo.',
};

/** What the person is told about a file that is waiting, running or failed on its way to the managed storage. */
export function transferBadge(transfer) {
  if (!transfer) return null;
  if (transfer.state === 'pending') return { tone: 'info', text: 'na fila para mover' };
  if (transfer.state === 'running') return { tone: 'info', text: 'sendo movido…' };
  if (transfer.state === 'failed') return { tone: 'error', text: `a última tentativa falhou: ${explainTransferError(transfer.lastError)}` };
  return null;
}

/** A file can be asked for only when it is there and nothing is moving it already. */
export const canMove = (file) => file.state === 'ok' && !(file.transfer && (file.transfer.state === 'pending' || file.transfer.state === 'running'));
