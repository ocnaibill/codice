import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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

const searchKey = (workId) => ['work', workId, 'cover-search'];

/** The last search for covers of a work (staff): {state, query, results}; null when none was made. It is read again while it waits. */
export function useCoverSearch(workId, enabled = true) {
  return useQuery({
    queryKey: searchKey(workId),
    enabled,
    staleTime: 0,
    gcTime: 0,
    queryFn: async () => {
      try {
        return (await api.get(`/works/${workId}/cover/search`)).data;
      } catch (error) {
        if (error?.response?.status === 404) return null;
        throw error;
      }
    },
    refetchInterval: (query) => (query.state.data?.state === 'pending' ? 1500 : false),
  });
}

/** Asks the worker for the covers the providers that are on have for the work; the text is optional (what the work says is used). */
export function useAskCoverSearch(workId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (query) => (await api.post(`/works/${workId}/cover/search`, { query })).data,
    onSuccess: (data) => queryClient.setQueryData(searchKey(workId), data),
  });
}
