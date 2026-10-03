import { useState, useEffect } from 'react';
import { authenticatedUrl } from '../../../../lib/api';

/** The text of a file the reader shows as text: what it says, whether it is still coming, why it did not, and a way to ask again. */
export function useTextFile(fileUrl) {
  const [state, setState] = useState({ content: '', loading: true, error: null });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setState({ content: '', loading: true, error: null });
    (async () => {
      try {
        const resp = await fetch(authenticatedUrl(fileUrl));
        if (!resp.ok) throw new Error(`O servidor respondeu com o erro ${resp.status}.`);
        const content = await resp.text();
        if (!cancelled) setState({ content, loading: false, error: null });
      } catch (err) {
        if (!cancelled) setState({ content: '', loading: false, error: err.message });
      }
    })();
    return () => { cancelled = true; };
  }, [fileUrl, attempt]);

  return { ...state, retry: () => setAttempt((n) => n + 1) };
}
