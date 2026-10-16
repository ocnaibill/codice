import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';
import { createRoot } from 'react-dom/client';

import { useToasts } from '../../components/ui/toast';
import { useArrivalNotice } from './useArrivalNotice';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const progress = (extra = {}) => ({ source: 'koreader', device: 'Kindle', revision: 3, locator: { type: 'epub', cfi: 'epubcfi(/6/4!/4/2/6)' }, ...extra });

let root;
let container;
let seen;
function Probe({ fileId, data, enabled = true }) {
  seen.current = useArrivalNotice(fileId, data, enabled);
  return null;
}
const render = (props) => act(async () => { root.render(<Probe {...props} />); });

beforeEach(() => {
  localStorage.clear();
  useToasts.getState().clear();
  seen = { current: undefined };
  container = document.createElement('div');
  root = createRoot(container);
});
afterEach(() => act(() => root.unmount()));

describe('useArrivalNotice', () => {
  it('says it once, and gives the place to underline', async () => {
    await render({ fileId: 5, data: progress() });
    expect(useToasts.getState().items.map((t) => t.title)).toEqual(['Seu Kindle parou bem aqui']);
    expect(seen.current).toEqual({ cfi: 'epubcfi(/6/4!/4/2/6)' });
    // the same save, the same book, opened again: nothing new to say
    useToasts.getState().clear();
    await act(async () => root.unmount());
    root = createRoot(container);
    await render({ fileId: 5, data: progress() });
    expect(useToasts.getState().items).toEqual([]);
    expect(seen.current).toBeNull();
  });

  it('says it again for a newer save of the device', async () => {
    await render({ fileId: 5, data: progress() });
    useToasts.getState().clear();
    await act(async () => root.unmount());
    root = createRoot(container);
    await render({ fileId: 5, data: progress({ revision: 4 }) });
    expect(useToasts.getState().items).toHaveLength(1);
  });

  it('is silent for the reader of the web, while a place was asked for, and before the progress is there', async () => {
    await render({ fileId: 5, data: progress({ source: '' }) });
    expect(useToasts.getState().items).toEqual([]);
    await render({ fileId: 6, data: progress(), enabled: false });
    expect(useToasts.getState().items).toEqual([]);
    await render({ fileId: 7, data: undefined });
    expect(useToasts.getState().items).toEqual([]);
    expect(seen.current).toBeNull();
  });

  it('goes with the file: another book has its own', async () => {
    await render({ fileId: 5, data: progress() });
    expect(seen.current).not.toBeNull();
    await render({ fileId: 9, data: undefined });
    expect(seen.current).toBeNull();
  });
});

describe('the message of the toast', () => {
  it('stays until read (6 seconds), and is replaced by another of the same kind rather than piled', async () => {
    const spy = vi.spyOn(useToasts.getState(), 'show');
    await render({ fileId: 5, data: progress() });
    expect(spy.mock.calls[0][0]).toMatchObject({ key: 'arrival', duration: 6000, tone: 'info' });
    spy.mockRestore();
  });
});
