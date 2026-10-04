import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const KEY = ['data-export'];

/** The latest request of this person for the file of "Meus dados": { export: null | { id, state, requestedAt, readyAt, expiresAt, bytes } },
 *  where state is pending, ready, failed or expired. While it is being made it asks again every few seconds. */
export function useDataExport() {
  return useQuery({
    queryKey: KEY,
    queryFn: async () => (await api.get('/auth/export')).data,
    // Belongs to one account: dropped as soon as the screen closes.
    gcTime: 0,
    staleTime: 0,
    refetchInterval: (query) => (query.state.data?.export?.state === 'pending' ? 3000 : false),
  });
}

export function useRequestDataExport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post('/auth/export')).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEY }),
    // "Already being made" is not a failure: the screen shows that one.
    onError: () => queryClient.invalidateQueries({ queryKey: KEY }),
  });
}

export function useDeleteDataExport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id) => api.delete(`/auth/export/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEY }),
  });
}
