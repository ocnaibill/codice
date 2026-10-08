import { hashFromRoute, isNewPlace, routeFromHash, routeFromState, stateFromRoute } from './route';

/**
 * Keeps the address of the page and the screens of the store the same thing (#182, DEC-135): what the address says is put on
 * screen when this starts (F5) and when the person goes back or forward, and a change of the screens is written to the address.
 *
 * What opens over a screen, the reader and another area are new places of the history, so the back button leaves them one at
 * a time. Closing one goes back, in the history, to the place that was before it when this session has been there, so that
 * closing a book and then pressing back does not open the book again. The shelf, the page and the order only replace the
 * address. Stopping it takes the address off, for the screen of login. What the page knows of the history is kept in the
 * session of the tab, so that F5 does not make it forget: closing a screen after F5 still goes back.
 *
 * `store` is the zustand store of the screens, `win` the window (the tests give their own).
 */
export function startRouteSync(store, win = window) {
  const { history, location } = win;
  const normal = (hash) => hashFromRoute(routeFromHash(hash));
  const here = () => normal(location.hash);
  const onScreen = () => routeFromState(store.getState());

  // The hash of each place of this session, by its place in the history: only those this tab has seen are known.
  let index = Number.isInteger(history.state?.codiceAt) ? history.state.codiceAt : 0;
  const entries = remembered(win, index, here());
  const keep = () => remember(win, entries);
  entries[index] = here();
  history.replaceState({ ...history.state, codiceAt: index }, '');
  store.setState(stateFromRoute(routeFromHash(location.hash)));

  // A step back asked for is waiting for the browser to do it; the screens it ends on are the ones already shown.
  let going = null;

  // Writes to the address what the store shows now, when the address says something else.
  const sync = () => {
    const to = onScreen();
    const hash = hashFromRoute(to);
    if (hash === here()) return;
    if (isNewPlace(routeFromHash(location.hash), to)) {
      for (let before = index - 1; before >= 0; before -= 1) {
        if (entries[before] === hash) {
          going = setTimeout(() => { going = null; }, 1000); // if the browser does not answer, do not stay deaf
          history.go(before - index);
          return;
        }
      }
      index += 1;
      entries[index] = hash;
      history.pushState({ codiceAt: index }, '', hash);
    } else {
      entries[index] = hash;
      history.replaceState({ codiceAt: index }, '', hash);
    }
    keep();
  };
  const unsubscribe = store.subscribe(() => {
    if (!going) sync();
  });

  const onPop = (event) => {
    const ours = !!going; // the step back this asked for, which ends on the screens it already shows
    if (going) {
      clearTimeout(going);
      going = null;
    }
    if (Number.isInteger(event.state?.codiceAt)) {
      index = event.state.codiceAt;
    } else {
      // An address written by hand is a place after this one.
      index += 1;
      history.replaceState({ codiceAt: index }, '');
    }
    entries[index] = here();
    if (ours) sync(); // something may have changed while the browser went back
    else if (hashFromRoute(onScreen()) !== here()) store.setState(stateFromRoute(routeFromHash(location.hash)));
  };
  win.addEventListener('popstate', onPop);

  // `keepAddress` is for a page that is going to be opened again with the same address (the tests do it for F5).
  return ({ keepAddress = false } = {}) => {
    unsubscribe();
    win.removeEventListener('popstate', onPop);
    if (going) clearTimeout(going);
    if (keepAddress) return;
    history.replaceState(null, '', location.pathname + location.search);
    forget(win);
  };
}

const KEY = 'codice:route';

// What this tab knew of its history, if it still agrees with the address: the place it is at must be the one that was kept.
function remembered(win, index, hash) {
  try {
    const kept = JSON.parse(win.sessionStorage.getItem(KEY));
    if (Array.isArray(kept) && kept[index] === hash) return kept;
  } catch {
    // No storage: the page knows only what it sees.
  }
  return [];
}

function remember(win, entries) {
  try {
    win.sessionStorage.setItem(KEY, JSON.stringify(Array.from(entries, (entry) => entry ?? null)));
  } catch {
    // Full or blocked: the history is followed all the same, F5 only forgets it.
  }
}

function forget(win) {
  try {
    win.sessionStorage.removeItem(KEY);
  } catch {
    // Nothing to forget.
  }
}
