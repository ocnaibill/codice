import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { post: vi.fn() } }));

import { api } from '../../../lib/api';
import { resetActivity, markActivity } from '../activity';
import { useReadingHeartbeat } from './useReadingHeartbeat';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

function Probe({ workId, fileId }) {
  useReadingHeartbeat(workId, fileId);
  return null;
}

let container;
let root;

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  api.post.mockResolvedValue({});
  resetActivity();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.useRealTimers();
});

const mount = () => act(async () => { root.render(<Probe workId={7} fileId={20} />); });
const tick = (ms) => act(async () => { vi.advanceTimersByTime(ms); });

describe('useReadingHeartbeat', () => {
  it('counts 25 seconds for the file being read while the person is active', async () => {
    await mount();
    await tick(25_000);
    expect(api.post).toHaveBeenCalledWith('/works/7/reading-heartbeat', { seconds: 25, fileId: 20 });
  });

  it('stops counting when nothing has been touched for 90 seconds, and resumes with the next touch', async () => {
    await mount();
    await tick(25_000 * 4); // 100 s with no interaction after opening
    const counted = api.post.mock.calls.length;
    expect(counted).toBe(3); // 25, 50 and 75 s are still inside the window
    await tick(25_000 * 2);
    expect(api.post.mock.calls.length).toBe(counted); // idle: nothing more

    markActivity(Date.now());
    await tick(25_000);
    expect(api.post.mock.calls.length).toBe(counted + 1);
  });

  it('does not count a tab that is not visible', async () => {
    await mount();
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    await tick(25_000);
    expect(api.post).not.toHaveBeenCalled();
  });
});
