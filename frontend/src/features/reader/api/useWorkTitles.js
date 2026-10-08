import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

/** Another name for a work, in a language or not (#185); for owner and admin. */
export function useAddWorkTitle(workId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ title, language }) => (await api.post(`/works/${workId}/titles`, { title, language })).data,
    onSuccess: () => refreshLibrary(queryClient),
  });
}

/** The work stops going by a name that was kept for it. */
export function useRemoveWorkTitle(workId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (titleId) => api.delete(`/works/${workId}/titles/${titleId}`),
    onSuccess: () => refreshLibrary(queryClient),
  });
}
