import { describe, it, expect, beforeEach } from 'vitest';
import { useGlobalStore } from './useGlobalStore';

const reset = () => useGlobalStore.getState().closeBook();

describe('reading state', () => {
  beforeEach(reset);

  it('opens the sheet without opening the reader, and the reader without the sheet', () => {
    useGlobalStore.getState().openWork(7);
    expect(useGlobalStore.getState()).toMatchObject({ sheetWorkId: 7, activeBookId: null });
    useGlobalStore.getState().openBook(7, 20);
    expect(useGlobalStore.getState()).toMatchObject({ sheetWorkId: null, activeBookId: 7, activeFileId: 20, fromStart: false, seek: null });
  });

  it('remembers the wish to start over only for that opening', () => {
    useGlobalStore.getState().openBook(7, 20, { fromStart: true });
    expect(useGlobalStore.getState().fromStart).toBe(true);
    useGlobalStore.getState().openBook(7, 20);
    expect(useGlobalStore.getState().fromStart).toBe(false);
  });

  it('counts each request for a place, so asking again reopens the viewer there', () => {
    const place = { type: 'pdf', page: 3 };
    useGlobalStore.getState().openBook(7, 20, { locator: place });
    const first = useGlobalStore.getState().seek;
    useGlobalStore.getState().openBook(7, 20, { locator: place });
    expect(first).toMatchObject({ locator: place, n: 1 });
    expect(useGlobalStore.getState().seek.n).toBe(2);
    useGlobalStore.getState().closeBook();
    expect(useGlobalStore.getState()).toMatchObject({ seek: null, activeBookId: null, activeFileId: null });
  });
});

describe('the page of a collection', () => {
  beforeEach(() => {
    reset();
    useGlobalStore.getState().closeCollection();
  });

  it('opens and closes, and the sheet of a work opens over it without closing it', () => {
    useGlobalStore.getState().openCollection(5);
    expect(useGlobalStore.getState().collectionSheetId).toBe(5);
    useGlobalStore.getState().openWork(7);
    expect(useGlobalStore.getState()).toMatchObject({ collectionSheetId: 5, sheetWorkId: 7 });
    useGlobalStore.getState().closeSheet();
    expect(useGlobalStore.getState().collectionSheetId).toBe(5);
    useGlobalStore.getState().closeCollection();
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
  });

  it('is left behind by changing the shelf and by opening a book', () => {
    useGlobalStore.getState().openCollection(5);
    useGlobalStore.getState().setLibraryView('audio');
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
    useGlobalStore.getState().openCollection(5);
    useGlobalStore.getState().openBook(7, 20);
    expect(useGlobalStore.getState().collectionSheetId).toBeNull();
  });
});

describe('the page of a person', () => {
  beforeEach(() => {
    reset();
    useGlobalStore.getState().closePerson();
  });

  it('opens in place of the sheet of the work it came from, and the sheet of a work opens over it', () => {
    useGlobalStore.getState().openWork(7);
    useGlobalStore.getState().openPerson(3);
    expect(useGlobalStore.getState()).toMatchObject({ personSheetId: 3, sheetWorkId: null });
    useGlobalStore.getState().openWork(8);
    expect(useGlobalStore.getState()).toMatchObject({ personSheetId: 3, sheetWorkId: 8 });
    useGlobalStore.getState().closeSheet();
    expect(useGlobalStore.getState().personSheetId).toBe(3);
    useGlobalStore.getState().closePerson();
    expect(useGlobalStore.getState().personSheetId).toBeNull();
  });

  it('is left behind by changing the shelf and by opening a book', () => {
    useGlobalStore.getState().openPerson(3);
    useGlobalStore.getState().setLibraryView('audio');
    expect(useGlobalStore.getState().personSheetId).toBeNull();
    useGlobalStore.getState().openPerson(3);
    useGlobalStore.getState().openBook(7, 20);
    expect(useGlobalStore.getState().personSheetId).toBeNull();
  });
});

describe('the page of a category (DEC-140)', () => {
  beforeEach(() => useGlobalStore.setState({ categoryPageId: null, libraryPage: 1, libraryView: 'all', adminOpen: false, notesOpen: false, searchQuery: '', sheetWorkId: null }));

  it('opens from the first page, and closes back to the library', () => {
    useGlobalStore.setState({ libraryPage: 4 });
    useGlobalStore.getState().openCategory(7);
    expect(useGlobalStore.getState()).toMatchObject({ categoryPageId: 7, libraryPage: 1 });
    useGlobalStore.setState({ libraryPage: 3 });
    useGlobalStore.getState().closeCategory();
    expect(useGlobalStore.getState()).toMatchObject({ categoryPageId: null, libraryPage: 1 });
  });

  it('takes the place of what was open: a search, the administration, the notes, a sheet, the reader', () => {
    useGlobalStore.setState({ searchQuery: 'duna', adminOpen: true, notesOpen: true, sheetWorkId: 3, activeBookId: 4, activeFileId: 5, collectionSheetId: 6, personSheetId: 7 });
    useGlobalStore.getState().openCategory(7);
    expect(useGlobalStore.getState()).toMatchObject({
      categoryPageId: 7, searchQuery: '', adminOpen: false, notesOpen: false, sheetWorkId: null, activeBookId: null, activeFileId: null, collectionSheetId: null, personSheetId: null,
    });
  });

  it('is closed by choosing a shelf, opening the administration or the notes, and searching', () => {
    for (const leave of [
      (s) => s.setLibraryView('ebooks'),
      (s) => s.openAdmin(),
      (s) => s.openNotes(),
      (s) => s.setSearchQuery('duna'),
    ]) {
      useGlobalStore.getState().openCategory(7);
      leave(useGlobalStore.getState());
      expect(useGlobalStore.getState().categoryPageId).toBeNull();
    }
  });

  it('stays open when the search is emptied, and when a sheet opens over it', () => {
    useGlobalStore.getState().openCategory(7);
    useGlobalStore.getState().setSearchQuery('');
    useGlobalStore.getState().setSearchQuery('   ');
    useGlobalStore.getState().openWork(12);
    expect(useGlobalStore.getState()).toMatchObject({ categoryPageId: 7, sheetWorkId: 12 });
  });
});

describe('the wiki of a work (RF-050)', () => {
  beforeEach(() => useGlobalStore.getState().goHome());

  it('opens in the place of the page of the work, and moves between entities and the graph', () => {
    const s = () => useGlobalStore.getState();
    s().openWork(7);
    s().openWiki(7);
    expect(s()).toMatchObject({ wikiWorkId: 7, wikiEntity: null, wikiGraph: false, sheetWorkId: null });
    s().openWikiEntity('luc');
    expect(s()).toMatchObject({ wikiEntity: 'luc', wikiGraph: false });
    s().openWikiGraph();
    expect(s()).toMatchObject({ wikiEntity: 'luc', wikiGraph: true });
    s().openWikiEntity('bea');
    expect(s()).toMatchObject({ wikiEntity: 'bea', wikiGraph: false });
  });

  it('stays under a book opened from it, and closing the book comes back to it', () => {
    const s = () => useGlobalStore.getState();
    s().openWiki(7, 'luc');
    s().openBook(7, 20, { locator: { type: 'epub', href: 'c1.xhtml' } });
    expect(s()).toMatchObject({ activeBookId: 7, wikiWorkId: 7, wikiEntity: 'luc' });
    s().closeBook();
    expect(s()).toMatchObject({ activeBookId: null, wikiWorkId: 7, wikiEntity: 'luc' });
  });

  it('closes when the person goes elsewhere: home, a shelf, a search, the page of the work', () => {
    const s = () => useGlobalStore.getState();
    for (const leave of [() => s().goHome(), () => s().setLibraryView('ebooks'), () => s().setSearchQuery('duna'), () => s().openWork(9), () => s().openNotes(), () => s().closeWiki()]) {
      s().openWiki(7, 'luc');
      leave();
      expect(s()).toMatchObject({ wikiWorkId: null, wikiEntity: null, wikiGraph: false });
    }
  });
});
