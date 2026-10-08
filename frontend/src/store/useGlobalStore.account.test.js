import { describe, it, expect, beforeEach } from 'vitest';
import { useGlobalStore } from './useGlobalStore';

const store = useGlobalStore;
beforeEach(() => store.setState({ accountDialogs: [], adminOpen: false, adminTab: 'jobs' }));

describe('the dialogs of the account (#182)', () => {
  it('has none open at first', () => {
    expect(store.getState().accountDialogs).toEqual([]);
  });

  it('opens one over the other, in the order asked, and each only once', () => {
    store.getState().openAccountDialog('sessoes');
    store.getState().openAccountDialog('aplicativos');
    store.getState().openAccountDialog('sessoes');
    expect(store.getState().accountDialogs).toEqual(['sessoes', 'aplicativos']);
  });

  it('closes the one named and leaves the others, and closing one that is not open changes nothing', () => {
    store.getState().openAccountDialog('sessoes');
    store.getState().openAccountDialog('aplicativos');
    store.getState().closeAccountDialog('sessoes');
    expect(store.getState().accountDialogs).toEqual(['aplicativos']);
    const before = store.getState().accountDialogs;
    store.getState().closeAccountDialog('senha');
    expect(store.getState().accountDialogs).toEqual(['aplicativos']);
    expect(store.getState().accountDialogs).toBe(before);
    store.getState().openAccountDialog('aplicativos'); // already open
    expect(store.getState().accountDialogs).toBe(before);
  });

  it('does not tell anybody when nothing changed', () => {
    let told = 0;
    const stop = store.subscribe(() => { told += 1; });
    store.getState().closeAccountDialog('senha');
    store.getState().openAccountDialog('senha');
    expect(told).toBe(1);
    store.getState().openAccountDialog('senha');
    expect(told).toBe(1);
    stop();
  });
});

describe('the tab of the administration (#182)', () => {
  it('is set on its own, and the administration opens on the first tab again', () => {
    store.getState().openAdmin();
    store.getState().setAdminTab('storage');
    expect(store.getState().adminTab).toBe('storage');
    store.getState().closeBook();
    store.getState().openAdmin();
    expect(store.getState()).toMatchObject({ adminOpen: true, adminTab: 'jobs' });
  });
});

describe('opening a collection (#187)', () => {
  it('opens it in the place of the sheet of a work and of a person that opened it, in one change', () => {
    store.setState({ sheetWorkId: 7, personSheetId: 3, collectionSheetId: null });
    let changes = 0;
    const stop = store.subscribe(() => { changes += 1; });
    store.getState().openCollection(4);
    stop();
    expect(store.getState()).toMatchObject({ collectionSheetId: 4, sheetWorkId: null, personSheetId: null });
    expect(changes).toBe(1);
  });

  it('is still over what is under it when a sheet opens after it', () => {
    store.getState().openCollection(4);
    store.getState().openWork(9);
    expect(store.getState()).toMatchObject({ collectionSheetId: 4, sheetWorkId: 9 });
  });
});

