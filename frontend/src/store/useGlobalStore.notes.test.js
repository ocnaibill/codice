import { describe, it, expect, beforeEach } from 'vitest';
import { useGlobalStore } from './useGlobalStore';

const state = () => useGlobalStore.getState();
beforeEach(() => useGlobalStore.setState({ notesOpen: false, adminOpen: false, searchQuery: '', activeBookId: null, sheetWorkId: null }));

describe('the screen of every note (#13)', () => {
  it('opens over the library and puts away whatever else was on screen', () => {
    useGlobalStore.setState({ adminOpen: true, searchQuery: 'duna', activeBookId: 4, sheetWorkId: 4 });
    state().openNotes();
    expect(state()).toMatchObject({ notesOpen: true, adminOpen: false, searchQuery: '', activeBookId: null, sheetWorkId: null });
  });

  it('is left by going to a library view, to the administration, or by searching', () => {
    for (const leave of [() => state().setLibraryView('all'), () => state().openAdmin(), () => state().setSearchQuery('duna')]) {
      state().openNotes();
      leave();
      expect(state().notesOpen).toBe(false);
    }
  });

  it('stays while the search is cleared, and the sheet of a work opens over it', () => {
    state().openNotes();
    state().setSearchQuery('   ');
    expect(state().notesOpen).toBe(true);
    state().openWork(9);
    expect(state()).toMatchObject({ notesOpen: true, sheetWorkId: 9 });
  });

  it('opening a book from it and closing the book returns to it, while closing a book opened from the library returns to the library', () => {
    state().openNotes();
    state().openBook(3, 30);
    expect(state()).toMatchObject({ activeBookId: 3, notesOpen: true });
    state().closeBook();
    expect(state()).toMatchObject({ activeBookId: null, notesOpen: true });
    state().setLibraryView('all');
    state().openBook(3, 30);
    state().closeBook();
    expect(state()).toMatchObject({ activeBookId: null, notesOpen: false });
  });
});

describe('the place a book is opened at (#14)', () => {
  it('keeps what asked for it, for when the place cannot be opened', () => {
    state().openBook(3, 30, { locator: { type: 'pdf', page: 11 }, context: { kind: 'note', quote: 'Fear' } });
    expect(state().seek).toMatchObject({ locator: { type: 'pdf', page: 11 }, context: { kind: 'note', quote: 'Fear' }, n: 1 });
    state().openBook(3, 30, { locator: { type: 'pdf', page: 11 } });
    expect(state().seek).toMatchObject({ context: null, n: 2 });
  });

  it('has no place when none was asked for', () => {
    state().openBook(3, 30, { context: { kind: 'note' } });
    expect(state().seek).toBeNull();
  });

  it('forgets the place asked for, to open at the saved position instead, and leaves the book open', () => {
    state().openBook(3, 30, { locator: { type: 'pdf', page: 11 } });
    state().clearSeek();
    expect(state()).toMatchObject({ seek: null, activeBookId: 3, activeFileId: 30 });
  });
});
