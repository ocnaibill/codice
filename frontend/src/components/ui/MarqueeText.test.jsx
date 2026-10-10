import { describe, it, expect, afterEach, vi } from 'vitest';
import { mount } from '../../features/admin/testUtils';
import { MarqueeText } from './MarqueeText';

let view;
afterEach(() => {
  view?.unmount();
  vi.unstubAllGlobals();
});

/** jsdom has no layout: what the text and its box measure is said by hand. */
function measuring({ text, box }) {
  const spy = (name, value) => vi.spyOn(HTMLElement.prototype, name, 'get').mockImplementation(function get() {
    return this.classList.contains('marquee-text') ? value.text : value.box;
  });
  spy('scrollWidth', { text, box });
  spy('clientWidth', { text, box });
}

describe('MarqueeText', () => {
  afterEach(() => vi.restoreAllMocks());

  it('is the text, on one line, and nothing more while it fits', async () => {
    measuring({ text: 100, box: 160 });
    view = await mount(<MarqueeText>Neuromancer</MarqueeText>);
    const el = document.body.querySelector('.marquee');
    expect(el.textContent).toBe('Neuromancer');
    expect(el.classList.contains('is-long')).toBe(false);
    expect(el.style.getPropertyValue('--marquee-distance')).toBe('');
  });

  it('says how far to run when it does not fit, and runs no further than the end of the text', async () => {
    measuring({ text: 400, box: 160 });
    view = await mount(<MarqueeText>O Guia do Mochileiro das Galáxias, edição comemorativa de 42 anos</MarqueeText>);
    const el = document.body.querySelector('.marquee');
    expect(el.classList.contains('is-long')).toBe(true);
    expect(el.style.getPropertyValue('--marquee-distance')).toBe('-240px');
    expect(el.style.getPropertyValue('--marquee-seconds')).toBe('8s');
    expect(el.textContent).toBe('O Guia do Mochileiro das Galáxias, edição comemorativa de 42 anos'); // all of it is in the page
  });

  it('is not long for a pixel or less of overflow, and is never faster than three seconds', async () => {
    measuring({ text: 161, box: 160 });
    view = await mount(<MarqueeText>Quase</MarqueeText>);
    expect(document.body.querySelector('.marquee').classList.contains('is-long')).toBe(false);
    view.unmount();
    measuring({ text: 200, box: 160 });
    view = await mount(<MarqueeText>Um pouco maior</MarqueeText>);
    expect(document.body.querySelector('.marquee').style.getPropertyValue('--marquee-seconds')).toBe('3s');
  });

  it('measures again when its box changes size, where the browser can tell', async () => {
    let notify;
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback) { notify = callback; }
      observe() {}
      disconnect() { this.gone = true; }
    });
    const widths = { text: 400, box: 160 };
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockImplementation(function get() { return this.classList.contains('marquee-text') ? widths.text : widths.box; });
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(function get() { return widths.box; });
    view = await mount(<MarqueeText>Um título muito comprido para uma coluna estreita</MarqueeText>);
    expect(document.body.querySelector('.marquee').classList.contains('is-long')).toBe(true);
    widths.box = 500;
    const { act } = await import('react');
    await act(async () => notify());
    expect(document.body.querySelector('.marquee').classList.contains('is-long')).toBe(false);
  });

  it('measures again when the text changes', async () => {
    const { useState } = await import('react');
    const widths = { text: 100, box: 160 };
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockImplementation(function get() { return this.classList.contains('marquee-text') ? widths.text : widths.box; });
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(() => widths.box);
    function Changing() {
      const [title, setTitle] = useState('Curto');
      return (
        <>
          <MarqueeText>{title}</MarqueeText>
          <button onClick={() => { widths.text = 400; setTitle('Um título que agora é bem mais comprido'); }}>muda</button>
        </>
      );
    }
    view = await mount(<Changing />);
    expect(document.body.querySelector('.marquee').classList.contains('is-long')).toBe(false);
    await view.click(view.button('muda'));
    expect(document.body.querySelector('.marquee').classList.contains('is-long')).toBe(true);
  });

  it('takes the element it is asked to be and the class it is given', async () => {
    measuring({ text: 10, box: 100 });
    view = await mount(<MarqueeText as="strong" className="x">Oi</MarqueeText>);
    const el = document.body.querySelector('strong.marquee');
    expect(el.className).toBe('marquee x');
  });
});
