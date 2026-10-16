import { useEffect, useRef, useState } from 'react';
import { toast } from '../../components/ui/toast';
import { arrivalOf, lastTold, markTold } from './arrival';

/**
 * Tells, once, that the place this file opens at is where another device left it (see arrival.js), and returns what the viewer needs to
 * underline it (`{ cfi }` for an EPUB) until the file changes. `enabled` is off while a place was asked for (a note, a reference) or the
 * person chose to start over: those do not open at the saved place.
 */
export function useArrivalNotice(fileId, progress, enabled) {
  const [arrival, setArrival] = useState(null);
  const told = useRef(null);
  useEffect(() => {
    setArrival(null);
    told.current = null;
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
  return arrival;
}
