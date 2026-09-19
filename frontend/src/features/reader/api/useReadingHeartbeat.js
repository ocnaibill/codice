import { useEffect } from 'react';
import { api } from '../../../lib/api';

const INTERVAL_MS = 25000;

/**
 * Reports real reading time to the backend while a work is open. Only ticks
 * while the tab is visible, so switching away or leaving the reader open in
 * a background tab doesn't inflate "time spent reading".
 */
export function useReadingHeartbeat(workId) {
  useEffect(() => {
    if (!workId) return undefined;

    const tick = () => {
      if (document.visibilityState !== 'visible') return;
      api
        .post(`/works/${workId}/reading-heartbeat`, { seconds: INTERVAL_MS / 1000 })
        .catch(() => {
          // Best-effort — losing an occasional heartbeat isn't worth surfacing to the user.
        });
    };

    const intervalId = setInterval(tick, INTERVAL_MS);
    return () => clearInterval(intervalId);
  }, [workId]);
}
