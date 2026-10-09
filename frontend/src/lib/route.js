import { ADMIN_TAB_KEYS, FIRST_ADMIN_TAB } from '../features/admin/tabs';

// The address of a screen (#182, DEC-135): what is open, written after the `#` of the page, so that F5 opens it again and the back
// button of a phone goes back one screen instead of leaving the app. It is a hash, not a path, because the server of the
// container sends every path but "/" to the API: a hash never reaches it, and works behind any proxy the person puts in front.
//
// The state of the screens lives in the global store; this file only translates it, in both directions, and says when a change of
// it is a new place in the history (an overlay, the reader, another area) and when it only adjusts the place the person is at
// (the page, the order, the shelf).

// What each shelf of the library is called in the address.
const VIEW_WORDS = {
  all: 'acervo',
  ebooks: 'livros',
  comics: 'quadrinhos',
  mangas: 'mangas',
  audio: 'audio',
  reading: 'leitura',
  favorites: 'favoritos',
  collections: 'colecoes',
  lists: 'listas',
};
const WORD_VIEWS = Object.fromEntries(Object.entries(VIEW_WORDS).map(([view, word]) => [word, view]));

const SORTS = ['title', 'author'];

// The dialogs of the account, by the name they have in the address (the store keeps these names).
const ACCOUNT_DIALOGS = ['senha', 'preferencias', 'aplicativos', 'sobre', 'sessoes'];

// What the search says is at most this long (the page cuts it there too).
const SEARCH_LENGTH = 200;

// What opens over a screen, by the name it has in the address: the sheet of a work, a collection, a person, and the reader.
const OVERLAYS = [
  ['obra', 'sheetWorkId'],
  ['colecao', 'collectionSheetId'],
  ['pessoa', 'personSheetId'],
  ['leitor', 'activeBookId'],
  ['arquivo', 'activeFileId'],
];

/** The default place: the whole library, first page, newest first, nothing open. */
export const HOME = Object.freeze({
  area: 'library', view: 'all', page: 1, sort: 'added', tab: FIRST_ADMIN_TAB, q: '', conta: [],
  obra: null, colecao: null, pessoa: null, leitor: null, arquivo: null,
  // The category whose page is open (DEC-140): a page of the library, in the place of its shelves.
  categoria: null,
});

/** What the store says is on screen, as a route. */
export function routeFromState(state) {
  const route = { ...HOME };
  const q = searchOf(state.searchQuery);
  // What is on screen wins in this order: the administration, a search (which closes the notes), the notes.
  if (state.adminOpen) route.area = 'admin';
  else if (q) route.area = 'search';
  else if (state.notesOpen) route.area = 'notes';
  if (route.area === 'admin' && ADMIN_TAB_KEYS.includes(state.adminTab)) route.tab = state.adminTab;
  if (route.area === 'search') route.q = q;
  route.conta = dialogsOf(state.accountDialogs);
  if (route.area === 'library') route.categoria = positive(state.categoryPageId);
  route.view = VIEW_WORDS[state.libraryView] ? state.libraryView : 'all';
  route.page = Number.isInteger(state.libraryPage) && state.libraryPage > 1 ? state.libraryPage : 1;
  route.sort = SORTS.includes(state.librarySort) ? state.librarySort : 'added';
  for (const [name, field] of OVERLAYS) route[name] = positive(state[field]);
  // A file is only the file of a work being read.
  if (!route.leitor) route.arquivo = null;
  return route;
}

/** The hash of a route: "#/acervo", "#/categorias/7", "#/mangas?p=2&ordem=title&obra=12", "#/busca?q=duna", "#/notas", "#/admin/storage". */
export function hashFromRoute(route) {
  let word = VIEW_WORDS[route.view] ?? 'acervo';
  if (route.area === 'admin') word = route.tab !== FIRST_ADMIN_TAB ? `admin/${route.tab}` : 'admin';
  else if (route.area === 'search') word = 'busca';
  else if (route.area === 'notes') word = 'notas';
  else if (route.categoria) word = `categorias/${route.categoria}`;
  const params = new URLSearchParams();
  if (route.area === 'library') {
    if (route.page > 1) params.set('p', String(route.page));
    if (SORTS.includes(route.sort)) params.set('ordem', route.sort);
  }
  if (route.area === 'search') params.set('q', route.q);
  for (const [name] of OVERLAYS) if (route[name]) params.set(name, String(route[name]));
  if (route.conta.length > 0) params.set('conta', route.conta.join(','));
  const query = params.toString();
  return `#/${word}${query ? `?${query}` : ''}`;
}

/** The route a hash says, with the default for what it does not say or says wrong; an empty or unknown hash is the library. */
export function routeFromHash(hash) {
  const route = { ...HOME };
  const text = String(hash ?? '').replace(/^#\/?/, '');
  const [path, query = ''] = text.split('?');
  const [word, sub = ''] = path.toLowerCase().split('/');
  const params = new URLSearchParams(query);
  if (word === 'admin') {
    route.area = 'admin';
    if (ADMIN_TAB_KEYS.includes(sub)) route.tab = sub;
  } else if (word === 'busca') {
    // A search that says nothing is not a search.
    const q = searchOf(params.get('q'));
    if (q) Object.assign(route, { area: 'search', q });
  } else if (word === 'notas') route.area = 'notes';
  else if (word === 'categorias') route.categoria = positive(sub);
  else if (WORD_VIEWS[word]) route.view = WORD_VIEWS[word];
  if (route.area === 'library') {
    const page = Number(params.get('p'));
    if (Number.isInteger(page) && page > 1) route.page = page;
    if (SORTS.includes(params.get('ordem'))) route.sort = params.get('ordem');
  }
  for (const [name] of OVERLAYS) route[name] = positive(params.get(name));
  if (!route.leitor) route.arquivo = null;
  route.conta = dialogsOf(String(params.get('conta') ?? '').split(','));
  return route;
}

/** The fields of the store that a route says: what to set so that the screens show it. */
export function stateFromRoute(route) {
  return {
    adminOpen: route.area === 'admin',
    notesOpen: route.area === 'notes',
    libraryView: route.view,
    libraryPage: route.page,
    librarySort: route.sort,
    adminTab: route.tab,
    searchQuery: route.q,
    accountDialogs: route.conta,
    sheetWorkId: route.obra,
    collectionSheetId: route.colecao,
    categoryPageId: route.categoria,
    personSheetId: route.pessoa,
    activeBookId: route.leitor,
    activeFileId: route.arquivo,
    // A reader opened by an address opens where the person was (their saved position), not at a place asked for or at the start.
    fromStart: false,
    seek: null,
  };
}

/**
 * Whether going from one route to the next is a new place in the history. What opens over a screen, the reader, and another
 * area (library, notes, administration) are places: the back button leaves them. The shelf, the page and the order only
 * adjust where the person is, and do not pile up one entry for each click.
 */
export function isNewPlace(from, to) {
  if (from.area !== to.area) return true;
  if (from.conta.join() !== to.conta.join()) return true;
  if (from.categoria !== to.categoria) return true;
  return OVERLAYS.some(([name]) => from[name] !== to[name]);
}

// The dialogs of the account that an address or the store may say, each once, in the order they were opened.
function dialogsOf(names) {
  return [...new Set(names)].filter((name) => ACCOUNT_DIALOGS.includes(name));
}

// What was typed in the search, as the search takes it: without the spaces at the ends and cut at its length.
function searchOf(text) {
  return String(text ?? '').trim().slice(0, SEARCH_LENGTH);
}

function positive(value) {
  const n = Number(value);
  return Number.isInteger(n) && n > 0 ? n : null;
}
