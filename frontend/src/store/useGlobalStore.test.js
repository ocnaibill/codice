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
