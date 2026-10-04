import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import MangaViewer from './MangaViewer';
import { setPreferenceOwner } from '../../preferences';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let onImmersiveChange;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const pagesOf = (n) => Array.from({ length: n }, (_, i) => `${i + 1}.jpg`);

async function open({ count = 10, mode = 'ltr', progress = '4', ...props } = {}) {
  localStorage.setItem('codice:comic-mode:ana', mode);
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => pagesOf(count) })));
  await act(async () => {
    root.render(<MangaViewer fileUrl="/files/7/x.cbz" workId={7} initialProgress={progress} onProgress={vi.fn().mockResolvedValue({})} onImmersiveChange={onImmersiveChange} {...props} />);
  });
  await flush();
}
const button = (label) => container.querySelector(`button[aria-label="${label}"]`);
const click = (el) => act(async () => { el.click(); });
const shown = () => [...container.querySelectorAll('img')].map((i) => i.getAttribute('alt'));
const counter = () => container.querySelector('[data-chrome="shown"], [data-chrome="hidden"]').textContent;
const key = (k, init = {}, target = window) => act(async () => { target.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, ...init })); });

beforeEach(() => {
  localStorage.clear();
  setPreferenceOwner('ana');
  onImmersiveChange = vi.fn();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('the comic reader: the buttons turn the page toward the side they are on', () => {
  it('left to right: the button at the right goes forward and the one at the left goes back', async () => {
    await open();
    expect(shown()).toEqual(['Página 5']);
    await click(button('Próxima página, à direita'));
    expect(shown()).toEqual(['Página 6']);
    await click(button('Página anterior, à esquerda'));
    await click(button('Página anterior, à esquerda'));
    expect(shown()).toEqual(['Página 4']);
    expect(counter()).toContain('4 / 10');
  });

  it('right to left: the button at the left goes forward and the one at the right goes back', async () => {
    await open({ mode: 'rtl' });
    await click(button('Próxima página, à esquerda'));
    expect(shown()).toEqual(['Página 6']);
    await click(button('Página anterior, à direita'));
    await click(button('Página anterior, à direita'));
    expect(shown()).toEqual(['Página 4']);
  });

  it('cannot go before the first page or past the last, whichever side is which', async () => {
    await open({ progress: '0' });
    expect(button('Página anterior, à esquerda').disabled).toBe(true);
    expect(button('Próxima página, à direita').disabled).toBe(false);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ progress: '9' });
    expect(button('Próxima página, à direita').disabled).toBe(true);
    expect(button('Página anterior, à esquerda').disabled).toBe(false);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ mode: 'rtl', progress: '0' });
    expect(button('Próxima página, à esquerda').disabled).toBe(false);
    expect(button('Página anterior, à direita').disabled).toBe(true);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ mode: 'rtl', progress: '9' });
    expect(button('Próxima página, à esquerda').disabled).toBe(true);
    expect(button('Página anterior, à direita').disabled).toBe(false);
  });

  it('two pages together: goes by two, shows both and the range', async () => {
    await open({ mode: 'double', progress: '2' });
    expect(shown()).toEqual(['Página 3', 'Página 4']);
    expect(counter()).toContain('3-4 / 10');
    await click(button('Próxima página, à direita'));
    expect(shown()).toEqual(['Página 5', 'Página 6']);
    await click(button('Página anterior, à esquerda'));
    expect(shown()).toEqual(['Página 3', 'Página 4']);
  });

  it('two pages together: the last spread may be a single page, and is the end', async () => {
    await open({ mode: 'double', count: 5, progress: '4' });
    expect(shown()).toEqual(['Página 5']);
    expect(counter()).toContain('5 / 5');
    expect(button('Próxima página, à direita').disabled).toBe(true);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ mode: 'double', count: 5, progress: '2' });
    expect(shown()).toEqual(['Página 3', 'Página 4']);
    expect(button('Próxima página, à direita').disabled).toBe(false);
    await click(button('Próxima página, à direita'));
    expect(shown()).toEqual(['Página 5']);
  });

  it('two pages together: a spread that ends on the last page is the end', async () => {
    await open({ mode: 'double', count: 6, progress: '4' });
    expect(shown()).toEqual(['Página 5', 'Página 6']);
    expect(button('Próxima página, à direita').disabled).toBe(true);
  });
});

describe('the comic reader: the keyboard', () => {
  it('the arrows go to the side they point to: forward at the right, but at the left when read right to left', async () => {
    await open();
    await key('ArrowRight');
    expect(shown()).toEqual(['Página 6']);
    await key('ArrowLeft');
    await key('ArrowLeft');
    expect(shown()).toEqual(['Página 4']);
    act(() => root.unmount());
    root = createRoot(container);
    await open({ mode: 'rtl' });
    await key('ArrowLeft');
    expect(shown()).toEqual(['Página 6']);
    await key('ArrowRight');
    await key('ArrowRight');
    expect(shown()).toEqual(['Página 4']);
  });

  it('two pages together: the arrows go by two', async () => {
    await open({ mode: 'double', progress: '2' });
    await key('ArrowRight');
    expect(shown()).toEqual(['Página 5', 'Página 6']);
  });

  it('does not go past the ends: at the last spread of two there is nothing to turn to, and no progress is saved for it', async () => {
    const onProgress = vi.fn().mockResolvedValue({});
    await open({ mode: 'double', count: 6, progress: '4', onProgress });
    await key('ArrowRight');
    expect(shown()).toEqual(['Página 5', 'Página 6']);
    for (const [progress, arrow] of [['4', 'ArrowRight'], ['0', 'ArrowLeft'], ['5', 'ArrowRight']]) {
      act(() => root.unmount());
      root = createRoot(container);
      await open({ mode: progress === '4' ? 'double' : 'ltr', count: 6, progress, onProgress });
      await key(arrow);
      await act(async () => { await new Promise((r) => setTimeout(r, 1100)); }); // the save is debounced by a second
    }
    expect(onProgress).not.toHaveBeenCalled();
  });

  it('is not taken from someone who is typing, and not used with a modifier', async () => {
    await open();
    const field = document.createElement('input');
    document.body.appendChild(field);
    await key('ArrowRight', {}, field);
    await key('ArrowRight', { ctrlKey: true });
    await key('ArrowRight', { metaKey: true });
    await key('ArrowRight', { altKey: true });
    expect(shown()).toEqual(['Página 5']);
    field.remove();
  });

  it('Escape shows the controls again when they are hidden, and does nothing when they are shown', async () => {
    await open({ immersive: true });
    await key('Escape');
    expect(onImmersiveChange).toHaveBeenCalledWith(false);
    onImmersiveChange.mockClear();
    act(() => root.unmount());
    root = createRoot(container);
    await open({ immersive: false });
    await key('Escape');
    expect(onImmersiveChange).not.toHaveBeenCalled();
  });
});

describe('the comic reader: the controls', () => {
  it('are shown, and the button hides them', async () => {
    await open();
    expect(container.querySelector('[data-chrome]').dataset.chrome).toBe('shown');
    expect(container.querySelector('[data-chrome]').hasAttribute('inert')).toBe(false);
    await click(button('Esconder os controles'));
    expect(onImmersiveChange).toHaveBeenCalledWith(true);
  });

  it('hidden, they cannot be reached and leave a mark of the place', async () => {
    await open({ immersive: true });
    const chrome = container.querySelector('[data-chrome]');
    expect(chrome.dataset.chrome).toBe('hidden');
    expect(chrome.hasAttribute('inert')).toBe(true);
    const mark = container.querySelector('[data-chrome-mark]');
    expect(mark.textContent).toBe('5 / 10');
    expect(mark.getAttribute('aria-hidden')).toBe('false');
  });

  it('shown, the mark of the place is out of the way of a screen reader', async () => {
    await open();
    expect(container.querySelector('[data-chrome-mark]').getAttribute('aria-hidden')).toBe('true');
  });

  it('the list of modes opens, names each one, marks the one in use and closes when one is chosen', async () => {
    await open({ mode: 'rtl' });
    const opener = container.querySelector('button[aria-haspopup="menu"]');
    expect(opener.getAttribute('aria-expanded')).toBe('false');
    expect(container.querySelector('[role="menu"]')).toBeNull();
    await click(opener);
    expect(opener.getAttribute('aria-expanded')).toBe('true');
    const items = [...container.querySelectorAll('[role="menuitemradio"]')];
    expect(items.map((i) => i.textContent.replace('✓', '').trim())).toEqual([
      'Esquerda para a direita', 'Direita para a esquerda', 'Tira para rolar', 'Página dupla',
    ]);
    expect(items.map((i) => i.getAttribute('aria-checked'))).toEqual(['false', 'true', 'false', 'false']);
    await click(items[3]);
    expect(container.querySelector('[role="menu"]')).toBeNull();
    expect(shown()).toEqual(['Página 5', 'Página 6']);
  });

  it('the list of modes takes the focus on the mode in use, and Escape gives it back to its button', async () => {
    await open({ mode: 'rtl' });
    const opener = container.querySelector('button[aria-haspopup="menu"]');
    opener.focus();
    await click(opener);
    expect(document.activeElement.getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement.textContent).toContain('Direita para a esquerda');
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })); });
    expect(container.querySelector('[role="menu"]')).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  it('the list of modes does not hold the Tab: it is a menu beside the page, not a dialog over it', async () => {
    await open({ mode: 'rtl' });
    const opener = container.querySelector('button[aria-haspopup="menu"]');
    await click(opener);
    const items = [...container.querySelectorAll('[role="menuitemradio"]')];
    items.at(-1).focus();
    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    document.activeElement.dispatchEvent(tab);
    expect(tab.defaultPrevented).toBe(false);
  });

  it('the list of modes closes by Escape and by a press outside it, but not by a press inside it', async () => {
    await open();
    const opener = () => container.querySelector('button[aria-haspopup="menu"]');
    await click(opener());
    await act(async () => { container.querySelector('[role="menu"]').dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); });
    expect(container.querySelector('[role="menu"]')).not.toBeNull();
    await act(async () => { document.body.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); });
    expect(container.querySelector('[role="menu"]')).toBeNull();
    await click(opener());
    await key('Escape');
    expect(container.querySelector('[role="menu"]')).toBeNull();
    expect(onImmersiveChange).not.toHaveBeenCalled(); // that Escape was for the list
  });

  it('the button of the list of modes closes it, and a press on it does not close it before the click does', async () => {
    await open();
    const opener = container.querySelector('button[aria-haspopup="menu"]');
    await click(opener);
    expect(container.querySelector('[role="menu"]')).not.toBeNull();
    await act(async () => { opener.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true })); });
    expect(container.querySelector('[role="menu"]')).not.toBeNull();
    await click(opener);
    expect(container.querySelector('[role="menu"]')).toBeNull();
  });

  it('a strip has no turning of pages: it names its size and scrolls a screen at a time', async () => {
    await open({ mode: 'webtoon' });
    expect(container.textContent).toContain('10 páginas');
    expect(button('Próxima página, à direita')).toBeNull();
    expect(container.querySelectorAll('img').length).toBe(10);
    // The images near the place are asked for at once; the rest wait until they are close.
    expect([...container.querySelectorAll('img')].map((i) => i.getAttribute('loading')).join(',')).toBe('eager,eager,eager,eager,eager,eager,eager,lazy,lazy,lazy');
    const scroller = container.firstElementChild.parentElement;
    scroller.scrollBy = vi.fn();
    Object.defineProperty(scroller, 'clientHeight', { value: 1000, configurable: true });
    await click(button('Rolar para baixo'));
    expect(scroller.scrollBy).toHaveBeenLastCalledWith({ top: 900, behavior: 'smooth' });
    await click(button('Rolar para cima'));
    expect(scroller.scrollBy).toHaveBeenLastCalledWith({ top: -900, behavior: 'smooth' });
    expect(container.querySelector('[data-chrome-mark]')).toBeNull();
  });

  it('a strip of one page says "1 página"', async () => {
    await open({ mode: 'webtoon', count: 1, progress: '' });
    expect(container.querySelector('[data-chrome]').textContent).toContain('1 página');
    expect(container.querySelector('[data-chrome]').textContent).not.toContain('1 páginas');
  });

  it('says which mode the file asked for in the controls, with the name of the mode', async () => {
    await open({ mode: 'ltr', declaredMode: 'rtl' });
    const chip = [...container.querySelectorAll('span')].find((s) => s.textContent.includes('pelo arquivo'));
    expect(chip.textContent).toBe('Direita para a esquerda · pelo arquivo');
    expect(chip.title).toBe('O arquivo indica leitura da direita para a esquerda.');
  });

  it('a strip said by the file explains itself too', async () => {
    await open({ declaredMode: 'webtoon' });
    const chip = [...container.querySelectorAll('span')].find((s) => s.textContent.includes('pelo arquivo'));
    expect(chip.textContent).toBe('Tira para rolar · pelo arquivo');
    expect(chip.title).toBe('O arquivo indica que é uma tira para rolar.');
  });

  it('the note about the file is out of the way while the list of modes is open', async () => {
    await open({ declaredMode: 'rtl' });
    await click(container.querySelector('button[aria-haspopup="menu"]'));
    expect([...container.querySelectorAll('span')].some((s) => s.textContent.includes('pelo arquivo'))).toBe(false);
  });
});

describe('the comic reader: when it cannot show the comic', () => {
  it('shows a placeholder while the list of pages comes', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})));
    await act(async () => { root.render(<MangaViewer fileUrl="/files/7/x.cbz" workId={7} onProgress={vi.fn()} />); });
    expect(container.querySelector('[role="status"]').getAttribute('aria-label')).toBe('Carregando as páginas');
  });

  it('says it in words, with the reason, and asks again when told to', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 500 })
      .mockResolvedValueOnce({ ok: true, json: async () => pagesOf(3) });
    vi.stubGlobal('fetch', fetchMock);
    await act(async () => { root.render(<MangaViewer fileUrl="/files/7/x.cbz" workId={7} onProgress={vi.fn()} />); });
    await flush();
    const alert = container.querySelector('[role="alert"]');
    expect(alert.textContent).toContain('Não foi possível abrir este quadrinho.');
    expect(alert.textContent).toContain('O servidor respondeu com o erro 500.');
    await click([...alert.querySelectorAll('button')].find((b) => b.textContent === 'Tentar de novo'));
    await flush();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(container.querySelector('img').getAttribute('alt')).toBe('Página 1');
  });

  it('says so when it cannot tell which work the file is of', async () => {
    vi.stubGlobal('fetch', vi.fn());
    await act(async () => { root.render(<MangaViewer onProgress={vi.fn()} />); });
    await flush();
    expect(container.querySelector('[role="alert"]').textContent).toContain('Não foi possível saber de qual obra é este arquivo.');
  });

  it('says so when the file has no pages', async () => {
    await open({ count: 0, progress: '' });
    expect(container.querySelector('[role="alert"]').textContent).toBe('Este arquivo não tem páginas de imagem que possam ser lidas.');
  });

  it('a page that does not load says so and tries again when asked', async () => {
    await open({ progress: '0' });
    const img = container.querySelector('img');
    await act(async () => { img.dispatchEvent(new Event('error')); });
    const alert = container.querySelector('[role="alert"]');
    expect(alert.textContent).toContain('Não foi possível carregar esta página.');
    expect(container.querySelector('img')).toBeNull();
    const loads = [];
    const Original = globalThis.Image;
    globalThis.Image = class { set src(v) { loads.push(v); queueMicrotask(() => this.onload?.()); } };
    await click(container.querySelector('[role="alert"] + button'));
    await flush();
    globalThis.Image = Original;
    expect(loads).toEqual(['/works/7/pages/0']);
    expect(container.querySelector('img').getAttribute('alt')).toBe('Página 1');
  });
});
