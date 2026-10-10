import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { WorkPreview } from './WorkPreview';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const seg = (sequence, extra = {}) => ({ sequence, text: `Primeiro parágrafo ${sequence}.\n\nSegundo parágrafo ${sequence}.`, chapter: '', part: 'body', locator: { type: 'epub', href: `c${sequence}.xhtml` }, ...extra });
const data = (extra = {}) => ({
  fileId: 10, total: 480, from: 3, prevFrom: 0, nextFrom: 6, current: 4,
  segments: [seg(3, { chapter: 'Capítulo 1' }), seg(4, { chapter: 'Capítulo 1' }), seg(5, { chapter: 'Capítulo 2' })],
  ...extra,
});

let container;
let root;
const render = async (props) => {
  await act(async () => {
    root.render(<WorkPreview title="Duna" data={data()} onPrev={vi.fn()} onNext={vi.fn()} onOpen={vi.fn()} {...props} />);
  });
};
const button = (text) => [...container.querySelectorAll('button')].find((b) => (b.getAttribute('aria-label') || b.textContent).trim() === text);
const article = () => container.querySelector('article');

beforeEach(() => {
  localStorage.clear();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('WorkPreview', () => {
  it('shows the passages as paragraphs, the title of the work and the name of a chapter where it starts', async () => {
    await render();
    expect(container.textContent).toContain('Duna');
    expect([...article().querySelectorAll('p')].map((p) => p.textContent).filter((t) => t.startsWith('Segundo'))).toHaveLength(3);
    expect([...article().querySelectorAll('h3')].map((h) => h.textContent)).toEqual(['Capítulo 1', 'Capítulo 2']); // not above every passage
  });

  it('says which passages of how many, and which one the person is in', async () => {
    await render();
    expect(container.textContent).toContain('Trecho 4–6 de 480');
    expect(container.textContent).toContain('onde você está');
    expect(article().querySelector('[data-here="true"]').textContent).toContain('Primeiro parágrafo 4.');
    await render({ data: data({ current: 200 }) });
    expect(container.textContent).not.toContain('onde você está');
    expect(article().querySelector('[data-here]')).toBeNull();
  });

  it('goes to the windows before and after, and has no way past the ends', async () => {
    const onPrev = vi.fn();
    const onNext = vi.fn();
    await render({ onPrev, onNext });
    await act(async () => { button('Anterior').click(); });
    expect(onPrev).toHaveBeenCalledWith(0);
    await act(async () => { button('Próxima').click(); });
    expect(onNext).toHaveBeenCalledWith(6);
    await render({ data: data({ prevFrom: null, nextFrom: null }) });
    expect(button('Anterior').disabled).toBe(true);
    expect(button('Próxima').disabled).toBe(true);
  });

  it('does not move while a window is on its way', async () => {
    await render({ busy: true });
    expect(button('Anterior').disabled).toBe(true);
    expect(button('Próxima').disabled).toBe(true);
    expect(article().getAttribute('aria-busy')).toBe('true');
  });

  it('opens the reader at the passage the person is in, or at the first when they are not in the window', async () => {
    const onOpen = vi.fn();
    await render({ onOpen });
    await act(async () => { button('Abrir no leitor').click(); });
    expect(onOpen.mock.calls[0][0].sequence).toBe(4);
    await render({ onOpen, data: data({ current: null }) });
    await act(async () => { button('Abrir no leitor').click(); });
    expect(onOpen.mock.calls[1][0].sequence).toBe(3);
  });

  it('changes the size of the letter in steps and stops at the ends', async () => {
    await render();
    expect(article().style.fontSize).toBe('18px');
    await act(async () => { button('Aumentar a letra').click(); });
    expect(article().style.fontSize).toBe('20px');
    await act(async () => { button('Diminuir a letra').click(); });
    await act(async () => { button('Diminuir a letra').click(); });
    expect(article().style.fontSize).toBe('16px');
    for (let i = 0; i < 5; i += 1) await act(async () => { button('Diminuir a letra').click(); });
    expect(article().style.fontSize).toBe('14px');
    expect(button('Diminuir a letra').disabled).toBe(true);
    for (let i = 0; i < 5; i += 1) await act(async () => { button('Aumentar a letra').click(); });
    expect(article().style.fontSize).toBe('26px');
    expect(button('Aumentar a letra').disabled).toBe(true);
  });

  it('changes the type and the background, and keeps the choice for the next visit', async () => {
    await render();
    expect(article().className).toContain('font-display');
    await act(async () => { button('Monoespaçada').click(); });
    expect(article().className).toContain('font-mono');
    expect(button('Monoespaçada').getAttribute('aria-pressed')).toBe('true');
    await act(async () => { button('Fundo escuro').click(); });
    expect(container.querySelector('section').className).toContain('bg-ink');
    expect(button('Fundo claro')).toBeTruthy();
    expect(JSON.parse(localStorage.getItem('codice:work-preview'))).toMatchObject({ mono: true, dark: true });
    act(() => root.unmount());
    root = createRoot(container);
    await render();
    expect(article().className).toContain('font-mono');
    expect(container.querySelector('section').className).toContain('bg-ink');
  });

  it('is the same when the device keeps nothing', async () => {
    const get = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
    const set = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
    await render();
    await act(async () => { button('Aumentar a letra').click(); });
    expect(article().style.fontSize).toBe('20px');
    get.mockRestore();
    set.mockRestore();
  });

  it('ignores what was kept that is not a size', async () => {
    localStorage.setItem('codice:work-preview', JSON.stringify({ size: 99, mono: 'yes', dark: 1 }));
    await render();
    expect(article().style.fontSize).toBe('18px');
    expect(article().className).toContain('font-display');
  });

  it('asks the browser for the full screen, and says how to leave it', async () => {
    const request = vi.fn();
    await render();
    container.querySelector('section').requestFullscreen = request;
    await act(async () => { button('Tela cheia').click(); });
    expect(request).toHaveBeenCalledTimes(1);
  });

  it('is not taller than a few lines of the text, which scrolls inside, and is not held back in the full screen', async () => {
    await render();
    expect(article().className).toContain('max-h-72');
    expect(article().className).toContain('overflow-y-auto');
    const section = container.querySelector('section');
    section.requestFullscreen = () => { Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => section }); document.dispatchEvent(new Event('fullscreenchange')); };
    await act(async () => { button('Tela cheia').click(); });
    expect(article().className).toContain('max-h-none');
    Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => null });
  });

  it('says nothing when the file has no text in the index', async () => {
    await render({ data: { fileId: 10, total: 0, segments: [] } });
    expect(container.querySelector('section')).toBeNull();
    await render({ data: undefined });
    expect(container.querySelector('section')).toBeNull();
  });

  it('says one passage in the singular position', async () => {
    await render({ data: data({ segments: [seg(7)], current: null, from: 7 }) });
    expect(container.textContent).toContain('Trecho 8 de 480');
  });
});
