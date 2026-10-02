import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useUploadQueue } from './uploadQueue';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let queue; // what the hook gives, as of the last render
let container;
let root;
let calls; // each upload that is going, waiting to be answered
let stamp = 0;
const upload = (f, onProgress) => new Promise((resolve, reject) => { calls.push({ file: f, onProgress, resolve, reject }); });
const describeError = (error) => `erro: ${error?.message}`;
const file = (name) => new File(['x'], name, { lastModified: ++stamp });
function Harness() {
  queue = useUploadQueue(upload, describeError);
  return null;
}
const statuses = () => queue.items.map((i) => i.status);
const settle = async (n, how = 'ok', value) => { await act(async () => { calls[n][how === 'ok' ? 'resolve' : 'reject'](value); }); };
const begin = async () => { let promise; await act(async () => { promise = queue.start(); }); return promise; };

beforeEach(async () => {
  calls = [];
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => { root.render(<Harness />); });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('the queue that sends one file after the other', () => {
  it('starts with nothing, and sends nothing for an empty list', async () => {
    expect(queue.items).toEqual([]);
    expect(queue.running).toBe(false);
    const summary = await begin();
    expect(calls).toHaveLength(0);
    expect(summary).toEqual({ sent: 0, duplicates: 0, errors: 0, cancelled: 0, firstSent: null, allDone: false });
  });

  it('says what adding did, and has the rows at once', async () => {
    let result;
    await act(async () => { result = queue.add([file('a.epub'), file('b.exe')]); });
    expect(result).toEqual({ added: 2, overCap: 0, repeated: 0 });
    expect(statuses()).toEqual(['queued', 'refused']);
    await act(async () => { result = queue.add([...[1, 2, 3, 4].map((n) => file(`${n}.pdf`))]); });
    expect(result).toEqual({ added: 3, overCap: 1, repeated: 0 });
    expect(queue.items).toHaveLength(5);
  });

  it('numbers the rows so that none repeats, even when added twice before it draws again', async () => {
    await act(async () => { queue.add([file('a.epub')]); queue.add([file('b.epub')]); });
    expect(queue.items.map((i) => i.id)).toEqual([1, 2]);
    await act(async () => { queue.remove(1); queue.add([file('c.epub')]); });
    expect(queue.items.map((i) => i.id)).toEqual([2, 3]);
  });

  it('counts the rows of an add that has not been drawn yet against the five and against repeats', async () => {
    const same = file('Duna.epub');
    let first;
    let second;
    let third;
    await act(async () => {
      first = queue.add([file('1.epub'), file('2.epub'), file('3.epub')]);
      second = queue.add([file('4.epub'), file('5.epub'), file('6.epub')]);
      third = queue.add([same, same]);
    });
    expect(first).toEqual({ added: 3, overCap: 0, repeated: 0 });
    expect(second).toEqual({ added: 2, overCap: 1, repeated: 0 });
    expect(third).toEqual({ added: 0, overCap: 2, repeated: 0 });
    expect(queue.items).toHaveLength(5);
    await act(async () => { queue.reset(); });
    await act(async () => { queue.add([same]); third = queue.add([same]); });
    expect(third).toEqual({ added: 0, overCap: 0, repeated: 1 });
  });

  it('does not keep a place for what was sent: a new lot takes the five places and the sent ones go', async () => {
    await act(async () => { queue.add([file('a.epub'), file('b.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    await settle(0);
    await settle(1, 'fail', new Error('boom'));
    await promise;
    expect(statuses()).toEqual(['done', 'error']);
    let result;
    await act(async () => { result = queue.add([file('1.epub'), file('2.epub'), file('3.epub'), file('4.epub')]); });
    expect(result).toEqual({ added: 4, overCap: 0, repeated: 0 });
    expect(queue.items.map((i) => i.file.name)).toEqual(['b.epub', '1.epub', '2.epub', '3.epub', '4.epub']);
    await act(async () => { result = queue.add([file('5.epub')]); });
    expect(result).toEqual({ added: 0, overCap: 1, repeated: 0 });
    expect(queue.items).toHaveLength(5);
  });

  it('keeps what was sent on the list when nothing new comes to take its place, and takes the same file again as a new row', async () => {
    await act(async () => { queue.add([file('a.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    await settle(0);
    await promise;
    await act(async () => { queue.add([]); });
    expect(statuses()).toEqual(['done']);
    // The same file again is not a repeat of a row that was sent: it goes as a new one (the server will say it is there).
    const same = queue.items[0].file;
    let result;
    await act(async () => { result = queue.add([same]); });
    expect(result).toEqual({ added: 1, overCap: 0, repeated: 0 });
    expect(statuses()).toEqual(['queued']);
  });

  it('does not run, and does not say it is all done, when asked to start with nothing to send', async () => {
    await act(async () => { queue.add([file('a.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    await settle(0);
    await promise;
    expect(statuses()).toEqual(['done']);
    let again;
    await act(async () => { again = queue.start(); });
    expect(await again).toEqual({ sent: 0, duplicates: 0, errors: 0, cancelled: 0, firstSent: null, allDone: false });
    expect(queue.running).toBe(false);
    expect(calls).toHaveLength(1);
  });

  it('says it is not all done for an empty list, even when asked for a row that is not there', async () => {
    let promise;
    await act(async () => { promise = queue.start([42]); });
    expect(await promise).toMatchObject({ sent: 0, errors: 0, allDone: false });
    expect(calls).toHaveLength(0);
  });

  it('sends one at a time, in the order of the list, and skips what is not queued', async () => {
    await act(async () => { queue.add([file('a.epub'), file('x.exe'), file('b.epub'), file('c.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    expect(calls.map((c) => c.file.name)).toEqual(['a.epub']);
    expect(queue.running).toBe(true);
    expect(queue.position).toEqual({ index: 1, total: 3 });
    expect(statuses()).toEqual(['uploading', 'refused', 'queued', 'queued']);
    await settle(0);
    expect(calls.map((c) => c.file.name)).toEqual(['a.epub', 'b.epub']);
    expect(queue.position).toEqual({ index: 2, total: 3 });
    await settle(1);
    await settle(2);
    const summary = await promise;
    expect(calls.map((c) => c.file.name)).toEqual(['a.epub', 'b.epub', 'c.epub']);
    expect(summary).toMatchObject({ sent: 3, duplicates: 0, errors: 0, cancelled: 0, firstSent: 'a.epub', allDone: false });
    expect(statuses()).toEqual(['done', 'refused', 'done', 'done']);
    expect(queue.running).toBe(false);
  });

  it('reports the progress of the file that is going, and says it is done at 100', async () => {
    await act(async () => { queue.add([file('a.epub')]); });
    await act(async () => { queue.start(); });
    await act(async () => { calls[0].onProgress(35); });
    expect(queue.items[0]).toMatchObject({ status: 'uploading', progress: 35 });
    await settle(0);
    expect(queue.items[0]).toMatchObject({ status: 'done', progress: 100 });
  });

  it('says it is all done only when every row is', async () => {
    await act(async () => { queue.add([file('a.epub'), file('b.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    await settle(0);
    await settle(1);
    expect((await promise).allDone).toBe(true);
  });

  it('keeps going after a failure, tells a 409 from the rest, and counts each', async () => {
    await act(async () => { queue.add([file('a.epub'), file('b.epub'), file('c.epub'), file('d.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    await settle(0);
    await settle(1, 'fail', Object.assign(new Error('x'), { response: { status: 409 } }));
    await settle(2, 'fail', new Error('boom'));
    await settle(3);
    const summary = await promise;
    expect(summary).toMatchObject({ sent: 2, duplicates: 1, errors: 1, cancelled: 0, allDone: false });
    expect(statuses()).toEqual(['done', 'duplicate', 'error', 'done']);
    expect(queue.items[1].message).toBe('erro: x');
    expect(queue.items[2].message).toBe('erro: boom');
    expect(queue.items[0].message).toBe('');
  });

  it('treats an error with no answer from the server as an error, not as a duplicate', async () => {
    await act(async () => { queue.add([file('a.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    await settle(0, 'fail', undefined);
    expect((await promise)).toMatchObject({ errors: 1, duplicates: 0 });
    expect(statuses()).toEqual(['error']);
  });

  it('clears the message and the progress of a file when it starts again', async () => {
    await act(async () => { queue.add([file('a.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    await act(async () => { calls[0].onProgress(80); });
    await settle(0, 'fail', new Error('boom'));
    await promise;
    expect(queue.items[0]).toMatchObject({ status: 'error', progress: 80, message: 'erro: boom' });
    await act(async () => { promise = queue.retry(1); });
    expect(queue.items[0]).toMatchObject({ status: 'uploading', progress: 0, message: '' });
    await settle(1);
    expect((await promise)).toMatchObject({ sent: 1, allDone: true });
  });

  it('cancels the rest on stop, lets the file that is going finish, and counts what was cancelled', async () => {
    await act(async () => { queue.add([file('a.epub'), file('b.epub'), file('c.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    await act(async () => { queue.stop(); });
    expect(statuses()).toEqual(['uploading', 'cancelled', 'cancelled']);
    await settle(0);
    const summary = await promise;
    expect(calls).toHaveLength(1);
    expect(summary).toMatchObject({ sent: 1, cancelled: 2, allDone: false });
    expect(statuses()).toEqual(['done', 'cancelled', 'cancelled']);
    expect(queue.running).toBe(false);
  });

  it('does nothing for a stop when nothing is going, and a stop does not outlive its run', async () => {
    await act(async () => { queue.add([file('a.epub')]); });
    await act(async () => { queue.stop(); });
    expect(statuses()).toEqual(['queued']);
    let promise;
    await act(async () => { promise = queue.start(); });
    await settle(0);
    await promise;
    await act(async () => { queue.add([file('b.epub'), file('c.epub')]); });
    await act(async () => { promise = queue.start(); });
    expect(calls).toHaveLength(2);
    await settle(1);
    await settle(2);
    expect((await promise).cancelled).toBe(0);
  });

  it('sends a cancelled file again, alone, and refuses to retry what was not left behind', async () => {
    await act(async () => { queue.add([file('a.epub'), file('b.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    await act(async () => { queue.stop(); });
    await settle(0);
    await promise;
    expect(statuses()).toEqual(['done', 'cancelled']);
    let none;
    await act(async () => { none = queue.retry(1); }); // already sent
    expect(calls).toHaveLength(1);
    expect(await none).toMatchObject({ sent: 0, errors: 0, cancelled: 0, allDone: false });
    await act(async () => { promise = queue.retry(2); });
    expect(calls.map((c) => c.file.name)).toEqual(['a.epub', 'b.epub']);
    await settle(1);
    expect((await promise).allDone).toBe(true);
    await act(async () => { promise = queue.retry(99); });
    expect(calls).toHaveLength(2);
  });

  it('holds the list while it sends: no adding, removing or resetting, and no second start or retry', async () => {
    await act(async () => { queue.add([file('a.epub'), file('b.epub')]); });
    let promise;
    await act(async () => { promise = queue.start(); });
    let added;
    await act(async () => { added = queue.add([file('c.epub')]); });
    expect(added).toEqual({ added: 0, overCap: 1, repeated: 0 });
    await act(async () => { queue.remove(2); queue.reset(); });
    expect(queue.items).toHaveLength(2);
    let second;
    let retried;
    await act(async () => { second = queue.start(); retried = queue.retry(1); });
    expect(calls).toHaveLength(1);
    expect(await second).toMatchObject({ sent: 0, allDone: false });
    expect(await retried).toMatchObject({ sent: 0, allDone: false });
    await settle(0);
    await settle(1);
    await promise;
  });

  it('takes rows off and empties the list when it is idle', async () => {
    await act(async () => { queue.add([file('a.epub'), file('b.epub')]); });
    await act(async () => { queue.remove(1); });
    expect(queue.items.map((i) => i.file.name)).toEqual(['b.epub']);
    await act(async () => { queue.reset(); });
    expect(queue.items).toEqual([]);
  });

  it('sends only the files it is asked for', async () => {
    await act(async () => { queue.add([file('a.epub'), file('b.epub'), file('c.epub')]); });
    let promise;
    await act(async () => { promise = queue.start([2]); });
    await settle(0);
    await promise;
    expect(calls.map((c) => c.file.name)).toEqual(['b.epub']);
    expect(statuses()).toEqual(['queued', 'done', 'queued']);
  });

  it('ignores a number of a row that is not there', async () => {
    await act(async () => { queue.add([file('a.epub')]); });
    let promise;
    await act(async () => { promise = queue.start([42, 1]); });
    await settle(0);
    const summary = await promise;
    expect(summary).toMatchObject({ sent: 1, errors: 0, duplicates: 0 });
    expect(calls).toHaveLength(1);
  });
});
