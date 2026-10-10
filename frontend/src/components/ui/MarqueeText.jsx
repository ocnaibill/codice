import { useEffect, useRef, useState } from 'react';

/**
 * A text on one line that, when it does not fit, runs along inside its own place instead of making its card taller: the end of it is faded, and
 * it moves while the card is under the mouse or has the focus (see `.marquee` in library-shell.css). It does not move for who asked for less
 * motion. The whole text is in the page whatever is on view (and the one who uses it puts the title on the element that holds this).
 */
export function MarqueeText({ children, className = '', as: Tag = 'span' }) {
  const box = useRef(null);
  const text = useRef(null);
  const [distance, setDistance] = useState(0);

  useEffect(() => {
    const measure = () => {
      if (!box.current || !text.current) return;
      const over = text.current.scrollWidth - box.current.clientWidth;
      setDistance(over > 1 ? over : 0);   // the browser measures in whole pixels
    };
    measure();
    if (typeof ResizeObserver !== 'function' || !box.current) return undefined;
    const observer = new ResizeObserver(measure);
    observer.observe(box.current);
    return () => observer.disconnect();
  }, [children]);

  const long = distance > 0;
  return (
    <Tag
      ref={box}
      className={`marquee${long ? ' is-long' : ''}${className ? ` ${className}` : ''}`}
      style={long ? { '--marquee-distance': `-${distance}px`, '--marquee-seconds': `${Math.max(3, distance / 30)}s` } : undefined}
    >
      <span ref={text} className="marquee-text">{children}</span>
    </Tag>
  );
}
