import { useCallback, useReducer, useRef, useState } from 'react';
import { extensionOf, isAccepted } from './accepted';

/** How many files can be on the list at once. */
export const MAX_FILES = 5;

// What each file of the list is going through. A file that is "refused" never goes to the server (its type is not one
// that is taken); "duplicate" is the server saying that it already holds these bytes.
export const STATUS = {
  QUEUED: 'queued',
  UPLOADING: 'uploading',
  DONE: 'done',
  DUPLICATE: 'duplicate',
  ERROR: 'error',
  REFUSED: 'refused',
  CANCELLED: 'cancelled',
};

const sign = (file) => `${file.name}|${file.size}|${file.lastModified}`;

function makeItem(id, file) {
  if (!isAccepted(file.name)) {
    const ext = extensionOf(file.name);
    return { id, file, status: STATUS.REFUSED, progress: 0, message: `Esse tipo de arquivo não é aceito${ext ? ` (.${ext})` : ''}.` };
  }
  return { id, file, status: STATUS.QUEUED, progress: 0, message: '' };
}

/**
 * What adding some files to the list does, without doing it: the new rows, in the order the files came, how many were
 * left out because the list is full (at most MAX_FILES, the first ones stay) and how many were left out because the
 * same file was already on it (same name, size and date).
 */
export function planAdd(items, files, firstId) {
  const added = [];
  const seen = new Set(items.map((item) => sign(item.file)));
  let overCap = 0;
  let repeated = 0;
  for (const file of files) {
    if (seen.has(sign(file))) {
      repeated += 1;
    } else if (items.length + added.length >= MAX_FILES) {
      overCap += 1;
    } else {
      seen.add(sign(file));
      added.push(makeItem(firstId + added.length, file));
    }
  }
  return { added, overCap, repeated };
}

export function queueReducer(items, action) {
  switch (action.type) {
    case 'append':
      return [...items, ...action.items];
    case 'patch':
      return items.map((item) => (item.id === action.id ? { ...item, ...action.patch } : item));
    case 'remove':
      return items.filter((item) => item.id !== action.id);
    case 'dropDone':
      return items.filter((item) => item.status !== STATUS.DONE);
    case 'cancelQueued':
      return items.map((item) => (item.status === STATUS.QUEUED ? { ...item, status: STATUS.CANCELLED } : item));
    case 'reset':
      return [];
    default:
      return items;
  }
}

/** How a row looks at the end of a run: the status it has, or the one this run gave it. */
const emptySummary = () => ({ sent: 0, duplicates: 0, errors: 0, cancelled: 0, firstSent: null, allDone: false, converted: [] });

/** How many rows take a place on the list: the ones that are not sent yet (a sent one is only there to be seen). */
export const placesTaken = (items) => items.filter((item) => item.status !== STATUS.DONE).length;

/**
 * The list of files to send, and the sending of them one after the other: a file starts when the one before it has
 * ended, because they would only fight for the same line and none would show a progress that means something.
 * `upload(file, onProgress)` sends one and answers when it is done; a failure with a 409 is "already in the library".
 *
 * start() sends what is queued (or the files asked for) and answers, when it is over, with what happened:
 * { sent, duplicates, errors, cancelled, firstSent, allDone, converted: [{ name, from }] }. stop() lets the file that is going finish and
 * cancels the ones that have not started.
 */
export function useUploadQueue(upload, describeError) {
  const [items, dispatch] = useReducer(queueReducer, []);
  const [running, setRunning] = useState(false);
  // Which file of the run is going, out of how many: "2 of 5".
  const [position, setPosition] = useState({ index: 0, total: 0 });
  const itemsRef = useRef(items);
  itemsRef.current = items;
  const runningRef = useRef(false);
  const stopRef = useRef(false);
  const nextId = useRef(1);

  const add = useCallback((files) => {
    if (runningRef.current) return { added: 0, overCap: files.length, repeated: 0 };
    // What was sent has done its part: it does not take a place on the list, and goes when a new lot comes.
    const waiting = itemsRef.current.filter((item) => item.status !== STATUS.DONE);
    const plan = planAdd(waiting, files, nextId.current);
    nextId.current += plan.added.length;
    if (plan.added.length) {
      itemsRef.current = [...waiting, ...plan.added];
      dispatch({ type: 'dropDone' });
      dispatch({ type: 'append', items: plan.added });
    }
    return { added: plan.added.length, overCap: plan.overCap, repeated: plan.repeated };
  }, []);

  const remove = useCallback((id) => {
    if (!runningRef.current) dispatch({ type: 'remove', id });
  }, []);

  const reset = useCallback(() => {
    if (!runningRef.current) dispatch({ type: 'reset' });
  }, []);

  const stop = useCallback(() => {
    if (!runningRef.current) return;
    stopRef.current = true;
    dispatch({ type: 'cancelQueued' });
  }, []);

  const start = useCallback(
    async (only) => {
      if (runningRef.current) return emptySummary();
      const ids = only ?? itemsRef.current.filter((item) => item.status === STATUS.QUEUED).map((item) => item.id);
      if (ids.length === 0) return emptySummary();
      runningRef.current = true;
      stopRef.current = false;
      setRunning(true);
      setPosition({ index: 0, total: ids.length });
      const result = emptySummary();
      const final = new Map(); // what each file of this run ended as
      for (let n = 0; n < ids.length; n += 1) {
        if (stopRef.current) {
          for (const id of ids.slice(n)) final.set(id, STATUS.CANCELLED);
          result.cancelled += ids.length - n;
          break;
        }
        const id = ids[n];
        const item = itemsRef.current.find((row) => row.id === id);
        if (!item) continue;
        setPosition({ index: n + 1, total: ids.length });
        dispatch({ type: 'patch', id, patch: { status: STATUS.UPLOADING, progress: 0, message: '' } });
        try {
          const answer = await upload(item.file, (progress) => dispatch({ type: 'patch', id, patch: { progress } }));
          // A text that came in another encoding is stored as UTF-8: the owner is told what was changed.
          const from = answer?.converted_from;
          if (from) result.converted.push({ name: item.file.name, from });
          dispatch({ type: 'patch', id, patch: { status: STATUS.DONE, progress: 100, message: from ? `Convertido de ${from} para UTF-8.` : '' } });
          final.set(id, STATUS.DONE);
          result.sent += 1;
          result.firstSent ??= item.file.name;
        } catch (error) {
          const duplicate = error?.response?.status === 409;
          dispatch({ type: 'patch', id, patch: { status: duplicate ? STATUS.DUPLICATE : STATUS.ERROR, message: describeError(error) } });
          final.set(id, duplicate ? STATUS.DUPLICATE : STATUS.ERROR);
          if (duplicate) result.duplicates += 1;
          else result.errors += 1;
        }
      }
      runningRef.current = false;
      setRunning(false);
      result.allDone = itemsRef.current.length > 0 && itemsRef.current.every((row) => (final.get(row.id) ?? row.status) === STATUS.DONE);
      return result;
    },
    [upload, describeError]
  );

  /** Puts a file that did not go back on the queue and sends it. */
  const retry = useCallback(
    (id) => {
      const item = itemsRef.current.find((row) => row.id === id);
      if (!item || runningRef.current || ![STATUS.ERROR, STATUS.CANCELLED].includes(item.status)) return Promise.resolve(emptySummary());
      return start([id]);
    },
    [start]
  );

  return { items, running, position, add, remove, reset, stop, start, retry };
}
