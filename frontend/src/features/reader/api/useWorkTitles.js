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

/** The title of one edition of a work, written by owner or admin (#185): from then on it is a name of the work, and the one it goes by while that edition is read. */
export function useEditEditionTitle(workId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ editionId, title }) => (await api.patch(`/works/${workId}/editions/${editionId}`, { title })).data,
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
