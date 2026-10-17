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
function Probe({ fileId, data, enabled = true, onGoTo }) {
  seen.current = useArrivalNotice(fileId, data, enabled, onGoTo);
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

describe('the place a device sent that was kept apart (DEC-191)', () => {
  const kept = (extra = {}) => ({ revision: 6, source: '', locator: { type: 'epub', cfi: 'epubcfi(/6/2!/4/2/4)' }, pending: { device: 'Kindle', percent: 42, locator: { type: 'epub', cfi: 'epubcfi(/6/8!/4/2/10)' }, at: '2026-10-11T12:00:00Z', ...extra } });

  it('asks once, with a way to go there, and going opens the book at the place of the device', async () => {
    const goTo = vi.fn();
    await render({ fileId: 5, data: kept(), onGoTo: goTo });
    const item = useToasts.getState().items.find((t) => t.key === 'pending');
    expect(item.title).toMatch(/^Seu Kindle parou em outro ponto \(42%\)/);
    expect(item.action.label).toBe('Ir para lá');
    item.action.onClick();
    expect(goTo).toHaveBeenCalledWith({ type: 'epub', cfi: 'epubcfi(/6/8!/4/2/10)' });
    // told, it is not told again for the same place
    useToasts.getState().clear();
    await act(async () => root.unmount());
    root = createRoot(container);
    await render({ fileId: 5, data: kept(), onGoTo: goTo });
    expect(useToasts.getState().items.find((t) => t.key === 'pending')).toBeUndefined();
  });

  it('is silent while a place was asked for', async () => {
    await render({ fileId: 5, data: kept(), enabled: false, onGoTo: vi.fn() });
    expect(useToasts.getState().items).toEqual([]);
  });
});
