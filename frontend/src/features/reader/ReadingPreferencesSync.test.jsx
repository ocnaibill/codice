import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }));

import { api } from '../../lib/api';
import { mount } from '../admin/testUtils';
import { ReadingPreferencesSync } from './ReadingPreferencesSync';
import { flushReadingSettings } from './readingSync';
import { getEpubSettings, hasSavedEpubSettings, saveEpubSettings, setPreferenceOwner } from './preferences';
import { DEFAULT_SETTINGS } from './epubThemes';

let view;
const server = { theme: 'sepia', font: 'dislexia', size: 130, spacing: 'ampla', margins: 'larga', justify: true };
const serve = (reader) => api.get.mockImplementation(async () => ({ data: { choice: '', library: 'given_first', effective: 'given_first', reader } }));

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  setPreferenceOwner('ana');
  api.put.mockResolvedValue({ data: {} });
  flushReadingSettings();
  api.put.mockClear();
});
afterEach(() => {
  view?.unmount();
  flushReadingSettings();
});

describe('how the text looks, from the server to this device', () => {
  it('is what this device starts with, when the person chose it somewhere else', async () => {
    serve(server);
    view = await mount(<ReadingPreferencesSync userId="ana" />);
    expect(getEpubSettings()).toEqual(server);
    expect(api.put).not.toHaveBeenCalled();
  });

  it('wins over what this device had', async () => {
    saveEpubSettings({ ...DEFAULT_SETTINGS, theme: 'preto', size: 90 });
    serve(server);
    view = await mount(<ReadingPreferencesSync userId="ana" />);
    expect(getEpubSettings()).toEqual(server);
  });

  it('keeps only what the lists have of what the server says', async () => {
    serve({ theme: 'rosa', font: 'comic', size: 133, spacing: 'x', margins: 'y', justify: 'sim', css: '<b>' });
    view = await mount(<ReadingPreferencesSync userId="ana" />);
    expect(getEpubSettings()).toEqual(DEFAULT_SETTINGS);
  });

  it('sends what this device chose when the server has nothing, so that it is not lost', async () => {
    saveEpubSettings({ ...DEFAULT_SETTINGS, theme: 'escuro', size: 120 });
    serve(null);
    view = await mount(<ReadingPreferencesSync userId="ana" />);
    expect(api.put).toHaveBeenCalledTimes(1);
    expect(api.put).toHaveBeenCalledWith('/auth/preferences', { reader: { ...DEFAULT_SETTINGS, theme: 'escuro', size: 120 } });
  });

  it('sends nothing when neither has a choice: the default is not a choice', async () => {
    serve(null);
    view = await mount(<ReadingPreferencesSync userId="ana" />);
    expect(api.put).not.toHaveBeenCalled();
    expect(hasSavedEpubSettings()).toBe(false);
  });

  it('does nothing until the person is known, and until the server has answered', async () => {
    serve(server);
    view = await mount(<ReadingPreferencesSync userId={undefined} />);
    expect(api.get).not.toHaveBeenCalled();
    view.unmount();
    api.get.mockReturnValue(new Promise(() => {}));
    view = await mount(<ReadingPreferencesSync userId="ana" />);
    expect(hasSavedEpubSettings()).toBe(false);
  });

  it('does nothing when it is not enabled', async () => {
    serve(server);
    view = await mount(<ReadingPreferencesSync userId="ana" enabled={false} />);
    expect(api.get).not.toHaveBeenCalled();
  });

  it('keeps the choice for the account that signed in, and not for another', async () => {
    setPreferenceOwner('bob');
    serve(server);
    view = await mount(<ReadingPreferencesSync userId="ana" />);
    expect(localStorage.getItem('codice:epub-settings:ana')).not.toBeNull();
    expect(localStorage.getItem('codice:epub-settings:bob')).toBeNull();
  });

  it('brings it once: what the person changes afterwards is theirs and is not taken back', async () => {
    serve(server);
    view = await mount(<ReadingPreferencesSync userId="ana" />);
    saveEpubSettings({ ...server, size: 160 });
    view.unmount();
    view = await mount(<ReadingPreferencesSync userId="ana" />); // a new mount is a new sign-in of the page: it brings it again
    expect(getEpubSettings().size).toBe(130);
  });

  it('draws nothing', async () => {
    serve(server);
    view = await mount(<ReadingPreferencesSync userId="ana" />);
    expect(view.container.textContent).toBe('');
  });

  it('brings it once while the page is open, even if the server\'s answer changes, which is the person\'s choice on another device and not a reason to take theirs back', async () => {
    serve(server);
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => { root.render(<QueryClientProvider client={client}><ReadingPreferencesSync userId="ana" /></QueryClientProvider>); });
    await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
    expect(getEpubSettings()).toEqual(server);
    saveEpubSettings({ ...server, size: 160 }); // what the person chose here, afterwards
    await act(async () => { client.setQueryData(['preferences'], { choice: '', library: 'given_first', effective: 'given_first', reader: { ...server, theme: 'preto', size: 90 } }); });
    await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
    expect(getEpubSettings()).toMatchObject({ theme: 'sepia', size: 160 });
    act(() => root.unmount());
    container.remove();
  });
});
