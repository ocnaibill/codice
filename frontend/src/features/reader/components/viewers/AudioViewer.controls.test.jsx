import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import AudioViewer from './AudioViewer';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
// jsdom has no media playback: a player that plays, pauses and says so, as a browser's does.
const player = { paused: true, time: 0, duration: 200, rate: 1, playResult: () => Promise.resolve(), loads: 0 };
const proto = HTMLMediaElement.prototype;
const originals = {};

beforeEach(() => {
  Object.assign(player, { paused: true, time: 0, duration: 200, rate: 1, playResult: () => Promise.resolve(), loads: 0 });
  for (const name of ['paused', 'duration', 'currentTime', 'readyState', 'playbackRate', 'play', 'pause', 'load']) originals[name] = Object.getOwnPropertyDescriptor(proto, name);
  Object.defineProperty(proto, 'paused', { configurable: true, get: () => player.paused });
  Object.defineProperty(proto, 'duration', { configurable: true, get: () => player.duration });
  Object.defineProperty(proto, 'readyState', { configurable: true, get: () => 4 });
  Object.defineProperty(proto, 'currentTime', { configurable: true, get: () => player.time, set: (v) => { player.time = v; } });
  Object.defineProperty(proto, 'playbackRate', { configurable: true, get: () => player.rate, set: (v) => { player.rate = v; } });
  Object.defineProperty(proto, 'play', {
    configurable: true,
    value() {
      const result = player.playResult();
      result.then(() => { player.paused = false; this.dispatchEvent(new Event('play')); }, () => {});
      return result;
    },
  });
  Object.defineProperty(proto, 'pause', { configurable: true, value() { player.paused = true; this.dispatchEvent(new Event('pause')); } });
  Object.defineProperty(proto, 'load', { configurable: true, value() { player.loads += 1; } });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  for (const [name, d] of Object.entries(originals)) {
    if (d) Object.defineProperty(proto, name, d); else delete proto[name];
  }
  vi.restoreAllMocks();
});

async function open(props = {}) {
  await act(async () => { root.render(<AudioViewer fileUrl="/a.mp3" onProgress={vi.fn().mockResolvedValue({})} {...props} />); });
}
const audio = () => container.querySelector('audio');
const button = (label) => container.querySelector(`button[aria-label="${label}"]`);
const click = (el) => act(async () => { el.click(); });
const slider = () => container.querySelector('input[type="range"]');
const keydown = (key, target = window, init = {}) => act(async () => { target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...init })); });
const tick = (t) => act(async () => { player.time = t; audio().dispatchEvent(new Event('timeupdate')); });

describe('the audio player: play and pause', () => {
  it('plays and pauses from the same button, which says what it will do', async () => {
    await open();
    expect(button('Tocar')).not.toBeNull();
    await click(button('Tocar'));
    expect(player.paused).toBe(false);
    expect(button('Pausar')).not.toBeNull();
    expect(button('Tocar')).toBeNull();
    await click(button('Pausar'));
    expect(player.paused).toBe(true);
    expect(button('Tocar')).not.toBeNull();
  });

  it('shows what the player does, not what was asked: a pause from outside (a headset) shows as paused', async () => {
    await open();
    await click(button('Tocar'));
    await act(async () => { player.paused = true; audio().dispatchEvent(new Event('pause')); });
    expect(button('Tocar')).not.toBeNull();
    await act(async () => { player.paused = false; audio().dispatchEvent(new Event('play')); });
    expect(button('Pausar')).not.toBeNull();
  });

  it('shows paused when the audio ends', async () => {
    await open();
    await click(button('Tocar'));
    await act(async () => { player.paused = true; audio().dispatchEvent(new Event('pause')); audio().dispatchEvent(new Event('ended')); });
    expect(button('Tocar')).not.toBeNull();
  });

  it('says so, in words, when the browser does not let it play', async () => {
    player.playResult = () => Promise.reject(new Error('NotAllowedError'));
    await open();
    await click(button('Tocar'));
    await act(async () => { await Promise.resolve(); });
    expect(container.querySelector('[role="alert"]').textContent).toContain('Não foi possível tocar este áudio.');
    expect(button('Tocar')).not.toBeNull();
  });

  it('takes the message away when it is told to play again', async () => {
    player.playResult = () => Promise.reject(new Error('x'));
    await open();
    await click(button('Tocar'));
    await act(async () => { await Promise.resolve(); });
    player.playResult = () => Promise.resolve();
    await click(button('Tocar'));
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('says it when the file cannot be loaded, and loads it again when asked', async () => {
    await open();
    await act(async () => { audio().dispatchEvent(new Event('error')); });
    const alert = container.querySelector('[role="alert"]');
    expect(alert.textContent).toContain('Não foi possível carregar este áudio.');
    await click([...alert.querySelectorAll('button')].find((b) => b.textContent === 'Tentar de novo'));
    expect(player.loads).toBe(1);
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });
});

describe('the audio player: moving through it', () => {
  it('goes back 15 seconds and forward 30, and says where it got to', async () => {
    await open();
    await tick(100);
    await click(button('Avançar 30 segundos'));
    expect(player.time).toBe(130);
    await click(button('Voltar 15 segundos'));
    expect(player.time).toBe(115);
    expect(container.textContent).toContain('1:55');
  });

  it('does not go before the start or past the end', async () => {
    await open();
    await tick(5);
    await click(button('Voltar 15 segundos'));
    expect(player.time).toBe(0);
    await tick(190);
    await click(button('Avançar 30 segundos'));
    expect(player.time).toBe(200);
  });

  it('saves the place that skipping went to, in milliseconds', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    await tick(100);
    await click(button('Avançar 30 segundos'));
    await act(async () => { await new Promise((r) => setTimeout(r, 1600)); });
    expect(onProgress.mock.calls.at(-1)[0]).toEqual({ type: 'audio', track: 0, ms: 130000 });
  });

  it('is in the slider, which can be moved to any point, and says the time in words', async () => {
    await open();
    expect(slider().getAttribute('aria-label')).toBe('Posição no áudio');
    expect(slider().max).toBe('200');
    expect(slider().getAttribute('aria-valuetext')).toBe('0 segundos de 3 minutos e 20 segundos');
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
    await act(async () => { set.call(slider(), '75'); slider().dispatchEvent(new Event('input', { bubbles: true })); });
    expect(player.time).toBe(75);
    expect(slider().getAttribute('aria-valuetext')).toBe('1 minuto e 15 segundos de 3 minutos e 20 segundos');
  });

  it('saves the place the slider was moved to', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
    await act(async () => { set.call(slider(), '75'); slider().dispatchEvent(new Event('input', { bubbles: true })); });
    await act(async () => { await new Promise((r) => setTimeout(r, 1600)); });
    expect(onProgress.mock.calls.at(-1)[0]).toEqual({ type: 'audio', track: 0, ms: 75000 });
  });

  it('shows the time and the duration, with hours when the audio is long', async () => {
    player.duration = 3725;
    await open();
    await tick(754);
    expect(container.textContent).toContain('12:34');
    expect(container.textContent).toContain('1:02:05');
  });

  it('cannot be moved before it knows how long it is', async () => {
    player.duration = NaN;
    await open();
    expect(slider().disabled).toBe(true);
    expect(button('Voltar 15 segundos').disabled).toBe(true);
    expect(button('Avançar 30 segundos').disabled).toBe(true);
    expect(container.textContent).toContain('–:––');
    expect(slider().getAttribute('aria-valuetext')).toContain('duração desconhecida');
  });

  it('can be moved once it knows', async () => {
    await open();
    expect(slider().disabled).toBe(false);
    expect(button('Voltar 15 segundos').disabled).toBe(false);
  });
});

describe('the audio player: speed', () => {
  it('goes through the speeds and back to the first, and says which it is at', async () => {
    await open();
    const seen = [];
    for (let i = 0; i < 6; i += 1) {
      const b = container.querySelector('button[aria-label^="Velocidade"]');
      seen.push([b.textContent, player.rate]);
      await click(b);
    }
    expect(seen).toEqual([['1×', 1], ['1.25×', 1.25], ['1.5×', 1.5], ['2×', 2], ['0.5×', 0.5], ['0.75×', 0.75]]);
    expect(container.querySelector('button[aria-label^="Velocidade"]').textContent).toBe('1×');
    expect(container.querySelector('button[aria-label^="Velocidade"]').getAttribute('aria-label')).toBe('Velocidade 1×, mudar');
  });
});

describe('the audio player: the keyboard', () => {
  it('plays and pauses with the space bar, and goes back and forward with the arrows', async () => {
    await open();
    await tick(100);
    await keydown(' ');
    expect(player.paused).toBe(false);
    await keydown(' ');
    expect(player.paused).toBe(true);
    await keydown('ArrowRight');
    expect(player.time).toBe(130);
    await keydown('ArrowLeft');
    expect(player.time).toBe(115);
  });

  it('keeps the page from scrolling when the space bar is used for it', async () => {
    await open();
    const event = new KeyboardEvent('keydown', { key: ' ', bubbles: true, cancelable: true });
    await act(async () => { window.dispatchEvent(event); });
    expect(event.defaultPrevented).toBe(true);
  });

  it('is not taken from someone who is typing, from the slider or a button (which use the same keys), or from a shortcut', async () => {
    await open();
    await tick(100);
    const field = document.createElement('input');
    document.body.appendChild(field);
    await keydown(' ', field);
    await keydown('ArrowRight', field);
    await keydown(' ', slider());
    await keydown('ArrowRight', slider());
    await keydown(' ', button('Tocar'));
    await keydown('ArrowRight', window, { ctrlKey: true });
    await keydown('ArrowRight', window, { metaKey: true });
    await keydown('ArrowRight', window, { altKey: true });
    expect(player.paused).toBe(true);
    expect(player.time).toBe(100);
    field.remove();
  });

  it('does not skip before it knows how long it is', async () => {
    player.duration = NaN;
    await open();
    await keydown('ArrowRight');
    expect(player.time).toBe(0);
  });
});
