import { describe, it, expect, vi, afterEach } from 'vitest';
import {
  FRAME_WIDTH, FRAME_HEIGHT, OUTPUT_WIDTH, OUTPUT_HEIGHT, baseScale, centered, clampOffset, cutToBlob, sourceRect, zoomAround,
} from './coverCrop';

afterEach(() => vi.restoreAllMocks());

describe('the geometry of the cut of a cover (DEC-173)', () => {
  it('scales the picture so that it covers the frame, whatever its proportion', () => {
    // Wider than 2:3: the height decides. Taller than 2:3: the width does.
    expect(baseScale(1200, FRAME_HEIGHT)).toBeCloseTo(1);
    expect(baseScale(FRAME_WIDTH, 1800)).toBeCloseTo(1);
    expect(baseScale(600, 900)).toBeCloseTo(FRAME_WIDTH / 600);
  });

  it('starts with the picture centred and, if it is 2:3, shown whole', () => {
    expect(centered(600, 900)).toEqual({ x: 0, y: 0 });
    // A landscape picture overflows left and right, evenly.
    const o = centered(1200, FRAME_HEIGHT);
    expect(o.y).toBe(0);
    expect(o.x).toBeCloseTo(-(1200 - FRAME_WIDTH) / 2);
  });

  it('never lets the picture leave an empty edge in the frame', () => {
    const o = clampOffset({ x: 50, y: -9999 }, 600, 900, 2);
    expect(o.x).toBe(0);
    expect(o.y).toBeCloseTo(FRAME_HEIGHT - 900 * baseScale(600, 900) * 2);
    // At zoom 1 a 2:3 picture has nowhere to go.
    expect(clampOffset({ x: -30, y: 40 }, 600, 900, 1)).toEqual({ x: 0, y: 0 });
  });

  it('keeps the point at the centre of the frame when the zoom changes', () => {
    const before = centered(1200, 900, 1);
    const after = zoomAround(before, 1200, 900, 1, 2);
    const centreOf = (offset, zoom) => ({
      x: (FRAME_WIDTH / 2 - offset.x) / (baseScale(1200, 900) * zoom),
      y: (FRAME_HEIGHT / 2 - offset.y) / (baseScale(1200, 900) * zoom),
    });
    expect(centreOf(after, 2).x).toBeCloseTo(centreOf(before, 1).x);
    expect(centreOf(after, 2).y).toBeCloseTo(centreOf(before, 1).y);
  });

  it('says which part of the picture the frame shows, in the pixels of the picture', () => {
    const whole = sourceRect({ x: 0, y: 0 }, 600, 900, 1);
    expect(whole).toEqual({ x: 0, y: 0, width: 600, height: 900 });
    const half = sourceRect({ x: 0, y: 0 }, 600, 900, 2);
    expect(half.width).toBeCloseTo(300);
    expect(half.height).toBeCloseTo(450);
    // Moved: the picture's corner went left and up by 60 frame pixels, which is 150 of its own at this scale.
    const moved = sourceRect({ x: -60, y: -90 }, 1200, 1800, 1.5);
    const scale = baseScale(1200, 1800) * 1.5;
    expect(moved.x).toBeCloseTo(60 / scale);
    expect(moved.y).toBeCloseTo(90 / scale);
  });

  it('draws the part shown into 600×900, over white, and gives a JPEG', async () => {
    const calls = [];
    const context = {
      fillRect: (...a) => calls.push(['fillRect', ...a]),
      drawImage: (...a) => calls.push(['drawImage', ...a]),
    };
    let size;
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function ctx() {
      size = [this.width, this.height];
      return context;
    });
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((cb, type, quality) => cb(new Blob(['x'], { type: `${type};q=${quality}` })));
    const image = { naturalWidth: 600, naturalHeight: 900 };
    const blob = await cutToBlob(image, { x: 0, y: 0 }, 2);
    expect(size).toEqual([OUTPUT_WIDTH, OUTPUT_HEIGHT]);
    expect(calls[0]).toEqual(['fillRect', 0, 0, OUTPUT_WIDTH, OUTPUT_HEIGHT]);
    const draw = calls.find((c) => c[0] === 'drawImage');
    expect(draw[1]).toBe(image);
    expect(draw[2]).toBeCloseTo(0);
    expect(draw[4]).toBeCloseTo(300);
    expect(draw.slice(6)).toEqual([0, 0, OUTPUT_WIDTH, OUTPUT_HEIGHT]);
    expect(blob.type).toMatch(/^image\/jpeg/);
  });

  it('fails, rather than sending nothing, when the browser cannot make the JPEG', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ fillRect() {}, drawImage() {} });
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((cb) => cb(null));
    await expect(cutToBlob({ naturalWidth: 600, naturalHeight: 900 }, { x: 0, y: 0 }, 1)).rejects.toThrow();
  });
});
