import { useEffect } from 'react';
import { api } from '../../../lib/api';
import { isActive, watchActivity } from '../activity';

const INTERVAL_MS = 25000;

/**
 * Reports real reading time to the backend while a file is open (DEC-076). A tick counts only if
 * the tab is visible AND the person did something in the last 90 seconds (or audio is playing),
 * so a reader left open in a background or an idle tab does not inflate "time spent reading". The
 * time belongs to the file being read, not to whichever one is the book's primary.
 */
export function useReadingHeartbeat(workId, fileId) {
  useEffect(() => {
    if (!workId) return undefined;
    const stopWatching = watchActivity();

    const tick = () => {
      if (document.visibilityState !== 'visible' || !isActive()) return;
      api
        .post(`/works/${workId}/reading-heartbeat`, { seconds: INTERVAL_MS / 1000, ...(fileId ? { fileId } : {}) })
        .catch(() => {
          // Best-effort — losing an occasional heartbeat isn't worth surfacing to the user.
        });
    };

    const intervalId = setInterval(tick, INTERVAL_MS);
    return () => {
      clearInterval(intervalId);
      stopWatching();
    };
  }, [workId, fileId]);
}
