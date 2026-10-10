import { act } from 'react';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { mount } from '../../features/admin/testUtils';
import { Carousel } from './Carousel';

let view;
afterEach(() => {
  view?.unmount();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

/** jsdom has no layout: the size of the row and how far it was scrolled are said by hand. */
function layout(state) {
  vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockImplementation(() => state.scrollWidth);
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(() => state.clientWidth);
  vi.spyOn(HTMLElement.prototype, 'scrollLeft', 'get').mockImplementation(() => state.scrollLeft);
}
const buttons = () => [...document.body.querySelectorAll('.library-carousel-button')].map((b) => b.getAttribute('aria-label'));
const items = (n) => Array.from({ length: n }, (_, i) => <span key={i}>Obra {i}</span>);

describe('Carousel', () => {
  it('is a list of the items, with a name, that can be reached by the keyboard', async () => {
    layout({ scrollWidth: 300, clientWidth: 300, scrollLeft: 0 });
    view = await mount(<Carousel label="Adicionados">{items(3)}</Carousel>);
    const list = document.body.querySelector('ul[aria-label="Adicionados"]');
    expect(list.tabIndex).toBe(0);
    expect([...list.querySelectorAll('li')].map((li) => li.textContent)).toEqual(['Obra 0', 'Obra 1', 'Obra 2']);
  });

  it('has no buttons when everything is on view', async () => {
    layout({ scrollWidth: 300, clientWidth: 300, scrollLeft: 0 });
    view = await mount(<Carousel label="x">{items(2)}</Carousel>);
    expect(buttons()).toEqual([]);
  });

  it('has the button for the next ones at the start, and the one for the previous at the end, and both in the middle', async () => {
    const state = { scrollWidth: 1000, clientWidth: 300, scrollLeft: 0 };
    layout(state);
    view = await mount(<Carousel label="x">{items(8)}</Carousel>);
    expect(buttons()).toEqual(['Próximos']);
    const list = document.body.querySelector('ul');
    state.scrollLeft = 300;
    await act(async () => list.dispatchEvent(new Event('scroll')));
    expect(buttons()).toEqual(['Anteriores', 'Próximos']);
    state.scrollLeft = 700;
    await act(async () => list.dispatchEvent(new Event('scroll')));
    expect(buttons()).toEqual(['Anteriores']);
  });

  it('counts a pixel of difference as being at the edge, on each side', async () => {
    const state = { scrollWidth: 1000, clientWidth: 300, scrollLeft: 1 };
    layout(state);
    view = await mount(<Carousel label="x">{items(8)}</Carousel>);
    expect(buttons()).toEqual(['Próximos']); // one pixel from the start is the start
    const list = document.body.querySelector('ul');
    state.scrollLeft = 699; // 699 + 300 = 999: one from the end
    await act(async () => list.dispatchEvent(new Event('scroll')));
    expect(buttons()).toEqual(['Anteriores']);
    state.scrollLeft = 2;
    await act(async () => list.dispatchEvent(new Event('scroll')));
    expect(buttons()).toEqual(['Anteriores', 'Próximos']);
  });

  it('goes by most of a screen, smoothly, in the direction of the button', async () => {
    const state = { scrollWidth: 1000, clientWidth: 300, scrollLeft: 0 };
    layout(state);
    const scrollBy = vi.fn();
    Element.prototype.scrollBy = scrollBy;
    view = await mount(<Carousel label="x">{items(8)}</Carousel>);
    await view.click(document.body.querySelector('[aria-label="Próximos"]'));
    expect(scrollBy).toHaveBeenLastCalledWith({ left: 270, behavior: 'smooth' });
    state.scrollLeft = 300;
    await act(async () => document.body.querySelector('ul').dispatchEvent(new Event('scroll')));
    await view.click(document.body.querySelector('[aria-label="Anteriores"]'));
    expect(scrollBy).toHaveBeenLastCalledWith({ left: -270, behavior: 'smooth' });
    delete Element.prototype.scrollBy;
  });

  it('does not animate for who asked for less motion', async () => {
    layout({ scrollWidth: 1000, clientWidth: 300, scrollLeft: 0 });
    vi.stubGlobal('matchMedia', (query) => ({ matches: query.includes('reduce'), addEventListener() {}, removeEventListener() {} }));
    const scrollBy = vi.fn();
    Element.prototype.scrollBy = scrollBy;
    view = await mount(<Carousel label="x">{items(8)}</Carousel>);
    await view.click(document.body.querySelector('[aria-label="Próximos"]'));
    expect(scrollBy).toHaveBeenLastCalledWith({ left: 270, behavior: 'auto' });
    delete Element.prototype.scrollBy;
  });

  it('asks again how much is on view when its box changes size, and stops asking when it is gone', async () => {
    const state = { scrollWidth: 300, clientWidth: 300, scrollLeft: 0 };
    layout(state);
    let notify;
    let disconnected = false;
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback) { notify = callback; }
      observe() {}
      disconnect() { disconnected = true; }
    });
    view = await mount(<Carousel label="x">{items(8)}</Carousel>);
    expect(buttons()).toEqual([]);
    state.scrollWidth = 1000;
    await act(async () => notify());
    expect(buttons()).toEqual(['Próximos']);
    view.unmount();
    expect(disconnected).toBe(true);
    view = await mount(<div />);
  });

  it('leaves out what is not an item', async () => {
    layout({ scrollWidth: 300, clientWidth: 300, scrollLeft: 0 });
    view = await mount(<Carousel label="x">{[null, <span key="a">Só esta</span>, undefined]}</Carousel>);
    expect(document.body.querySelectorAll('li')).toHaveLength(1);
  });
});
