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
