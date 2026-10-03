import React, { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { menuPlacement } from '../selection';

const item =
  'flex min-h-11 items-center gap-1.5 rounded-full px-3.5 text-sm font-medium text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95 disabled:opacity-40 disabled:hover:bg-transparent';

/**
 * The menu that floats by a selected passage (#108): copy it, highlight it, or write a note on it. With a mouse it is
 * above the selection, with a finger below it (the system draws its own menu above). It closes with Esc, and the
 * buttons do not take the selection from the text when they are pressed.
 */
export function SelectionMenu({ selection, onCopy, onHighlight, onNote, onDictionary, onClose, busy = false }) {
  const ref = useRef(null);
  const [place, setPlace] = useState(null);

  // The place needs the size of the menu, which is known once it is drawn: it is not shown before that.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setPlace(menuPlacement({
      rect: selection.rect,
      viewport: { width: window.innerWidth, height: window.innerHeight },
      size: { width: el.offsetWidth, height: el.offsetHeight },
      touch: selection.touch,
    }));
  }, [selection]);

  useEffect(() => {
    const onKey = (event) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <div
      ref={ref}
      role="toolbar"
      aria-label="O que fazer com o trecho selecionado"
      onMouseDown={(event) => event.preventDefault()}
      style={{ position: 'fixed', left: place?.left ?? 0, top: place?.top ?? 0, visibility: place ? 'visible' : 'hidden' }}
      className="z-[70] flex items-center gap-0.5 rounded-full border border-border-hairline bg-white px-1.5 py-1 shadow-xl animate-pop-in"
    >
      <button onClick={onCopy} className={item}>Copiar</button>
      <button onClick={onHighlight} disabled={busy} className={item}>Destacar</button>
      <button onClick={onNote} className={item}>Nota</button>
      {onDictionary && <button onClick={onDictionary} className={item}>Dicionário</button>}
    </div>
  );
}
