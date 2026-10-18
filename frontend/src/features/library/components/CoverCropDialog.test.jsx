import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import axe from 'axe-core';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../coverCrop', async (importOriginal) => ({ ...(await importOriginal()), cutToBlob: vi.fn() }));

import { cutToBlob } from '../coverCrop';
import { CoverCropDialog } from './CoverCropDialog';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let size = { w: 1200, h: 900 };
let broken = false;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);

class FakeImage {
  set src(value) {
    this._src = value;
    queueMicrotask(() => {
      if (broken) { this.onerror?.(); return; }
      this.naturalWidth = size.w;
      this.naturalHeight = size.h;
      this.onload?.();
    });
  }
  get src() { return this._src; }
}

async function show(props = {}) {
  const file = new File(['x'], 'capa.png', { type: 'image/png' });
  const handlers = { onConfirm: vi.fn(), onCancel: vi.fn() };
  await act(async () => { root.render(<CoverCropDialog file={file} {...handlers} {...props} />); });
  await flush();
  return handlers;
}
const translate = () => container.querySelector('[data-testid="cover-frame"] img').style.transform;
const frame = () => container.querySelector('[data-testid="cover-frame"]');

beforeEach(() => {
  vi.clearAllMocks();
  size = { w: 1200, h: 900 };
  broken = false;
  vi.stubGlobal('Image', FakeImage);
  URL.createObjectURL = vi.fn(() => 'blob:picture');
  URL.revokeObjectURL = vi.fn();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('CoverCropDialog (DEC-173)', () => {
  it('shows the picture under a frame, centred, and says what the proportion is', async () => {
    await show();
    expect(container.querySelector('[role="dialog"]').getAttribute('aria-label')).toBe('Recortar a capa');
    expect(container.textContent).toContain('2:3');
    // 1200×900 at the scale that covers a 200×300 frame is 400×300: it overflows 200 px, which is split between both sides.
    expect(translate()).toBe('translate(-100px, 0px)');
    expect(container.querySelector('[data-testid="cover-frame"] img').style.width).toBe('400px');
  });

  it('moves the picture with the arrow keys, and not past its edge', async () => {
    await show();
    await act(async () => { frame().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true })); });
    expect(translate()).toBe('translate(-112px, 0px)');
    for (let i = 0; i < 20; i++) {
      await act(async () => { frame().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true })); });
    }
    expect(translate()).toBe('translate(-200px, 0px)');
    // The picture is as tall as the frame: nowhere to go up or down.
    await act(async () => { frame().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true })); });
    expect(translate()).toBe('translate(-200px, 0px)');
  });

  it('moves it with the pointer', async () => {
    await show();
    const down = new MouseEvent('pointerdown', { bubbles: true, clientX: 100, clientY: 100 });
    const move = new MouseEvent('pointermove', { bubbles: true, clientX: 130, clientY: 100 });
    await act(async () => { frame().dispatchEvent(down); });
    await act(async () => { frame().dispatchEvent(move); });
    // Dragging the picture to the right shows more of its left side.
    expect(translate()).toBe('translate(-70px, 0px)');
  });

  it('zooms with the slider, keeping the middle of the picture in the middle', async () => {
    await show();
    const slider = container.querySelector('input[type="range"]');
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(slider, '2');
      slider.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect(container.querySelector('[data-testid="cover-frame"] img').style.width).toBe('800px');
    expect(translate()).toBe('translate(-300px, -150px)');
  });

  it('gives the cut, with the picture where it was left, when it is confirmed', async () => {
    const blob = new Blob(['jpeg'], { type: 'image/jpeg' });
    cutToBlob.mockResolvedValue(blob);
    const { onConfirm } = await show();
    await act(async () => { frame().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true })); });
    await act(async () => { button('Usar esta capa').click(); });
    await flush();
    expect(onConfirm).toHaveBeenCalledWith(blob);
    const [, offset, zoom] = cutToBlob.mock.calls[0];
    expect(offset).toEqual({ x: -112, y: 0 });
    expect(zoom).toBe(1);
  });

  it('says so when the cut cannot be made, and sends nothing', async () => {
    cutToBlob.mockRejectedValue(new Error('no canvas'));
    const { onConfirm } = await show();
    await act(async () => { button('Usar esta capa').click(); });
    await flush();
    expect(onConfirm).not.toHaveBeenCalled();
    expect(container.querySelector('[role="alert"]').textContent).toContain('recortar');
  });

  it('refuses a picture that cannot be opened', async () => {
    broken = true;
    await show();
    expect(container.querySelector('[role="alert"]').textContent).toContain('Não foi possível abrir');
    expect(button('Usar esta capa').disabled).toBe(true);
  });

  it('is cancelled with the button and with Escape, but not while it is sending', async () => {
    const { onCancel } = await show();
    await act(async () => { button('Cancelar').click(); });
    expect(onCancel).toHaveBeenCalledTimes(1);
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(onCancel).toHaveBeenCalledTimes(2);

    const sending = await show({ busy: true, error: 'Falhou.' });
    expect(button('Salvando…').disabled).toBe(true);
    expect(button('Cancelar').disabled).toBe(true);
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(sending.onCancel).not.toHaveBeenCalled();
    expect(container.querySelector('[role="alert"]').textContent).toBe('Falhou.');
  });

  it('has no accessibility violations', async () => {
    await show();
    const results = await axe.run(container, { rules: { 'color-contrast': { enabled: false } } });
    expect(results.violations.map((v) => v.id)).toEqual([]);
  });

  it('is the wide frame of the banner of a series when asked, and says so', async () => {
    size = { w: 1500, h: 500 };
    await show({ banner: true });
    const frame = container.querySelector('[data-testid="cover-frame"]');
    expect([frame.style.width, frame.style.height]).toEqual(['360px', '120px']);
    expect(container.querySelector('[role="dialog"]').getAttribute('aria-label')).toBe('Recortar o fundo');
    expect(button('Usar este fundo')).toBeTruthy();
  });
});
