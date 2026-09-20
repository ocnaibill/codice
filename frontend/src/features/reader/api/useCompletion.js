import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

/**
 * Marks a file as finished, or reopens it (DEC-077): the explicit action, with no reading involved.
 * `restart` (with completed false) is "read it again": the position goes too (DEC-080).
 */
export function useSetCompletion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ fileId, completed, restart = false }) =>
      api.put(`/progress/files/${fileId}/completion`, restart ? { completed, restart } : { completed }),
    onSuccess: () => refreshLibrary(queryClient),
  });
}

/** Marks the whole work as finished, or takes the mark off (DEC-080). */
export function useSetWorkFinished() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ workId, finished }) => api.put(`/progress/works/${workId}/finished`, { finished }),
    onSuccess: () => refreshLibrary(queryClient),
  });
}
