import { describe, it, expect } from 'vitest';
import { HOME, hashFromRoute, isNewPlace, routeFromHash, routeFromState, stateFromRoute } from './route';

const state = (over = {}) => ({
  adminOpen: false, notesOpen: false, libraryView: 'all', libraryPage: 1, librarySort: 'added',
  sheetWorkId: null, collectionSheetId: null, personSheetId: null, activeBookId: null, activeFileId: null, ...over,
});
const hashOf = (over) => hashFromRoute(routeFromState(state(over)));

describe('the address of the screens (#182)', () => {
  it('is the library when nothing else is open', () => {
    expect(hashOf()).toBe('#/acervo');
    expect(routeFromHash('')).toEqual(HOME);
    expect(routeFromHash('#/')).toEqual(HOME);
    expect(routeFromHash('#/acervo')).toEqual(HOME);
  });

  it('names each shelf in Portuguese, and reads it back', () => {
    for (const [view, word] of [['ebooks', 'livros'], ['comics', 'quadrinhos'], ['mangas', 'mangas'], ['audio', 'audio'], ['reading', 'leitura'], ['favorites', 'favoritos'], ['collections', 'colecoes'], ['lists', 'listas']]) {
      expect(hashOf({ libraryView: view })).toBe(`#/${word}`);
      expect(routeFromHash(`#/${word}`).view).toBe(view);
    }
  });

  it('writes the page and the order only when they are not the first page and the newest first', () => {
    expect(hashOf({ libraryPage: 1 })).toBe('#/acervo');
    expect(hashOf({ libraryPage: 3 })).toBe('#/acervo?p=3');
    expect(hashOf({ librarySort: 'title' })).toBe('#/acervo?ordem=title');
    expect(hashOf({ libraryView: 'mangas', libraryPage: 2, librarySort: 'author' })).toBe('#/mangas?p=2&ordem=author');
    expect(routeFromHash('#/mangas?p=2&ordem=author')).toMatchObject({ view: 'mangas', page: 2, sort: 'author' });
  });

  it('writes what is open over the screen, in a fixed order', () => {
    expect(hashOf({ sheetWorkId: 12 })).toBe('#/acervo?obra=12');
    expect(hashOf({ collectionSheetId: 4, sheetWorkId: 12 })).toBe('#/acervo?obra=12&colecao=4');
    expect(hashOf({ personSheetId: 7 })).toBe('#/acervo?pessoa=7');
    expect(hashOf({ libraryView: 'mangas', activeBookId: 7, activeFileId: 33 })).toBe('#/mangas?leitor=7&arquivo=33');
    expect(routeFromHash('#/mangas?leitor=7&arquivo=33')).toMatchObject({ view: 'mangas', leitor: 7, arquivo: 33 });
    expect(routeFromHash('#/acervo?obra=12&colecao=4')).toMatchObject({ obra: 12, colecao: 4 });
  });

  it('writes the notes and the administration as areas of their own', () => {
    expect(hashOf({ notesOpen: true })).toBe('#/notas');
    expect(hashOf({ adminOpen: true })).toBe('#/admin');
    expect(hashOf({ adminOpen: true, notesOpen: true })).toBe('#/admin');
    expect(routeFromHash('#/notas').area).toBe('notes');
    expect(routeFromHash('#/admin').area).toBe('admin');
    // The page and the order belong to the library.
    expect(hashOf({ notesOpen: true, libraryPage: 4, librarySort: 'title' })).toBe('#/notas');
    expect(routeFromHash('#/admin?p=4&ordem=title')).toMatchObject({ area: 'admin', page: 1, sort: 'added' });
  });

  it('takes a file only with the work being read', () => {
    expect(hashOf({ activeFileId: 33 })).toBe('#/acervo');
    expect(routeFromHash('#/acervo?arquivo=33').arquivo).toBeNull();
  });

  it('reads what is wrong as the default, and never as something else', () => {
    expect(routeFromHash('#/nada')).toEqual(HOME);
    expect(routeFromHash('#/ACERVO')).toEqual(HOME);
    expect(routeFromHash('#/livros?p=0&ordem=nada&obra=abc&colecao=-3&pessoa=1.5&leitor=')).toMatchObject({
      view: 'ebooks', page: 1, sort: 'added', obra: null, colecao: null, pessoa: null, leitor: null,
    });
    expect(routeFromHash('#/livros?p=2.5').page).toBe(1);
    expect(routeFromHash(undefined)).toEqual(HOME);
    expect(routeFromHash(null)).toEqual(HOME);
    expect(hashOf({ libraryView: 'nada', libraryPage: 'x', librarySort: 'y', sheetWorkId: 'z' })).toBe('#/acervo');
  });

  it('goes from the store to the address and back to the same screens', () => {
    for (const over of [
      {}, { libraryView: 'comics', libraryPage: 5 }, { librarySort: 'author', sheetWorkId: 9 }, { collectionSheetId: 2, sheetWorkId: 9 },
      { personSheetId: 3 }, { libraryView: 'favorites', activeBookId: 8, activeFileId: 21 }, { notesOpen: true }, { adminOpen: true },
    ]) {
      const route = routeFromState(state(over));
      expect(routeFromHash(hashFromRoute(route))).toEqual(route);
    }
  });

  it('says what the store is to be set to, with no place asked for and the saved position for the reader', () => {
    expect(stateFromRoute(routeFromHash('#/mangas?p=2&obra=12&leitor=7&arquivo=33'))).toEqual({
      adminOpen: false, notesOpen: false, libraryView: 'mangas', libraryPage: 2, librarySort: 'added',
      sheetWorkId: 12, collectionSheetId: null, personSheetId: null, activeBookId: 7, activeFileId: 33, fromStart: false, seek: null,
    });
    expect(stateFromRoute(HOME)).toMatchObject({ adminOpen: false, notesOpen: false, libraryView: 'all', sheetWorkId: null, activeBookId: null });
    expect(stateFromRoute(routeFromHash('#/admin'))).toMatchObject({ adminOpen: true, notesOpen: false });
  });
});

describe('what is a new place of the history', () => {
  const route = (hash) => routeFromHash(hash);
  it('is what opens over a screen, the reader and another area', () => {
    expect(isNewPlace(route('#/acervo'), route('#/acervo?obra=1'))).toBe(true);
    expect(isNewPlace(route('#/acervo?obra=1'), route('#/acervo?obra=2'))).toBe(true);
    expect(isNewPlace(route('#/acervo'), route('#/acervo?colecao=1'))).toBe(true);
    expect(isNewPlace(route('#/acervo'), route('#/acervo?pessoa=1'))).toBe(true);
    expect(isNewPlace(route('#/acervo'), route('#/acervo?leitor=1'))).toBe(true);
    expect(isNewPlace(route('#/acervo?leitor=1&arquivo=2'), route('#/acervo?leitor=1&arquivo=3'))).toBe(true);
    expect(isNewPlace(route('#/acervo'), route('#/notas'))).toBe(true);
    expect(isNewPlace(route('#/notas'), route('#/admin'))).toBe(true);
    expect(isNewPlace(route('#/acervo?obra=1'), route('#/acervo'))).toBe(true);
  });

  it('is not the shelf, the page or the order', () => {
    expect(isNewPlace(route('#/acervo'), route('#/mangas'))).toBe(false);
    expect(isNewPlace(route('#/acervo'), route('#/acervo?p=2'))).toBe(false);
    expect(isNewPlace(route('#/acervo?p=2'), route('#/acervo?p=2&ordem=title'))).toBe(false);
    expect(isNewPlace(route('#/acervo?obra=1'), route('#/mangas?obra=1'))).toBe(false);
  });
});
