import { useEffect } from 'react';
import { api } from '../../../lib/api';

const INTERVAL_MS = 25000;

/**
 * Reports real reading time to the backend while a file is open. Only ticks
 * while the tab is visible, so switching away or leaving the reader open in
 * a background tab doesn't inflate "time spent reading". The time belongs to the
 * file being read, not to whichever one is the book's primary.
 */
export function useReadingHeartbeat(workId, fileId) {
  useEffect(() => {
    if (!workId) return undefined;

    const tick = () => {
      if (document.visibilityState !== 'visible') return;
      api
        .post(`/works/${workId}/reading-heartbeat`, { seconds: INTERVAL_MS / 1000, ...(fileId ? { fileId } : {}) })
        .catch(() => {
          // Best-effort — losing an occasional heartbeat isn't worth surfacing to the user.
        });
    };

    const intervalId = setInterval(tick, INTERVAL_MS);
    return () => clearInterval(intervalId);
  }, [workId, fileId]);
}
