import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ api: { get: vi.fn(async () => ({ data: new ArrayBuffer(8) })) }, authenticatedUrl: (u) => u }));

// epub.js is not what is under test: a book with the chapters a test gives it, and a page that records where it
// was asked to go (and fails the way epub.js does when told to go somewhere it cannot).
const state = { chapters: [], displayError: null, displayed: [] };
vi.mock('epubjs', () => ({
  default: () => {
    const rendition = {
      display: vi.fn(async (target) => {
        state.displayed.push(target ?? null);
        if (state.displayError && target && String(target).startsWith('epubcfi(')) throw state.displayError;
      }),
      on: () => {},
      themes: { register: () => {}, select: () => {}, fontSize: () => {} },
      prev: () => {},
      next: () => {},
      destroy: () => {},
    };
    return {
      loaded: { navigation: Promise.resolve({ toc: [] }) },
      opened: Promise.resolve(),
      spine: {
        spineItems: state.chapters,
        // A CFI "epubcfi(/6/N!...)" points at chapter N/2 - 1, as in epub.js; a href is looked up by name.
        get: (target) => {
          if (typeof target === 'string' && target.startsWith('epubcfi(')) {
            const m = /^epubcfi\(\/6\/(\d+)/.exec(target);
            if (!m) throw new Error('malformed CFI');
            return state.chapters[Number(m[1]) / 2 - 1] ?? null;
          }
          return state.chapters.find((c) => c.href === target) ?? null;
        },
      },
      locations: { length: () => 0, generate: () => Promise.resolve(), percentageFromCfi: () => null },
      renderTo: () => rendition,
      destroy: () => {},
    };
  },
}));

import EpubViewer from './EpubViewer';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
async function open(props = {}) {
  await act(async () => { root.render(<EpubViewer fileUrl="/f.epub" onProgress={vi.fn()} {...props} />); });
  await flush();
  await flush();
}

beforeEach(() => {
  state.chapters = [{ href: 'Text/c1.xhtml' }, { href: 'Text/c2.xhtml' }, { href: 'Text/c3.xhtml' }];
  state.displayError = null;
  state.displayed = [];
  vi.spyOn(console, 'warn').mockImplementation(() => {});
  vi.spyOn(console, 'log').mockImplementation(() => {});
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.restoreAllMocks();
});

describe('EpubViewer: a place that is not there (#14)', () => {
  it('opens the place asked for when the book still has that chapter, and says nothing', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: 'epubcfi(/6/4!/4/2)', locator: { type: 'epub', cfi: 'epubcfi(/6/4!/4/2)', href: 'c2.xhtml' }, onPlaceFailed });
    expect(state.displayed).toEqual(['epubcfi(/6/4!/4/2)']);
    expect(onPlaceFailed).not.toHaveBeenCalled();
  });

  it('opens a place saved without a chapter, as it always did', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: 'epubcfi(/6/2!/4)', locator: { type: 'epub', cfi: 'epubcfi(/6/2!/4)' }, onPlaceFailed });
    expect(state.displayed).toEqual(['epubcfi(/6/2!/4)']);
    expect(onPlaceFailed).not.toHaveBeenCalled();
  });

  it('shows the start and says why when the book has no chapter at that position any more', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: 'epubcfi(/6/40!/4)', locator: { type: 'epub', cfi: 'epubcfi(/6/40!/4)', href: 'c20.xhtml' }, onPlaceFailed });
    expect(state.displayed).toEqual([null]); // the start, and not the place
    expect(onPlaceFailed).toHaveBeenCalledTimes(1);
    expect(onPlaceFailed.mock.calls[0][0].reason).toContain('não existe mais neste EPUB');
  });

  it('says the book changed when that position is now another chapter, and does not open it', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: 'epubcfi(/6/4!/4/2)', locator: { type: 'epub', cfi: 'epubcfi(/6/4!/4/2)', href: 'cap-que-sumiu.xhtml' }, onPlaceFailed });
    expect(state.displayed).toEqual([null]);
    expect(onPlaceFailed.mock.calls[0][0].reason).toContain('não é mais o mesmo');
  });

  it('says so when epub.js cannot open the CFI, and shows the start', async () => {
    state.displayError = new Error('No Section Found');
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: 'epubcfi(/6/4!/4/2)', locator: { type: 'epub', cfi: 'epubcfi(/6/4!/4/2)', href: 'c2.xhtml' }, onPlaceFailed });
    expect(state.displayed).toEqual(['epubcfi(/6/4!/4/2)', null]);
    expect(onPlaceFailed).toHaveBeenCalledWith({ reason: 'Não foi possível abrir este ponto no EPUB: o arquivo pode ter mudado.' });
  });

  it('says so when the CFI is not a CFI epub.js can read', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: 'epubcfi(garbage)', locator: { type: 'epub', cfi: 'epubcfi(garbage)' }, onPlaceFailed });
    expect(state.displayed).toEqual([null]);
    expect(onPlaceFailed).toHaveBeenCalledTimes(1);
  });

  it('opens a chapter given by name, says when there is none, and opens the start when nothing was asked', async () => {
    const onPlaceFailed = vi.fn();
    await open({ initialProgress: 'Text/c3.xhtml', onPlaceFailed });
    expect(state.displayed).toEqual(['Text/c3.xhtml']);
    expect(onPlaceFailed).not.toHaveBeenCalled();
    act(() => root.unmount());
    root = createRoot(container);
    state.displayed = [];
    await open({ initialProgress: 'Text/c99.xhtml', onPlaceFailed });
    expect(state.displayed).toEqual([null]);
    expect(onPlaceFailed.mock.calls[0][0].reason).toContain('não existe mais neste EPUB');
    act(() => root.unmount());
    root = createRoot(container);
    state.displayed = [];
    onPlaceFailed.mockClear();
    await open({ onPlaceFailed });
    expect(state.displayed).toEqual([null]);
    expect(onPlaceFailed).not.toHaveBeenCalled();
  });

  it('works without anyone listening', async () => {
    await open({ initialProgress: 'epubcfi(/6/40!/4)', locator: { type: 'epub', cfi: 'x' } });
    expect(state.displayed).toEqual([null]);
    expect(container.textContent).not.toContain('Failed to load EPUB');
  });
});
