import React, { act, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { useDialog } from './useDialog';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const roots = [];
afterEach(() => {
  while (roots.length) {
    const { root, container } = roots.pop();
    act(() => root.unmount());
    container.remove();
  }
});

async function render(element) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  roots.push({ root, container });
  await act(async () => root.render(element));
  return container;
}

const press = (key, options = {}) =>
  act(async () => {
    const target = document.activeElement ?? document.body;
    target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...options }));
  });

function Dialog({ label, onEscape, children, initialFocusId }) {
  const ref = useRef(null);
  const first = useRef(null);
  useDialog(ref, { onEscape, initialFocus: initialFocusId ? first : undefined });
  return (
    <div ref={ref} role="dialog" aria-modal="true" aria-label={label}>
      {children ?? (
        <>
          <button id={`${label}-a`}>A</button>
          <input id={`${label}-b`} ref={initialFocusId ? first : undefined} />
          <button id={`${label}-c`}>C</button>
        </>
      )}
    </div>
  );
}

function Page({ onEscape }) {
  const [open, setOpen] = useState(false);
  return (
    <div>
      <button id="opener" onClick={() => setOpen(true)}>Abrir</button>
      <button id="other">Outro</button>
      {open && <Dialog label="d" onEscape={() => { onEscape?.(); setOpen(false); }} />}
    </div>
  );
}

const $ = (id) => document.getElementById(id);

describe('useDialog: focus in', () => {
  it('goes to the first thing the keyboard can reach', async () => {
    await render(<Dialog label="d" />);
    expect(document.activeElement).toBe($('d-a'));
  });

  it('goes to the element the dialog asks for', async () => {
    await render(<Dialog label="d" initialFocusId />);
    expect(document.activeElement).toBe($('d-b'));
  });

  it('leaves the focus where an autoFocus inside already put it', async () => {
    await render(<Dialog label="d"><button>x</button><input id="auto" autoFocus /></Dialog>);
    expect(document.activeElement).toBe($('auto'));
  });

  it('goes to the dialog itself when there is nothing to reach in it', async () => {
    await render(<Dialog label="d"><p>só texto</p></Dialog>);
    expect(document.activeElement).toBe(document.querySelector('[role="dialog"]'));
  });

  it('skips what is disabled, hidden or inert', async () => {
    await render(
      <Dialog label="d">
        <button disabled>no</button>
        <button style={{ display: 'none' }}>hidden</button>
        <button style={{ visibility: 'hidden' }}>invisible</button>
        <div inert><button>inert</button></div>
        <button id="ok">ok</button>
      </Dialog>,
    );
    expect(document.activeElement).toBe($('ok'));
  });
});

describe('useDialog: Tab stays inside', () => {
  it('goes from the last to the first with Tab, and from the first to the last with Shift+Tab', async () => {
    await render(<Dialog label="d" />);
    $('d-c').focus();
    await press('Tab');
    expect(document.activeElement).toBe($('d-a'));
    await press('Tab', { shiftKey: true });
    expect(document.activeElement).toBe($('d-c'));
  });

  it('leaves the ones in the middle to the browser', async () => {
    await render(<Dialog label="d" />);
    $('d-b').focus();
    const event = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    document.activeElement.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
  });

  it('brings the focus back in when it was on the page behind (after a click on the backdrop)', async () => {
    await render(<div><button id="behind">atrás</button><Dialog label="d" /></div>);
    $('behind').focus();
    await press('Tab');
    expect(document.activeElement).toBe($('d-a'));
    $('behind').focus();
    await press('Tab', { shiftKey: true });
    expect(document.activeElement).toBe($('d-c'));
  });

  it('keeps the focus on the dialog when it has nothing to reach', async () => {
    await render(<Dialog label="d"><p>só texto</p></Dialog>);
    const event = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    document.activeElement.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true); // the browser would otherwise take the focus out of it
    expect(document.activeElement).toBe(document.querySelector('[role="dialog"]'));
  });

  it('is the one on top that keeps it: the one under does not pull the focus back', async () => {
    await render(<div><Dialog label="under" /><Dialog label="over" /></div>);
    expect(document.activeElement).toBe($('over-a'));
    $('over-c').focus();
    await press('Tab');
    expect(document.activeElement).toBe($('over-a'));
  });
});

describe('useDialog: Escape', () => {
  it('answers on the one on top only', async () => {
    const under = vi.fn();
    const over = vi.fn();
    await render(<div><Dialog label="under" onEscape={under} /><Dialog label="over" onEscape={over} /></div>);
    await press('Escape');
    expect(over).toHaveBeenCalledTimes(1);
    expect(under).not.toHaveBeenCalled();
  });

  it('is not taken when something already handled it', async () => {
    const onEscape = vi.fn();
    await render(<Dialog label="d" onEscape={onEscape} />);
    const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
    event.preventDefault();
    await act(async () => { document.activeElement.dispatchEvent(event); });
    expect(onEscape).not.toHaveBeenCalled();
  });

  it('does nothing when the dialog gives no answer', async () => {
    await render(<Dialog label="d" />);
    await press('Escape');
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();
  });
});

describe('useDialog: focus back', () => {
  it('returns to what opened it', async () => {
    await render(<Page />);
    $('opener').focus();
    await act(async () => $('opener').click());
    expect(document.activeElement).toBe($('d-a'));
    await press('Escape');
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe($('opener'));
  });

  it('returns to what opened it even when the dialog took the focus with an autoFocus', async () => {
    function AutoPage() {
      const [open, setOpen] = useState(false);
      const ref = useRef(null);
      return (
        <div>
          <button id="opener" onClick={() => setOpen(true)}>Abrir</button>
          {open && <Wrapper onClose={() => setOpen(false)} refProp={ref} />}
        </div>
      );
    }
    function Wrapper({ onClose }) {
      const ref = useRef(null);
      useDialog(ref, { onEscape: onClose });
      return <div ref={ref} role="dialog"><input id="auto" autoFocus /></div>;
    }
    await render(<AutoPage />);
    $('opener').focus();
    await act(async () => $('opener').click());
    expect(document.activeElement).toBe($('auto'));
    await press('Escape');
    expect(document.activeElement).toBe($('opener'));
  });

  it('does not take the focus from where the app put it on purpose', async () => {
    function Moves() {
      const [open, setOpen] = useState(false);
      return (
        <div>
          <button id="opener" onClick={() => setOpen(true)}>Abrir</button>
          <button id="elsewhere">Outro lugar</button>
          {open && <Dialog label="d" onEscape={() => { $('elsewhere').focus(); setOpen(false); }} />}
        </div>
      );
    }
    await render(<Moves />);
    $('opener').focus();
    await act(async () => $('opener').click());
    await press('Escape');
    expect(document.activeElement).toBe($('elsewhere'));
  });

  it('returns to the opener of each one when two are open and the top one closes', async () => {
    function Two() {
      const [a, setA] = useState(false);
      const [b, setB] = useState(false);
      return (
        <div>
          <button id="open-a" onClick={() => setA(true)}>A</button>
          {a && (
            <Dialog label="a" onEscape={() => setA(false)}>
              <button id="open-b" onClick={() => setB(true)}>B</button>
              {b && <Dialog label="b" onEscape={() => setB(false)} />}
            </Dialog>
          )}
        </div>
      );
    }
    await render(<Two />);
    $('open-a').focus();
    await act(async () => $('open-a').click());
    expect(document.activeElement).toBe($('open-b'));
    await act(async () => $('open-b').click());
    expect(document.activeElement).toBe($('b-a'));
    await press('Escape');
    expect(document.activeElement).toBe($('open-b'));
    await press('Escape');
    expect(document.activeElement).toBe($('open-a'));
  });
});

describe('useDialog: a component that is always mounted and opens the dialog by a flag', () => {
  function Sheet({ open, onEscape }) {
    const ref = useRef(null);
    useDialog(ref, { active: open, onEscape });
    if (!open) return null;
    return <div ref={ref} role="dialog" aria-modal="true"><button id="in-a">A</button><button id="in-b">B</button></div>;
  }
  function Host() {
    const [open, setOpen] = useState(false);
    return (
      <div>
        <button id="opener" onClick={() => setOpen(true)}>Abrir</button>
        <Sheet open={open} onEscape={() => setOpen(false)} />
      </div>
    );
  }

  it('takes the focus in when it opens, traps Tab, and gives it back to the opener when it closes', async () => {
    await render(<Host />);
    $('opener').focus();
    await act(async () => $('opener').click());
    expect(document.activeElement).toBe($('in-a'));
    $('in-b').focus();
    await press('Tab');
    expect(document.activeElement).toBe($('in-a'));
    await press('Escape');
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe($('opener'));
  });

  it('does nothing while it is closed, and again each time it opens', async () => {
    await render(<Host />);
    $('opener').focus();
    await press('Tab');
    await press('Escape');
    expect(document.activeElement).not.toBe(null);
    for (let i = 0; i < 2; i += 1) {
      $('opener').focus();
      await act(async () => $('opener').click());
      expect(document.activeElement).toBe($('in-a'));
      await press('Escape');
      expect(document.activeElement).toBe($('opener'));
    }
  });
});

describe('useDialog: a dialog that is not modal (trap: false)', () => {
  function Card({ onEscape }) {
    const ref = useRef(null);
    useDialog(ref, { onEscape, trap: false });
    return <aside ref={ref} role="dialog"><button id="card-a">A</button></aside>;
  }

  it('takes the focus in and answers Escape, but lets Tab leave', async () => {
    const onEscape = vi.fn();
    await render(<div><button id="page">página</button><Card onEscape={onEscape} /></div>);
    expect(document.activeElement).toBe($('card-a'));
    const event = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    document.activeElement.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    $('page').focus();
    const back = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    document.activeElement.dispatchEvent(back);
    expect(back.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe($('page'));
    await press('Escape');
    expect(onEscape).toHaveBeenCalledTimes(1);
  });
});

describe('useDialog: the edges of Tab', () => {
  it('from the dialog itself (where the focus starts when it asks for it), Shift+Tab goes to the last', async () => {
    function OnPanel() {
      const ref = useRef(null);
      useDialog(ref, { initialFocus: ref });
      return <div ref={ref} role="dialog"><button id="p-a">A</button><button id="p-b">B</button></div>;
    }
    await render(<OnPanel />);
    expect(document.activeElement).toBe(document.querySelector('[role="dialog"]'));
    await press('Tab', { shiftKey: true });
    expect(document.activeElement).toBe($('p-b'));
  });

  it('leaves Shift+Tab on the last to the browser, and Tab on the first', async () => {
    await render(<Dialog label="d" />);
    $('d-c').focus();
    const back = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true });
    document.activeElement.dispatchEvent(back);
    expect(back.defaultPrevented).toBe(false);
    $('d-a').focus();
    const forward = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    document.activeElement.dispatchEvent(forward);
    expect(forward.defaultPrevented).toBe(false);
  });
});

describe('useDialog: the opener is the one of the moment it opened', () => {
  it('is not lost when the dialog renders again with the focus already inside it (typing)', async () => {
    function Typing() {
      const [open, setOpen] = useState(false);
      return (
        <div>
          <button id="opener" onClick={() => setOpen(true)}>Abrir</button>
          {open && <Form onClose={() => setOpen(false)} />}
        </div>
      );
    }
    function Form({ onClose }) {
      const ref = useRef(null);
      const [text, setText] = useState('');
      useDialog(ref, { onEscape: onClose });
      return <div ref={ref} role="dialog"><input id="typed" autoFocus value={text} onChange={(e) => setText(e.target.value)} /></div>;
    }
    await render(<Typing />);
    $('opener').focus();
    await act(async () => $('opener').click());
    expect(document.activeElement).toBe($('typed'));
    // a render with the focus inside the dialog
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
      setter.call($('typed'), 'abc');
      $('typed').dispatchEvent(new Event('input', { bubbles: true }));
    });
    await press('Escape');
    expect(document.activeElement).toBe($('opener'));
  });

  it('takes the focus back from inside a dialog that stays in the page when it is turned off', async () => {
    function Stays() {
      const [open, setOpen] = useState(false);
      const ref = useRef(null);
      useDialog(ref, { active: open });
      return (
        <div>
          <button id="opener" onClick={() => setOpen(true)}>Abrir</button>
          <div ref={ref} role="dialog"><button id="inside" onClick={() => setOpen(false)}>Fechar</button></div>
        </div>
      );
    }
    await render(<Stays />);
    $('opener').focus();
    await act(async () => $('opener').click());
    expect(document.activeElement).toBe($('inside'));
    await act(async () => $('inside').click());
    expect(document.activeElement).toBe($('opener'));
  });
});
