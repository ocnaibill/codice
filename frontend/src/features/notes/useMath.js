import { useEffect, useState } from 'react';

let loaded = null;

/** The plugins that draw formulas, once they are loaded, when `wanted` (the note has a formula); null before that. */
export function useMath(wanted) {
  const [math, setMath] = useState(loaded);
  useEffect(() => {
    if (!wanted || math) return;
    let current = true;
    import('./mathPlugins').then((m) => {
      loaded = m;
      if (current) setMath(m);
    });
    return () => {
      current = false;
    };
  }, [wanted, math]);
  return wanted ? math : null;
}
