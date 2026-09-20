import { describe, it, expect } from 'vitest';
import { otherVersionsInProgress } from './finishPrompt';

const work = {
  editions: [
    { id: 1, language: 'pt', files: [
      { id: 10, format: 'epub', availability: 'available', started: true, completed: true },
      { id: 11, format: 'pdf', availability: 'available', started: true, completed: false, percentComplete: 42 },
    ] },
    { id: 2, language: 'en', files: [
      { id: 20, format: 'epub', availability: 'available', started: false, completed: false },
      { id: 21, format: 'cbz', availability: 'missing', started: true, completed: false },
      { id: 22, format: 'mp3', availability: 'available', started: true, completed: false, percentComplete: 10 },
    ] },
  ],
};

describe('otherVersionsInProgress', () => {
  it('lists the versions that were begun and are not finished, except the one just finished', () => {
    const others = otherVersionsInProgress(work, 10);
    expect(others.map((o) => o.file.id)).toEqual([11, 22]);
    expect(others[0].edition.language).toBe('pt');
  });

  it('leaves out versions never begun, finished ones and files that are gone', () => {
    expect(otherVersionsInProgress(work, 11).map((o) => o.file.id)).toEqual([22]);
    expect(otherVersionsInProgress({ editions: [] }, 1)).toEqual([]);
    expect(otherVersionsInProgress(undefined, 1)).toEqual([]);
  });
});
