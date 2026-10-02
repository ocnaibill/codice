import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import { isActive, resetActivity } from '../../activity';
import AudioViewer from './AudioViewer';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;

beforeEach(() => {
  resetActivity();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

// jsdom has no media playback: give the element the time and duration a player would.
async function play(onProgress) {
  await act(async () => { root.render(<AudioViewer fileUrl="/a.mp3" onProgress={onProgress} />); });
  const audio = container.querySelector('audio');
  Object.defineProperty(audio, 'duration', { value: 100, configurable: true });
  const at = (t) => {
    Object.defineProperty(audio, 'currentTime', { value: t, configurable: true, writable: true });
    return act(async () => { audio.dispatchEvent(new Event('timeupdate')); });
  };
  return { audio, at };
}
const saved = async () => { await act(async () => { await new Promise((r) => setTimeout(r, 1600)); }); };

describe('AudioViewer', () => {
  it('counts playing audio as reading time', async () => {
    const { at } = await play(vi.fn().mockResolvedValue({}));
    expect(isActive()).toBe(false);
    await at(10);
    expect(isActive()).toBe(true);
  });

  it('saves the time in milliseconds, finishes only near the end, and never says "not finished"', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    const { at } = await play(onProgress);

    await at(97);
    await saved();
    let [locator, extras] = onProgress.mock.calls.at(-1);
    expect(locator).toEqual({ type: 'audio', track: 0, ms: 97000 });
    expect(extras.completed).toBe(true); // 97% of it

    await at(20); // the listener goes back
    await saved();
    [locator, extras] = onProgress.mock.calls.at(-1);
    expect(locator.ms).toBe(20000);
    expect(extras.completed).toBeUndefined();
  });

  it('finishes when the audio ends', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    const { audio } = await play(onProgress);
    await act(async () => { audio.dispatchEvent(new Event('ended')); });
    await saved();
    expect(onProgress.mock.calls.at(-1)[1].completed).toBe(true);
  });
});

describe('AudioViewer: a point that is not there (#14)', () => {
  async function open(initialProgress, onPlaceFailed) {
    await act(async () => { root.render(<AudioViewer fileUrl="/a.mp3" onProgress={vi.fn()} initialProgress={initialProgress} onPlaceFailed={onPlaceFailed} />); });
    const audio = container.querySelector('audio');
    Object.defineProperty(audio, 'duration', { value: 100, configurable: true });
    Object.defineProperty(audio, 'currentTime', { value: 0, configurable: true, writable: true });
    await act(async () => { audio.dispatchEvent(new Event('loadedmetadata')); });
    return audio;
  }

  it('goes to the point saved when it is inside the audio, and says nothing', async () => {
    const onPlaceFailed = vi.fn();
    const audio = await open('42.5', onPlaceFailed);
    expect(audio.currentTime).toBe(42.5);
    expect(onPlaceFailed).not.toHaveBeenCalled();
  });

  it('stays at the start and says so when the point is past the end', async () => {
    const onPlaceFailed = vi.fn();
    const audio = await open('500', onPlaceFailed);
    expect(audio.currentTime).toBe(0);
    expect(onPlaceFailed).toHaveBeenCalledWith({ reason: 'O ponto passa do fim do áudio.' });
  });

  it('says so when the point is not a time, and works with nobody listening', async () => {
    const onPlaceFailed = vi.fn();
    await open('abc', onPlaceFailed);
    expect(onPlaceFailed).toHaveBeenCalledWith({ reason: 'O ponto pedido não é válido.' });
    act(() => root.unmount());
    root = createRoot(container);
    const audio = await open('500', undefined);
    expect(audio.currentTime).toBe(0);
  });

  it('asks for nothing when nothing was asked for', async () => {
    const onPlaceFailed = vi.fn();
    await open(undefined, onPlaceFailed);
    expect(onPlaceFailed).not.toHaveBeenCalled();
  });
});

describe('AudioViewer: a file that is here before the listener is (#25)', () => {
  // A player that already has its metadata when the element is created: no "loadedmetadata" will come.
  let restore;
  const ready = (state) => {
    const proto = HTMLMediaElement.prototype;
    const saved = ['readyState', 'duration', 'currentTime'].map((name) => [name, Object.getOwnPropertyDescriptor(proto, name)]);
    let time = 0;
    Object.defineProperty(proto, 'readyState', { configurable: true, get: () => state });
    Object.defineProperty(proto, 'duration', { configurable: true, get: () => 150 });
    Object.defineProperty(proto, 'currentTime', { configurable: true, get: () => time, set: (v) => { time = v; } });
    restore = () => saved.forEach(([name, d]) => (d ? Object.defineProperty(proto, name, d) : delete proto[name]));
  };
  afterEach(() => restore?.());

  it('opens the place asked for and knows the duration, with no event at all', async () => {
    ready(4);
    await act(async () => { root.render(<AudioViewer fileUrl="/a.m4b" onProgress={vi.fn()} initialProgress="40" />); });
    expect(container.querySelector('audio').currentTime).toBe(40);
    expect(container.textContent).toContain('2:30'); // the duration, 150 s
  });

  it('opens the place when the player has only the metadata (what preload="metadata" asks for)', async () => {
    ready(1);
    await act(async () => { root.render(<AudioViewer fileUrl="/a.m4b" onProgress={vi.fn()} initialProgress="95" />); });
    expect(container.querySelector('audio').currentTime).toBe(95);
  });

  it('says a place past the end is not there, also when the metadata was already here', async () => {
    ready(4);
    const onPlaceFailed = vi.fn();
    await act(async () => { root.render(<AudioViewer fileUrl="/a.m4b" onProgress={vi.fn()} initialProgress="500" onPlaceFailed={onPlaceFailed} />); });
    expect(onPlaceFailed).toHaveBeenCalledTimes(1);
    expect(container.querySelector('audio').currentTime).toBe(0);
  });

  it('waits for the player when it has nothing yet, and then opens the place', async () => {
    ready(0);
    await act(async () => { root.render(<AudioViewer fileUrl="/a.m4b" onProgress={vi.fn()} initialProgress="40" />); });
    const audio = container.querySelector('audio');
    expect(audio.currentTime).toBe(0);
    await act(async () => { audio.dispatchEvent(new Event('loadedmetadata')); });
    expect(audio.currentTime).toBe(40);
  });

  it('starts at the beginning when no place was asked for', async () => {
    ready(4);
    await act(async () => { root.render(<AudioViewer fileUrl="/a.m4b" onProgress={vi.fn()} />); });
    expect(container.querySelector('audio').currentTime).toBe(0);
    expect(container.textContent).toContain('2:30');
  });
});
