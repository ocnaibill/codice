import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({ api: { put: vi.fn() } }));

import { api } from '../../lib/api';
import { PUSH_DELAY, flushReadingSettings, pushReadingSettings } from './readingSync';
import { DEFAULT_SETTINGS } from './epubThemes';

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  api.put.mockResolvedValue({ data: {} });
  flushReadingSettings();
  api.put.mockClear();
});
afterEach(() => {
  flushReadingSettings();
  vi.useRealTimers();
});

const choice = (extra = {}) => ({ ...DEFAULT_SETTINGS, ...extra });

describe('sending how the text looks to the server', () => {
  it('waits a moment after the last change, and sends one request for many', () => {
    pushReadingSettings(choice({ size: 110 }));
    vi.advanceTimersByTime(PUSH_DELAY - 100);
    pushReadingSettings(choice({ size: 120 }));
    vi.advanceTimersByTime(PUSH_DELAY - 100);
    pushReadingSettings(choice({ size: 130 }));
    expect(api.put).not.toHaveBeenCalled();
    vi.advanceTimersByTime(PUSH_DELAY);
    expect(api.put).toHaveBeenCalledTimes(1);
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { reader: choice({ size: 130 }) });
  });

  it('sends what the last change left, whole, and only the fields the lists have', () => {
    pushReadingSettings({ theme: 'preto', font: 'dislexia', size: 150, spacing: 'ampla', margins: 'larga', justify: true, extra: '<script>' });
    vi.advanceTimersByTime(PUSH_DELAY);
    expect(api.put.mock.calls[0][1]).toEqual({ reader: { theme: 'preto', font: 'dislexia', size: 150, spacing: 'ampla', margins: 'larga', justify: true } });
  });

  it('sends what is not valid as the default, and does not send what the person did not choose', () => {
    pushReadingSettings({ theme: 'rosa', size: 999 });
    vi.advanceTimersByTime(PUSH_DELAY);
    expect(api.put.mock.calls[0][1]).toEqual({ reader: DEFAULT_SETTINGS });
  });

  it('sends at once when it is told not to wait, and sends nothing twice', () => {
    pushReadingSettings(choice({ font: 'sem-serifa' }), 0);
    expect(api.put).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(PUSH_DELAY * 3);
    expect(api.put).toHaveBeenCalledTimes(1);
  });

  it('sends what is waiting when the page is hidden or left, which is when a quick change would be lost', () => {
    pushReadingSettings(choice({ size: 140 }));
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' });
    document.dispatchEvent(new Event('visibilitychange'));
    expect(api.put).toHaveBeenCalledTimes(1);
    pushReadingSettings(choice({ size: 150 }));
    window.dispatchEvent(new Event('pagehide'));
    expect(api.put).toHaveBeenCalledTimes(2);
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' });
    pushReadingSettings(choice({ size: 160 }));
    document.dispatchEvent(new Event('visibilitychange'));
    expect(api.put).toHaveBeenCalledTimes(2); // shown again is not a reason to send
  });

  it('has nothing to send when nothing is waiting', () => {
    flushReadingSettings();
    expect(api.put).not.toHaveBeenCalled();
  });

  it('does not make a failure of the server, or a missing request, the problem of the reader', async () => {
    api.put.mockRejectedValue(new Error('offline'));
    expect(() => { pushReadingSettings(choice(), 0); }).not.toThrow();
    api.put.mockImplementation(() => { throw new Error('no api'); });
    expect(() => { pushReadingSettings(choice({ size: 90 }), 0); }).not.toThrow();
    await vi.runAllTimersAsync();
  });
});
