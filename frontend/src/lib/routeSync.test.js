import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { useGlobalStore } from '../store/useGlobalStore';
import { startRouteSync } from './routeSync';

const store = useGlobalStore;
const settle = () => new Promise((resolve) => setTimeout(resolve, 40));
const hash = () => window.location.hash;
const pick = () => {
  const s = store.getState();
  return {
    view: s.libraryView, page: s.libraryPage, sort: s.librarySort, sheet: s.sheetWorkId, collection: s.collectionSheetId,
    person: s.personSheetId, book: s.activeBookId, file: s.activeFileId, notes: s.notesOpen, admin: s.adminOpen,
  };
};
const blank = {
  view: 'all', page: 1, sort: 'added', sheet: null, collection: null, person: null, book: null, file: null, notes: false, admin: false,
};

let stop;
const start = () => {
  stop = startRouteSync(store, window);
};

beforeEach(() => {
  window.history.replaceState(null, '', '/');
  store.setState({
    libraryView: 'all', libraryPage: 1, librarySort: 'added', sheetWorkId: null, collectionSheetId: null, personSheetId: null,
    activeBookId: null, activeFileId: null, notesOpen: false, adminOpen: false, fromStart: false, seek: null, searchQuery: '',
  });
});
afterEach(() => {
  stop?.();
  stop = null;
});

describe('startRouteSync: the address is the source of the screens (#182)', () => {
  it('puts on screen what the address says, as F5 does, and leaves the address as it is', async () => {
    window.history.replaceState(null, '', '/#/mangas?p=2&ordem=title&obra=12');
    start();
    expect(pick()).toEqual({ ...blank, view: 'mangas', page: 2, sort: 'title', sheet: 12 });
    expect(hash()).toBe('#/mangas?p=2&ordem=title&obra=12');
  });

  it('opens the reader of a work and its file from the address, from the saved position', async () => {
    store.setState({ fromStart: true, seek: { n: 1 } });
    window.history.replaceState(null, '', '/#/livros?leitor=7&arquivo=33');
    start();
    expect(pick()).toEqual({ ...blank, view: 'ebooks', book: 7, file: 33 });
    expect(store.getState().fromStart).toBe(false);
    expect(store.getState().seek).toBeNull();
  });

  it('opens the library when the address says nothing, whatever the store held', () => {
    store.setState({ libraryView: 'audio', libraryPage: 4, sheetWorkId: 3 });
    start();
    expect(pick()).toEqual(blank);
    expect(hash()).toBe('');
  });

  it('reads the name of a place with any capitals', () => {
    window.history.replaceState(null, '', '/#/MANGAS?P=2');
    start();
    expect(pick()).toEqual({ ...blank, view: 'mangas' });
  });

  it('opens the notes and the administration from the address', () => {
    window.history.replaceState(null, '', '/#/notas');
    start();
    expect(pick().notes).toBe(true);
    stop();
    window.history.replaceState(null, '', '/#/admin');
    start();
    expect(pick().admin).toBe(true);
  });

  it('leaves what the address does not own alone', () => {
    store.setState({ searchQuery: 'duna' });
    window.history.replaceState(null, '', '/#/mangas');
    start();
    expect(store.getState().searchQuery).toBe('duna');
  });
});

describe('startRouteSync: what the person does is written to the address', () => {
  it('makes a place of what opens over a screen, and the back button closes it', async () => {
    start();
    const before = window.history.length;
    store.getState().openWork(12);
    expect(hash()).toBe('#/acervo?obra=12');
    expect(window.history.length).toBe(before + 1);
    window.history.back();
    await settle();
    expect(pick()).toEqual(blank);
    expect(hash()).toBe('');
    window.history.forward();
    await settle();
    expect(pick().sheet).toBe(12);
    expect(hash()).toBe('#/acervo?obra=12');
  });

  it('only replaces the address when the shelf, the page or the order change', async () => {
    start();
    const before = window.history.length;
    store.getState().setLibraryView('mangas');
    store.getState().setLibrarySort('title'); // the order takes the person back to the first page
    store.getState().setLibraryPage(3);
    expect(hash()).toBe('#/mangas?p=3&ordem=title');
    expect(window.history.length).toBe(before);
  });

  it('goes back in the history when what was opened is closed, instead of piling up places', async () => {
    start();
    const before = window.history.length;
    store.getState().openWork(12);
    store.getState().closeSheet();
    await settle();
    expect(pick()).toEqual(blank);
    expect(hash()).toBe('');
    expect(window.history.length).toBe(before + 1); // the sheet is still ahead, for the forward button, and nothing was added
    window.history.forward();
    await settle();
    expect(pick().sheet).toBe(12);
  });

  it('goes on writing the address after it went back', async () => {
    start();
    store.getState().openWork(12);
    store.getState().closeSheet();
    await settle();
    store.getState().openWork(5);
    expect(hash()).toBe('#/acervo?obra=5');
    store.getState().setLibraryPage(2);
    expect(hash()).toBe('#/acervo?p=2&obra=5');
  });

  it('keeps what changed while the browser was going back', async () => {
    start();
    store.getState().openWork(12);
    store.getState().closeSheet();
    store.getState().setLibraryView('mangas'); // before the browser answers
    await settle();
    expect(pick()).toEqual({ ...blank, view: 'mangas' });
    expect(hash()).toBe('#/mangas');
  });

  it('closes the book to the place before the collection it was opened from', async () => {
    start();
    store.getState().setLibraryView('mangas');
    store.getState().openCollection(4);
    expect(hash()).toBe('#/mangas?colecao=4');
    store.getState().openBook(7);
    expect(hash()).toBe('#/mangas?leitor=7');
    store.getState().closeBook();
    await settle();
    // The history has no place that says "the library with nothing open" after the book: it went back to the one before the collection.
    expect(hash()).toBe('#/mangas');
    expect(pick()).toEqual({ ...blank, view: 'mangas' });
    window.history.back();
    await settle();
    expect(pick().book).toBeNull();
  });

  it('opens the sheet of a work over a collection, and back closes the sheet and leaves the collection', async () => {
    start();
    store.getState().openCollection(4);
    store.getState().openWork(12);
    expect(hash()).toBe('#/acervo?obra=12&colecao=4');
    window.history.back();
    await settle();
    expect(pick()).toEqual({ ...blank, collection: 4 });
    window.history.back();
    await settle();
    expect(pick()).toEqual(blank);
  });

  it('makes a place of the notes and of the administration', async () => {
    start();
    store.getState().openNotes();
    expect(hash()).toBe('#/notas');
    store.getState().openAdmin();
    expect(hash()).toBe('#/admin');
    window.history.back();
    await settle();
    expect(pick().notes).toBe(true);
    expect(pick().admin).toBe(false);
  });

  it('writes the file of the book that is read, and a change of file is a place', async () => {
    start();
    store.getState().openBook(7, 33);
    expect(hash()).toBe('#/acervo?leitor=7&arquivo=33');
    const before = window.history.length;
    store.getState().openBook(7, 34);
    expect(hash()).toBe('#/acervo?leitor=7&arquivo=34');
    expect(window.history.length).toBe(before + 1);
  });

  it('does nothing about what the address does not carry', () => {
    start();
    const before = window.history.length;
    store.getState().setSearchQuery('duna');
    store.getState().openUploadModal();
    expect(hash()).toBe('');
    expect(window.history.length).toBe(before);
  });
});

describe('startRouteSync: the history of this page', () => {
  it('follows the back button to places it did not make, once F5 lost what it knew', async () => {
    start();
    store.getState().openWork(1);
    store.getState().openWork(2);
    stop({ keepAddress: true });
    // F5, in a tab whose session lost what the page knew (a private window, a blocked storage).
    window.sessionStorage.clear();
    start();
    expect(pick().sheet).toBe(2);
    window.history.back();
    await settle();
    expect(pick().sheet).toBe(1);
    window.history.back();
    await settle();
    expect(pick().sheet).toBe(null);
  });

  it('keeps what it knew of the history through F5: closing a screen still goes back', async () => {
    start();
    store.getState().openCollection(4);
    const before = window.history.length;
    stop({ keepAddress: true });
    // F5: the page is new, the address and the session of the tab are not.
    store.setState({ collectionSheetId: null });
    start();
    expect(pick().collection).toBe(4);
    store.getState().closeCollection();
    await settle();
    expect(pick()).toEqual(blank);
    expect(hash()).toBe('');
    expect(window.history.length).toBe(before);
  });

  it('forgets it when it stops, and does not stumble on a session that says something else', () => {
    start();
    store.getState().openWork(1);
    stop();
    stop = null;
    expect(window.sessionStorage.getItem('codice:route')).toBeNull();
    window.sessionStorage.setItem('codice:route', 'not json');
    start();
    expect(pick()).toEqual(blank);
    stop();
    window.sessionStorage.setItem('codice:route', JSON.stringify(['#/livros', '#/mangas']));
    window.history.replaceState({ codiceAt: 1 }, '', '/#/acervo');
    start();
    store.getState().openWork(3);
    expect(hash()).toBe('#/acervo?obra=3'); // it did not trust what disagreed with the address
  });

  it('does not go back to a place that only a session that disagreed with the address remembered', () => {
    start();
    stop();
    stop = null;
    window.sessionStorage.setItem('codice:route', JSON.stringify(['#/mangas?obra=9', '#/zzz']));
    window.history.replaceState({ codiceAt: 1 }, '', '/#/acervo');
    start();
    const before = window.history.length;
    store.getState().setLibraryView('mangas');
    store.getState().openWork(9);
    expect(hash()).toBe('#/mangas?obra=9');
    expect(window.history.length).toBe(before + 1); // a place was made: it did not go back to the one it never saw
  });

  it('keeps an address written by hand through F5 too', async () => {
    start();
    window.location.hash = '#/livros';
    await settle();
    stop({ keepAddress: true });
    store.setState({ libraryView: 'all' });
    start();
    const before = window.history.length;
    store.getState().openWork(5);
    store.getState().closeSheet();
    await settle();
    expect(hash()).toBe('#/livros');
    expect(window.history.length).toBe(before + 1);
  });

  it('takes an address written by hand as a place', async () => {
    start();
    window.location.hash = '#/livros?p=2';
    await settle();
    expect(pick()).toEqual({ ...blank, view: 'ebooks', page: 2 });
    store.getState().openWork(5);
    expect(hash()).toBe('#/livros?p=2&obra=5');
  });

  it('stops when told to: the address goes, and the screens are no longer followed', async () => {
    start();
    store.getState().openWork(12);
    stop();
    stop = null;
    expect(hash()).toBe('');
    store.getState().openWork(13);
    expect(hash()).toBe('');
    window.history.back();
    await settle();
    expect(store.getState().sheetWorkId).toBe(13);
  });
});

// A window whose history only goes where it is told, to see what the sync does while the browser has not answered yet.
function fakeWindow(hash = '') {
  const listeners = [];
  const calls = [];
  const places = [{ state: null, hash }];
  let at = 0;
  const location = { pathname: '/', search: '', hash };
  const history = {
    get state() { return places[at].state; },
    pushState: (state, _title, url) => { calls.push(['push', state, url]); places.length = at + 1; places.push({ state, hash: url.slice(url.indexOf('#')) }); at += 1; location.hash = places[at].hash; },
    replaceState: (state, _title, url) => { calls.push(['replace', state, url]); places[at] = { state, hash: url === undefined ? places[at].hash : url.slice(url.indexOf('#')) }; location.hash = places[at].hash; },
    go: (delta) => { calls.push(['go', delta]); },
  };
  const win = { history, location, addEventListener: (_n, fn) => listeners.push(fn), removeEventListener: (_n, fn) => listeners.splice(listeners.indexOf(fn), 1) };
  // What the browser does when it answers: it arrives at a place of the history, or at an address written by hand.
  const arrive = (delta) => { at += delta; location.hash = places[at].hash; listeners.forEach((fn) => fn({ state: places[at].state })); };
  const writeByHand = (next) => { places.length = at + 1; places.push({ state: null, hash: next }); at += 1; location.hash = next; listeners.forEach((fn) => fn({ state: null })); };
  return { win, calls, arrive, writeByHand, history };
}

describe('startRouteSync: while the browser has not answered', () => {
  it('writes nothing to the address until the step back it asked for is done, then writes what changed meanwhile', () => {
    const fake = fakeWindow();
    stop = startRouteSync(store, fake.win);
    store.getState().openWork(12);
    store.getState().closeSheet();
    expect(fake.calls.filter(([kind]) => kind === 'go')).toEqual([['go', -1]]);
    const written = () => fake.calls.filter(([kind]) => kind === 'push' || kind === 'replace').length;
    const before = written();
    store.getState().setLibraryView('mangas'); // before the browser answers
    expect(written()).toBe(before);
    fake.arrive(-1);
    expect(fake.win.location.hash).toBe('#/mangas');
    expect(pick()).toEqual({ ...blank, view: 'mangas' });
  });

  it('numbers an address written by hand as the place after the one it was at', () => {
    const fake = fakeWindow();
    stop = startRouteSync(store, fake.win);
    store.getState().openWork(12); // place 1
    fake.writeByHand('#/livros'); // place 2
    expect(fake.calls.filter(([kind]) => kind === 'replace').at(-1)[1]).toEqual({ codiceAt: 2 });
    store.getState().openWork(5);
    expect(fake.calls.filter(([kind]) => kind === 'push').at(-1)[1]).toEqual({ codiceAt: 3 });
  });
});
