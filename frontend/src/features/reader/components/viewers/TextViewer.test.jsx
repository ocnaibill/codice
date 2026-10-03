import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import TextViewer from './TextViewer';
import { setPreferenceOwner } from '../../preferences';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let onImmersiveChange;
let sizes;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });
const TEXT = 'Era uma vez um texto longo que se lê rolando a tela.';

function Scene(props) {
  return (
    <div id="scroller" style={{ overflowY: 'auto' }}>
      <TextViewer fileUrl="/files/a.txt" onProgress={vi.fn().mockResolvedValue({})} onImmersiveChange={onImmersiveChange} {...props} />
    </div>
  );
}
async function open(props = {}, body = TEXT) {
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, status: 200, text: async () => body })));
  await act(async () => { root.render(<Scene {...props} />); });
  await flush();
}
const scroller = () => container.querySelector('#scroller');
const button = (label) => container.querySelector(`button[aria-label="${label}"]`);
const click = (el) => act(async () => { el.click(); });
const chrome = () => container.querySelector('[data-chrome]');
const page = () => container.querySelector('#scroller').firstElementChild;
const text = () => container.querySelector('[data-reading-text]');
const keydown = (key, target = window) => act(async () => { target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true })); });
const scrollTo = (top) => act(async () => { scroller().scrollTop = top; scroller().dispatchEvent(new Event('scroll')); });

// A finger: down at `from`, up at `to` `ms` later, on `target` (the text by default).
async function touch({ from = [200, 300], to = from, ms = 80, type = 'touch', button: btn = 0, target = null } = {}) {
  const el = target ?? text() ?? page();
  const fire = (name, [x, y]) => {
    const event = new MouseEvent(name, { bubbles: true, cancelable: true, clientX: x, clientY: y, button: btn });
    Object.defineProperty(event, 'pointerType', { value: type });
    el.dispatchEvent(event);
  };
  await act(async () => { fire('pointerdown', from); });
  vi.setSystemTime(Date.now() + ms);
  await act(async () => { fire('pointerup', to); });
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  localStorage.clear();
  setPreferenceOwner('ana');
  onImmersiveChange = vi.fn();
  sizes = { scrollHeight: 2000, clientHeight: 500 };
  Object.defineProperty(HTMLElement.prototype, 'scrollHeight', { configurable: true, get() { return this.id === 'scroller' ? sizes.scrollHeight : 0; } });
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, get() { return this.id === 'scroller' ? sizes.clientHeight : 0; } });
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  delete HTMLElement.prototype.scrollHeight;
  delete HTMLElement.prototype.clientHeight;
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('the text reader: opening the file', () => {
  it('shows the text as it is, on the paper page', async () => {
    await open();
    expect(container.querySelector('pre').textContent).toBe(TEXT);
    expect(page().style.backgroundColor).toBe('rgb(250, 248, 244)');
    expect(text().style.color).toBe('rgb(24, 24, 27)');
    expect(text().style.fontSize).toBe('16px');
  });

  it('shows a placeholder while the text comes', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})));
    await act(async () => { root.render(<Scene />); });
    expect(container.querySelector('[role="status"]').getAttribute('aria-label')).toBe('Carregando o texto');
    expect(container.querySelector('pre')).toBeNull();
  });

  it('says in words that it could not, with the reason, and asks again when told to', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 404 })
      .mockResolvedValueOnce({ ok: true, status: 200, text: async () => TEXT });
    vi.stubGlobal('fetch', fetchMock);
    await act(async () => { root.render(<Scene />); });
    await flush();
    const alert = container.querySelector('[role="alert"]');
    expect(alert.textContent).toContain('Não foi possível abrir este texto.');
    expect(alert.textContent).toContain('O servidor respondeu com o erro 404.');
    expect(container.querySelector('[role="status"]')).toBeNull();
    await click([...alert.querySelectorAll('button')].find((b) => b.textContent === 'Tentar de novo'));
    await flush();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(container.querySelector('pre').textContent).toBe(TEXT);
  });

  it('shows the placeholder again while it asks again, and not the failure', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: false, status: 500 }).mockImplementationOnce(() => new Promise(() => {}));
    vi.stubGlobal('fetch', fetchMock);
    await act(async () => { root.render(<Scene />); });
    await flush();
    await click(container.querySelector('[role="alert"] button'));
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(container.querySelector('[role="status"]')).not.toBeNull();
  });

  it('is not changed by a file that arrives late, when another one was asked for since', async () => {
    const pending = {};
    vi.stubGlobal('fetch', vi.fn((url) => new Promise((resolve, reject) => { pending[url] = { resolve, reject }; })));
    await act(async () => { root.render(<Scene fileUrl="/a.txt" />); });
    await act(async () => { root.render(<Scene fileUrl="/b.txt" />); });
    await act(async () => { pending['/b.txt'].resolve({ ok: true, status: 200, text: async () => 'texto de b' }); });
    await flush();
    await act(async () => { pending['/a.txt'].resolve({ ok: true, status: 200, text: async () => 'texto de a' }); });
    await flush();
    expect(container.querySelector('pre').textContent).toBe('texto de b');
    // and one that fails late does not put an error over the one that came
    vi.stubGlobal('fetch', vi.fn((url) => new Promise((resolve, reject) => { pending[url] = { resolve, reject }; })));
    await act(async () => { root.render(<Scene fileUrl="/c.txt" />); });
    await act(async () => { root.render(<Scene fileUrl="/d.txt" />); });
    await act(async () => { pending['/d.txt'].resolve({ ok: true, status: 200, text: async () => 'texto de d' }); });
    await flush();
    await act(async () => { pending['/c.txt'].reject(new Error('caiu')); });
    await flush();
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(container.querySelector('pre').textContent).toBe('texto de d');
  });

  it('says a network failure the same way', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new Error('Failed to fetch'); }));
    await act(async () => { root.render(<Scene />); });
    await flush();
    expect(container.querySelector('[role="alert"]').textContent).toContain('Failed to fetch');
  });

  it('says so when the file is empty', async () => {
    await open({}, '  \n ');
    expect(container.textContent).toContain('Este arquivo está vazio.');
    expect(container.querySelector('pre')).toBeNull();
  });

  it('goes back to where it was left, and does not save that as a new place', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress, initialProgress: String(Math.round(TEXT.length / 2)) });
    expect(scroller().scrollTop).toBe(750);
    vi.useRealTimers();
    await wait(1100);
    expect(onProgress).not.toHaveBeenCalled();
  });
});

describe('the text reader: how far through', () => {
  it('says so, as the text is scrolled', async () => {
    await open();
    expect(chrome().textContent).toContain('0%');
    await scrollTo(750);
    expect(chrome().textContent).toContain('50%');
    expect(container.querySelector('[data-chrome-mark]').textContent).toBe('50%');
    await scrollTo(1500);
    expect(chrome().textContent).toContain('100%');
  });

  it('rounds to the nearest whole number', async () => {
    await open();
    await scrollTo(760); // 50.67%
    expect(chrome().textContent).toContain('51%');
  });

  it('saves where the person got to, as the text is scrolled', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ onProgress });
    vi.useRealTimers();
    await scrollTo(750);
    await wait(1100);
    expect(onProgress).toHaveBeenCalledTimes(1);
    expect(onProgress.mock.calls[0][0]).toEqual({ type: 'text', offset: Math.round(TEXT.length / 2) });
    expect(onProgress.mock.calls[0][1].percent).toBe(50);
  });

  it('a text that fits on the screen has been seen to its end', async () => {
    sizes = { scrollHeight: 300, clientHeight: 500 };
    await open();
    expect(chrome().textContent).toContain('100%');
  });

  it('shows no percentage while there is no text', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})));
    await act(async () => { root.render(<Scene />); });
    expect(chrome().textContent).toContain('–');
  });

  it('scrolls a screen at a time, by 90% of what is seen', async () => {
    await open();
    scroller().scrollBy = vi.fn();
    await click(button('Rolar para baixo'));
    expect(scroller().scrollBy).toHaveBeenLastCalledWith({ top: 450, behavior: 'smooth' });
    await click(button('Rolar para cima'));
    expect(scroller().scrollBy).toHaveBeenLastCalledWith({ top: -450, behavior: 'smooth' });
  });

  it('copes with a scroller that cannot scroll by', async () => {
    await open();
    delete scroller().scrollBy;
    await click(button('Rolar para baixo'));
    expect(chrome()).not.toBeNull();
  });
});

describe('the text reader: the look of the text', () => {
  const settingsKey = 'codice:epub-settings:ana';
  const panel = () => container.querySelector('[role="dialog"]');
  const radio = (group, label) => [...panel().querySelector(`[role="group"][aria-label="${group}"]`).querySelectorAll('[role="radio"]')].find((b) => b.textContent === label);

  it('starts with what was chosen, as the EPUB reader keeps it (the same choice for both)', async () => {
    localStorage.setItem(settingsKey, JSON.stringify({ theme: 'preto', font: 'serifada', size: 150, spacing: 'ampla' }));
    await open();
    expect(page().style.backgroundColor).toBe('rgb(0, 0, 0)');
    expect(text().style.color).toBe('rgb(212, 212, 216)');
    expect(text().style.fontSize).toBe('24px');
    expect(text().style.lineHeight).toBe('1.8');
    expect(text().style.fontFamily).toContain('Newsreader Variable');
  });

  it('opens a panel from a button and closes it from the same button', async () => {
    await open();
    expect(panel()).toBeNull();
    await click(button('Aparência do texto'));
    expect(panel()).not.toBeNull();
    expect(button('Aparência do texto').getAttribute('aria-expanded')).toBe('true');
    await click(button('Aparência do texto'));
    expect(panel()).toBeNull();
  });

  it('calls what the file has "Padrão", and not "Do livro"', async () => {
    await open();
    await click(button('Aparência do texto'));
    expect(radio('Fonte', 'Padrão')).toBeDefined();
    expect(radio('Entrelinha', 'Padrão')).toBeDefined();
    expect(radio('Fonte', 'Do livro')).toBeUndefined();
  });

  it('changes the page color, the size, the font and the spacing at once, and remembers them', async () => {
    await open();
    await click(button('Aparência do texto'));
    await click(button('Sépia'));
    expect(page().style.backgroundColor).toBe('rgb(242, 232, 213)');
    expect(text().style.color).toBe('rgb(74, 55, 40)');
    await click(button('Aumentar a letra'));
    await click(button('Aumentar a letra'));
    expect(text().style.fontSize).toBe('19.2px');
    await click(radio('Fonte', 'Sem serifa'));
    expect(text().style.fontFamily).toContain('Plus Jakarta Sans Variable');
    await click(radio('Entrelinha', 'Média'));
    expect(text().style.lineHeight).toBe('1.5');
    expect(JSON.parse(localStorage.getItem(settingsKey))).toEqual({ theme: 'sepia', font: 'sem-serifa', size: 120, spacing: 'media' });
  });

  it('does not take the style of the person from the file', async () => {
    localStorage.setItem(settingsKey, JSON.stringify({ theme: '"><script>', size: 999 }));
    await open();
    expect(page().style.backgroundColor).toBe('rgb(250, 248, 244)');
    expect(text().style.fontSize).toBe('16px');
  });

  it('stays at the same fraction of the text when the letter gets bigger and the text longer', async () => {
    await open();
    await scrollTo(750); // half
    await click(button('Aparência do texto'));
    sizes = { scrollHeight: 3000, clientHeight: 500 }; // the text is longer once the letter is bigger
    await click(button('Aumentar a letra'));
    expect(scroller().scrollTop).toBe(1250); // half of 3000 - 500
  });

  it('does the same for a change of font or of spacing', async () => {
    await open();
    await scrollTo(750);
    await click(button('Aparência do texto'));
    const radio = (group, label) => [...container.querySelector(`[role="group"][aria-label="${group}"]`).querySelectorAll('[role="radio"]')].find((b) => b.textContent === label);
    sizes = { scrollHeight: 3000, clientHeight: 500 };
    await click(radio('Fonte', 'Serifada'));
    expect(scroller().scrollTop).toBe(1250);
    await scrollTo(250); // a fifth
    sizes = { scrollHeight: 4000, clientHeight: 500 };
    await click(radio('Entrelinha', 'Ampla'));
    expect(scroller().scrollTop).toBe(350);
  });

  it('does not move the text for a change of the color of the page', async () => {
    await open();
    await scrollTo(750);
    await click(button('Aparência do texto'));
    sizes = { scrollHeight: 3000, clientHeight: 500 };
    await click(button('Sépia'));
    expect(scroller().scrollTop).toBe(750);
  });

  it('closes by Escape, and by a press outside the page, but not by a press inside it', async () => {
    await open();
    await click(button('Aparência do texto'));
    await act(async () => { panel().dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); });
    expect(panel()).not.toBeNull();
    await act(async () => { document.body.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); });
    expect(panel()).toBeNull();
    await click(button('Aparência do texto'));
    await keydown('Escape');
    expect(panel()).toBeNull();
    expect(onImmersiveChange).not.toHaveBeenCalled(); // that Escape was for the panel
  });

  it('goes away with the controls when they are hidden', async () => {
    await open();
    await click(button('Aparência do texto'));
    await act(async () => { root.render(<Scene immersive />); });
    expect(panel()).toBeNull();
  });
});

describe('the text reader: the controls', () => {
  it('hide, leaving a mark of the place, out of reach while hidden', async () => {
    await open();
    expect(chrome().dataset.chrome).toBe('shown');
    expect(chrome().hasAttribute('inert')).toBe(false);
    expect(container.querySelector('[data-chrome-mark]').getAttribute('aria-hidden')).toBe('true');
    await click(button('Esconder os controles'));
    expect(onImmersiveChange).toHaveBeenCalledWith(true);
    await act(async () => { root.render(<Scene immersive onImmersiveChange={onImmersiveChange} />); });
    expect(chrome().dataset.chrome).toBe('hidden');
    expect(chrome().hasAttribute('inert')).toBe(true);
    const mark = container.querySelector('[data-chrome-mark]');
    expect(mark.getAttribute('aria-hidden')).toBe('false');
    expect(mark.className).toContain('!opacity-60');
  });

  it('come back with Escape, and Escape does nothing when they are shown', async () => {
    await act(async () => { root.render(<Scene immersive />); });
    await flush();
    await keydown('Escape');
    expect(onImmersiveChange).toHaveBeenCalledWith(false);
    onImmersiveChange.mockClear();
    await act(async () => { root.render(<Scene immersive={false} />); });
    await keydown('Escape');
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('do not take Escape from someone who is typing', async () => {
    await act(async () => { root.render(<Scene immersive />); });
    await flush();
    const field = document.createElement('input');
    document.body.appendChild(field);
    await keydown('Escape', field);
    expect(onImmersiveChange).not.toHaveBeenCalled();
    field.remove();
  });
});

describe('the text reader under a finger', () => {
  it('a tap on the text hides the controls, and shows them when they are hidden', async () => {
    await open();
    await touch();
    expect(onImmersiveChange).toHaveBeenLastCalledWith(true);
    await act(async () => { root.render(<Scene immersive onImmersiveChange={onImmersiveChange} />); });
    await touch();
    expect(onImmersiveChange).toHaveBeenLastCalledWith(false);
  });

  it('a tap anywhere on the page does it, not only on the lines', async () => {
    await open();
    await touch({ target: page() });
    expect(onImmersiveChange).toHaveBeenCalledTimes(1);
  });

  it('a mouse does not: it has the button and Escape, and clicking a text is for selecting it', async () => {
    await open();
    await touch({ type: 'mouse' });
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('a pen does', async () => {
    await open();
    await touch({ type: 'pen' });
    expect(onImmersiveChange).toHaveBeenCalledTimes(1);
  });

  it('a finger that moved or stayed is scrolling or pressing, not tapping', async () => {
    await open();
    await touch({ from: [200, 300], to: [200, 360] });
    await touch({ from: [200, 300], ms: 900 });
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('does nothing on a link or a button, or while text is selected, or for a secondary button', async () => {
    await open();
    await touch({ target: button('Aparência do texto') });
    const link = document.createElement('a');
    link.href = '#x';
    text().appendChild(link);
    await touch({ target: link });
    await touch({ button: 2 });
    vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => 'um trecho' });
    await touch();
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('with the panel open, a tap only closes it', async () => {
    await open();
    await click(button('Aparência do texto'));
    await touch();
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('a cancelled touch (a scroll that starts) is not a tap, and an up with no down is nothing', async () => {
    await open();
    const el = text();
    await act(async () => {
      const down = new MouseEvent('pointerdown', { bubbles: true, clientX: 10, clientY: 10 });
      Object.defineProperty(down, 'pointerType', { value: 'touch' });
      el.dispatchEvent(down);
      el.dispatchEvent(new MouseEvent('pointercancel', { bubbles: true }));
    });
    const up = new MouseEvent('pointerup', { bubbles: true, clientX: 10, clientY: 10 });
    Object.defineProperty(up, 'pointerType', { value: 'touch' });
    await act(async () => { el.dispatchEvent(up); });
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });

  it('leaves the vertical scroll and the pinch to the browser', async () => {
    await open();
    expect(page().style.touchAction).toBe('pan-y pinch-zoom');
  });
});
