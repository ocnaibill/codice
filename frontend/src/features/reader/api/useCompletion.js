import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

/** Marks a file as finished, or reopens it (DEC-077): the explicit action, with no reading involved. */
export function useSetCompletion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ fileId, completed }) => api.put(`/progress/files/${fileId}/completion`, { completed }),
    onSuccess: () => refreshLibrary(queryClient),
  });
}
