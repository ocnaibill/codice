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
