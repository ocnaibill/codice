import React, { act, useRef } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { useSelectionWatcher, MOUSE_DELAY, TOUCH_DELAY } from './useSelectionWatcher';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let onSelection;

function Page({ enabled = true }) {
  const ref = useRef(null);
  useSelectionWatcher(ref, onSelection, enabled);
  return (
    <div>
      <article id="inside" ref={ref}><p id="p">Era uma vez um texto que se lê devagar.</p></article>
      <p id="outside">Fora do texto.</p>
    </div>
  );
}
const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });
const select = async (id, from, to) => {
  const node = container.querySelector(`#${id}`).firstChild;
  const range = document.createRange();
  range.setStart(node, from);
  range.setEnd(node, to);
  const s = window.getSelection();
  s.removeAllRanges();
  s.addRange(range);
  await act(async () => { document.dispatchEvent(new Event('selectionchange')); });
};
const press = (pointerType) => act(async () => {
  const e = new MouseEvent('pointerdown', { bubbles: true });
  Object.defineProperty(e, 'pointerType', { value: pointerType });
  container.querySelector('#p').dispatchEvent(e);
});

beforeEach(() => {
  Range.prototype.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 16, bottom: 36, right: 110 });
  onSelection = vi.fn();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  window.getSelection().removeAllRanges();
  delete Range.prototype.getBoundingClientRect;
});

describe('useSelectionWatcher', () => {
  it('tells what was selected once it has rested, with where it is, and how to clear it', async () => {
    await act(async () => { root.render(<Page />); });
    await press('mouse');
    await select('p', 0, 11);
    expect(onSelection).not.toHaveBeenCalled(); // not before it rests
    await wait(MOUSE_DELAY + 60);
    expect(onSelection).toHaveBeenCalledTimes(1);
    const found = onSelection.mock.calls[0][0];
    expect(found.text).toBe('Era uma vez');
    expect(found.rect.top).toBe(20);
    expect(found.touch).toBe(false);
    found.clear();
    expect(window.getSelection().isCollapsed).toBe(true);
  });

  it('waits longer for a finger, which drags handles, and says it was a touch', async () => {
    await act(async () => { root.render(<Page />); });
    await press('touch');
    await select('p', 0, 11);
    await wait(MOUSE_DELAY + 60);
    expect(onSelection).not.toHaveBeenCalled();
    await wait(TOUCH_DELAY);
    expect(onSelection).toHaveBeenCalledTimes(1);
    expect(onSelection.mock.calls[0][0].touch).toBe(true);
  });

  it('says a pen is a touch too', async () => {
    await act(async () => { root.render(<Page />); });
    await press('pen');
    await select('p', 0, 11);
    await wait(TOUCH_DELAY + 60);
    expect(onSelection.mock.calls[0][0].touch).toBe(true);
  });

  it('says nothing again while the selection is changing, and the new one when it rests', async () => {
    await act(async () => { root.render(<Page />); });
    await press('mouse');
    await select('p', 0, 11);
    await wait(MOUSE_DELAY + 60);
    await select('p', 0, 20);
    expect(onSelection).toHaveBeenLastCalledWith(null);
    await wait(MOUSE_DELAY + 60);
    expect(onSelection.mock.calls.at(-1)[0].text).toBe('Era uma vez um texto');
  });

  it('says nothing when the selection goes away, and only once', async () => {
    await act(async () => { root.render(<Page />); });
    await press('mouse');
    await select('p', 0, 11);
    await wait(MOUSE_DELAY + 60);
    window.getSelection().removeAllRanges();
    await act(async () => { document.dispatchEvent(new Event('selectionchange')); });
    await wait(MOUSE_DELAY + 60);
    expect(onSelection).toHaveBeenLastCalledWith(null);
    const calls = onSelection.mock.calls.length;
    await act(async () => { document.dispatchEvent(new Event('selectionchange')); });
    await wait(MOUSE_DELAY + 60);
    expect(onSelection.mock.calls.length).toBe(calls);
  });

  it('does not tell about what is selected outside its page', async () => {
    await act(async () => { root.render(<Page />); });
    await press('mouse');
    await select('outside', 0, 4);
    await wait(MOUSE_DELAY + 60);
    expect(onSelection).not.toHaveBeenCalled();
  });

  it('says nothing when the page is scrolled, while a selection is shown', async () => {
    await act(async () => { root.render(<Page />); });
    await press('mouse');
    await act(async () => { container.querySelector('#inside').dispatchEvent(new Event('scroll')); });
    expect(onSelection).not.toHaveBeenCalled(); // nothing was shown: nothing to hide
    await select('p', 0, 11);
    await wait(MOUSE_DELAY + 60);
    await act(async () => { container.querySelector('#inside').dispatchEvent(new Event('scroll')); });
    expect(onSelection).toHaveBeenLastCalledWith(null);
  });

  it('listens to nothing while it is off', async () => {
    await act(async () => { root.render(<Page enabled={false} />); });
    await select('p', 0, 11);
    await wait(MOUSE_DELAY + 60);
    expect(onSelection).not.toHaveBeenCalled();
  });

  it('takes the menu away when the page goes away', async () => {
    await act(async () => { root.render(<Page />); });
    await press('mouse');
    await select('p', 0, 11);
    await wait(MOUSE_DELAY + 60);
    act(() => root.unmount());
    expect(onSelection).toHaveBeenLastCalledWith(null);
    root = createRoot(container);
    await act(async () => { root.render(<div />); });
  });

  it('does not tell anything after it was unmounted, not even about what is still selected, or what was about to be told', async () => {
    await act(async () => { root.render(<Page />); });
    await press('mouse');
    await select('p', 0, 11);
    act(() => root.unmount()); // before the selection has rested
    onSelection.mockClear();
    await wait(MOUSE_DELAY + 60);
    expect(onSelection).not.toHaveBeenCalled();
    await act(async () => { document.dispatchEvent(new Event('selectionchange')); });
    await wait(MOUSE_DELAY + 60);
    expect(onSelection).not.toHaveBeenCalled();
    root = createRoot(container);
  });

  it('gives back every listener it took, and takes none it does not need', async () => {
    const added = vi.spyOn(document, 'addEventListener');
    const removed = vi.spyOn(document, 'removeEventListener');
    await act(async () => { root.render(<Page />); });
    const mine = (calls) => calls.filter(([type]) => ['selectionchange', 'pointerdown', 'scroll'].includes(type)).map(([type, fn, capture]) => `${type}:${fn.name}:${capture === true}`).sort();
    act(() => root.unmount());
    expect(mine(removed.mock.calls)).toEqual(mine(added.mock.calls));
    expect(mine(added.mock.calls).length).toBe(3);
    root = createRoot(container);
  });

  it('does not tell about a selection while its page is not there (not drawn yet)', async () => {
    function Missing() {
      const ref = useRef(null);
      useSelectionWatcher(ref, onSelection, true);
      return <p id="loose">texto solto</p>;
    }
    await act(async () => { root.render(<Missing />); });
    await select('loose', 0, 5);
    await wait(MOUSE_DELAY + 60);
    expect(onSelection).not.toHaveBeenCalled();
  });
});
