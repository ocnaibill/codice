import { describe, it, expect } from 'vitest';
import { STATE_HINT, STATE_LABEL, canMove, explainCleanupReason, explainTransferError, transferBadge } from './storageText';

describe('explainTransferError', () => {
  it('says in Portuguese what the server said about the files that did not move', () => {
    expect(explainTransferError('the original file is missing: Livros/Duna.epub')).toContain('não está mais na pasta de origem');
    expect(explainTransferError('the original file changed since it was catalogued')).toContain('Varra a pasta de novo');
    expect(explainTransferError('the destination is already taken: Autor/Obra/x.epub')).toContain('Nada foi sobrescrito');
    expect(explainTransferError('the path is not a safe relative path')).toContain('não é seguro');
    expect(explainTransferError('the file is not in a referenced location')).toContain('já tenha sido movido');
    expect(explainTransferError('not a referenced file that is available')).toContain('já tenha sido movido');
    expect(explainTransferError('another file of this work is being moved: ask again when it ends')).toContain('Outro arquivo desta obra');
    expect(explainTransferError('could not queue the transfer')).toContain('fila');
  });

  it('does not mind the case or what the server added after the reason', () => {
    expect(explainTransferError('The Original File Is Missing: x')).toContain('não está mais');
    expect(explainTransferError('  the original file is missing  ')).toContain('não está mais');
  });

  it('shows a reason it does not know as it came, and says something when there is none', () => {
    expect(explainTransferError('disk on fire')).toBe('disk on fire');
    expect(explainTransferError('')).toBe('Não foi possível mover o arquivo.');
    expect(explainTransferError(null)).toBe('Não foi possível mover o arquivo.');
    expect(explainTransferError(undefined)).toBe('Não foi possível mover o arquivo.');
  });
});

describe('transferBadge', () => {
  it('has nothing to say without a transfer, or one that ended well', () => {
    expect(transferBadge(null)).toBeNull();
    expect(transferBadge(undefined)).toBeNull();
    expect(transferBadge({ state: 'succeeded' })).toBeNull();
    expect(transferBadge({ state: 'cancelled' })).toBeNull();
  });

  it('says it is waiting, running, or failed and why', () => {
    expect(transferBadge({ state: 'pending' })).toEqual({ tone: 'info', text: 'na fila para mover' });
    expect(transferBadge({ state: 'running' })).toEqual({ tone: 'info', text: 'sendo movido…' });
    expect(transferBadge({ state: 'failed', lastError: 'the original file is missing: x' })).toEqual({
      tone: 'error',
      text: 'a última tentativa falhou: O original não está mais na pasta de origem.',
    });
  });
});

describe('canMove', () => {
  it('is for a file that is there and that nothing is moving', () => {
    expect(canMove({ state: 'ok', transfer: null })).toBe(true);
    expect(canMove({ state: 'ok' })).toBe(true);
    expect(canMove({ state: 'ok', transfer: { state: 'failed' } })).toBe(true); // a failed one may be asked again
    expect(canMove({ state: 'ok', transfer: { state: 'pending' } })).toBe(false);
    expect(canMove({ state: 'ok', transfer: { state: 'running' } })).toBe(false);
    expect(canMove({ state: 'missing' })).toBe(false);
    expect(canMove({ state: 'conflict' })).toBe(false);
  });
});

describe('the words for the states', () => {
  it('has a label for each state and a hint for those that need one', () => {
    expect(STATE_LABEL).toMatchObject({ ok: 'no disco', missing: 'ausente do disco', conflict: 'mudou depois de catalogado' });
    expect(STATE_HINT.ok).toBeUndefined();
    expect(STATE_HINT.missing).toContain('Varra a pasta');
    expect(STATE_HINT.conflict).toContain('Varra a pasta');
  });
});

describe('explainCleanupReason', () => {
  it('says in Portuguese why an original stayed, with what the system said after it', () => {
    expect(explainCleanupReason('the original could not be removed: remove /a/b.epub: permission denied')).toBe('Não foi possível apagar o original: remove /a/b.epub: permission denied');
    expect(explainCleanupReason('could not read the original: open /a: no such file')).toBe('Não foi possível ler o original para conferi-lo: open /a: no such file');
  });

  it('says the ones that need no detail, whole', () => {
    expect(explainCleanupReason('the original changed after it was copied; it was kept')).toBe('O original mudou depois de copiado, então foi mantido.');
    expect(explainCleanupReason('the original no longer has the content that was copied; it was kept')).toBe('O original não tem mais o conteúdo que foi copiado, então foi mantido.');
    expect(explainCleanupReason('waiting to be removed')).toBe('Aguardando para ser apagado.');
  });

  it('shows a reason it does not know as it came, and nothing for nothing', () => {
    expect(explainCleanupReason('disk on fire')).toBe('disk on fire');
    expect(explainCleanupReason('')).toBe('');
    expect(explainCleanupReason(null)).toBe('');
    expect(explainCleanupReason('   ')).toBe('');
  });

  it('does not mind the case', () => {
    expect(explainCleanupReason('The Original Could Not Be Removed: x')).toBe('Não foi possível apagar o original: x');
  });
});
