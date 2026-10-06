import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { createLibraryEvents, refreshWindowMs } from './libraryEvents';

let clock;
let hidden;
let refresh;
let notify;
let events;
const make = (extra = {}) => {
  events = createLibraryEvents({ refresh, notify, openWork: vi.fn(), isHidden: () => hidden, now: () => Date.now(), ...extra });
  return events;
};
const ready = (id, title = `Obra ${id}`) => ({ type: 'WORK_READY', work_id: id, title });
const analyzing = (id) => ({ type: 'WORK_ANALYZING', work_id: id });
const failed = (id) => ({ type: 'WORK_ERROR', work_id: id, error: 'broken' });

beforeEach(() => {
  vi.useFakeTimers();
  clock = 1_000_000;
  vi.setSystemTime(clock);
  hidden = false;
  refresh = vi.fn();
  notify = vi.fn();
});
afterEach(() => {
  events?.dispose();
  vi.useRealTimers();
});

describe('a burst of messages of the real-time channel', () => {
  it('refreshes at once for the first message, so that one work that finishes is seen quickly', () => {
    make().handle(analyzing(1));
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('is ONE more refresh at the end of the window, however many messages came inside it', () => {
    make();
    events.handle(analyzing(1));
    for (let i = 2; i <= 200; i += 1) {
      events.handle(i % 2 ? analyzing(i) : ready(i));
      vi.advanceTimersByTime(10);
    }
    expect(refresh).toHaveBeenCalledTimes(1); // the first, not 200
    vi.advanceTimersByTime(refreshWindowMs);
    expect(refresh).toHaveBeenCalledTimes(2); // and the one that gathers the rest
    vi.advanceTimersByTime(refreshWindowMs * 3);
    expect(refresh).toHaveBeenCalledTimes(2); // nothing is left to ask
  });

  it('keeps the rate: at most one refresh per window, for as long as the messages go on', () => {
    make();
    for (let t = 0; t < 60_000; t += 100) {
      events.handle(analyzing(t));
      vi.advanceTimersByTime(100);
    }
    vi.advanceTimersByTime(refreshWindowMs);
    // 60 s in windows of 4 s: about 15, and never the 600 messages
    expect(refresh.mock.calls.length).toBeLessThanOrEqual(Math.ceil(60_000 / refreshWindowMs) + 1);
    expect(refresh.mock.calls.length).toBeGreaterThanOrEqual(14);
  });

  it('refreshes at once again for a message that comes after a quiet window', () => {
    make();
    events.handle(analyzing(1));
    vi.advanceTimersByTime(refreshWindowMs + 1);
    events.handle(analyzing(2));
    expect(refresh).toHaveBeenCalledTimes(2);
  });

  it('counts the window from the last refresh, to the millisecond: at exactly one window it is at once, a little before it is not', () => {
    make();
    events.handle(analyzing(1));
    vi.advanceTimersByTime(refreshWindowMs - 1);
    events.handle(analyzing(2));
    expect(refresh).toHaveBeenCalledTimes(1); // one millisecond short: it waits
    vi.advanceTimersByTime(1);
    expect(refresh).toHaveBeenCalledTimes(2); // and it comes right when the window ends
    vi.advanceTimersByTime(refreshWindowMs);
    events.handle(analyzing(3));
    expect(refresh).toHaveBeenCalledTimes(3); // after the window, at once
  });

  it('waits only what is left of the window, not a whole window more', () => {
    make();
    events.handle(analyzing(1)); // at once, at t = 0
    vi.advanceTimersByTime(3000);
    events.handle(analyzing(2)); // 3 s later: there is 1 s of the window left
    vi.advanceTimersByTime(999);
    expect(refresh).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(1);
    expect(refresh).toHaveBeenCalledTimes(2);
  });

  it('does not refresh for a message that is not about a work', () => {
    make();
    events.handle({ type: 'SOMETHING_ELSE' });
    events.handle(null);
    events.handle(undefined);
    vi.advanceTimersByTime(refreshWindowMs * 2);
    expect(refresh).not.toHaveBeenCalled();
  });
});

describe('a tab that is not being looked at', () => {
  it('asks nothing of the server, and catches up when it is seen again', () => {
    hidden = true;
    make();
    events.handle(analyzing(1)); // the first one is also held back
    events.handle(analyzing(2));
    vi.advanceTimersByTime(refreshWindowMs * 3);
    expect(refresh).not.toHaveBeenCalled();
    hidden = false;
    document.dispatchEvent(new Event('visibilitychange'));
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('does not catch up when it comes back and nothing happened', () => {
    make();
    document.dispatchEvent(new Event('visibilitychange'));
    expect(refresh).not.toHaveBeenCalled();
  });

  it('stays quiet through a visibility change while it is still hidden', () => {
    hidden = true;
    make();
    events.handle(analyzing(1));
    vi.advanceTimersByTime(refreshWindowMs);
    document.dispatchEvent(new Event('visibilitychange'));
    expect(refresh).not.toHaveBeenCalled();
  });

  it('keeps the notices that came while it was hidden, and says them when it is seen', () => {
    hidden = true;
    make();
    events.handle(ready(7, 'Duna'));
    vi.advanceTimersByTime(refreshWindowMs);
    expect(notify).not.toHaveBeenCalled();
    hidden = false;
    document.dispatchEvent(new Event('visibilitychange'));
    expect(notify).toHaveBeenCalledTimes(1);
    expect(notify.mock.calls[0][0].message).toBe('Duna');
  });
});

describe('the notices', () => {
  it('says a work that is ready as it did, at once, with the way to open it', () => {
    make().handle(ready(7, 'Duna'));
    expect(notify).toHaveBeenCalledTimes(1);
    const notice = notify.mock.calls[0][0];
    expect(notice).toMatchObject({ tone: 'success', title: 'Metadados atualizados', message: 'Duna', key: 'work-7' });
    expect(notice.action.label).toBe('Ver obra');
  });

  it('says a few one by one, and many as a count', () => {
    make();
    events.handle(ready(1)); // at once
    events.handle(ready(2));
    events.handle(ready(3));
    expect(notify).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(refreshWindowMs);
    // 2 inside the window: below the line, said one by one
    expect(notify).toHaveBeenCalledTimes(3);
    for (let i = 10; i < 25; i += 1) events.handle(ready(i));
    vi.advanceTimersByTime(refreshWindowMs);
    const last = notify.mock.calls.at(-1)[0];
    expect(last).toMatchObject({ tone: 'success', key: 'works-ready' });
    expect(last.message).toBe('15 obras ficaram prontas.'); // all of them came inside the window that had just opened
    expect(last.action).toBeUndefined();
  });

  it('says exactly three as a count, and exactly two one by one', () => {
    make();
    events.handle(ready(1)); // at once, opens the window
    notify.mockClear();
    events.handle(ready(2));
    events.handle(ready(3));
    vi.advanceTimersByTime(refreshWindowMs);
    expect(notify.mock.calls.map(([n]) => n.key)).toEqual(['work-2', 'work-3']);
    notify.mockClear();
    events.handle(ready(4));
    events.handle(ready(5));
    events.handle(ready(6));
    vi.advanceTimersByTime(refreshWindowMs);
    expect(notify.mock.calls.map(([n]) => n.key)).toEqual(['works-ready']);
    expect(notify.mock.calls[0][0].message).toBe('3 obras ficaram prontas.');
  });

  it('says the failures the same way, apart from the ready ones', () => {
    make();
    events.handle(failed(1));
    for (let i = 2; i <= 6; i += 1) events.handle(failed(i));
    events.handle(ready(100));
    vi.advanceTimersByTime(refreshWindowMs);
    const kinds = notify.mock.calls.map(([n]) => n.key);
    expect(kinds).toContain('works-failed');
    const summary = notify.mock.calls.find(([n]) => n.key === 'works-failed')[0];
    expect(summary).toMatchObject({ tone: 'error', title: 'Não foi possível processar' });
    expect(summary.message).toBe('5 obras não puderam ser processadas.');
    expect(notify.mock.calls.some(([n]) => n.key === 'work-100')).toBe(true);
  });

  it('says nothing for a work that is only being analysed', () => {
    make();
    events.handle(analyzing(1));
    vi.advanceTimersByTime(refreshWindowMs);
    expect(notify).not.toHaveBeenCalled();
  });
});

describe('dispose', () => {
  it('does not catch up for a tab that was hidden when it was disposed of', () => {
    hidden = true;
    make();
    events.handle(analyzing(1));
    events.dispose();
    hidden = false;
    document.dispatchEvent(new Event('visibilitychange'));
    vi.advanceTimersByTime(refreshWindowMs);
    expect(refresh).not.toHaveBeenCalled();
  });

  it('stops the timer and the listening', () => {
    make();
    events.handle(analyzing(1));
    events.handle(analyzing(2));
    events.dispose();
    vi.advanceTimersByTime(refreshWindowMs * 2);
    expect(refresh).toHaveBeenCalledTimes(1);
    hidden = false;
    document.dispatchEvent(new Event('visibilitychange'));
    expect(refresh).toHaveBeenCalledTimes(1);
  });
});
