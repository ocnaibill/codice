import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { SelectionMenu } from './SelectionMenu';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let handlers;
const selection = (extra = {}) => ({ text: 'um trecho', rect: { left: 100, top: 300, width: 100, height: 20, bottom: 320, right: 200 }, touch: false, ...extra });
const menu = () => container.querySelector('[role="toolbar"]');
const button = (label) => [...container.querySelectorAll('button')].find((b) => b.textContent === label);
async function show(sel = selection(), props = {}) {
  await act(async () => { root.render(<SelectionMenu selection={sel} {...handlers} {...props} />); });
}

beforeEach(() => {
  handlers = { onCopy: vi.fn(), onHighlight: vi.fn(), onNote: vi.fn(), onClose: vi.fn() };
  // jsdom lays nothing out: the menu is given the size a browser would measure.
  Object.defineProperty(HTMLElement.prototype, 'offsetWidth', { configurable: true, get() { return this.getAttribute('role') === 'toolbar' ? 200 : 0; } });
  Object.defineProperty(HTMLElement.prototype, 'offsetHeight', { configurable: true, get() { return this.getAttribute('role') === 'toolbar' ? 48 : 0; } });
  vi.spyOn(window, 'innerWidth', 'get').mockReturnValue(400);
  vi.spyOn(window, 'innerHeight', 'get').mockReturnValue(800);
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  delete HTMLElement.prototype.offsetWidth;
  delete HTMLElement.prototype.offsetHeight;
  vi.restoreAllMocks();
});

describe('SelectionMenu', () => {
  it('offers to copy, highlight and write a note, and says what it is for', async () => {
    await show();
    expect(menu().getAttribute('aria-label')).toBe('O que fazer com o trecho selecionado');
    expect([...menu().querySelectorAll('button')].map((b) => b.textContent)).toEqual(['Copiar', 'Destacar', 'Nota']);
  });

  it('offers the dictionary only when it is told what to do with it, and does it', async () => {
    await show();
    expect([...menu().querySelectorAll('button')].map((b) => b.textContent)).not.toContain('Dicionário');
    const onDictionary = vi.fn();
    await show(selection(), { onDictionary });
    expect([...menu().querySelectorAll('button')].map((b) => b.textContent)).toEqual(['Copiar', 'Destacar', 'Nota', 'Dicionário']);
    await act(async () => { button('Dicionário').click(); });
    expect(onDictionary).toHaveBeenCalledTimes(1);
  });

  it('does what each button says', async () => {
    await show();
    await act(async () => { button('Copiar').click(); });
    expect(handlers.onCopy).toHaveBeenCalledTimes(1);
    await act(async () => { button('Destacar').click(); });
    expect(handlers.onHighlight).toHaveBeenCalledTimes(1);
    await act(async () => { button('Nota').click(); });
    expect(handlers.onNote).toHaveBeenCalledTimes(1);
  });

  it('does not highlight twice while a highlight is being saved', async () => {
    await show(selection(), { busy: true });
    expect(button('Destacar').disabled).toBe(true);
    expect(button('Copiar').disabled).toBe(false);
    expect(button('Nota').disabled).toBe(false);
  });

  it('is above the selection with a mouse, and drawn only once its place is known', async () => {
    await show();
    expect(menu().style.visibility).toBe('visible');
    expect(menu().style.position).toBe('fixed');
    expect(menu().style.left).toBe('50px');
    expect(menu().style.top).toBe('244px'); // 300 - 48 - 8
  });

  it('is below the selection with a finger', async () => {
    await show(selection({ touch: true }));
    expect(menu().style.top).toBe('348px'); // 320 + 28
  });

  it('moves with the selection', async () => {
    await show();
    await show(selection({ rect: { left: 100, top: 500, width: 100, height: 20, bottom: 520, right: 200 } }));
    expect(menu().style.top).toBe('444px');
  });

  it('closes with Escape, and not with another key', async () => {
    await show();
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' })); });
    expect(handlers.onClose).not.toHaveBeenCalled();
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(handlers.onClose).toHaveBeenCalledTimes(1);
  });

  it('does not listen to the keyboard once it is gone', async () => {
    await show();
    await act(async () => { root.render(<div />); });
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(handlers.onClose).not.toHaveBeenCalled();
  });

  it('keeps the selection in the text when a button is pressed (the press is not passed on)', async () => {
    await show();
    const event = new MouseEvent('mousedown', { bubbles: true, cancelable: true });
    await act(async () => { button('Copiar').dispatchEvent(event); });
    expect(event.defaultPrevented).toBe(true);
  });
});
