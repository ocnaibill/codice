import { noticeForWorkEvent } from '../features/library/workNotice';

// What the server says through the real-time channel about the works it processes (a work is analysed, is ready, or failed),
// and what the screens do about it. While a library is imported the server says it TWICE FOR EACH WORK, several times a
// second, and every open tab used to refresh the whole home (three lists, the counters, the favorites) at each message: with
// a few thousand works and a few tabs that was dozens of heavy queries a second, enough to hold every connection of the
// database and to leave nobody able to sign in (found in the first real round of tests).
//
// Now a burst is one refresh: the first message refreshes at once (a work that finishes is seen quickly), and what comes
// next is gathered into ONE more refresh at the end of the window. A tab that is not being looked at refreshes nothing: it
// does it when it comes back. The notices are gathered too: a few are said one by one, many are said as a count.

const SUMMARY_FROM = 3; // that many notices of one kind in a window, or more, become one that counts them

export const refreshWindowMs = 4000;

const plural = (n, one, many) => (n === 1 ? one : many);

export function createLibraryEvents({ refresh, notify, openWork, wait = refreshWindowMs, isHidden = () => document.hidden, now = () => Date.now() }) {
  let timer = null;
  let lastRun = -Infinity;
  let needsRefresh = false;
  let ready = [];
  let failed = [];
  let waitingToBeSeen = false;

  const say = (events, summary) => {
    if (events.length === 0) return;
    if (events.length < SUMMARY_FROM) {
      for (const event of events) {
        const notice = noticeForWorkEvent(event, openWork);
        if (notice) notify(notice);
      }
      return;
    }
    notify(summary(events.length));
  };

  const run = () => {
    timer = null;
    if (isHidden()) {
      // Nobody is looking: nothing is asked of the server, and the tab catches up when it is seen again.
      waitingToBeSeen = true;
      return;
    }
    waitingToBeSeen = false;
    lastRun = now();
    if (needsRefresh) {
      needsRefresh = false;
      refresh();
    }
    const readyNow = ready;
    const failedNow = failed;
    ready = [];
    failed = [];
    say(readyNow, (n) => ({ tone: 'success', title: 'Metadados atualizados', message: `${n} ${plural(n, 'obra', 'obras')} ${plural(n, 'ficou pronta', 'ficaram prontas')}.`, key: 'works-ready' }));
    say(failedNow, (n) => ({ tone: 'error', title: 'Não foi possível processar', message: `${n} ${plural(n, 'obra', 'obras')} ${plural(n, 'não pôde', 'não puderam')} ser processada${n === 1 ? '' : 's'}.`, key: 'works-failed' }));
  };

  const schedule = () => {
    if (timer !== null) return;
    const sinceLast = now() - lastRun;
    if (sinceLast >= wait) run();
    else timer = setTimeout(run, wait - sinceLast);
  };

  const onVisible = () => {
    if (!isHidden() && waitingToBeSeen) schedule();
  };
  if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisible);

  return {
    /** A message of the real-time channel. */
    handle(event) {
      if (event?.type === 'WORK_READY') ready.push(event);
      else if (event?.type === 'WORK_ERROR') failed.push(event);
      else if (event?.type !== 'WORK_ANALYZING') return;
      needsRefresh = true;
      schedule();
    },
    dispose() {
      if (timer !== null) clearTimeout(timer);
      timer = null;
      if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisible);
    },
  };
}
