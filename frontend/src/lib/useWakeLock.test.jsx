import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useWakeLock } from './useWakeLock';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let locks; // each lock that was granted: { release, listeners }
let request;

function Probe({ active }) {
  useWakeLock(active);
  return null;
}
const render = async (active) => { await act(async () => { root.render(<Probe active={active} />); }); };
const settle = () => act(async () => { await Promise.resolve(); await Promise.resolve(); });
const setVisibility = async (state) => {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true });
  await act(async () => { document.dispatchEvent(new Event('visibilitychange')); });
  await settle();
};
// What the browser does when the page is hidden: it lets the lock go and says so.
const browserReleases = async (lock) => { await act(async () => { lock.listeners.forEach((fn) => fn()); }); };

beforeEach(() => {
  locks = [];
  request = vi.fn(async () => {
    const lock = { listeners: [], release: vi.fn(async () => {}), addEventListener: (type, fn) => type === 'release' && lock.listeners.push(fn) };
    locks.push(lock);
    return lock;
  });
  Object.defineProperty(navigator, 'wakeLock', { value: { request }, configurable: true });
  Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  delete navigator.wakeLock;
});

describe('keeping the screen on while reading', () => {
  it('asks for the screen lock once when it is active, and never when it is not', async () => {
    await render(false);
    await settle();
    expect(request).not.toHaveBeenCalled();
    await render(true);
    await settle();
    expect(request).toHaveBeenCalledTimes(1);
    expect(request).toHaveBeenCalledWith('screen');
  });

  it('lets the lock go when it stops being active, and asks again when it is again', async () => {
    await render(true);
    await settle();
    await render(false);
    await settle();
    expect(locks[0].release).toHaveBeenCalledTimes(1);
    await render(true);
    await settle();
    expect(request).toHaveBeenCalledTimes(2);
  });

  it('lets the lock go when the component goes', async () => {
    await render(true);
    await settle();
    act(() => root.unmount());
    expect(locks[0].release).toHaveBeenCalledTimes(1);
    root = createRoot(container); // for the cleanup
  });

  it('asks again when the page is shown again, because the browser let the lock go when it was hidden', async () => {
    await render(true);
    await settle();
    await browserReleases(locks[0]);
    await setVisibility('hidden');
    expect(request).toHaveBeenCalledTimes(1); // hidden, and the lock is gone: nothing to ask until it is shown
    await setVisibility('visible');
    expect(request).toHaveBeenCalledTimes(2);
  });

  it('does not ask a second time while it still holds the lock', async () => {
    await render(true);
    await settle();
    await setVisibility('visible');
    expect(request).toHaveBeenCalledTimes(1);
  });

  it('lets go at once a lock that came after it was not wanted any more', async () => {
    let grant;
    request.mockImplementationOnce(() => new Promise((resolve) => { grant = resolve; }));
    await render(true);
    await render(false); // gone before the answer
    const late = { release: vi.fn(async () => {}), addEventListener: vi.fn() };
    await act(async () => { grant(late); });
    await settle();
    expect(late.release).toHaveBeenCalledTimes(1);
  });

  it('goes on reading when the browser refuses, with nothing said', async () => {
    request.mockRejectedValueOnce(Object.assign(new Error('not allowed'), { name: 'NotAllowedError' }));
    await render(true);
    await settle();
    expect(request).toHaveBeenCalledTimes(1);
    expect(locks).toHaveLength(0);
  });

  it('does nothing where the browser has no such lock', async () => {
    delete navigator.wakeLock;
    await expect(render(true)).resolves.toBeUndefined();
    await settle();
    expect(request).not.toHaveBeenCalled();
  });

  it('does not fail when letting go is refused', async () => {
    await render(true);
    await settle();
    locks[0].release.mockRejectedValueOnce(new Error('already released'));
    await expect(render(false)).resolves.toBeUndefined();
    await settle();
  });
});
