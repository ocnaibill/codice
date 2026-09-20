import { useCallback, useEffect, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

// A short name for this device, kept with the position so someone with two devices can tell
// where they stopped. It is information only.
function deviceLabel() {
  const platform = navigator.userAgentData?.platform || navigator.platform || 'web';
  return `Web (${platform})`.slice(0, 100);
}

/**
 * The caller's reading position in one file, and the way to save it. The position is the
 * file's own (DEC-030): another format or edition of the same book has another.
 *
 * `save(locator, { percent, completed })` sends the revision it last saw. If another device
 * saved in the meantime the server answers 409 with the current state; the person reading
 * here is the one moving, so the write is repeated once on top of that revision instead of
 * being lost. The initial state is fetched fresh each time a file is opened.
 */
export function useFileProgress(fileId) {
  const query = useQuery({
    queryKey: ['file-progress', fileId],
    queryFn: async () => (await api.get(`/progress/files/${fileId}`)).data,
    enabled: !!fileId,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });

  const revision = useRef(0);
  useEffect(() => {
    if (query.data) revision.current = query.data.revision;
  }, [query.data]);

  const save = useCallback(
    async (locator, extras = {}) => {
      if (!fileId) return null;
      const send = () =>
        api.put(`/progress/files/${fileId}`, {
          locator,
          ...extras,
          device: deviceLabel(),
          clientTime: new Date().toISOString(),
          baseRevision: revision.current,
        });
      let response;
      try {
        response = await send();
      } catch (err) {
        if (err?.response?.status !== 409) throw err;
        revision.current = err.response.data.revision;
        response = await send();
      }
      revision.current = response.data.revision;
      return response.data;
    },
    [fileId]
  );

  return { ...query, save };
}
