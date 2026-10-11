import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

/** Sends the cover of a work, already cut to 2:3 (staff). Answers with the address of the cover. */
export function useSetWorkCover(workId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (blob) => {
      const form = new FormData();
      form.append('image', blob, 'capa.jpg');
      return (await api.put(`/works/${workId}/cover`, form)).data;
    },
    onSuccess: () => refreshLibrary(queryClient),
  });
}

/** Goes back to the cover the file has (staff); the lock is released. Answers with the address of that cover. */
export function useResetWorkCover(workId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.delete(`/works/${workId}/cover`)).data,
    onSuccess: () => refreshLibrary(queryClient),
  });
}
