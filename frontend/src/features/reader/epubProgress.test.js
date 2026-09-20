import { describe, it, expect } from 'vitest';
import { buildEpubProgress } from './epubProgress';

const location = (over = {}) => ({
  atEnd: false,
  start: { cfi: 'epubcfi(/6/8!/4/2)', href: 'text/ch3.xhtml', displayed: { page: 3, total: 10 }, ...over },
});

describe('buildEpubProgress', () => {
  it('saves the exact place, the chapter and how far into it', () => {
    const { locator } = buildEpubProgress(location(), () => null);
    expect(locator).toEqual({ type: 'epub', cfi: 'epubcfi(/6/8!/4/2)', href: 'text/ch3.xhtml', progression: 0.2 });
  });

  it('gives no percentage until the positions of the book are known, and does not say it is finished', () => {
    const { extras } = buildEpubProgress(location(), () => null);
    expect(extras.percent).toBeUndefined();
    expect(extras.completed).toBeUndefined();
  });

  it('gives the percentage of the whole book, rounded to a tenth', () => {
    const { extras } = buildEpubProgress(location(), (cfi) => (cfi === 'epubcfi(/6/8!/4/2)' ? 0.4237 : null));
    expect(extras.percent).toBe(42.4);
    expect(extras.completed).toBeUndefined();
  });

  it('is 100% at the end of the book, even when the positions of the book are not known', () => {
    // a book of one chapter starts and ends in the same place
    expect(buildEpubProgress({ ...location(), atEnd: true }, () => null).extras).toEqual({ percent: 100, completed: true });
    expect(buildEpubProgress({ ...location(), atEnd: true }, () => 0).extras.percent).toBe(100);
  });

  it('is finished at the end of the book, or from 95% on, and says nothing before', () => {
    expect(buildEpubProgress({ ...location(), atEnd: true }, () => null).extras.completed).toBe(true);
    expect(buildEpubProgress(location(), () => 0.96).extras.completed).toBe(true);
    expect(buildEpubProgress(location(), () => 0.5).extras.completed).toBeUndefined();
  });

  it('does not invent what it does not have', () => {
    const { locator } = buildEpubProgress(location({ href: undefined, displayed: undefined }), () => null);
    expect(locator).toEqual({ type: 'epub', cfi: 'epubcfi(/6/8!/4/2)' });
    expect(buildEpubProgress(location({ displayed: { page: 1, total: 0 } }), () => null).locator.progression).toBeUndefined();
    expect(buildEpubProgress({ start: {} }, () => null)).toBeNull();
    expect(buildEpubProgress(null, () => null)).toBeNull();
    // a percentage that is not a number is not used
    expect(buildEpubProgress(location(), () => NaN).extras.percent).toBeUndefined();
  });

  it('keeps values inside their ranges', () => {
    expect(buildEpubProgress(location({ displayed: { page: 99, total: 10 } }), () => 1.7).locator.progression).toBe(1);
    expect(buildEpubProgress(location(), () => 1.7).extras.percent).toBe(100);
    expect(buildEpubProgress(location(), () => -0.2).extras.percent).toBe(0);
  });
});
