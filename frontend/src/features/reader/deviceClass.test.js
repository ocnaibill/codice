import { afterEach, describe, expect, it, vi } from 'vitest';
import { DEVICES, deviceClass, otherDevice } from './deviceClass';

afterEach(() => vi.unstubAllGlobals());

describe('what kind of device this is (#180)', () => {
  it('is a touch device when the screen is pointed at with a finger and cannot hover', () => {
    const asked = [];
    vi.stubGlobal('matchMedia', (query) => { asked.push(query); return { matches: true }; });
    expect(deviceClass()).toBe('touch');
    expect(asked).toEqual(['(hover: none) and (pointer: coarse)']);
  });

  it('is a computer when it is not', () => {
    vi.stubGlobal('matchMedia', () => ({ matches: false }));
    expect(deviceClass()).toBe('desktop');
  });

  it('is a computer when it cannot be told (no way to ask, or asking fails)', () => {
    vi.stubGlobal('matchMedia', undefined);
    expect(deviceClass()).toBe('desktop');
    vi.stubGlobal('matchMedia', () => { throw new Error('no'); });
    expect(deviceClass()).toBe('desktop');
  });

  it('knows the two kinds and the other of each', () => {
    expect(DEVICES).toEqual(['touch', 'desktop']);
    expect(otherDevice('touch')).toBe('desktop');
    expect(otherDevice('desktop')).toBe('touch');
  });
});
