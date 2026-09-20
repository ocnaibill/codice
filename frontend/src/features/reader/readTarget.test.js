import { describe, it, expect } from 'vitest';
import { readLabel, readTarget } from './readTarget';

const work = (over = {}) => ({ id: 7, fileCount: 1, inProgress: false, continue: null, ...over });

describe('readTarget', () => {
  it('goes straight to the version that counts when the work is in progress', () => {
    const w = work({ fileCount: 3, inProgress: true, continue: { fileId: 22, completed: false } });
    expect(readTarget(w)).toEqual({ kind: 'file', fileId: 22 });
    expect(readLabel(w)).toBe('Continuar de onde parou');
  });

  it('opens the sheet the first time when there is something to choose', () => {
    expect(readTarget(work({ fileCount: 2 }))).toEqual({ kind: 'sheet' });
    expect(readLabel(work({ fileCount: 2 }))).toBe('Escolher versão para ler');
  });

  it('opens the only file of a work that has one, when nothing has been read', () => {
    expect(readTarget(work())).toEqual({ kind: 'file', fileId: null });
    expect(readLabel(work())).toBe('Ler');
  });

  it('opens the sheet of a work that was finished, so the count and "read again" are there', () => {
    const finished = work({ continue: { fileId: 5, completed: true } });
    expect(readTarget(finished)).toEqual({ kind: 'sheet' });
  });

  it('opens the sheet when the whole work was marked finished, even with a version half way', () => {
    const w = work({ fileCount: 2, inProgress: false, finished: true, continue: { fileId: 5, completed: false } });
    expect(readTarget(w)).toEqual({ kind: 'sheet' });
  });

  it('is safe for a card from an older server, with no file count', () => {
    expect(readTarget({ id: 1 })).toEqual({ kind: 'file', fileId: null });
  });
});
