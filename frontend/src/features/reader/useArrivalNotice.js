import { useEffect, useRef, useState } from 'react';
import { toast } from '../../components/ui/toast';
import { arrivalOf, lastTold, markPendingTold, markTold, pendingOf, pendingTold } from './arrival';

/**
 * Tells, once, that the place this file opens at is where another device left it (see arrival.js), and returns what the viewer needs to
 * underline it (`{ cfi }` for an EPUB) until the file changes. `enabled` is off while a place was asked for (a note, a reference) or the
 * person chose to start over: those do not open at the saved place.
 *
 * When a device sent a place that was kept apart because the web had a newer one (DEC-191), the person is asked once: the notice has "Ir para
 * lá" (`onGoTo(locator)` opens the book there), and leaving it is staying where they are.
 */
export function useArrivalNotice(fileId, progress, enabled, onGoTo) {
  const [arrival, setArrival] = useState(null);
  const told = useRef(null);
  const asked = useRef(null);
  useEffect(() => {
    setArrival(null);
    told.current = null;
    asked.current = null;
  }, [fileId]);
  useEffect(() => {
    if (!enabled || !fileId || told.current === fileId) return;
    const found = arrivalOf(progress, lastTold(fileId));
    if (!found) return;
    told.current = fileId;
    markTold(fileId, found.revision);
    toast.info(found.message, { key: 'arrival', duration: 6000 });
    setArrival({ cfi: found.cfi });
  }, [enabled, fileId, progress]);
  useEffect(() => {
    if (!enabled || !fileId || asked.current === fileId) return;
    const found = pendingOf(progress, pendingTold(fileId));
    if (!found) return;
    asked.current = fileId;
    markPendingTold(fileId, found.at);
    toast.info(found.message, {
      key: 'pending',
      duration: 15000,
      action: onGoTo ? { label: 'Ir para lá', onClick: () => onGoTo(found.locator) } : undefined,
    });
  }, [enabled, fileId, progress, onGoTo]);
  return arrival;
}
