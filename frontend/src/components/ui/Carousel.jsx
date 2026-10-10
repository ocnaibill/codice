import { Children, useCallback, useEffect, useRef, useState } from 'react';
import { LibraryIcon } from './LibraryIcon';

/**
 * A row of cards that goes on past the edge, with a button for the next ones (and the way back). The buttons are there only when there is
 * more to see in that direction; the row itself is reachable by keyboard and by the finger. Each child is one item.
 */
export function Carousel({ label, children, className = '' }) {
  const track = useRef(null);
  const [edge, setEdge] = useState({ start: true, end: true });

  const update = useCallback(() => {
    const el = track.current;
    if (!el) return;
    setEdge({ start: el.scrollLeft <= 1, end: el.scrollLeft + el.clientWidth >= el.scrollWidth - 1 });
  }, []);

  useEffect(() => {
    const el = track.current;
    if (!el) return undefined;
    update();
    el.addEventListener('scroll', update, { passive: true });
    const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(update) : null;
    observer?.observe(el);
    return () => {
      el.removeEventListener('scroll', update);
      observer?.disconnect();
    };
  }, [update, children]);

  const go = (direction) => {
    const el = track.current;
    if (!el) return;
    const reduced = typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    el.scrollBy({ left: direction * el.clientWidth * 0.9, behavior: reduced ? 'auto' : 'smooth' });
  };

  return (
    <div className={`library-carousel ${className}`}>
      {!edge.start && (
        <button type="button" className="library-carousel-button" data-side="start" aria-label="Anteriores" onClick={() => go(-1)}>
          <LibraryIcon name="arrow" />
        </button>
      )}
      <ul ref={track} className="library-carousel-track" aria-label={label} tabIndex={0}>
        {Children.map(children, (child) => (child == null ? null : <li className="library-carousel-item">{child}</li>))}
      </ul>
      {!edge.end && (
        <button type="button" className="library-carousel-button" data-side="end" aria-label="Próximos" onClick={() => go(1)}>
          <LibraryIcon name="arrow" />
        </button>
      )}
    </div>
  );
}
